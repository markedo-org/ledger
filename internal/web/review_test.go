package web_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/markedo-org/ledger/internal/app"
	"github.com/markedo-org/ledger/internal/store"
	"github.com/markedo-org/ledger/internal/types"
	"github.com/markedo-org/ledger/internal/web"
)

// reviewSession mints a review link from the owner's admin token, opens it the
// way a human would, and returns the cookies the browser is left holding.
func reviewSession(t *testing.T) (http.Handler, []*http.Cookie) {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	admin, err := store.NewToken()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Bootstrap(context.Background(), "markedo", "meta", "maria", admin); err != nil {
		t.Fatal(err)
	}
	a := app.New(s)
	tok, err := a.Auth(context.Background(), admin)
	if err != nil {
		t.Fatal(err)
	}
	if tok.Role != types.RoleAdmin {
		t.Fatalf("bootstrap token role = %q, wanted an admin token to mint from", tok.Role)
	}
	url, _, err := a.MintReviewURL(context.Background(), tok)
	if err != nil {
		t.Fatal(err)
	}
	code := url[strings.Index(url, "code=")+len("code="):]

	srv, err := web.New(a)
	if err != nil {
		t.Fatal(err)
	}
	h := srv.Handler()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/login/review?code="+code, nil))
	if rec.Code != http.StatusSeeOther && rec.Code != http.StatusFound {
		t.Fatalf("opening the review link answered %d", rec.Code)
	}
	return h, rec.Result().Cookies()
}

func withCookies(req *http.Request, cookies []*http.Cookie) *http.Request {
	for _, c := range cookies {
		req.AddCookie(c)
	}
	return req
}

// The link is built to be pasted into a chat, where it gets logged, forwarded
// and screen-shared. Minting it from an admin token used to hand admin to
// everyone downstream of that message: the reviewer could rewrite retention,
// which decides when finished work is deleted.
func TestAReviewLinkOnlyEverReads(t *testing.T) {
	h, cookies := reviewSession(t)

	// Reading is the whole point, so that must still work.
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, withCookies(httptest.NewRequest(http.MethodGet, "/markedo/meta", nil), cookies))
	if rec.Code != http.StatusOK {
		t.Fatalf("a review link could not read the board it was minted for: %d", rec.Code)
	}

	// Changing retention is not.
	form := strings.NewReader("title=Renamed&purge_done_after_days=1")
	req := httptest.NewRequest(http.MethodPost, "/markedo/meta/settings", form)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, withCookies(req, cookies))
	if rec.Code == http.StatusOK || rec.Code == http.StatusSeeOther {
		t.Fatalf("a review session changed ledger settings: %d", rec.Code)
	}

	// And the settings page itself stays shut, rather than rendering a form
	// that only fails on submit.
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, withCookies(httptest.NewRequest(http.MethodGet, "/markedo/meta/settings", nil), cookies))
	if rec.Code == http.StatusOK {
		t.Fatal("a review session was shown the settings form")
	}
}

// The owner page offered a Settings link to any session bound to that ledger,
// including ones the handler then refused. An invitation to a 403 is a bug in
// the page, not a safeguard.
func TestTheOwnerPageOffersNoSettingsItWillRefuse(t *testing.T) {
	h, cookies := reviewSession(t)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, withCookies(httptest.NewRequest(http.MethodGet, "/markedo", nil), cookies))
	if rec.Code != http.StatusOK {
		t.Fatalf("owner page answered %d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "/meta/settings") {
		t.Fatal("the owner page offered a Settings link to a session that cannot use it")
	}
}
