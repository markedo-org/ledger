package web_test

import (
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

// The rotation over HTTP: list, mint, revoke with the new token, and confirm
// the old bearer is dead on a normal route.
func TestTokenListAndRevokeHTTP(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	boot, err := store.NewToken()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Bootstrap(context.Background(), "markedo", "meta", "maria", boot); err != nil {
		t.Fatal(err)
	}
	a := app.New(s)
	srv, err := web.New(a)
	if err != nil {
		t.Fatal(err)
	}
	h := srv.Handler()

	call := func(method, path, bearer, body string) *httptest.ResponseRecorder {
		var rdr *strings.Reader
		if body == "" {
			rdr = strings.NewReader("")
		} else {
			rdr = strings.NewReader(body)
		}
		req := httptest.NewRequest(method, path, rdr)
		req.Header.Set("Authorization", "Bearer "+bearer)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}

	rec := call(http.MethodGet, "/v1/markedo/tokens", boot, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("list %d %s", rec.Code, rec.Body.String())
	}
	var listed struct {
		Tokens []struct {
			ID   string `json:"id"`
			Role string `json:"role"`
		} `json:"tokens"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Tokens) != 1 {
		t.Fatalf("expected the bootstrap token only, got %d", len(listed.Tokens))
	}
	if strings.Contains(rec.Body.String(), boot) {
		t.Fatal("the listing leaked the secret")
	}
	oldID := listed.Tokens[0].ID

	rec = call(http.MethodDelete, "/v1/markedo/tokens/"+oldID, boot, "")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("a token must not revoke itself: %d %s", rec.Code, rec.Body.String())
	}

	rec = call(http.MethodPost, "/v1/markedo/tokens", boot, `{"actor":"maria","role":"admin"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("mint %d %s", rec.Code, rec.Body.String())
	}
	var minted struct {
		ID    string `json:"id"`
		Token string `json:"token"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &minted); err != nil {
		t.Fatal(err)
	}
	if minted.ID == "" || minted.Token == "" {
		t.Fatalf("mint must return the id and the plaintext: %s", rec.Body.String())
	}

	rec = call(http.MethodDelete, "/v1/markedo/tokens/"+oldID, minted.Token, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("revoke %d %s", rec.Code, rec.Body.String())
	}

	rec = call(http.MethodGet, "/v1/markedo/meta/tasks", boot, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("the revoked bearer still works: %d %s", rec.Code, rec.Body.String())
	}
	rec = call(http.MethodGet, "/v1/markedo/meta/tasks", minted.Token, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("the replacement does not work: %d %s", rec.Code, rec.Body.String())
	}
}

// A ledger-bound write token must not be able to see or kill the owner's keys.
func TestWriteTokenRefusedOnTokenRoutes(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	boot, err := store.NewToken()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Bootstrap(context.Background(), "markedo", "meta", "maria", boot); err != nil {
		t.Fatal(err)
	}
	a := app.New(s)
	auth, err := a.Auth(context.Background(), boot)
	if err != nil {
		t.Fatal(err)
	}
	issued, err := a.CreateToken(context.Background(), auth, "markedo", app.CreateTokenInput{
		Actor: "bot", Ledger: "meta", Role: "write",
	})
	if err != nil {
		t.Fatal(err)
	}
	srv, err := web.New(a)
	if err != nil {
		t.Fatal(err)
	}
	h := srv.Handler()

	for _, tc := range []struct{ method, path string }{
		{http.MethodGet, "/v1/markedo/tokens"},
		{http.MethodDelete, "/v1/markedo/tokens/" + auth.ID},
	} {
		req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(""))
		req.Header.Set("Authorization", "Bearer "+issued.Plain)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("%s %s: %d %s", tc.method, tc.path, rec.Code, rec.Body.String())
		}
	}
}
