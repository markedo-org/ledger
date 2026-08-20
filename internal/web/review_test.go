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

// reviewSession mints a review link from a token of the given role, opens it
// the way a human would, and returns the cookies the browser is left holding.
func reviewSession(t *testing.T, role string) (http.Handler, []*http.Cookie) {
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
	adminTok, err := a.Auth(context.Background(), admin)
	if err != nil {
		t.Fatal(err)
	}
	tok := adminTok
	if role != types.RoleAdmin {
		issued, err := a.CreateToken(context.Background(), adminTok, "markedo", app.CreateTokenInput{
			Actor:  "reader",
			Ledger: "meta",
			Role:   role,
		})
		if err != nil {
			t.Fatal(err)
		}
		if tok, err = a.Auth(context.Background(), issued.Plain); err != nil {
			t.Fatal(err)
		}
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

// A review link is how an agent hands its own human a way into their own
// board, and that human is usually the owner. Sending them off to find another
// credential to change their own retention would be friction, not safety, so
// the session carries the role of the token that minted it.
func TestAReviewLinkCarriesTheRoleThatMintedIt(t *testing.T) {
	h, cookies := reviewSession(t, types.RoleAdmin)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, withCookies(httptest.NewRequest(http.MethodGet, "/markedo/meta", nil), cookies))
	if rec.Code != http.StatusOK {
		t.Fatalf("a review link could not read the board it was minted for: %d", rec.Code)
	}

	form := strings.NewReader("title=Renamed&purge_done_after_days=1")
	req := httptest.NewRequest(http.MethodPost, "/markedo/meta/settings", form)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, withCookies(req, cookies))
	if rec.Code != http.StatusOK && rec.Code != http.StatusSeeOther {
		t.Fatalf("an admin-minted review link could not change the owner's own settings: %d", rec.Code)
	}
}

// Choosing the power means choosing the token. A link minted from a write
// token opens the board and stops there.
func TestAWriteMintedReviewLinkStopsAtSettings(t *testing.T) {
	h, cookies := reviewSession(t, types.RoleWrite)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, withCookies(httptest.NewRequest(http.MethodGet, "/markedo/meta", nil), cookies))
	if rec.Code != http.StatusOK {
		t.Fatalf("a write-minted review link could not read the board: %d", rec.Code)
	}

	form := strings.NewReader("title=Renamed&purge_done_after_days=1")
	req := httptest.NewRequest(http.MethodPost, "/markedo/meta/settings", form)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, withCookies(req, cookies))
	if rec.Code == http.StatusOK || rec.Code == http.StatusSeeOther {
		t.Fatalf("a write-minted review link changed ledger settings: %d", rec.Code)
	}
}

// The owner page offered a Settings link to any session bound to that ledger,
// including write sessions the handler then refused. An invitation to a 403 is
// a bug in the page, not a safeguard.
func TestTheOwnerPageOffersNoSettingsItWillRefuse(t *testing.T) {
	h, cookies := reviewSession(t, types.RoleWrite)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, withCookies(httptest.NewRequest(http.MethodGet, "/markedo", nil), cookies))
	if rec.Code != http.StatusOK {
		t.Fatalf("owner page answered %d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "/meta/settings") {
		t.Fatal("the owner page offered a Settings link to a session that cannot use it")
	}
}
