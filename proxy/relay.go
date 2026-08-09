package proxy

import (
	"encoding/base64"
	"log"
	"net"
	data2 "server/data"
	"server/proxy/socks"
	"strconv"
	"sync/atomic"
	"time"
)

var (
	connectTimeout = 5 * time.Second
)

type ClientStats struct {
	ConnectTime   time.Time
	ActiveConns   int32
	BytesSent     uint64
	BytesReceived uint64
	CryptoAddr    string
	CountryCode   string
}

func HandleSocksConn(conn net.Conn) {
	defer conn.Close()

	host, port, params, err := socks.HandleSocksHandshake(conn)
	country := "global"
	if _, exists := params["country"]; exists {
		country = params["country"]
	}

	if err != nil {
		log.Printf("SOCKS handshake failed for %s, %v", conn.RemoteAddr(), err)
		return
	}

	var client *QuicClient

	pc := CreateConnection(conn)

	_, err = conn.Write([]byte{5, 0, 0, 1, 0, 0, 0, 0, 0, 0}) // success
	if err != nil {
		log.Printf("Failed to send SOCKS success response to %s: %v", conn.RemoteAddr(), err)
		return
	}

	// Premake connect message
	buffer := make([]byte, 32*1024)
	var connData string
	n, err := pc.Conn.Read(buffer)
	if err != nil {
		return
	}
	if n > 0 {
		connData = base64.StdEncoding.EncodeToString(buffer[:n])
		pc.Features.Inbound[time.Since(pc.Features.StartTime).Microseconds()] += uint16(n)
	}
	msg := Message{Type: "connect", ID: pc.ID, Addr: net.JoinHostPort(host, strconv.Itoa(port)), Data: connData}

	success := false
	attempts := 0

	for !success && attempts < 3 {
		attempts++
		client = FindClientByCountry(country)
		if client == nil {
			log.Println("No available clients found for this request")
			return
		}

		pc.resetHandshake()
		client.userMutex.Lock()
		client.userConns[pc.ID] = pc
		client.userMutex.Unlock()
		atomic.AddInt32(&client.Stats.ActiveConns, 1)

		if err := client.SendMessage(msg); err != nil {
			log.Println("WriteJSON error:", err)
			detach(client, pc)
			continue
		}

		// Wait for the node's explicit verdict. Any payload it sends meanwhile
		// simply queues in DataChan and is flushed once the relay starts.
		select {
		case <-pc.ready:
			success = true
		case <-pc.failed:
			log.Printf("Node %s could not reach %s, trying another node", client.ID, msg.Addr)
			detach(client, pc)
			continue
		case <-time.After(connectTimeout):
			log.Printf("Node %s timed out on %s, trying another node", client.ID, msg.Addr)
			detach(client, pc)
			continue
		}

		atomic.AddUint64(&client.Stats.BytesSent, uint64(n))
		go relayFromSocksToQuic(client, pc)
		relayFromChanToSocks(client, pc)
		return
	}

	// The SOCKS success reply already went out above (it has to, so the user
	// sends the payload we bundle into the connect message). Once that is on
	// the wire the stream is raw tunnel data, so a failure reply here would be
	// read as garbage by the user -- all we can do now is drop the connection.
	log.Printf("No node could reach %s after %d attempts", msg.Addr, attempts)
}

func relayFromSocksToQuic(client *QuicClient, pc *Connection) {
	buf := make([]byte, 4096)
	for {
		n, err := pc.Conn.Read(buf)
		// Read may return data *and* an error (typically io.EOF) in the same
		// call, so flush what we got before acting on the error.
		if n > 0 {
			atomic.AddUint64(&client.Stats.BytesSent, uint64(n))
			pc.Features.Outbound[time.Since(pc.Features.StartTime).Microseconds()] += uint16(n)

			data := base64.StdEncoding.EncodeToString(buf[:n])
			msg := Message{Type: "data", ID: pc.ID, Data: data}
			if client.conn != nil {
				client.SendMessage(msg)
			}
		}
		if err != nil {
			client.SendCloseMessage(pc.ID)
			return
		}
	}
}

// detach unregisters pc from a node that failed the handshake, leaving the
// user's connection intact so the next node can be tried.
func detach(client *QuicClient, pc *Connection) {
	client.userMutex.Lock()
	delete(client.userConns, pc.ID)
	client.userMutex.Unlock()
	atomic.AddInt32(&client.Stats.ActiveConns, -1)
}

func relayFromChanToSocks(client *QuicClient, pc *Connection) {
	for data := range pc.DataChan {
		n, err := pc.Conn.Write(data)
		atomic.AddUint64(&client.Stats.BytesReceived, uint64(n))
		pc.Features.Inbound[time.Since(pc.Features.StartTime).Microseconds()] += uint16(n)
		if err != nil {
			return
		}
	}
}

func (c *QuicClient) SendCloseMessage(id string) {
	msg := Message{Type: "close", ID: id}
	if c.conn != nil {
		c.SendMessage(msg)
	}

	c.userMutex.Lock()
	sc := c.userConns[id]
	delete(c.userConns, id)
	c.userMutex.Unlock()

	if sc != nil {
		data2.LogConnection(sc.Features)
		atomic.AddInt32(&c.Stats.ActiveConns, -1)
		sc.Conn.Close()
	} else {
		println("é- double closing")
	}

}
