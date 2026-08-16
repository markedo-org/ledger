// Package workloop is the v1.0 MCP proof: create, get, claim, note, close.
// Tests and scripts/smokemcp call the same sequence.
package workloop

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

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

// Connect opens a Streamable HTTP MCP session against url with a bearer token.
func Connect(ctx context.Context, url, token string) (*mcp.ClientSession, error) {
	client := mcp.NewClient(&mcp.Implementation{Name: "ledger-workloop", Version: "v1"}, nil)
	return client.Connect(ctx, &mcp.StreamableClientTransport{
		Endpoint: url,
		HTTPClient: &http.Client{Transport: bearerTransport{
			base:  http.DefaultTransport,
			token: token,
		}},
	}, nil)
}

// Run exercises the agent loop against an open session.
// key is the create idempotency key (must be unique per ledger).
func Run(ctx context.Context, session *mcp.ClientSession, key string) error {
	if key == "" {
		key = "workloop-1"
	}
	created, err := call(ctx, session, "create_task", map[string]any{
		"title":           "Work loop",
		"body":            "v1.0 MCP proof",
		"idempotency_key": key,
	})
	if err != nil {
		return err
	}
	handle, _ := created["handle"].(string)
	if handle == "" {
		return fmt.Errorf("create_task: no handle in %v", created)
	}
	got, err := call(ctx, session, "get_task", map[string]any{"handle": handle})
	if err != nil {
		return err
	}
	if got["phase"] != "NOW" {
		return fmt.Errorf("get after create: phase %v", got["phase"])
	}
	claimed, err := call(ctx, session, "claim_task", map[string]any{"handle": handle})
	if err != nil {
		return err
	}
	if strings.TrimSpace(fmt.Sprint(claimed["claimed_by"])) == "" {
		return fmt.Errorf("claim_task: empty claimed_by")
	}
	claimID, _ := claimed["claim_id"].(string)
	if !strings.HasPrefix(claimID, "clm_") {
		return fmt.Errorf("claim_task: missing claim_id in %v", claimed)
	}
	if _, ok := got["claim_id"]; ok {
		return fmt.Errorf("get_task must omit claim_id")
	}
	note, err := call(ctx, session, "add_note", map[string]any{
		"handle": handle,
		"body":   "workloop note",
	})
	if err != nil {
		return err
	}
	if note["body"] != "workloop note" {
		return fmt.Errorf("add_note: %v", note)
	}
	closed, err := call(ctx, session, "close_task", map[string]any{
		"handle":   handle,
		"evidence": "internal/workloop",
		"claim_id": claimID,
	})
	if err != nil {
		return err
	}
	if closed["phase"] != "DONE" {
		return fmt.Errorf("close_task: phase %v", closed["phase"])
	}
	done, err := call(ctx, session, "get_task", map[string]any{"handle": handle})
	if err != nil {
		return err
	}
	if done["phase"] != "DONE" {
		return fmt.Errorf("get after close: phase %v", done["phase"])
	}
	if done["evidence"] != "internal/workloop" {
		return fmt.Errorf("get after close: evidence %v", done["evidence"])
	}
	raw, _ := json.Marshal(done["notes"])
	if !strings.Contains(string(raw), "workloop note") {
		return fmt.Errorf("get after close: notes %s", raw)
	}
	return nil
}

func call(ctx context.Context, session *mcp.ClientSession, name string, args map[string]any) (map[string]any, error) {
	res, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	if res.IsError {
		return nil, fmt.Errorf("%s: tool error %+v", name, res.Content)
	}
	raw, err := json.Marshal(res.StructuredContent)
	if err != nil {
		return nil, fmt.Errorf("%s: encode: %w", name, err)
	}
	out := map[string]any{}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("%s: decode %s: %w", name, raw, err)
	}
	return out, nil
}
