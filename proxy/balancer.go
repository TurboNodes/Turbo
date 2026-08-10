package proxy

import (
	"log"
	"math/rand"
	data2 "server/data"
	"sort"
	"sync"
	"sync/atomic"
)

// globalPoolKey is the routing key used when a request has no country
// preference, and the country stored for nodes whose location is unknown.
const globalPoolKey = "global"

// routingTable is an immutable snapshot of the client pools. Readers grab it
// with a single atomic load and then only touch read-only data, so a lookup
// never takes a lock and never contends with a rebuild.
type routingTable struct {
	global    *CountryPool
	countries map[string]*CountryPool // ISO-3166-1 alpha-2 -> pool
}

var (
	// pools holds the current *routingTable.
	pools atomic.Pointer[routingTable]

	// rebuildMutex serialises rebuilds so a slow one cannot publish a table
	// that is already out of date by the time it stores.
	rebuildMutex sync.Mutex
)

type CountryPool struct {
	clients           []*QuicClient
	cumulativeWeights []float64 // Pre-computed for O(log n) selection
	totalWeight       float64
}

func FindClient() *QuicClient {
	return FindClientByCountry(globalPoolKey)
}

// FindClientByCountry picks a weighted-random healthy client located in
// countryCode, or nil when that country has none. Passing globalPoolKey (or an
// empty string) selects from every client regardless of location.
func FindClientByCountry(countryCode string) *QuicClient {
	table := pools.Load()
	if table == nil {
		log.Print("DEBUG: No healthy clients found. Routing table is empty")
		return nil
	}

	pool := table.global
	if countryCode != globalPoolKey && countryCode != "" {
		// Reading an immutable map needs no synchronisation.
		pool = table.countries[countryCode]
	}

	if client := selectFromPool(pool); client != nil {
		return client
	}

	log.Printf("DEBUG: No healthy clients found for %q. Global pool size: %d, countries tracked: %d",
		countryCode, len(table.global.clients), len(table.countries))
	return nil
}

func selectFromPool(pool *CountryPool) *QuicClient {
	if pool == nil || pool.totalWeight == 0 || len(pool.clients) == 0 {
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

// add appends client to the pool, extending the cumulative weight index that
// selectFromPool binary-searches.
func (p *CountryPool) add(client *QuicClient, weight float64) {
	p.totalWeight += weight
	p.clients = append(p.clients, client)
	p.cumulativeWeights = append(p.cumulativeWeights, p.totalWeight)
}

// updatePools rebuilds the routing table from the live client set and swaps it
// in wholesale, so a country that lost its last node disappears instead of
// lingering with dead entries.
func updatePools() {
	rebuildMutex.Lock()
	defer rebuildMutex.Unlock()

	QuicMutex.RLock()
	healthy := make([]*QuicClient, 0, len(QuicClients))
	for _, client := range QuicClients {
		if client.isHealthy() {
			healthy = append(healthy, client)
		}
	}
	QuicMutex.RUnlock()

	table := &routingTable{
		global:    &CountryPool{},
		countries: make(map[string]*CountryPool),
	}

	for _, client := range healthy {
		weight := client.Metrics.Score
		if weight < 1 {
			weight = 1
		}

		table.global.add(client, weight)

		country := client.Stats.CountryCode
		if country == globalPoolKey || country == "" {
			continue
		}
		pool, ok := table.countries[country]
		if !ok {
			pool = &CountryPool{}
			table.countries[country] = pool
		}
		pool.add(client, weight)
	}

	pools.Store(table)
	publishPoolGauges(table)
}

// publishPoolGauges mirrors the new table into Prometheus. Reset first so a
// country that lost its last node stops reporting a stale count.
func publishPoolGauges(table *routingTable) {
	data2.ResetActiveNodes()
	data2.SetActiveNodes("quic", globalPoolKey, len(table.global.clients))
	for country, pool := range table.countries {
		data2.SetActiveNodes("quic", country, len(pool.clients))
	}
}
