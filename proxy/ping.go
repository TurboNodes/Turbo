package proxy

import (
	"math"
	"math/rand"
	"time"
)

type Metrics struct {
	Latency        float64
	latencyReports float64
	Availability   float64
	Reliability    float64
	Score          float64
}

func ReportPing() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		QuicMutex.RLock()
		clientSnapshot := make([]*QuicClient, 0, len(QuicClients))
		for _, client := range QuicClients {
			clientSnapshot = append(clientSnapshot, client)
		}
		QuicMutex.RUnlock()

		for _, client := range clientSnapshot {
			if client.kicked.Load() {
				continue
			}

			client.lastPing = time.Now()
			if client.lastPingID != "" {
				client.Kick("ping timeout")
				continue
			}

			pingID := string(rune(rand.Int()))

			err := client.SendMessage(Message{
				Type: "ping",
				ID:   pingID,
			})
			if err != nil {
				client.Kick("ping send error")
				continue
			}
			client.lastPingID = pingID
		}
	}
}
func (c *QuicClient) Pong() {
	c.lastPingID = ""
	c.Metrics.Latency = float64(int16(time.Since(c.lastPing).Milliseconds()))
	c.UpdateScore()
}

func (c *QuicClient) UpdateScore() {
	latencyScore := math.Max(0, math.Min(1.0, 1.0-(c.Metrics.Latency-10)/500))
	reliabilityScore := c.Metrics.Reliability

	// weighted score: 60% latency, 40% reliability
	score := 100 * ((0.6 * latencyScore) + (0.4 * reliabilityScore))

	if reliabilityScore > 1.2 {
		c.Metrics.Reliability = 1.2
	}

	c.Metrics.Score = score
}
