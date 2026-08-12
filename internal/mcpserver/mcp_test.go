package mcpserver_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

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

	listed, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "list_tasks", Arguments: map[string]any{}})
	if err != nil || listed.IsError {
		t.Fatalf("list %v %+v", err, listed)
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
