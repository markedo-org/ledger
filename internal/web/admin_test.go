package web_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/markedo-org/ledger/internal/app"
	"github.com/markedo-org/ledger/internal/store"
	"github.com/markedo-org/ledger/internal/web"
)

func TestOperatorHTTPAndAdminPage(t *testing.T) {
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
	a := app.New(s)
	a.SetOperatorToken("lgr_op_http")
	srv, err := web.New(a)
	if err != nil {
		t.Fatal(err)
	}
	h := srv.Handler()

	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusFound || !strings.Contains(rec.Header().Get("Location"), "/login") {
		t.Fatalf("admin gate %d %s", rec.Code, rec.Header().Get("Location"))
	}

	body := bytes.NewBufferString(`{"slug":"acme","ledger":"inbox","actor":"ada"}`)
	req = httptest.NewRequest(http.MethodPost, "/v1/owners", body)
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("owner admin create owner %d %s", rec.Code, rec.Body.String())
	}

	body = bytes.NewBufferString(`{"slug":"acme","ledger":"inbox","actor":"ada"}`)
	req = httptest.NewRequest(http.MethodPost, "/v1/owners", body)
	req.Header.Set("Authorization", "Bearer lgr_op_http")
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create owner %d %s", rec.Code, rec.Body.String())
	}
	var created map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created["slug"] != "acme" || created["token"] == nil {
		t.Fatalf("body %+v", created)
	}

	body = bytes.NewBufferString(`{"max_ledgers":1}`)
	req = httptest.NewRequest(http.MethodPatch, "/v1/owners/acme", body)
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("owner patch %d %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/login", nil)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	csrf := cookieVal(rec, "ledger_csrf")
	form := strings.NewReader("token=lgr_op_http&csrf=" + csrf)
	req = httptest.NewRequest(http.MethodPost, "/login", form)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "ledger_csrf", Value: csrf})
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/admin" {
		t.Fatalf("operator login %d loc=%s", rec.Code, rec.Header().Get("Location"))
	}
	sess := cookieVal(rec, "ledger_session")
	req = httptest.NewRequest(http.MethodGet, "/admin", nil)
	req.AddCookie(&http.Cookie{Name: "ledger_session", Value: sess})
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !bytes.Contains(rec.Body.Bytes(), []byte("acme")) {
		t.Fatalf("admin page %d %s", rec.Code, rec.Body.String())
	}
	page := rec.Body.Bytes()
	for _, want := range []string{`class="desk"`, "Create owner", "Set cap", "Create ledger", "Mint token"} {
		if !bytes.Contains(page, []byte(want)) {
			t.Fatalf("admin framing missing %q", want)
		}
	}
}
