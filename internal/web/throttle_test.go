package web_test

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/markedo-org/ledger/internal/app"
	"github.com/markedo-org/ledger/internal/store"
	"github.com/markedo-org/ledger/internal/web"
)

func throttleHandler(t *testing.T) http.Handler {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	srv, err := web.New(app.New(s))
	if err != nil {
		t.Fatal(err)
	}
	return srv.Handler()
}

// attempt posts a wrong token at the sign-in form, which is what a guessing
// attacker does, and reports the status.
func attempt(h http.Handler, remote, forwarded string) int {
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader("token=lgr_wrong"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.RemoteAddr = remote
	if forwarded != "" {
		req.Header.Set("X-Forwarded-For", forwarded)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Code
}

// The sign-in limit is the only thing counting guesses at a bearer token, a
// magic code or a review code. Gin trusts every proxy by default, so it read
// the leftmost X-Forwarded-For entry, and that entry is written by the caller.
// A new value per request meant a fresh allowance per request and the ceiling
// was never reached.
func TestForwardedHeaderCannotBuyMoreAttempts(t *testing.T) {
	h := throttleHandler(t)

	blocked := false
	for i := 0; i < 40; i++ {
		// A different claimed address every time, from one real caller.
		if attempt(h, "203.0.113.9:51000", "10.9.9."+string(rune('1'+i%9))) == http.StatusTooManyRequests {
			blocked = true
			break
		}
	}
	if !blocked {
		t.Fatal("40 guesses from one address were all allowed by varying a header the caller writes")
	}
}

// The other half, and the worse one. An attacker who cannot get past the limit
// can still spend a victim's allowance by claiming to be them, locking that
// person out of their own account from an address they do not control.
func TestNobodyCanBurnSomeoneElsesAllowance(t *testing.T) {
	h := throttleHandler(t)

	victim := "198.51.100.7"
	for i := 0; i < 30; i++ {
		attempt(h, "203.0.113.9:51000", victim)
	}

	// The victim arrives through our own proxy, which is the only way an
	// address other than the peer is believed at all.
	if code := attempt(h, "127.0.0.1:8080", victim); code == http.StatusTooManyRequests {
		t.Fatal("a stranger locked the victim out of their own sign-in")
	}
}

// And the limit still has to work for the deployment we actually run, where
// every request arrives from nginx on loopback. If the peer were used there,
// all visitors would share one bucket and the first guesser would lock out the
// whole service.
func TestBehindOurProxyEachVisitorIsCountedSeparately(t *testing.T) {
	h := throttleHandler(t)

	for i := 0; i < 30; i++ {
		attempt(h, "127.0.0.1:8080", "203.0.113.9")
	}
	if code := attempt(h, "127.0.0.1:8080", "203.0.113.9"); code != http.StatusTooManyRequests {
		t.Fatalf("a guesser behind the proxy was never throttled: %d", code)
	}
	if code := attempt(h, "127.0.0.1:8080", "198.51.100.7"); code == http.StatusTooManyRequests {
		t.Fatal("one guesser locked out every other visitor behind the proxy")
	}
}
