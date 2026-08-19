package web_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
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

func TestOwnerPageAndLedgerSettings(t *testing.T) {
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
	srv, err := web.New(a)
	if err != nil {
		t.Fatal(err)
	}
	h := srv.Handler()

	req := httptest.NewRequest(http.MethodGet, "/markedo", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("owner %d", rec.Code)
	}
	page := rec.Body.String()
	if !strings.Contains(page, "of <strong>8</strong> in use") {
		t.Fatalf("tally missing: %s", page)
	}
	if strings.Contains(page, "billing") || strings.Contains(page, "/settings") {
		t.Fatal("public owner page must not show billing or settings")
	}

	srv.SiteURL = "https://www.task-ledger.com"
	req = httptest.NewRequest(http.MethodGet, "/markedo", nil)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if !strings.Contains(rec.Body.String(), `href="https://www.task-ledger.com/billing"`) {
		t.Fatalf("billing missing: %s", rec.Body.String())
	}

	body := bytes.NewBufferString(`{"title":"The meta board"}`)
	req = httptest.NewRequest(http.MethodPatch, "/v1/markedo/ledgers/meta", body)
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !bytes.Contains(rec.Body.Bytes(), []byte("The meta board")) {
		t.Fatalf("patch title %d %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/login", nil)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	csrf := cookieVal(rec, "ledger_csrf")
	form := strings.NewReader("token=" + tok + "&csrf=" + csrf)
	req = httptest.NewRequest(http.MethodPost, "/login", form)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "ledger_csrf", Value: csrf})
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	sess := cookieVal(rec, "ledger_session")
	if sess == "" {
		t.Fatal("missing session")
	}

	req = httptest.NewRequest(http.MethodGet, "/markedo", nil)
	req.AddCookie(&http.Cookie{Name: "ledger_session", Value: sess})
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if !strings.Contains(rec.Body.String(), `/markedo/meta/settings`) {
		t.Fatalf("settings link missing: %s", rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/markedo/meta/settings", nil)
	req.AddCookie(&http.Cookie{Name: "ledger_session", Value: sess})
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	page = rec.Body.String()
	if rec.Code != http.StatusOK || !strings.Contains(page, "The meta board") {
		t.Fatalf("settings get %d %s", rec.Code, page)
	}
	if !strings.Contains(page, `<form class="settings"`) || strings.Contains(page, `form.signin`) || strings.Contains(page, `class="signin`) {
		t.Fatalf("settings should use form.settings, not signin: %s", page)
	}
	if !strings.Contains(page, "<fieldset>") || !strings.Contains(page, "Retention") {
		t.Fatalf("settings missing retention group: %s", page)
	}
	csrf = cookieVal(rec, "ledger_csrf")
	if csrf == "" {
		t.Fatal("settings csrf")
	}

	vals := url.Values{
		"csrf":                    {csrf},
		"title":                   {"Abuse manager"},
		"archive_done_after_days": {"7"},
		"purge_done_after_days":   {"0"},
	}
	req = httptest.NewRequest(http.MethodPost, "/markedo/meta/settings", strings.NewReader(vals.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "ledger_session", Value: sess})
	req.AddCookie(&http.Cookie{Name: "ledger_csrf", Value: csrf})
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("settings post %d %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/markedo", nil)
	req.AddCookie(&http.Cookie{Name: "ledger_session", Value: sess})
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if !strings.Contains(rec.Body.String(), "Abuse manager") {
		t.Fatalf("owner missing new title: %s", rec.Body.String())
	}
}

func TestReviewURLHTTP(t *testing.T) {
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
	a.PublicURL = "https://ledger.example"
	srv, err := web.New(a)
	if err != nil {
		t.Fatal(err)
	}
	srv.Auth.RequireHTML = true
	h := srv.Handler()

	req := httptest.NewRequest(http.MethodPost, "/v1/review", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("mint %d %s", rec.Code, rec.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	raw, _ := out["url"].(string)
	if !strings.Contains(raw, "/login/review?code=lgv_") {
		t.Fatalf("url %s", raw)
	}
	if strings.Contains(raw, tok) {
		t.Fatal("bearer token leaked into review url")
	}
	path := raw[strings.Index(raw, "/login/review"):]

	req = httptest.NewRequest(http.MethodGet, path, nil)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/markedo/meta" {
		t.Fatalf("consume %d loc=%s", rec.Code, rec.Header().Get("Location"))
	}
	sess := cookieVal(rec, "ledger_session")
	if sess == "" {
		t.Fatal("missing session cookie")
	}

	req = httptest.NewRequest(http.MethodGet, path, nil)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code == http.StatusFound && rec.Header().Get("Location") == "/markedo/meta" {
		t.Fatal("review code reused")
	}

	req = httptest.NewRequest(http.MethodGet, "/markedo/meta", nil)
	req.AddCookie(&http.Cookie{Name: "ledger_session", Value: sess})
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("board %d", rec.Code)
	}
}

func TestHTMLTagsFilter(t *testing.T) {
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
		Title: "Trackveil work", IdempotencyKey: "html-tag-1", Tags: []string{"trackveil"},
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := a.Create(context.Background(), auth, "markedo", "meta", app.CreateInput{
		Title: "Finance work", IdempotencyKey: "html-tag-2", Tags: []string{"finance"},
	}); err != nil {
		t.Fatal(err)
	}
	srv, err := web.New(a)
	if err != nil {
		t.Fatal(err)
	}
	h := srv.Handler()

	req := httptest.NewRequest(http.MethodGet, "/markedo/meta", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	page := rec.Body.String()
	if rec.Code != http.StatusOK || !strings.Contains(page, `class="row-tags"`) || !strings.Contains(page, ">trackveil<") {
		t.Fatalf("board chips: %d %s", rec.Code, page)
	}
	if !strings.Contains(page, `href="?tag=trackveil"`) {
		t.Fatalf("tag nav missing: %s", page)
	}

	req = httptest.NewRequest(http.MethodGet, "/markedo/meta?tag=trackveil", nil)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	page = rec.Body.String()
	if !strings.Contains(page, "Trackveil work") || strings.Contains(page, "Finance work") {
		t.Fatalf("filter: %s", page)
	}

	req = httptest.NewRequest(http.MethodGet, "/v1/markedo/meta/tasks?tag=finance", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Finance work") || strings.Contains(rec.Body.String(), "Trackveil work") {
		t.Fatalf("http filter %d %s", rec.Code, rec.Body.String())
	}

	body := bytes.NewBufferString(`{"tags":["host"]}`)
	req = httptest.NewRequest(http.MethodPost, "/v1/markedo/meta/tasks/T-001/tags", body)
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"host"`) {
		t.Fatalf("set tags %d %s", rec.Code, rec.Body.String())
	}
}
