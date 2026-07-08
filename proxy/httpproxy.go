package proxy

import (
	"encoding/base64"
	"log"
	"net/http"
	http2 "server/proxy/http"
	"sync/atomic"
	"time"
)

type HTTPProxy struct {
}

func (p *HTTPProxy) ServeHTTP(wr http.ResponseWriter, req *http.Request) {
	valid, params := http2.Authenticate(req)
	if !valid {
		wr.Header().Set("Proxy-Authenticate", "Basic realm=\"Turbo Proxy\"")
		http.Error(wr, "Proxy authentication required", http.StatusProxyAuthRequired)
		return
	}

	country := "global"
	if _, exists := params["country"]; exists {
		country = params["country"]
	}

	for k, v := range req.Header {
		log.Println("Header: ", k, ":", v)
	}

	if req.Method != "CONNECT" {
		http.Error(wr, "Non-HTTPS websites are blocked by default, contact support to unlock", http.StatusBadRequest)
		return
	}

	client := FindClientByCountry(country)
	if client == nil {
		log.Println("No active clients available in country:", country)
		http.Error(wr, "No active clients available", http.StatusServiceUnavailable)
		return
	}

	wr.WriteHeader(http.StatusOK)
	hijacker, ok := wr.(http.Hijacker)
	if !ok {
		http.Error(wr, "Hijacking not supported", http.StatusInternalServerError)
		return
	}
	conn, _, err := hijacker.Hijack()
	if err != nil {
		http.Error(wr, err.Error(), http.StatusServiceUnavailable)
		return
	}
	defer conn.Close()

	pc := CreateConnection(conn)

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

	client.userMutex.Lock()
	client.userConns[pc.ID] = pc
	client.userMutex.Unlock()
	atomic.AddInt32(&client.Stats.ActiveConns, 1)
	client.SendMessage(Message{
		Type: "connect",
		ID:   pc.ID,
		Addr: req.Host,
		Data: connData,
	})

	go relayFromSocksToQuic(client, pc)
	relayFromChanToSocks(client, pc)
}
