package web_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/markedo-org/ledger/internal/app"
	"github.com/markedo-org/ledger/internal/store"
	"github.com/markedo-org/ledger/internal/web"
)

func TestCreateAndHTML(t *testing.T) {
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

	body := bytes.NewBufferString(`{"title":"Ship the slice","idempotency_key":"s1"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/markedo/meta/tasks", body)
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create %d %s", rec.Code, rec.Body.String())
	}
	var created map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created["handle"] != "T-001" {
		t.Fatalf("handle %#v", created["handle"])
	}

	body = bytes.NewBufferString(`{"title":"No key"}`)
	req = httptest.NewRequest(http.MethodPost, "/v1/markedo/meta/tasks", body)
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("missing key %d %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/markedo/meta", nil)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !bytes.Contains(rec.Body.Bytes(), []byte("T-001")) {
		t.Fatalf("html %d %s", rec.Code, rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`class="row" href="/markedo/meta/T-001"`)) {
		t.Fatalf("board row link missing: %s", rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`href="/markedo/meta?done=1"`)) {
		t.Fatalf("archive link missing: %s", rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`id="theme-flip"`)) {
		t.Fatalf("theme flip missing: %s", rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`/static/badge.svg`)) {
		t.Fatalf("badge missing: %s", rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/static/badge.svg", nil)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !bytes.Contains(rec.Body.Bytes(), []byte("#1A2B22")) {
		t.Fatalf("badge static %d %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/markedo/meta.md", nil)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !bytes.Contains(rec.Body.Bytes(), []byte("do not edit")) {
		t.Fatalf("md %d %s", rec.Code, rec.Body.String())
	}

	body = bytes.NewBufferString(`{"title":"Checks","idempotency_key":"c1","checks":["API"]}`)
	req = httptest.NewRequest(http.MethodPost, "/v1/markedo/meta/tasks", body)
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create checks %d %s", rec.Code, rec.Body.String())
	}
	body = bytes.NewBufferString(`{"n":1,"done":true}`)
	req = httptest.NewRequest(http.MethodPost, "/v1/markedo/meta/tasks/T-002/checks", body)
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !bytes.Contains(rec.Body.Bytes(), []byte(`"done":true`)) {
		t.Fatalf("tick %d %s", rec.Code, rec.Body.String())
	}

	body = bytes.NewBufferString(`{"body":"looks good"}`)
	req = httptest.NewRequest(http.MethodPost, "/v1/markedo/meta/tasks/T-002/notes", body)
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("note %d %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/markedo/meta", nil)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !bytes.Contains(rec.Body.Bytes(), []byte("1 note")) || !bytes.Contains(rec.Body.Bytes(), []byte("1/1 checks")) {
		t.Fatalf("board meta %d %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/markedo/meta/T-002", nil)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !bytes.Contains(rec.Body.Bytes(), []byte(`class="done"`)) || !bytes.Contains(rec.Body.Bytes(), []byte("looks good")) {
		t.Fatalf("task page %d %s", rec.Code, rec.Body.String())
	}
}

func TestHTMLArchiveAndHTTPDone(t *testing.T) {
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
	auth, err := a.Auth(context.Background(), tok)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := a.Create(context.Background(), auth, "markedo", "meta", app.CreateInput{
		Title: "Closed", IdempotencyKey: "arch-1",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Close(context.Background(), auth, "markedo", "meta", "T-001", "closed", ""); err != nil {
		t.Fatal(err)
	}
	l, _ := a.Ledger(context.Background(), auth, "markedo", "meta")
	if err := s.SetClosedAt(context.Background(), l.ID, "T-001", time.Now().UTC().AddDate(0, 0, -9)); err != nil {
		t.Fatal(err)
	}
	srv, err := web.New(a)
	if err != nil {
		t.Fatal(err)
	}
	e := srv.Handler()

	req := httptest.NewRequest(http.MethodGet, "/markedo/meta", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || bytes.Contains(rec.Body.Bytes(), []byte("T-001")) {
		t.Fatalf("board should hide old DONE: %d %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/markedo/meta?done=1", nil)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !bytes.Contains(rec.Body.Bytes(), []byte("T-001")) {
		t.Fatalf("archive missing T-001: %d %s", rec.Code, rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`href="/markedo/meta"`)) {
		t.Fatalf("board link missing: %s", rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/v1/markedo/meta/tasks", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || bytes.Contains(rec.Body.Bytes(), []byte("T-001")) {
		t.Fatalf("HTTP default list: %d %s", rec.Code, rec.Body.String())
	}
	req = httptest.NewRequest(http.MethodGet, "/v1/markedo/meta/tasks?done=1", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !bytes.Contains(rec.Body.Bytes(), []byte("T-001")) {
		t.Fatalf("HTTP done list: %d %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/markedo/meta/T-001", nil)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !bytes.Contains(rec.Body.Bytes(), []byte("Closed")) {
		t.Fatalf("get hidden DONE: %d %s", rec.Code, rec.Body.String())
	}
}
