package proxy

import (
	"encoding/json"
	"fmt"
	"log"
	"server/data/database/supabase"
	"time"
)

const (
	pairingRetryDelay   = 30 * time.Second
	pairingPollInterval = 5 * time.Second
	pairingTTL          = 2 * time.Hour
)

type pendingPairing struct {
	UUID      string
	NodeIP    string
	StartedAt time.Time
	ExpiresAt time.Time
}

const WEBSITE_URL = "https://turbo-node.vercel.app"

// runPairing owns the full pairing lifecycle for this session:
// create UUID → send pairing_url → poll until paired or expiry → refresh URL on expiry.
func (c *QuicClient) runPairing() {
	for {
		if c.kicked.Load() {
			return
		}

		userID, err := supabase.GetNodeUserID(supabase.SupabaseDB, c.NodeIP)
		if err != nil {
			// Ownership lookup must not block issuing a pairing URL — the client
			// expects pairing_url on launch. Retry the lookup after sending.
			log.Printf("pairing owner lookup %s: %v (issuing pairing_url anyway)", c.NodeIP, err)
		} else if userID != "" {
			if err := c.sendPaired(userID); err != nil {
				log.Printf("paired send %s: %v", c.NodeIP, err)
				if !c.pairingSleep(pairingRetryDelay) {
					return
				}
				continue
			}
			log.Printf("paired sent node=%s", c.NodeIP)
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

		if expiresAt.IsZero() {
			expiresAt = time.Now().Add(pairingTTL)
		}

		// Poll for claim until the UUID expires, then mint a fresh one.
		for time.Now().Before(expiresAt) {
			if !c.pairingSleep(pairingPollInterval) {
				return
			}
			if c.kicked.Load() {
				return
			}

			userID, err := supabase.GetNodeUserID(supabase.SupabaseDB, c.NodeIP)
			if err != nil {
				log.Printf("pairing owner poll %s: %v", c.NodeIP, err)
				continue
			}
			if userID != "" {
				if err := c.sendPaired(userID); err != nil {
					log.Printf("paired send %s: %v", c.NodeIP, err)
					continue
				}
				log.Printf("paired sent node=%s", c.NodeIP)
				c.clearPending()
				return
			}
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

func (c *QuicClient) sendPaired(userID string) error {
	tokens, err := supabase.IssueUserSessionTokens(userID)
	if err != nil {
		return err
	}
	data, err := json.Marshal(tokens)
	if err != nil {
		return err
	}
	return c.SendMessage(Message{Type: "paired", Data: string(data)})
}
