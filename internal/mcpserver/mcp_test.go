package mcpserver_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/markedo-org/ledger/internal/app"
	"github.com/markedo-org/ledger/internal/mcpserver"
	"github.com/markedo-org/ledger/internal/store"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type bearerTransport struct {
	base  http.RoundTripper
	token string
}

func (b bearerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	r := req.Clone(req.Context())
	r.Header.Set("Authorization", "Bearer "+b.token)
	return b.base.RoundTrip(r)
}

func TestMCPCreateAndList(t *testing.T) {
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
	httpSrv := httptest.NewServer(mcpserver.Handler(app.New(s)))
	t.Cleanup(httpSrv.Close)

	ctx := context.Background()
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "v0.1.0"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{
		Endpoint:   httpSrv.URL,
		HTTPClient: &http.Client{Transport: bearerTransport{base: http.DefaultTransport, token: tok}},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })

	tools, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(tools.Tools) < 8 {
		t.Fatalf("tools %d", len(tools.Tools))
	}

	empty, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "list_tasks", Arguments: map[string]any{}})
	if err != nil || empty.IsError {
		t.Fatalf("empty list %v %+v", err, empty)
	}
	raw, _ := json.Marshal(empty.StructuredContent)
	if strings.Contains(string(raw), `"tasks":null`) {
		t.Fatalf("empty list encoded null: %s", raw)
	}

	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "create_task",
		Arguments: map[string]any{
			"title":           "From MCP",
			"idempotency_key": "mcp-1",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("tool error: %+v", res.Content)
	}

	missing, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "create_task",
		Arguments: map[string]any{"title": "No key"},
	})
	if err == nil && (missing == nil || !missing.IsError) {
		t.Fatal("create_task without idempotency_key should be rejected")
	}

	listed, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "list_tasks", Arguments: map[string]any{}})
	if err != nil || listed.IsError {
		t.Fatalf("list %v %+v", err, listed)
	}
}

func TestMCPCheckAndClose(t *testing.T) {
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
	httpSrv := httptest.NewServer(mcpserver.Handler(app.New(s)))
	t.Cleanup(httpSrv.Close)

	ctx := context.Background()
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "v0.1.0"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{
		Endpoint:   httpSrv.URL,
		HTTPClient: &http.Client{Transport: bearerTransport{base: http.DefaultTransport, token: tok}},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })

	created, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "create_task",
		Arguments: map[string]any{
			"title":           "Checks",
			"idempotency_key": "chk-mcp",
			"checks":          []string{"one", "two"},
		},
	})
	if err != nil || created.IsError {
		t.Fatalf("create %v %+v", err, created)
	}
	if _, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "set_check",
		Arguments: map[string]any{"handle": "T-001", "n": 1, "done": true},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "set_check",
		Arguments: map[string]any{"handle": "T-001", "body": "two", "done": true},
	}); err != nil {
		t.Fatal(err)
	}
	closed, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "close_task",
		Arguments: map[string]any{"handle": "T-001", "evidence": "mcp test"},
	})
	if err != nil || closed.IsError {
		t.Fatalf("close %v %+v", err, closed)
	}
}

func TestMCPInitializeJSON(t *testing.T) {
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
	httpSrv := httptest.NewServer(mcpserver.Handler(app.New(s)))
	t.Cleanup(httpSrv.Close)

	body := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"cursor","version":"1"}}}`
	req, err := http.NewRequest(http.MethodPost, httpSrv.URL, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d", resp.StatusCode)
	}
	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "application/json") {
		t.Fatalf("content-type %q, want json not sse", ct)
	}
}

func TestMCPUnauthorized(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	tok, _ := store.NewToken()
	_, _ = s.Bootstrap(context.Background(), "markedo", "meta", "maria", tok)
	httpSrv := httptest.NewServer(mcpserver.Handler(app.New(s)))
	t.Cleanup(httpSrv.Close)

	resp, err := http.Post(httpSrv.URL, "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status %d", resp.StatusCode)
	}
}

func TestMCPSetPhaseForce(t *testing.T) {
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
	httpSrv := httptest.NewServer(mcpserver.Handler(app.New(s)))
	t.Cleanup(httpSrv.Close)

	ctx := context.Background()
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "v0.1.0"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{
		Endpoint:   httpSrv.URL,
		HTTPClient: &http.Client{Transport: bearerTransport{base: http.DefaultTransport, token: tok}},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })

	if res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "create_task",
		Arguments: map[string]any{
			"title":           "Slippery",
			"idempotency_key": "force-1",
		},
	}); err != nil || res.IsError {
		t.Fatalf("create %v %+v", err, res)
	}
	for _, phase := range []string{"NEXT", "LATER", "GATED"} {
		res, err := session.CallTool(ctx, &mcp.CallToolParams{
			Name: "set_phase",
			Arguments: map[string]any{
				"handle": "T-001",
				"phase":  phase,
				"reason": "not now",
			},
		})
		if err != nil || res.IsError {
			t.Fatalf("defer %s: %v %+v", phase, err, res)
		}
	}
	blocked, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "set_phase",
		Arguments: map[string]any{
			"handle": "T-001",
			"phase":  "PARKED",
			"reason": "again",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !blocked.IsError {
		t.Fatal("fourth deferral should be blocked")
	}
	forced, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "set_phase",
		Arguments: map[string]any{
			"handle": "T-001",
			"phase":  "PARKED",
			"reason": "park it",
			"force":  true,
		},
	})
	if err != nil || forced.IsError {
		t.Fatalf("force %v %+v", err, forced)
	}
}

func TestMCPOwnerScopedDefaultsToOnlyLedger(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	boot, err := store.NewToken()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Bootstrap(context.Background(), "acme", "inbox", "ada", boot); err != nil {
		t.Fatal(err)
	}
	a := app.New(s)
	ctx := context.Background()
	admin, err := a.Auth(ctx, boot)
	if err != nil {
		t.Fatal(err)
	}
	issued, err := a.CreateToken(ctx, admin, "acme", app.CreateTokenInput{Actor: "pat", Role: "write"})
	if err != nil || issued.Token.LedgerSlug != "" {
		t.Fatalf("owner-scoped token %+v %v", issued.Token, err)
	}

	httpSrv := httptest.NewServer(mcpserver.Handler(a))
	t.Cleanup(httpSrv.Close)
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "v0.1.0"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{
		Endpoint:   httpSrv.URL,
		HTTPClient: &http.Client{Transport: bearerTransport{base: http.DefaultTransport, token: issued.Plain}},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })

	listed, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "list_tasks", Arguments: map[string]any{}})
	if err != nil || listed.IsError {
		t.Fatalf("list without ledger %v %+v", err, listed)
	}
	created, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "create_task",
		Arguments: map[string]any{
			"title":           "From owner-scoped token",
			"idempotency_key": "own-1",
		},
	})
	if err != nil || created.IsError {
		t.Fatalf("create without ledger %v %+v", err, created)
	}
}

func TestMCPCreateLedgerReturnsProjectMCP(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	tok, err := store.NewToken()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Bootstrap(context.Background(), "acme", "inbox", "ada", tok); err != nil {
		t.Fatal(err)
	}
	a := app.New(s)
	a.PublicURL = "https://ledger.example"
	httpSrv := httptest.NewServer(mcpserver.Handler(a))
	t.Cleanup(httpSrv.Close)
	ctx := context.Background()
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "v0.1.0"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{
		Endpoint:   httpSrv.URL,
		HTTPClient: &http.Client{Transport: bearerTransport{base: http.DefaultTransport, token: tok}},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })

	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "create_ledger",
		Arguments: map[string]any{"slug": "jobs"},
	})
	if err != nil || res.IsError {
		t.Fatalf("create_ledger %v %+v", err, res)
	}
	raw, _ := json.Marshal(res.StructuredContent)
	if !strings.Contains(string(raw), `"task-ledger-jobs"`) || !strings.Contains(string(raw), `"token"`) {
		t.Fatalf("expected project token and mcp: %s", raw)
	}
}

func TestMCPListDoneOnly(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	tok, err := store.NewToken()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Bootstrap(context.Background(), "acme", "inbox", "ada", tok); err != nil {
		t.Fatal(err)
	}
	a := app.New(s)
	httpSrv := httptest.NewServer(mcpserver.Handler(a))
	t.Cleanup(httpSrv.Close)
	ctx := context.Background()
	auth, err := a.Auth(ctx, tok)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := a.Create(ctx, auth, "acme", "inbox", app.CreateInput{
		Title: "Old done", IdempotencyKey: "mcp-done-1",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Close(ctx, auth, "acme", "inbox", "T-001", "closed", ""); err != nil {
		t.Fatal(err)
	}
	l, _ := a.Ledger(ctx, auth, "acme", "inbox")
	if err := s.SetClosedAt(ctx, l.ID, "T-001", time.Now().UTC().AddDate(0, 0, -10)); err != nil {
		t.Fatal(err)
	}

	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "v0.1.0"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{
		Endpoint:   httpSrv.URL,
		HTTPClient: &http.Client{Transport: bearerTransport{base: http.DefaultTransport, token: tok}},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })

	def, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "list_tasks", Arguments: map[string]any{}})
	if err != nil || def.IsError {
		t.Fatalf("default list %v %+v", err, def)
	}
	raw, _ := json.Marshal(def.StructuredContent)
	if strings.Contains(string(raw), "T-001") {
		t.Fatalf("default list should hide old DONE: %s", raw)
	}
	only, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "list_tasks",
		Arguments: map[string]any{"done": true},
	})
	if err != nil || only.IsError {
		t.Fatalf("done list %v %+v", err, only)
	}
	raw, _ = json.Marshal(only.StructuredContent)
	if !strings.Contains(string(raw), "T-001") {
		t.Fatalf("done list missing T-001: %s", raw)
	}
}

func TestMCPReviewURL(t *testing.T) {
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
	httpSrv := httptest.NewServer(mcpserver.Handler(a))
	t.Cleanup(httpSrv.Close)
	ctx := context.Background()
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "v0.1.0"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{
		Endpoint:   httpSrv.URL,
		HTTPClient: &http.Client{Transport: bearerTransport{base: http.DefaultTransport, token: tok}},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })

	tools, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, tool := range tools.Tools {
		if tool.Name == "review_url" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("review_url missing from tools/list")
	}

	res, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "review_url", Arguments: map[string]any{}})
	if err != nil || res.IsError {
		t.Fatalf("review_url %v %+v", err, res)
	}
	raw, _ := json.Marshal(res.StructuredContent)
	if !strings.Contains(string(raw), "/login/review?code=lgv_") {
		t.Fatalf("expected review url: %s", raw)
	}
	if strings.Contains(string(raw), tok) {
		t.Fatal("bearer token leaked into review_url result")
	}
}

func TestMCPTags(t *testing.T) {
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
	httpSrv := httptest.NewServer(mcpserver.Handler(app.New(s)))
	t.Cleanup(httpSrv.Close)
	ctx := context.Background()
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "v0.1.0"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{
		Endpoint:   httpSrv.URL,
		HTTPClient: &http.Client{Transport: bearerTransport{base: http.DefaultTransport, token: tok}},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })

	created, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "create_task",
		Arguments: map[string]any{
			"title":           "Tagged",
			"idempotency_key": "mcp-tag-1",
			"tags":            []string{"ledger"},
		},
	})
	if err != nil || created.IsError {
		t.Fatalf("create %v %+v", err, created)
	}
	raw, _ := json.Marshal(created.StructuredContent)
	if !strings.Contains(string(raw), `"ledger"`) {
		t.Fatalf("create missing tag: %s", raw)
	}
	listed, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "list_tasks",
		Arguments: map[string]any{"tag": "ledger"},
	})
	if err != nil || listed.IsError {
		t.Fatalf("list %v %+v", err, listed)
	}
	raw, _ = json.Marshal(listed.StructuredContent)
	if !strings.Contains(string(raw), "T-001") {
		t.Fatalf("list filter: %s", raw)
	}
	replaced, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "set_tags",
		Arguments: map[string]any{"handle": "T-001", "tags": []string{"site"}},
	})
	if err != nil || replaced.IsError {
		t.Fatalf("set_tags %v %+v", err, replaced)
	}
}

func TestMCPResetLedger(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	tok, err := store.NewToken()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Bootstrap(context.Background(), "iq", "abusemanager", "ada", tok); err != nil {
		t.Fatal(err)
	}
	a := app.New(s)
	httpSrv := httptest.NewServer(mcpserver.Handler(a))
	t.Cleanup(httpSrv.Close)
	ctx := context.Background()
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "v0.1.0"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{
		Endpoint:   httpSrv.URL,
		HTTPClient: &http.Client{Transport: bearerTransport{base: http.DefaultTransport, token: tok}},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })

	created, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "create_task",
		Arguments: map[string]any{
			"title":           "Old work",
			"idempotency_key": "mcp-rst-1",
		},
	})
	if err != nil || created.IsError {
		t.Fatalf("create %v %+v", err, created)
	}
	reset, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "reset_ledger",
		Arguments: map[string]any{"confirm": "iq/abusemanager"},
	})
	if err != nil || reset.IsError {
		t.Fatalf("reset_ledger %v %+v", err, reset)
	}
	raw, _ := json.Marshal(reset.StructuredContent)
	if !strings.Contains(string(raw), `"tasks_deleted":1`) {
		t.Fatalf("reset payload %s", raw)
	}
	again, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "create_task",
		Arguments: map[string]any{
			"title":           "Start over",
			"idempotency_key": "mcp-rst-2",
		},
	})
	if err != nil || again.IsError {
		t.Fatalf("create after reset %v %+v", err, again)
	}
	raw, _ = json.Marshal(again.StructuredContent)
	if !strings.Contains(string(raw), `"T-001"`) {
		t.Fatalf("expected T-001: %s", raw)
	}
}
