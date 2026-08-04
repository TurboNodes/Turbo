package proxy

import (
	"fmt"
	"log"
	"server/data/database/supabase"
	"time"
)

const (
	pairingRetryDelay = 30 * time.Second
	pairingTTL        = 2 * time.Hour
)

type pendingPairing struct {
	UUID      string
	NodeIP    string
	StartedAt time.Time
	ExpiresAt time.Time
}

const WEBSITE_URL = "https://turbo-node.vercel.app"

// runPairing owns the full pairing lifecycle for this session:
// create UUID → send pairing_url → on expiry refresh → stop when paired or disconnected.
func (c *QuicClient) runPairing() {
	for {
		if c.kicked.Load() {
			return
		}

		userID, err := supabase.GetNodeUserID(supabase.SupabaseDB, c.NodeIP)
		if err != nil {
			log.Printf("pairing owner lookup %s: %v", c.NodeIP, err)
			if !c.pairingSleep(pairingRetryDelay) {
				return
			}
			continue
		}
		if userID != "" {
			c.clearPending()
			return
		}

		id, expiresAt, err := supabase.CreateNodeConnectRequest(supabase.SupabaseDB, c.NodeIP)
		if err != nil {
			log.Printf("pairing insert %s: %v", c.NodeIP, err)
			if !c.pairingSleep(pairingRetryDelay) {
				return
			}
			continue
		}

		c.pairingMu.Lock()
		c.pending = &pendingPairing{
			UUID:      id,
			NodeIP:    c.NodeIP,
			StartedAt: time.Now(),
			ExpiresAt: expiresAt,
		}
		c.pairingMu.Unlock()

		url := fmt.Sprintf("%s/connect?uuid=%s", WEBSITE_URL, id)
		if err := c.SendMessage(Message{Type: "pairing_url", ID: id, Data: url}); err != nil {
			log.Printf("pairing url send %s: %v", c.NodeIP, err)
			if !c.pairingSleep(pairingRetryDelay) {
				return
			}
			continue
		}
		log.Printf("pairing_url sent node=%s uuid=%s expires=%s", c.NodeIP, id, expiresAt.Format(time.RFC3339))

		wait := time.Until(expiresAt)
		if wait <= 0 {
			wait = pairingTTL
		}
		if !c.pairingSleep(wait) {
			return
		}
	}
}

func (c *QuicClient) pairingSleep(d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return true
	case <-c.pairingStop:
		return false
	}
}

func (c *QuicClient) stopPairing() {
	c.pairingMu.Lock()
	defer c.pairingMu.Unlock()
	c.pairingStopOnce.Do(func() {
		close(c.pairingStop)
	})
	c.pending = nil
}

func (c *QuicClient) clearPending() {
	c.pairingMu.Lock()
	c.pending = nil
	c.pairingMu.Unlock()
}
