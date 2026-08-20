package web

import (
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	loginAttempts = 10
	loginWindow   = time.Minute
)

// signInGate is a fixed-window counter per client address for the sign-in
// routes. A bearer token, a magic code, and a review code are all guessable
// given enough attempts, and nothing else in front of the app counts tries.
// In-process only: it resets on restart and does not span replicas, which is
// enough for one binary on one host.
type signInGate struct {
	mu     sync.Mutex
	hits   map[string][]time.Time
	limit  int
	window time.Duration
}

func newSignInGate(limit int, window time.Duration) *signInGate {
	return &signInGate{hits: map[string][]time.Time{}, limit: limit, window: window}
}

func (g *signInGate) allow(key string, now time.Time) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	cutoff := now.Add(-g.window)
	kept := g.hits[key][:0]
	for _, t := range g.hits[key] {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	if len(kept) >= g.limit {
		g.hits[key] = kept
		return false
	}
	g.hits[key] = append(kept, now)
	if len(g.hits) > 10000 {
		g.sweep(cutoff)
	}
	return true
}

// sweep drops addresses with no attempt left in the window so a flood of
// one-shot clients cannot grow the map without bound.
func (g *signInGate) sweep(cutoff time.Time) {
	for k, ts := range g.hits {
		live := false
		for _, t := range ts {
			if t.After(cutoff) {
				live = true
				break
			}
		}
		if !live {
			delete(g.hits, k)
		}
	}
}

// trustedProxies is what gin is allowed to believe when it reads
// X-Forwarded-For. Gin's default is to trust every proxy, which means it takes
// the leftmost entry, and that entry is written by whoever is calling. An
// attacker varies it per request and never meets the sign-in limit; worse,
// they can put a victim's address there and lock that person out of their own
// account. Naming the hop we actually run makes gin walk the header from the
// right instead, past the addresses our own proxy appended.
//
// Loopback covers the shipped layout, nginx and the binary on one host. A
// proxy elsewhere, or a CDN in front, goes in LEDGER_TRUSTED_PROXIES as
// comma-separated CIDRs. An empty list is the safe answer for a binary exposed
// directly: no forwarded header is believed at all.
func trustedProxies() []string {
	out := []string{"127.0.0.1/32", "::1/128"}
	for _, p := range strings.Split(os.Getenv("LEDGER_TRUSTED_PROXIES"), ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func (s *Server) throttleSignIn(c *gin.Context) {
	if s.signIn == nil {
		c.Next()
		return
	}
	if !s.signIn.allow(c.ClientIP(), time.Now()) {
		c.Header("Retry-After", "60")
		c.String(http.StatusTooManyRequests, "Too many sign-in attempts. Wait a minute and try again.")
		c.Abort()
		return
	}
	c.Next()
}
