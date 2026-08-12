package mcpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/markedo-org/ledger/internal/app"
	"github.com/markedo-org/ledger/internal/render"
	"github.com/markedo-org/ledger/internal/types"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const ProtocolRevision = "2025-06-18"

type ctxKey struct{}

func Handler(a *app.App) http.Handler {
	inner := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		tok, _ := r.Context().Value(ctxKey{}).(types.Token)
		return newServer(a, tok)
	}, &mcp.StreamableHTTPOptions{Stateless: true})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tok, err := a.Auth(r.Context(), bearer(r))
		if err != nil {
			w.Header().Set("WWW-Authenticate", `Bearer realm="task-ledger"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		inner.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, tok)))
	})
}

func bearer(r *http.Request) string {
	h := r.Header.Get("Authorization")
	t := strings.TrimPrefix(h, "Bearer ")
	if t == h {
		return ""
	}
	return strings.TrimSpace(t)
}

func newServer(a *app.App, tok types.Token) *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{Name: "task-ledger", Version: "0.2.0"}, nil)
	h := &host{app: a, tok: tok}
	mcp.AddTool(s, &mcp.Tool{
		Name:        "list_tasks",
		Description: "List tasks in a ledger, grouped by phase. Defaults to the ledger bound to the bearer token. Use before picking work.",
	}, h.listTasks)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_task",
		Description: "Get one task by handle (T-001), including notes, checks, and claim state.",
	}, h.getTask)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "create_task",
		Description: "Create a task. Allocates the next handle in the series (default T). Always send idempotency_key. Does not claim the task.",
	}, h.createTask)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "claim_task",
		Description: "Claim a task with a lease (default 30 minutes). Heartbeat to extend. steal=true with a reason takes a live claim from another actor.",
	}, h.claimTask)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "next_task",
		Description: "Atomically claim the next eligible NOW task in the series. Prefer this over listing then claiming.",
	}, h.nextTask)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "add_note",
		Description: "Append a note to a task. Notes are append-only; two agents can both write without clobbering.",
	}, h.addNote)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "set_phase",
		Description: "Move a task between NOW, NEXT, LATER, GATED, PARKED. Moving later requires a reason. Closing is close_task, not this.",
	}, h.setPhase)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "close_task",
		Description: "Close a task. Evidence is required (commit, query result, or observed behaviour). All checks must be ticked.",
	}, h.closeTask)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "heartbeat_task",
		Description: "Extend the current actor's lease on a claimed task.",
	}, h.heartbeat)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "release_task",
		Description: "Drop the current actor's claim so another agent can take the task.",
	}, h.release)
	s.AddResource(&mcp.Resource{
		URI:         "ledger://live",
		Name:        "Live ledger snapshot",
		Description: "Markdown snapshot of the token's ledger. Read-only. Do not treat this as a write path.",
		MIMEType:    "text/markdown",
	}, h.readLive)
	return s
}

type host struct {
	app *app.App
	tok types.Token
}

func (h *host) scope(owner, ledger string) (string, string, error) {
	if owner == "" {
		owner = h.tok.OwnerSlug
	}
	if ledger == "" {
		ledger = h.tok.LedgerSlug
	}
	if owner == "" || ledger == "" {
		return "", "", fmtErr("owner and ledger are required (or use a token bound to one ledger)")
	}
	return owner, ledger, nil
}

func fmtErr(msg string) error { return &toolError{msg} }

type toolError struct{ msg string }

func (e *toolError) Error() string { return e.msg }

type scopeIn struct {
	Owner  string `json:"owner,omitempty" jsonschema:"Owner slug. Defaults to the token's owner."`
	Ledger string `json:"ledger,omitempty" jsonschema:"Ledger slug. Defaults to the token's ledger."`
}

type listIn struct {
	Owner  string `json:"owner,omitempty" jsonschema:"Owner slug. Defaults to the token's owner."`
	Ledger string `json:"ledger,omitempty" jsonschema:"Ledger slug. Defaults to the token's ledger."`
}

type listOut struct {
	Owner  string      `json:"owner"`
	Ledger string      `json:"ledger"`
	Tasks  []taskBrief `json:"tasks"`
}

type taskBrief struct {
	Handle    string `json:"handle"`
	Title     string `json:"title"`
	Phase     string `json:"phase"`
	Size      string `json:"size,omitempty"`
	ClaimedBy string `json:"claimed_by,omitempty"`
	Pushed    int    `json:"pushed,omitempty"`
}

func brief(t types.Task) taskBrief {
	b := taskBrief{Handle: t.Handle, Title: t.Title, Phase: string(t.Phase), Size: string(t.Size), Pushed: t.Pushed}
	if t.ClaimedBy != "" && t.ClaimedUntil != nil && t.ClaimedUntil.After(time.Now().UTC()) {
		b.ClaimedBy = t.ClaimedBy
	}
	return b
}

func (h *host) listTasks(ctx context.Context, _ *mcp.CallToolRequest, in listIn) (*mcp.CallToolResult, listOut, error) {
	owner, ledger, err := h.scope(in.Owner, in.Ledger)
	if err != nil {
		return nil, listOut{}, err
	}
	l, tasks, err := h.app.List(ctx, h.tok, owner, ledger)
	if err != nil {
		return nil, listOut{}, err
	}
	out := listOut{Owner: l.OwnerSlug, Ledger: l.Slug}
	for _, t := range tasks {
		out.Tasks = append(out.Tasks, brief(t))
	}
	return nil, out, nil
}

type getIn struct {
	Owner  string `json:"owner,omitempty" jsonschema:"Owner slug. Defaults to the token's owner."`
	Ledger string `json:"ledger,omitempty" jsonschema:"Ledger slug. Defaults to the token's ledger."`
	Handle string `json:"handle" jsonschema:"Task handle, for example T-001"`
}

func (h *host) getTask(ctx context.Context, _ *mcp.CallToolRequest, in getIn) (*mcp.CallToolResult, map[string]any, error) {
	owner, ledger, err := h.scope(in.Owner, in.Ledger)
	if err != nil {
		return nil, nil, err
	}
	t, err := h.app.Get(ctx, h.tok, owner, ledger, in.Handle)
	if err != nil {
		return nil, nil, err
	}
	return nil, taskMap(t), nil
}

type createIn struct {
	Owner          string   `json:"owner,omitempty" jsonschema:"Owner slug. Defaults to the token's owner."`
	Ledger         string   `json:"ledger,omitempty" jsonschema:"Ledger slug. Defaults to the token's ledger."`
	Title          string   `json:"title" jsonschema:"Short task title"`
	Body           string   `json:"body,omitempty" jsonschema:"Standing description"`
	Phase          string   `json:"phase,omitempty" jsonschema:"NOW, NEXT, LATER, GATED, or PARKED. Default NOW."`
	Size           string   `json:"size,omitempty" jsonschema:"S, M, L, or empty"`
	Prefix         string   `json:"prefix,omitempty" jsonschema:"One-letter series. Default T."`
	Ref            string   `json:"ref,omitempty" jsonschema:"Pointer to a detail doc"`
	IdempotencyKey string   `json:"idempotency_key" jsonschema:"Required. Agents retry; this prevents duplicate IDs."`
	Checks         []string `json:"checks,omitempty" jsonschema:"Optional sub-checkboxes, not separate tasks"`
}

func (h *host) createTask(ctx context.Context, _ *mcp.CallToolRequest, in createIn) (*mcp.CallToolResult, map[string]any, error) {
	owner, ledger, err := h.scope(in.Owner, in.Ledger)
	if err != nil {
		return nil, nil, err
	}
	t, _, err := h.app.Create(ctx, h.tok, owner, ledger, app.CreateInput{
		Prefix: in.Prefix, Title: in.Title, Body: in.Body, Phase: in.Phase, Size: in.Size,
		Ref: in.Ref, IdempotencyKey: in.IdempotencyKey, Checks: in.Checks,
	})
	if err != nil {
		return nil, nil, err
	}
	return nil, taskMap(t), nil
}

type claimIn struct {
	Owner      string `json:"owner,omitempty" jsonschema:"Owner slug. Defaults to the token's owner."`
	Ledger     string `json:"ledger,omitempty" jsonschema:"Ledger slug. Defaults to the token's ledger."`
	Handle     string `json:"handle" jsonschema:"Task handle, for example T-001"`
	TTLSeconds int    `json:"ttl_seconds,omitempty" jsonschema:"Lease length in seconds. Default 1800, max 86400."`
	Steal      bool   `json:"steal,omitempty" jsonschema:"Take a live claim from another actor. Requires reason."`
	Reason     string `json:"reason,omitempty" jsonschema:"Required when steal is true"`
}

func (h *host) claimTask(ctx context.Context, _ *mcp.CallToolRequest, in claimIn) (*mcp.CallToolResult, map[string]any, error) {
	owner, ledger, err := h.scope(in.Owner, in.Ledger)
	if err != nil {
		return nil, nil, err
	}
	var ttl time.Duration
	if in.TTLSeconds > 0 {
		ttl = time.Duration(in.TTLSeconds) * time.Second
	}
	t, err := h.app.Claim(ctx, h.tok, owner, ledger, in.Handle, app.ClaimInput{TTL: ttl, Steal: in.Steal, Reason: in.Reason})
	if err != nil {
		return nil, nil, err
	}
	return nil, taskMap(t), nil
}

type nextIn struct {
	Owner      string `json:"owner,omitempty" jsonschema:"Owner slug. Defaults to the token's owner."`
	Ledger     string `json:"ledger,omitempty" jsonschema:"Ledger slug. Defaults to the token's ledger."`
	Prefix     string `json:"prefix,omitempty" jsonschema:"Series prefix filter. Default T."`
	TTLSeconds int    `json:"ttl_seconds,omitempty" jsonschema:"Lease length in seconds. Default 1800."`
}

func (h *host) nextTask(ctx context.Context, _ *mcp.CallToolRequest, in nextIn) (*mcp.CallToolResult, map[string]any, error) {
	owner, ledger, err := h.scope(in.Owner, in.Ledger)
	if err != nil {
		return nil, nil, err
	}
	var ttl time.Duration
	if in.TTLSeconds > 0 {
		ttl = time.Duration(in.TTLSeconds) * time.Second
	}
	prefix := in.Prefix
	if prefix == "" {
		prefix = "T"
	}
	t, err := h.app.Next(ctx, h.tok, owner, ledger, prefix, ttl)
	if err != nil {
		return nil, nil, err
	}
	return nil, taskMap(t), nil
}

type noteIn struct {
	Owner  string `json:"owner,omitempty" jsonschema:"Owner slug. Defaults to the token's owner."`
	Ledger string `json:"ledger,omitempty" jsonschema:"Ledger slug. Defaults to the token's ledger."`
	Handle string `json:"handle" jsonschema:"Task handle, for example T-001"`
	Body   string `json:"body" jsonschema:"Note text to append"`
}

func (h *host) addNote(ctx context.Context, _ *mcp.CallToolRequest, in noteIn) (*mcp.CallToolResult, map[string]any, error) {
	owner, ledger, err := h.scope(in.Owner, in.Ledger)
	if err != nil {
		return nil, nil, err
	}
	n, err := h.app.AddNote(ctx, h.tok, owner, ledger, in.Handle, in.Body)
	if err != nil {
		return nil, nil, err
	}
	return nil, map[string]any{"actor": n.Actor, "body": n.Body, "at": n.CreatedAt.UTC().Format(time.RFC3339)}, nil
}

type phaseIn struct {
	Owner  string `json:"owner,omitempty" jsonschema:"Owner slug. Defaults to the token's owner."`
	Ledger string `json:"ledger,omitempty" jsonschema:"Ledger slug. Defaults to the token's ledger."`
	Handle string `json:"handle" jsonschema:"Task handle, for example T-001"`
	Phase  string `json:"phase" jsonschema:"NOW, NEXT, LATER, GATED, or PARKED"`
	Reason string `json:"reason,omitempty" jsonschema:"Required when moving to a later phase"`
}

func (h *host) setPhase(ctx context.Context, _ *mcp.CallToolRequest, in phaseIn) (*mcp.CallToolResult, map[string]any, error) {
	owner, ledger, err := h.scope(in.Owner, in.Ledger)
	if err != nil {
		return nil, nil, err
	}
	t, err := h.app.SetPhase(ctx, h.tok, owner, ledger, in.Handle, app.PhaseInput{Phase: in.Phase, Reason: in.Reason})
	if err != nil {
		return nil, nil, err
	}
	return nil, taskMap(t), nil
}

type closeIn struct {
	Owner    string `json:"owner,omitempty" jsonschema:"Owner slug. Defaults to the token's owner."`
	Ledger   string `json:"ledger,omitempty" jsonschema:"Ledger slug. Defaults to the token's ledger."`
	Handle   string `json:"handle" jsonschema:"Task handle, for example T-001"`
	Evidence string `json:"evidence" jsonschema:"Commit, query result, or observed behaviour. Required."`
}

func (h *host) closeTask(ctx context.Context, _ *mcp.CallToolRequest, in closeIn) (*mcp.CallToolResult, map[string]any, error) {
	owner, ledger, err := h.scope(in.Owner, in.Ledger)
	if err != nil {
		return nil, nil, err
	}
	t, err := h.app.Close(ctx, h.tok, owner, ledger, in.Handle, in.Evidence)
	if err != nil {
		return nil, nil, err
	}
	return nil, taskMap(t), nil
}

type handleIn struct {
	Owner      string `json:"owner,omitempty" jsonschema:"Owner slug. Defaults to the token's owner."`
	Ledger     string `json:"ledger,omitempty" jsonschema:"Ledger slug. Defaults to the token's ledger."`
	Handle     string `json:"handle" jsonschema:"Task handle, for example T-001"`
	TTLSeconds int    `json:"ttl_seconds,omitempty" jsonschema:"New lease length in seconds for heartbeat"`
}

func (h *host) heartbeat(ctx context.Context, _ *mcp.CallToolRequest, in handleIn) (*mcp.CallToolResult, map[string]any, error) {
	owner, ledger, err := h.scope(in.Owner, in.Ledger)
	if err != nil {
		return nil, nil, err
	}
	var ttl time.Duration
	if in.TTLSeconds > 0 {
		ttl = time.Duration(in.TTLSeconds) * time.Second
	}
	t, err := h.app.Heartbeat(ctx, h.tok, owner, ledger, in.Handle, ttl)
	if err != nil {
		return nil, nil, err
	}
	return nil, taskMap(t), nil
}

func (h *host) release(ctx context.Context, _ *mcp.CallToolRequest, in handleIn) (*mcp.CallToolResult, map[string]any, error) {
	owner, ledger, err := h.scope(in.Owner, in.Ledger)
	if err != nil {
		return nil, nil, err
	}
	t, err := h.app.Release(ctx, h.tok, owner, ledger, in.Handle)
	if err != nil {
		return nil, nil, err
	}
	return nil, taskMap(t), nil
}

func (h *host) readLive(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
	owner, ledger, err := h.scope("", "")
	if err != nil {
		return nil, err
	}
	l, tasks, err := h.app.List(ctx, h.tok, owner, ledger)
	if err != nil {
		return nil, err
	}
	body := render.Markdown(l, tasks, "localhost")
	return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{
		URI:      req.Params.URI,
		MIMEType: "text/markdown",
		Text:     body,
	}}}, nil
}

func taskMap(t types.Task) map[string]any {
	b, _ := json.Marshal(map[string]any{
		"handle":     t.Handle,
		"title":      t.Title,
		"body":       t.Body,
		"phase":      t.Phase,
		"size":       t.Size,
		"pushed":     t.Pushed,
		"claimed_by": t.ClaimedBy,
		"evidence":   t.Evidence,
		"ref":        t.Ref,
	})
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	return m
}
