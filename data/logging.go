package data

import (
	"context"
	"log"
	"math"
	"server/data/database"
	"time"

	"github.com/google/uuid"
)

type ConnectionFeatures struct {
	Protocol  string
	StartTime time.Time
	Inbound   map[int64]uint16 // 16-bit because max buffer size is 4096
	Outbound  map[int64]uint16
}

func LogConnection(features *ConnectionFeatures) {
	if features == nil {
		return
	}
	if len(features.Inbound) == 0 && len(features.Outbound) == 0 {
		return
	}

	inboundPackets := len(features.Inbound)
	outboundPackets := len(features.Outbound)
	var inoutRatio float64
	if outboundPackets == 0 {
		inoutRatio = float64(inboundPackets)
	} else {
		inoutRatio = float64(inboundPackets) / float64(outboundPackets)
	}
	totalPackets := inboundPackets + outboundPackets
	inboundBytes := uint16(0)
	inboundTimes := make([]int64, 0, inboundPackets)
	outboundBytes := uint16(0)
	outboundTimes := make([]int64, 0, outboundPackets)

	for timemicro, bytes := range features.Inbound {
		inboundBytes += bytes
		inboundTimes = append(inboundTimes, timemicro)
	}

	for timemicro, bytes := range features.Outbound {
		outboundBytes += bytes
		outboundTimes = append(outboundTimes, timemicro)
	}

	totalBytes := inboundBytes + outboundBytes

	mergedTimes := make([]int64, 0, len(inboundTimes)+len(outboundTimes))
	i, j := 0, 0
	for i < len(inboundTimes) && j < len(outboundTimes) {
		if inboundTimes[i] < outboundTimes[j] {
			mergedTimes = append(mergedTimes, inboundTimes[i])
			i++
		} else {
			mergedTimes = append(mergedTimes, outboundTimes[j])
			j++
		}
	}
	for ; i < len(inboundTimes); i++ {
		mergedTimes = append(mergedTimes, inboundTimes[i])
	}
	for ; j < len(outboundTimes); j++ {
		mergedTimes = append(mergedTimes, outboundTimes[j])
	}

	var meanInterArrival float64
	var minInterArrival int64
	var maxInterArrival int64
	var stdInterArrival float64
	if len(mergedTimes) > 1 {
		intervals := make([]int64, 0, len(mergedTimes)-1)
		for i := 1; i < len(mergedTimes); i++ {
			interval := mergedTimes[i] - mergedTimes[i-1]
			intervals = append(intervals, interval)
		}

		minInterArrival = intervals[0]
		maxInterArrival = intervals[0]
		sum := int64(0)
		for _, interval := range intervals {
			if interval < minInterArrival {
				minInterArrival = interval
			}
			if interval > maxInterArrival {
				maxInterArrival = interval
			}
			sum += interval
		}

		meanInterArrival = float64(sum) / float64(len(intervals))

		variance := 0.0
		for _, interval := range intervals {
			diff := float64(interval) - meanInterArrival
			variance += diff * diff
		}
		variance /= float64(len(intervals))
		stdInterArrival = math.Sqrt(variance)
	}

	meanPktSize := float64(totalBytes) / float64(totalPackets)
	var minPktSize = ^uint16(0) // initialize to max so comparisons work
	var maxPktSize uint16
	var stdPktSize float64
	maxPktSize = 0
	sumPktSize := float64(0)
	for _, bytes := range features.Inbound {
		if bytes < minPktSize {
			minPktSize = bytes
		}
		if bytes > maxPktSize {
			maxPktSize = bytes
		}
		sumPktSize += float64(bytes)
	}
	for _, bytes := range features.Outbound {
		if bytes < minPktSize {
			minPktSize = bytes
		}
		if bytes > maxPktSize {
			maxPktSize = bytes
		}
		sumPktSize += float64(bytes)
	}
	stdPktSize = math.Sqrt(sumPktSize/float64(totalPackets) - meanPktSize*meanPktSize)

	duration := mergedTimes[len(mergedTimes)-1]

	var throughputMbps float64
	if len(mergedTimes) > 0 {
		durationSeconds := float64(duration) / 1_000_000
		if durationSeconds > 0 {
			throughputMbps = (float64(totalBytes) * 8) / (durationSeconds * 1_000_000) // convert bytes to bits, then to Mbps
		}
	}

	// Insert directly into ClickHouse (typed values)
	if database.ClickHouseConn == nil {
		// ClickHouse not available; skip DB write but still log bytes transferred
		LogBytesTransferred(features.Protocol, "in", "unknown", int(inboundBytes))
		LogBytesTransferred(features.Protocol, "out", "unknown", int(outboundBytes))
		return
	}

	batch, err := database.ClickHouseConn.PrepareBatch(context.Background(), "INSERT INTO session_features (session_id, protocol, start_time, inbound_packets, outbound_packets, inout_ratio, total_packets, inbound_bytes, outbound_bytes, total_bytes, mean_inter_arrival, min_inter_arrival, max_inter_arrival, std_inter_arrival, mean_pkt_size, min_pkt_size, max_pkt_size, std_pkt_size, duration, throughput_mbps)")
	if err != nil {
		log.Printf("clickhouse: prepare batch error: %v", err)
		return
	}

	sessionID := uuid.New().String()
	if err := batch.Append(
		sessionID,
		features.Protocol,
		features.StartTime,
		uint32(inboundPackets),
		uint32(outboundPackets),
		inoutRatio,
		uint32(totalPackets),
		uint64(inboundBytes),
		uint64(outboundBytes),
		uint64(totalBytes),
		meanInterArrival,
		minInterArrival,
		maxInterArrival,
		stdInterArrival,
		meanPktSize,
		uint64(minPktSize),
		uint64(maxPktSize),
		stdPktSize,
		duration,
		throughputMbps,
	); err != nil {
		log.Printf("clickhouse: append error: %v", err)
		return
	}

	if err := batch.Send(); err != nil {
		log.Printf("clickhouse: send batch error: %v", err)
		return
	}

	LogBytesTransferred(features.Protocol, "in", "unknown", int(inboundBytes))
	LogBytesTransferred(features.Protocol, "out", "unknown", int(outboundBytes))
}
