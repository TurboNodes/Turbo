package proxy

import (
	"net"
	"server/data"
	"strconv"
	"time"
)

type Connection struct {
	ID       string
	Conn     net.Conn
	DataChan chan []byte
	Features *data.ConnectionFeatures
}

var nextID int

func CreateConnection(conn net.Conn) *Connection {
	dataChan := make(chan []byte, 100)
	nextID++
	return &Connection{
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
}
