package data

import "github.com/prometheus/client_golang/prometheus"

// Prometheus metrics for your specific requirements
var (
	// Active client nodes
	activeNodes = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "proxy_active_nodes_total",
			Help: "Number of active nodes",
		},
		[]string{"protocol", "country"}, // SOCKS5, HTTP, QUIC + country
	)

	responseTime = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "proxy_response_time_seconds",
			Help:    "Response time for proxy requests in seconds",
			Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0, 10.0},
		},
		[]string{"protocol", "target_country", "client_country"},
	)

	bytesTransferred = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "proxy_bytes_transferred_total",
			Help: "Total bytes transferred through proxy",
		},
		[]string{"protocol", "direction", "client_country"}, // in/out
	)
)

func LogBytesTransferred(protocol, direction, clientCountry string, bytes int) {
	bytesTransferred.WithLabelValues(protocol, direction, clientCountry).Add(float64(bytes))
}
