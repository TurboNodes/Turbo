package proxy

import (
	"log"
	"math/rand"
	"sort"
	"sync"
)

var (
	// Lock-free reads with sync.Map
	countryClients sync.Map // 2-digit country code -> *CountryPool
	globalClients  sync.Map // "global" -> *CountryPool

	updateMutex sync.RWMutex
)

type CountryPool struct {
	clients           []*QuicClient
	cumulativeWeights []float64 // Pre-computed for O(log n) selection
	totalWeight       float64
	lastUpdated       int64
}

func FindClient() *QuicClient {
	if client := FindClientByCountry("global"); client != nil {
		return client
	} else {
		// Logs pool sizes for debugging
		globalPoolSize := 0
		if pool, ok := globalClients.Load("global"); ok {
			globalPool := pool.(*CountryPool)
			globalPoolSize = len(globalPool.clients)
		}

		countryPoolSize := 0
		countryClients.Range(func(key, value any) bool {
			pool := value.(*CountryPool)
			countryPoolSize += len(pool.clients)
			return true
		})

		log.Printf("DEBUG: No healthy clients found. Global pool size: %d, Country pool size: %d", globalPoolSize, countryPoolSize)
		return nil
	}
}

func FindClientByCountry(countryCode string) *QuicClient {
	var pool interface{}
	var ok bool

	if countryCode == "global" {
		pool, ok = globalClients.Load(countryCode)
	} else {
		pool, ok = countryClients.Load(countryCode)
	}

	if ok {
		countryPool := pool.(*CountryPool)
		if client := selectFromPool(countryPool); client != nil {
			return client
		}
	}

	return nil
}

func selectFromPool(pool *CountryPool) *QuicClient {
	if pool.totalWeight == 0 || len(pool.clients) == 0 {
		return nil
	}

	// Try up to 3 times to find a healthy client
	for attempts := 0; attempts < 3; attempts++ {
		randomPoint := rand.Float64() * pool.totalWeight
		idx := sort.SearchFloat64s(pool.cumulativeWeights, randomPoint)
		if idx >= len(pool.clients) {
			idx = len(pool.clients) - 1
		}

		client := pool.clients[idx]

		if client.isHealthy() {
			return client
		}
	}

	return nil // All attempts failed
}

func (c *QuicClient) isHealthy() bool {
	return c != nil && c.conn != nil
}

func updatePools() {
	updateMutex.Lock()
	defer updateMutex.Unlock()

	var globalPool CountryPool
	for _, client := range QuicClients {
		if client.isHealthy() {
			weight := client.Metrics.Score
			if weight < 1 {
				weight = 1
			}
			globalPool.clients = append(globalPool.clients, client)
			if len(globalPool.cumulativeWeights) == 0 {
				globalPool.cumulativeWeights = append(globalPool.cumulativeWeights, weight)
			} else {
				cumWeight := globalPool.cumulativeWeights[len(globalPool.cumulativeWeights)-1]
				globalPool.cumulativeWeights = append(globalPool.cumulativeWeights, cumWeight+weight)
			}
			globalPool.totalWeight += weight
		}
	}
	globalClients.Store("global", &globalPool)

	countryMap := make(map[string]*CountryPool)
	for _, client := range QuicClients {
		if client.isHealthy() {
			country := client.Stats.CountryCode
			if country == "global" {
				continue
			}

			if _, exists := countryMap[country]; !exists {
				countryMap[country] = &CountryPool{}
			}
			pool := countryMap[country]
			weight := client.Metrics.Score
			if weight < 1 {
				weight = 1
			}
			pool.clients = append(pool.clients, client)
			if len(pool.cumulativeWeights) == 0 {
				pool.cumulativeWeights = append(pool.cumulativeWeights, weight)
			} else {
				cumWeight := pool.cumulativeWeights[len(pool.cumulativeWeights)-1]
				pool.cumulativeWeights = append(pool.cumulativeWeights, cumWeight+weight)
			}
			pool.totalWeight += weight
		}
	}
	for country, pool := range countryMap {
		countryClients.Store(country, pool)
	}
}
