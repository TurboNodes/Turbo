package website

import (
	"fmt"
	"html/template"
	"math"
	"net/http"
	"server/proxy"
	"sync/atomic"
	"time"
)

var templates, _ = template.ParseFiles("./website/templates/stats.html")

type ClientData struct {
	ID              string
	CryptoAddr      string
	ActiveTime      string
	ActiveConns     int32
	BytesIn         string
	BytesOut        string
	TotalBytes      string
	Ping            string
	Score           string
	EstimatedReward string
}

type ViewData struct {
	Title    string
	Headers  []string
	Clients  []ClientData
	Address  string
	NotFound bool
}

func getClientData(id string, client *proxy.QuicClient) ClientData {
	bytesIn := atomic.LoadUint64(&client.Stats.BytesReceived)
	bytesOut := atomic.LoadUint64(&client.Stats.BytesSent)
	totalBytes := bytesIn + bytesOut
	activeTime := time.Since(client.Stats.ConnectTime).Round(time.Second)
	activeConns := atomic.LoadInt32(&client.Stats.ActiveConns)

	return ClientData{
		ID:              id,
		CryptoAddr:      client.Stats.CryptoAddr,
		ActiveTime:      activeTime.String(),
		ActiveConns:     activeConns,
		BytesIn:         formatBytes(bytesIn),
		BytesOut:        formatBytes(bytesOut),
		TotalBytes:      formatBytes(totalBytes),
		Ping:            fmt.Sprintf("%.1f ms", client.Metrics.Latency),
		Score:           fmt.Sprintf("%.0f/100", client.Metrics.Score),
		EstimatedReward: fmt.Sprintf("$%.4f", float64(totalBytes)/math.Pow10(9)*0.10), //0. TODO: proper reward calculation
	}
}

func StatsHandler(w http.ResponseWriter, r *http.Request) {
	address := r.URL.Query().Get("address")

	headers := []string{
		"Client",
		"Crypto Address",
		"Active Since",
		"Active Connections",
		"Bytes Received",
		"Bytes Sent",
		"Total Bandwidth",
		"Ping",
		"Score",
		"Estimated Reward",
	}

	viewData := ViewData{
		Title:   "Client Statistics",
		Headers: headers,
		Address: address,
		Clients: []ClientData{},
	}

	proxy.QuicMutex.RLock()
	defer proxy.QuicMutex.RUnlock()

	if address != "" {
		for id, client := range proxy.QuicClients {
			if client.Stats.CryptoAddr == address {
				viewData.Clients = append(viewData.Clients, getClientData(id, client))
			}
		}
		viewData.NotFound = len(viewData.Clients) == 0
	} else {
		for _, client := range proxy.QuicClients {
			viewData.Clients = append(viewData.Clients, getClientData(client.ID, client))
		}
	}

	err := templates.ExecuteTemplate(w, "stats.html", viewData)
	if err != nil {
		http.Error(w, fmt.Sprintf("Internal Server Error: %v", err), http.StatusInternalServerError)
	}
}

func formatBytes(bytes uint64) string {
	const unit = 1000
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := uint64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.2f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}
