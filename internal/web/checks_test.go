package web_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/markedo-org/ledger/internal/app"
	"github.com/markedo-org/ledger/internal/store"
	"github.com/markedo-org/ledger/internal/web"
)

// The API and MCP are the same product, so a batch that works over MCP has to
// work over HTTP too.
func TestChecksEndpointTakesSeveralIndexes(t *testing.T) {
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

	post := func(path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(body))
		req.Header.Set("Authorization", "Bearer "+tok)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		return rec
	}

	rec := post("/v1/markedo/meta/tasks", `{"title":"Ship the slice","idempotency_key":"s1","checks":["one","two","three"]}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create %d %s", rec.Code, rec.Body.String())
	}

	rec = post("/v1/markedo/meta/tasks/T-001/checks", `{"ns":[1,3],"done":true}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("batch %d %s", rec.Code, rec.Body.String())
	}
	var out struct {
		Checks []struct {
			N    int  `json:"n"`
			Done bool `json:"done"`
		} `json:"checks"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	want := map[int]bool{1: true, 2: false, 3: true}
	for _, c := range out.Checks {
		if c.Done != want[c.N] {
			t.Fatalf("check %d done=%v, want %v", c.N, c.Done, want[c.N])
		}
	}

	// One index, the old shape, still works.
	rec = post("/v1/markedo/meta/tasks/T-001/checks", `{"n":2,"done":true}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("single %d %s", rec.Code, rec.Body.String())
	}
}
