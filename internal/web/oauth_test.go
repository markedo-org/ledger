package web_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/markedo-org/ledger/internal/app"
	"github.com/markedo-org/ledger/internal/githuboauth"
	"github.com/markedo-org/ledger/internal/store"
	"github.com/markedo-org/ledger/internal/web"
)

type fakeGitHub struct {
	login string
	id    int64
}

func (f fakeGitHub) AuthURL(state string) string {
	return "https://github.example/login/oauth/authorize?state=" + state
}

func (f fakeGitHub) Exchange(context.Context, string) (string, error) {
	return "gho_test", nil
}

func (f fakeGitHub) User(context.Context, string) (githuboauth.User, error) {
	return githuboauth.User{ID: f.id, Login: f.login}, nil
}

func oauthServer(t *testing.T, allow []string, gh web.GitHub) http.Handler {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	tok, err := store.NewToken()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Bootstrap(context.Background(), "markedo", "meta", "maria", tok); err != nil {
		t.Fatal(err)
	}
	srv, err := web.New(app.New(s))
	if err != nil {
		t.Fatal(err)
	}
	srv.Auth = web.AuthConfig{
		ClientID:     "id",
		ClientSecret: "secret",
		CallbackURL:  "http://127.0.0.1:8080/auth/github/callback",
		Allowlist:    allow,
	}
	srv.GitHub = gh
	return srv.Handler()
}

func TestOAuthGatesHTML(t *testing.T) {
	h := oauthServer(t, []string{"lgforsberg"}, fakeGitHub{login: "lgforsberg", id: 1})
	req := httptest.NewRequest(http.MethodGet, "/markedo/meta", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusFound || !strings.Contains(rec.Header().Get("Location"), "/login") {
		t.Fatalf("gate %d %s", rec.Code, rec.Header().Get("Location"))
	}
}

func TestOAuthCallbackAllowlist(t *testing.T) {
	h := oauthServer(t, []string{"lgforsberg"}, fakeGitHub{login: "lgforsberg", id: 42})
	req := httptest.NewRequest(http.MethodGet, "/auth/github/callback?code=abc&state=xyz", nil)
	req.AddCookie(&http.Cookie{Name: "ledger_oauth_state", Value: "xyz"})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("callback %d %s", rec.Code, rec.Body.String())
	}
	var session string
	for _, c := range rec.Result().Cookies() {
		if c.Name == "ledger_session" {
			session = c.Value
		}
	}
	if session == "" {
		t.Fatal("missing session cookie")
	}
	req = httptest.NewRequest(http.MethodGet, "/markedo/meta", nil)
	req.AddCookie(&http.Cookie{Name: "ledger_session", Value: session})
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "markedo/meta") {
		t.Fatalf("html %d %s", rec.Code, rec.Body.String())
	}
}

func TestOAuthAllowlistRejects(t *testing.T) {
	h := oauthServer(t, []string{"lgforsberg"}, fakeGitHub{login: "stranger", id: 9})
	req := httptest.NewRequest(http.MethodGet, "/auth/github/callback?code=abc&state=xyz", nil)
	req.AddCookie(&http.Cookie{Name: "ledger_oauth_state", Value: "xyz"})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status %d %s", rec.Code, rec.Body.String())
	}
}

func TestOwnerIndex(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	tok, _ := store.NewToken()
	_, _ = s.Bootstrap(context.Background(), "markedo", "meta", "maria", tok)
	srv, err := web.New(app.New(s))
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/markedo", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "meta") {
		t.Fatalf("owner %d %s", rec.Code, rec.Body.String())
	}
}

func TestTokenLogin(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	tok, err := store.NewToken()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Bootstrap(context.Background(), "markedo", "meta", "maria", tok); err != nil {
		t.Fatal(err)
	}
	srv, err := web.New(app.New(s))
	if err != nil {
		t.Fatal(err)
	}
	srv.Auth.RequireHTML = true
	h := srv.Handler()

	req := httptest.NewRequest(http.MethodGet, "/markedo/meta", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusFound || !strings.Contains(rec.Header().Get("Location"), "/login") {
		t.Fatalf("gate %d loc=%s", rec.Code, rec.Header().Get("Location"))
	}

	form := strings.NewReader("token=" + tok)
	req = httptest.NewRequest(http.MethodPost, "/login", form)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code == http.StatusFound {
		t.Fatal("login without csrf must not redirect")
	}

	req = httptest.NewRequest(http.MethodGet, "/login", nil)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("login form %d", rec.Code)
	}
	csrf := cookieVal(rec, "ledger_csrf")
	if csrf == "" {
		t.Fatal("missing csrf cookie")
	}
	form = strings.NewReader("token=" + tok + "&csrf=" + csrf)
	req = httptest.NewRequest(http.MethodPost, "/login", form)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "ledger_csrf", Value: csrf})
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("login %d %s", rec.Code, rec.Body.String())
	}
	var session string
	for _, c := range rec.Result().Cookies() {
		if c.Name == "ledger_session" {
			session = c.Value
		}
	}
	if session == "" {
		t.Fatal("missing session cookie")
	}
	req = httptest.NewRequest(http.MethodGet, "/markedo/meta", nil)
	req.AddCookie(&http.Cookie{Name: "ledger_session", Value: session})
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "maria") {
		t.Fatalf("html %d %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/elsewhere/meta", nil)
	req.AddCookie(&http.Cookie{Name: "ledger_session", Value: session})
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("other owner %d %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/logout", nil)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound && rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("get logout %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodPost, "/logout", strings.NewReader("csrf="+csrf))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "ledger_session", Value: session})
	req.AddCookie(&http.Cookie{Name: "ledger_csrf", Value: csrf})
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("logout %d %s", rec.Code, rec.Body.String())
	}
}

func cookieVal(rec *httptest.ResponseRecorder, name string) string {
	for _, c := range rec.Result().Cookies() {
		if c.Name == name {
			return c.Value
		}
	}
	return ""
}

func TestLoginGitHubStartsAuthorize(t *testing.T) {
	h := oauthServer(t, []string{"lgforsberg"}, fakeGitHub{login: "lgforsberg", id: 1})
	req := httptest.NewRequest(http.MethodGet, "/login/github", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusFound || !strings.Contains(rec.Header().Get("Location"), "github.example") {
		t.Fatalf("github start %d %s", rec.Code, rec.Header().Get("Location"))
	}
}
