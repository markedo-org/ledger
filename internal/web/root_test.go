package web_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/markedo-org/ledger/internal/app"
	"github.com/markedo-org/ledger/internal/store"
	"github.com/markedo-org/ledger/internal/web"
)

func TestParseRoot(t *testing.T) {
	if _, err := web.ParseRoot("", "", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := web.ParseRoot("javascript", "", ""); err == nil {
		t.Fatal("bad mode")
	}
	if _, err := web.ParseRoot("url", "/relative", ""); err == nil {
		t.Fatal("relative url")
	}
	if _, err := web.ParseRoot("url", "javascript:alert(1)", ""); err == nil {
		t.Fatal("javascript url")
	}
	if _, err := web.ParseRoot("url", "https://evil.example", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := web.ParseRoot("file", "", ""); err == nil {
		t.Fatal("file without path")
	}
}

func TestRootLoginDefault(t *testing.T) {
	h := rootHandler(t, web.RootConfig{})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/login" {
		t.Fatalf("%d %s", rec.Code, rec.Header().Get("Location"))
	}
}

func TestRootURL(t *testing.T) {
	h := rootHandler(t, web.RootConfig{Mode: web.RootURL, URL: "https://www.task-ledger.com"})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusMovedPermanently || rec.Header().Get("Location") != "https://www.task-ledger.com" {
		t.Fatalf("%d %s", rec.Code, rec.Header().Get("Location"))
	}
}

func TestRootFile(t *testing.T) {
	p := filepath.Join(t.TempDir(), "index.html")
	if err := os.WriteFile(p, []byte("<h1>hello</h1>"), 0o644); err != nil {
		t.Fatal(err)
	}
	h := rootHandler(t, web.RootConfig{Mode: web.RootFile, File: p})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || rec.Body.String() != "<h1>hello</h1>" {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
}

func rootHandler(t *testing.T, root web.RootConfig) http.Handler {
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
	if _, err := s.Bootstrap(t.Context(), "markedo", "meta", "maria", tok); err != nil {
		t.Fatal(err)
	}
	srv, err := web.New(app.New(s))
	if err != nil {
		t.Fatal(err)
	}
	srv.Root = root
	return srv.Handler()
}
