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

// One authenticated write token could send a body of any size at all, which
// went into SQLite and stayed there.
func TestABodyBiggerThanTheCapNeverReachesTheDatabase(t *testing.T) {
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
	e := srv.Handler()

	huge, err := json.Marshal(map[string]string{
		"title":           strings.Repeat("a", web.MaxRequestBody+1024),
		"idempotency_key": "huge",
	})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/markedo/meta/tasks", bytes.NewReader(huge))
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status %d, want 413: %s", rec.Code, rec.Body.String())
	}

	// And the board is still empty, which is the part that matters.
	req = httptest.NewRequest(http.MethodGet, "/v1/markedo/meta/tasks", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	var out struct {
		Tasks []map[string]any `json:"tasks"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Tasks) != 0 {
		t.Fatalf("%d tasks were created", len(out.Tasks))
	}
}

// A title inside the request cap but past what a title is for is refused by the
// app, so every surface gets the same answer.
func TestALongTitleIsRefusedWithAUsefulMessage(t *testing.T) {
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
	e := srv.Handler()

	body, err := json.Marshal(map[string]string{
		"title":           strings.Repeat("a", app.MaxTitle+1),
		"idempotency_key": "long-title",
	})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/markedo/meta/tasks", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "title must be") {
		t.Fatalf("the message should name the field and the limit: %s", rec.Body.String())
	}
}
