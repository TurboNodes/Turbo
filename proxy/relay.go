package proxy

import (
	"encoding/base64"
	"fmt"
	"log"
	"net"
	data2 "server/data"
	"server/proxy/socks"
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
	msg := Message{Type: "connect", ID: pc.ID, Addr: fmt.Sprintf("%s:%d", host, port), Data: connData}

	success := false
	attempts := 0

	for !success && attempts < 3 {
		attempts++
		client = FindClientByCountry(country)
		if client == nil {
			log.Println("No available clients found for this request")
			return
		}

		client.userMutex.Lock()
		client.userConns[pc.ID] = pc
		client.userMutex.Unlock()
		atomic.AddInt32(&client.Stats.ActiveConns, 1)

		err = client.SendMessage(msg)
		if err != nil {
			log.Println("WriteJSON error:", err)
			client.userMutex.Lock()
			delete(client.userConns, pc.ID)
			client.userMutex.Unlock()
			atomic.AddInt32(&client.Stats.ActiveConns, -1)
			continue
		}

		select {
		case <-pc.DataChan:
			success = true
		case <-time.After(connectTimeout):
			log.Printf("Connection timeout for client %s, retrying with another client", client.ID)
			client.userMutex.Lock()
			delete(client.userConns, pc.ID)
			client.userMutex.Unlock()
			atomic.AddInt32(&client.Stats.ActiveConns, -1)
			continue
		}

		if success {
			atomic.AddUint64(&client.Stats.BytesSent, uint64(n))
			go relayFromSocksToQuic(client, pc)
			relayFromChanToSocks(client, pc)
			return
		}
	}

	conn.Write([]byte{5, 1, 0, 1, 0, 0, 0, 0, 0, 0})
}

func relayFromSocksToQuic(client *QuicClient, pc *Connection) {
	buf := make([]byte, 4096)
	for {
		n, err := pc.Conn.Read(buf)
		if err != nil {
			pc.Features.Outbound[time.Since(pc.Features.StartTime).Microseconds()] += uint16(n)
			client.SendCloseMessage(pc.ID)
			return
		}

		dataSize := uint64(n)
		atomic.AddUint64(&client.Stats.BytesSent, dataSize)
		pc.Features.Outbound[time.Since(pc.Features.StartTime).Microseconds()] += uint16(n)

		data := base64.StdEncoding.EncodeToString(buf[:n])
		msg := Message{Type: "data", ID: pc.ID, Data: data}
		if client.conn != nil {
			client.SendMessage(msg)
		}
	}
}

func relayFromChanToSocks(client *QuicClient, pc *Connection) {
	for data := range pc.DataChan {
		n, err := pc.Conn.Write(data)
		atomic.AddUint64(&client.Stats.BytesReceived, uint64(n))
		pc.Features.Inbound[time.Since(pc.Features.StartTime).Microseconds()] += uint16(n)
		if err != nil {
			//client.SendCloseMessage(pc.ID)
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
