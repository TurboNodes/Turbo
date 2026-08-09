package proxy

import (
	"net"
	"server/data"
	"strconv"
	"sync"
	"time"
)

type Connection struct {
	ID       string
	Conn     net.Conn
	DataChan chan []byte
	Features *data.ConnectionFeatures

	// ready is closed when the node acks the connect; failed is closed when it
	// reports it could not reach the target. Exactly one fires per attempt, so
	// HandleSocksConn never has to guess the outcome from relayed payload.
	ready     chan struct{}
	failed    chan struct{}
	readyOnce sync.Once
	failOnce  sync.Once
}

var nextID int

func CreateConnection(conn net.Conn) *Connection {
	dataChan := make(chan []byte, 100)
	nextID++
	c := &Connection{
		ID:       strconv.Itoa(nextID),
		Conn:     conn,
		DataChan: dataChan,
		Features: &data.ConnectionFeatures{
			StartTime: time.Now(),
			Protocol:  conn.RemoteAddr().Network(),
			Inbound:   make(map[int64]uint16),
			Outbound:  make(map[int64]uint16),
		},
	}
	c.resetHandshake()
	return c
}

// resetHandshake arms a fresh ready/failed pair. Called before each attempt so
// a node that failed does not leave the next node's handshake pre-resolved.
func (c *Connection) resetHandshake() {
	c.ready = make(chan struct{})
	c.failed = make(chan struct{})
	c.readyOnce = sync.Once{}
	c.failOnce = sync.Once{}
}

// MarkReady records that the node reached the target.
func (c *Connection) MarkReady() { c.readyOnce.Do(func() { close(c.ready) }) }

// MarkFailed records that the node could not reach the target.
func (c *Connection) MarkFailed() { c.failOnce.Do(func() { close(c.failed) }) }

// Established reports whether the node has acked this connection.
func (c *Connection) Established() bool {
	select {
	case <-c.ready:
		return true
	default:
		return false
	}
}
