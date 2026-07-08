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
	"os"
	"server/data/database"
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
	ID         string
	conn       *quic.Conn
	stream     *quic.Stream
	mutex      sync.Mutex
	userConns  map[string]*Connection
	userMutex  sync.Mutex
	lastPing   time.Time
	lastPingID string
	Metrics    *Metrics
	Stats      *ClientStats
	kicked     atomic.Bool
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

	// Accept a bidirectional stream
	stream, err := conn.AcceptStream(context.Background())
	if err != nil {
		log.Printf("Failed to accept QUIC stream: %v", err)
		conn.CloseWithError(1, "stream accept failed")
		return
	}

	client := &QuicClient{
		ID:        clientID,
		conn:      conn,
		stream:    stream,
		userConns: make(map[string]*Connection),
		lastPing:  time.Now(),
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

	country := "global"
	if ip, _, err := net.SplitHostPort(conn.RemoteAddr().String()); err == nil {
		resp, err := http.Get("http://ip-api.com/csv/" + ip + "?fields=countryCode")
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
		case "uid-register":
			db, err := database.InitDatabase(os.Getenv("DATABASE_URL"))
			if err != nil {
				log.Println(err)
			}

			err = database.AddNode(db, msg.ID, client.ID)
			if err != nil {
				log.Printf("Error adding node to %s, %v", client.ID, err)
			} else {
				log.Printf("Registered Node %s for client %s", msg.ID, client.ID)
			}

			db.Close()

			/*
				TODO(architecture):
					- Put Quic client stats into database
					- Link Distant node with Quic client so that,
					- Server can update Node stats.
			*/
		}
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
