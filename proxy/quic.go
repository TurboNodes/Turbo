package proxy

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"server/data/database/supabase"
	"server/proxy/user"
	"sync"
	"sync/atomic"
	"time"

	"github.com/quic-go/quic-go"
)

type Message struct {
	Type string `json:"type"`
	ID   string `json:"ID"`
	// Addr also contains port of the target website
	Addr string `json:"addr,omitempty"`
	Data string `json:"data,omitempty"`
}

var (
	QuicClients           = make(map[string]*QuicClient)
	QuicMutex             sync.RWMutex
	quicListener          *quic.Listener
	BrowserScreenshotData = make(chan []byte)
)

// QuicClient represents a connected QUIC client
type QuicClient struct {
	ID              string
	NodeIP          string
	conn            *quic.Conn
	stream          *quic.Stream
	mutex           sync.Mutex
	userConns       map[string]*Connection
	userMutex       sync.Mutex
	lastPing        time.Time
	lastPingID      string
	Metrics         *Metrics
	Stats           *ClientStats
	kicked          atomic.Bool
	pairingMu       sync.Mutex
	pending         *pendingPairing
	pairingStop     chan struct{}
	pairingStopOnce sync.Once
}

// StartQuicServer initializes the QUIC server
func StartQuicServer(addr string, tlsConfig *tls.Config) error {
	listener, err := quic.ListenAddr(addr, tlsConfig, nil)
	if err != nil {
		return fmt.Errorf("failed to start QUIC server: %w", err)
	}

	quicListener = listener
	log.Printf("QUIC server listening on %s", addr)

	go acceptQuicConnections(quicListener)

	go ReportPing()

	return nil
}

func acceptQuicConnections(listener *quic.Listener) {
	for {
		conn, err := listener.Accept(context.Background())
		if err != nil {
			log.Printf("QUIC accept error: %v", err)
			continue
		}

		go handleQuicConnection(conn)
	}
}

func handleQuicConnection(conn *quic.Conn) {
	clientID := conn.RemoteAddr().String()
	log.Printf("New QUIC client connected: %s", clientID)

	nodeIP, _, err := net.SplitHostPort(clientID)
	if err != nil {
		log.Printf("Failed to parse node IP from %s: %v", clientID, err)
		conn.CloseWithError(1, "invalid remote address")
		return
	}

	// Accept a bidirectional stream
	stream, err := conn.AcceptStream(context.Background())
	if err != nil {
		log.Printf("Failed to accept QUIC stream: %v", err)
		conn.CloseWithError(1, "stream accept failed")
		return
	}

	client := &QuicClient{
		ID:          clientID,
		NodeIP:      nodeIP,
		conn:        conn,
		stream:      stream,
		userConns:   make(map[string]*Connection),
		lastPing:    time.Now(),
		pairingStop: make(chan struct{}),
		Metrics: &Metrics{
			Reliability: 0.7,
			Score:       50,
		},
		Stats: &ClientStats{
			ConnectTime: time.Now(),
			CryptoAddr:  "",
		},
	}

	QuicMutex.Lock()
	QuicClients[clientID] = client
	QuicMutex.Unlock()

	go quicReader(client)
	go client.runPairing()
	go markNodeActive(nodeIP, true)

	country := "global"
	resp, err := http.Get("http://ip-api.com/csv/" + nodeIP + "?fields=countryCode")
	if err == nil {
		defer resp.Body.Close()
		if resp.StatusCode == 200 {
			body, err := io.ReadAll(resp.Body)
			if err == nil {
				if user.IsValidCountryCode(country) {
					country = string(body)
				}
			}
		}
	}
	client.Stats.CountryCode = country

	updatePools()
}

func quicReader(client *QuicClient) {
	defer func() {
		QuicMutex.Lock()
		delete(QuicClients, client.ID)
		log.Printf("QUIC client disconnected: %s. Remaining clients: %d", client.ID, len(QuicClients))
		QuicMutex.Unlock()

		client.stopPairing()
		go markNodeActive(client.NodeIP, false)

		client.stream.Close()
		client.conn.CloseWithError(0, "client disconnected")
	}()

	decoder := json.NewDecoder(client.stream)
	for {
		var msg Message
		if err := decoder.Decode(&msg); err != nil {
			if client.kicked.Load() {
				return
			}
			log.Printf("QUIC read error for client %s: %v", client.ID, err)
			return
		}

		switch msg.Type {
		case "data":
			client.userMutex.Lock()
			if sc, ok := client.userConns[msg.ID]; ok {
				if data, err := base64.StdEncoding.DecodeString(msg.Data); err == nil {
					sc.DataChan <- data
				} else {
					log.Println("WARN: Suspicious data received from client", client.ID)
				}
			}
			client.userMutex.Unlock()
		case "close":
			client.userMutex.Lock()
			if sc, ok := client.userConns[msg.ID]; ok {
				sc.Conn.Close()
				delete(client.userConns, msg.ID)
			}
			client.userMutex.Unlock()
		case "address":
			client.Stats.CryptoAddr = msg.ID
		case "pong":
			client.Pong()
		}
	}
}

func markNodeActive(nodeIP string, active bool) {
	if nodeIP == "" {
		return
	}
	db := supabase.SupabaseDB
	if err := supabase.SetNodeActive(db, nodeIP, active); err != nil {
		// Unpaired nodes have no row yet — ignore.
		log.Printf("node activity update for %s: %v", nodeIP, err)
	}
}

func (c *QuicClient) SendMessage(msg Message) error {
	if c == nil {
		return fmt.Errorf("client is nil")
	}

	c.mutex.Lock()
	defer c.mutex.Unlock()

	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	data = append(data, '\n') // Add newline for JSON decoder

	_, err = c.stream.Write(data)
	return err
}

func (c *QuicClient) Kick(reason string) {
	if !c.kicked.CompareAndSwap(false, true) {
		return // Already kicked
	}

	c.stopPairing()
	c.conn.CloseWithError(0, reason)

	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.stream.Close()

	for id, sc := range c.userConns {
		sc.Conn.Close()
		delete(c.userConns, id)
	}

	QuicMutex.Lock()
	delete(QuicClients, c.ID)
	QuicMutex.Unlock()

	updatePools() // TODO: Inefficient, optimize client erasure

	log.Printf("Kicked QUIC client %s for \"%s\"", c.ID, reason)
}
