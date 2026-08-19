package mcpserver

import (
	"context"
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
	}, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tok, err := a.Auth(r.Context(), bearer(r))
		if err != nil {
			w.Header().Set("WWW-Authenticate", `Bearer realm="task-ledger"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		// The SDK requires Accept to list both application/json and
		// text/event-stream. Some clients (Cursor included) send only JSON on
		// the Streamable HTTP POST. We are JSON-response, not legacy SSE.
		if !strings.Contains(r.Header.Get("Accept"), "text/event-stream") {
			r = r.Clone(r.Context())
			r.Header.Set("Accept", "application/json, text/event-stream")
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
	s := mcp.NewServer(&mcp.Implementation{Name: "task-ledger", Version: "0.4.0"}, nil)
	h := &host{app: a, tok: tok}
	mcp.AddTool(s, &mcp.Tool{
		Name:        "list_ledgers",
		Description: "List ledgers for an owner. Defaults to the token's owner.",
	}, h.listLedgers)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "create_ledger",
		Description: "Create a ledger under an owner. Admin only. Enforces max_ledgers. Creates series T. Owner admin: mints a ledger-bound write token (shown once) and returns an MCP config named for the project. Operator: pass actor to mint, or use the returned mcp object after create_token.",
	}, h.createLedger)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "create_token",
		Description: "Mint a bearer token (plaintext once). Admin only. For a project agent, set ledger and role write, and put that token in an MCP server named for the project. Omit ledger for an owner-scoped token; name that MCP server for admin. Optional email binds the token for magic-link sign-in.",
	}, h.createToken)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "create_owner",
		Description: "Create an owner. Operator token only. Optional first ledger and actor mints an owner-scoped admin token (plaintext once), not a ledger-bound token. Default max_ledgers is 1. For project agents, create_token with ledger set and role write.",
	}, h.createOwner)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "set_max_ledgers",
		Description: "Set max_ledgers on an owner. Operator token only. 0 means unlimited. Extra newest ledgers become read-only when the cap is below the current count.",
	}, h.setMaxLedgers)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "list_tasks",
		Description: "List tasks in a ledger (handle, title, phase, size, tags, claimant). Thin index only: get_task before you act. Default list hides DONE older than archive_done_after_days (7 unless the ledger overrides). Pass done=true for every DONE task, and only those. Pass tag to keep tasks with that slug. get_task still loads a hidden handle. Do not delete DONE tasks.",
	}, h.listTasks)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_task",
		Description: "Get one task by handle (T-001), including body, notes, checks, and claim state. Does not return claim_id. Use this before acting. list_tasks is only an index.",
	}, h.getTask)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "create_task",
		Description: "Create a task. Allocates the next handle in the series (default T). Always send idempotency_key. Optional tags: at most three lowercase slugs. Does not claim the task.",
	}, h.createTask)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "claim_task",
		Description: "Claim a task with a lease (default 30 minutes). Returns claim_id once. Keep it in this chat and pass it on heartbeat, re-claim, release, close, and phase while the lease is live. steal=true with a reason takes a live claim from another actor and issues a new claim_id.",
	}, h.claimTask)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "next_task",
		Description: "Atomically claim the next eligible NOW task in the series. Returns claim_id once. Keep it in this chat. Prefer this over listing then claiming.",
	}, h.nextTask)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "add_note",
		Description: "Append a note to a task. Notes are append-only; two agents can both write without clobbering.",
	}, h.addNote)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "set_check",
		Description: "Tick or untick a sub-checkbox. Identify by n (1-based, from get_task) or by exact body text. All checks must be ticked before close_task.",
	}, h.setCheck)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "set_tags",
		Description: "Replace the tags on a task. Pass tags as lowercase slugs, at most three. Empty list clears them. A tag is a filter, not a ledger.",
	}, h.setTags)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "set_phase",
		Description: "Move a task between NOW, NEXT, LATER, GATED, PARKED. Moving later requires a reason. Closing is close_task, not this. Pass claim_id while a lease is live.",
	}, h.setPhase)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "close_task",
		Description: "Close a task. Evidence is required (commit, query result, or observed behaviour). All checks must be ticked. Pass claim_id while a lease is live.",
	}, h.closeTask)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "verify_task",
		Description: "Mark a task as verified now. Use after reviewing a closed or standing task. Does not close it.",
	}, h.verify)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "heartbeat_task",
		Description: "Extend the current actor's lease. Pass claim_id from claim_task or next_task.",
	}, h.heartbeat)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "release_task",
		Description: "Drop this chat's claim so another agent can take the task. Pass claim_id. Admin or operator can release another actor without it.",
	}, h.release)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "review_url",
		Description: "Mint a one-time URL that signs a browser into the HTML board as this token. Open it for the human. Do not paste the bearer token. Do not put the URL in a task note. The link works once and expires in 15 minutes. Write tokens may mint.",
	}, h.reviewURL)
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

func (h *host) scope(ctx context.Context, owner, ledger string) (string, string, error) {
	if owner == "" {
		owner = h.tok.OwnerSlug
	}
	if ledger == "" {
		ledger = h.tok.LedgerSlug
	}
	if owner == "" {
		return "", "", fmtErr("owner is required")
	}
	if ledger != "" {
		return owner, ledger, nil
	}
	ledgers, err := h.app.ListLedgers(ctx, h.tok, owner)
	if err != nil {
		return "", "", err
	}
	if len(ledgers) == 1 {
		return owner, ledgers[0].Slug, nil
	}
	if len(ledgers) == 0 {
		return "", "", fmtErr("no ledgers under this owner")
	}
	return "", "", fmtErr("owner-scoped token covers several ledgers; pass ledger or mint a token bound to one project")
}

func (h *host) ownerScope(owner string) (string, error) {
	if owner == "" {
		owner = h.tok.OwnerSlug
	}
	if owner == "" {
		return "", fmtErr("owner is required")
	}
	return owner, nil
}

type listLedgersIn struct {
	Owner string `json:"owner,omitempty" jsonschema:"Owner slug. Defaults to the token's owner."`
}

type listLedgersOut struct {
	Owner   string        `json:"owner"`
	Ledgers []ledgerBrief `json:"ledgers"`
}

type ledgerBrief struct {
	Slug  string `json:"slug"`
	Title string `json:"title"`
}

func (h *host) listLedgers(ctx context.Context, _ *mcp.CallToolRequest, in listLedgersIn) (*mcp.CallToolResult, listLedgersOut, error) {
	owner, err := h.ownerScope(in.Owner)
	if err != nil {
		return nil, listLedgersOut{}, err
	}
	ledgers, err := h.app.ListLedgers(ctx, h.tok, owner)
	if err != nil {
		return nil, listLedgersOut{}, err
	}
	out := listLedgersOut{Owner: owner, Ledgers: []ledgerBrief{}}
	for _, l := range ledgers {
		out.Ledgers = append(out.Ledgers, ledgerBrief{Slug: l.Slug, Title: l.Title})
	}
	return nil, out, nil
}

type createLedgerIn struct {
	Owner string `json:"owner,omitempty" jsonschema:"Owner slug. Defaults to the token's owner."`
	Slug  string `json:"slug" jsonschema:"Ledger slug, for example iq or dispatch"`
	Title string `json:"title,omitempty" jsonschema:"Display title. Defaults to the slug."`
	Actor string `json:"actor,omitempty" jsonschema:"Actor for the project write token. Owner admin defaults to the creating token. Operator must set this to mint."`
}

func (h *host) createLedger(ctx context.Context, _ *mcp.CallToolRequest, in createLedgerIn) (*mcp.CallToolResult, map[string]any, error) {
	owner, err := h.ownerScope(in.Owner)
	if err != nil {
		return nil, nil, err
	}
	l, err := h.app.CreateLedger(ctx, h.tok, owner, app.CreateLedgerInput{Slug: in.Slug, Title: in.Title})
	if err != nil {
		return nil, nil, err
	}
	issued, err := h.app.MintProjectWrite(ctx, h.tok, owner, l.Slug, app.ProjectActor(h.tok, in.Actor))
	if err != nil {
		return nil, nil, err
	}
	return nil, h.app.LedgerCreatedView(l, issued), nil
}

type createTokenIn struct {
	Owner  string `json:"owner,omitempty" jsonschema:"Owner slug. Defaults to the token's owner."`
	Actor  string `json:"actor" jsonschema:"Actor name baked into the token, for example batty"`
	Ledger string `json:"ledger,omitempty" jsonschema:"Bind to this ledger. Omit for an owner-scoped token."`
	Role   string `json:"role,omitempty" jsonschema:"write or admin. Default write."`
	Email  string `json:"email,omitempty" jsonschema:"Optional email for magic-link sign-in. One token per address."`
}

func (h *host) createToken(ctx context.Context, _ *mcp.CallToolRequest, in createTokenIn) (*mcp.CallToolResult, map[string]any, error) {
	owner, err := h.ownerScope(in.Owner)
	if err != nil {
		return nil, nil, err
	}
	issued, err := h.app.CreateToken(ctx, h.tok, owner, app.CreateTokenInput{Actor: in.Actor, Ledger: in.Ledger, Role: in.Role, Email: in.Email})
	if err != nil {
		return nil, nil, err
	}
	out := map[string]any{
		"actor": issued.Token.Actor,
		"role":  issued.Token.Role,
		"token": issued.Plain,
	}
	if issued.Token.LedgerSlug != "" {
		out["ledger"] = issued.Token.LedgerSlug
		out["note"] = "Ledger-bound. Name this MCP server for the project. Do not reuse the owner admin token here."
	} else {
		out["note"] = "Owner-scoped. Name this MCP server for admin. For a project-only agent, mint again with ledger set and role write."
	}
	if issued.Token.Email != "" {
		out["email"] = issued.Token.Email
	}
	return nil, out, nil
}

type createOwnerIn struct {
	Slug       string `json:"slug" jsonschema:"Owner slug, for example acme"`
	MaxLedgers int    `json:"max_ledgers,omitempty" jsonschema:"Cap. 0 or omit means 1 on create."`
	Ledger     string `json:"ledger,omitempty" jsonschema:"Optional first ledger slug"`
	Title      string `json:"title,omitempty" jsonschema:"Optional first ledger title"`
	Actor      string `json:"actor,omitempty" jsonschema:"If set with ledger, mint an admin token for this actor"`
	Email      string `json:"email,omitempty" jsonschema:"Optional email bound to the minted token for magic-link sign-in"`
}

func (h *host) createOwner(ctx context.Context, _ *mcp.CallToolRequest, in createOwnerIn) (*mcp.CallToolResult, map[string]any, error) {
	created, err := h.app.CreateOwner(ctx, h.tok, app.CreateOwnerInput{
		Slug: in.Slug, MaxLedgers: in.MaxLedgers, Ledger: in.Ledger, Title: in.Title, Actor: in.Actor, Email: in.Email,
	})
	if err != nil {
		return nil, nil, err
	}
	out := map[string]any{"slug": created.Owner.Slug, "max_ledgers": created.Owner.MaxLedgers}
	if created.Ledger != nil {
		out["ledger"] = created.Ledger.Slug
	}
	if created.Token != nil {
		out["actor"] = created.Token.Token.Actor
		out["role"] = created.Token.Token.Role
		out["token"] = created.Token.Plain
		out["mcp"] = h.app.AgentMCPConfig("task-ledger-admin", created.Token.Plain)
		out["note"] = "Owner admin, not bound to the first ledger. Name its MCP server task-ledger-admin. The write_token is for the first ledger only."
	}
	if created.WriteToken != nil && created.Ledger != nil {
		out["write_token"] = created.WriteToken.Plain
		out["write_role"] = created.WriteToken.Token.Role
		out["write_ledger"] = created.Ledger.Slug
		out["write_mcp"] = h.app.AgentMCPConfig("task-ledger-"+created.Ledger.Slug, created.WriteToken.Plain)
	}
	return nil, out, nil
}

type setMaxIn struct {
	Owner      string `json:"owner" jsonschema:"Owner slug"`
	MaxLedgers int    `json:"max_ledgers" jsonschema:"New cap. 0 means unlimited."`
}

func (h *host) setMaxLedgers(ctx context.Context, _ *mcp.CallToolRequest, in setMaxIn) (*mcp.CallToolResult, map[string]any, error) {
	o, err := h.app.SetMaxLedgers(ctx, h.tok, in.Owner, in.MaxLedgers)
	if err != nil {
		return nil, nil, err
	}
	return nil, map[string]any{"slug": o.Slug, "max_ledgers": o.MaxLedgers}, nil
}

func fmtErr(msg string) error { return &toolError{msg} }

type toolError struct{ msg string }

func (e *toolError) Error() string { return e.msg }

type scopeIn struct {
	Owner  string `json:"owner,omitempty" jsonschema:"Owner slug. Defaults to the token's owner."`
	Ledger string `json:"ledger,omitempty" jsonschema:"Ledger slug. Defaults to the token's ledger, or the owner's only ledger."`
}

type listIn struct {
	Owner  string `json:"owner,omitempty" jsonschema:"Owner slug. Defaults to the token's owner."`
	Ledger string `json:"ledger,omitempty" jsonschema:"Ledger slug. Defaults to the token's ledger, or the owner's only ledger."`
	Done   bool   `json:"done,omitempty" jsonschema:"If true, list every DONE task and only DONE tasks. Default list hides DONE older than archive_done_after_days."`
	Tag    string `json:"tag,omitempty" jsonschema:"Keep tasks that have this tag slug. One tag only."`
}

type listOut struct {
	Owner  string      `json:"owner"`
	Ledger string      `json:"ledger"`
	Tasks  []taskBrief `json:"tasks"`
}

type taskBrief struct {
	Handle    string   `json:"handle"`
	Title     string   `json:"title"`
	Phase     string   `json:"phase"`
	Size      string   `json:"size,omitempty"`
	ClaimedBy string   `json:"claimed_by,omitempty"`
	Pushed    int      `json:"pushed,omitempty"`
	Tags      []string `json:"tags,omitempty"`
}

func brief(t types.Task) taskBrief {
	b := taskBrief{Handle: t.Handle, Title: t.Title, Phase: string(t.Phase), Size: string(t.Size), Pushed: t.Pushed, Tags: t.Tags}
	if t.ClaimedBy != "" && t.ClaimedUntil != nil && t.ClaimedUntil.After(time.Now().UTC()) {
		b.ClaimedBy = t.ClaimedBy
	}
	return b
}

func (h *host) listTasks(ctx context.Context, _ *mcp.CallToolRequest, in listIn) (*mcp.CallToolResult, listOut, error) {
	owner, ledger, err := h.scope(ctx, in.Owner, in.Ledger)
	if err != nil {
		return nil, listOut{}, err
	}
	l, tasks, err := h.app.List(ctx, h.tok, owner, ledger, app.ListQuery{DoneOnly: in.Done, Tag: strings.ToLower(strings.TrimSpace(in.Tag))})
	if err != nil {
		return nil, listOut{}, err
	}
	out := listOut{Owner: l.OwnerSlug, Ledger: l.Slug, Tasks: []taskBrief{}}
	for _, t := range tasks {
		out.Tasks = append(out.Tasks, brief(t))
	}
	return nil, out, nil
}

type getIn struct {
	Owner  string `json:"owner,omitempty" jsonschema:"Owner slug. Defaults to the token's owner."`
	Ledger string `json:"ledger,omitempty" jsonschema:"Ledger slug. Defaults to the token's ledger, or the owner's only ledger."`
	Handle string `json:"handle" jsonschema:"Task handle, for example T-001"`
}

func (h *host) getTask(ctx context.Context, _ *mcp.CallToolRequest, in getIn) (*mcp.CallToolResult, map[string]any, error) {
	owner, ledger, err := h.scope(ctx, in.Owner, in.Ledger)
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
	Tags           []string `json:"tags,omitempty" jsonschema:"Optional filter slugs, at most three. Same charset as owner slugs."`
}

func (h *host) createTask(ctx context.Context, _ *mcp.CallToolRequest, in createIn) (*mcp.CallToolResult, map[string]any, error) {
	owner, ledger, err := h.scope(ctx, in.Owner, in.Ledger)
	if err != nil {
		return nil, nil, err
	}
	t, _, err := h.app.Create(ctx, h.tok, owner, ledger, app.CreateInput{
		Prefix: in.Prefix, Title: in.Title, Body: in.Body, Phase: in.Phase, Size: in.Size,
		Ref: in.Ref, IdempotencyKey: in.IdempotencyKey, Checks: in.Checks, Tags: in.Tags,
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
	ClaimID    string `json:"claim_id,omitempty" jsonschema:"Required to refresh your own live lease from this chat. From claim_task or next_task."`
}

func (h *host) claimTask(ctx context.Context, _ *mcp.CallToolRequest, in claimIn) (*mcp.CallToolResult, map[string]any, error) {
	owner, ledger, err := h.scope(ctx, in.Owner, in.Ledger)
	if err != nil {
		return nil, nil, err
	}
	var ttl time.Duration
	if in.TTLSeconds > 0 {
		ttl = time.Duration(in.TTLSeconds) * time.Second
	}
	t, err := h.app.Claim(ctx, h.tok, owner, ledger, in.Handle, app.ClaimInput{TTL: ttl, Steal: in.Steal, Reason: in.Reason, ClaimID: in.ClaimID})
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
	owner, ledger, err := h.scope(ctx, in.Owner, in.Ledger)
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
	Ledger string `json:"ledger,omitempty" jsonschema:"Ledger slug. Defaults to the token's ledger, or the owner's only ledger."`
	Handle string `json:"handle" jsonschema:"Task handle, for example T-001"`
	Body   string `json:"body" jsonschema:"Note text to append"`
}

func (h *host) addNote(ctx context.Context, _ *mcp.CallToolRequest, in noteIn) (*mcp.CallToolResult, map[string]any, error) {
	owner, ledger, err := h.scope(ctx, in.Owner, in.Ledger)
	if err != nil {
		return nil, nil, err
	}
	n, err := h.app.AddNote(ctx, h.tok, owner, ledger, in.Handle, in.Body)
	if err != nil {
		return nil, nil, err
	}
	return nil, map[string]any{"actor": n.Actor, "body": n.Body, "at": n.CreatedAt.UTC().Format(time.RFC3339)}, nil
}

type checkIn struct {
	Owner  string `json:"owner,omitempty" jsonschema:"Owner slug. Defaults to the token's owner."`
	Ledger string `json:"ledger,omitempty" jsonschema:"Ledger slug. Defaults to the token's ledger, or the owner's only ledger."`
	Handle string `json:"handle" jsonschema:"Task handle, for example T-001"`
	N      int    `json:"n,omitempty" jsonschema:"1-based check index from get_task. Prefer this when body text is not unique."`
	Body   string `json:"body,omitempty" jsonschema:"Exact check text. Used when n is omitted."`
	Done   bool   `json:"done" jsonschema:"true to tick, false to untick"`
}

func (h *host) setCheck(ctx context.Context, _ *mcp.CallToolRequest, in checkIn) (*mcp.CallToolResult, map[string]any, error) {
	owner, ledger, err := h.scope(ctx, in.Owner, in.Ledger)
	if err != nil {
		return nil, nil, err
	}
	t, err := h.app.SetCheck(ctx, h.tok, owner, ledger, in.Handle, in.N, in.Body, in.Done)
	if err != nil {
		return nil, nil, err
	}
	return nil, taskMap(t), nil
}

type tagsIn struct {
	Owner  string   `json:"owner,omitempty" jsonschema:"Owner slug. Defaults to the token's owner."`
	Ledger string   `json:"ledger,omitempty" jsonschema:"Ledger slug. Defaults to the token's ledger, or the owner's only ledger."`
	Handle string   `json:"handle" jsonschema:"Task handle, for example T-001"`
	Tags   []string `json:"tags" jsonschema:"Replacement tag slugs, at most three. Empty clears."`
}

func (h *host) setTags(ctx context.Context, _ *mcp.CallToolRequest, in tagsIn) (*mcp.CallToolResult, map[string]any, error) {
	owner, ledger, err := h.scope(ctx, in.Owner, in.Ledger)
	if err != nil {
		return nil, nil, err
	}
	t, err := h.app.SetTags(ctx, h.tok, owner, ledger, in.Handle, in.Tags)
	if err != nil {
		return nil, nil, err
	}
	return nil, taskMap(t), nil
}

type phaseIn struct {
	Owner   string `json:"owner,omitempty" jsonschema:"Owner slug. Defaults to the token's owner."`
	Ledger  string `json:"ledger,omitempty" jsonschema:"Ledger slug. Defaults to the token's ledger, or the owner's only ledger."`
	Handle  string `json:"handle" jsonschema:"Task handle, for example T-001"`
	Phase   string `json:"phase" jsonschema:"NOW, NEXT, LATER, GATED, or PARKED"`
	Reason  string `json:"reason,omitempty" jsonschema:"Required when moving to a later phase"`
	Force   bool   `json:"force,omitempty" jsonschema:"Override the fourth-deferral policy block."`
	ClaimID string `json:"claim_id,omitempty" jsonschema:"Required while a lease is live. From claim_task or next_task."`
}

func (h *host) setPhase(ctx context.Context, _ *mcp.CallToolRequest, in phaseIn) (*mcp.CallToolResult, map[string]any, error) {
	owner, ledger, err := h.scope(ctx, in.Owner, in.Ledger)
	if err != nil {
		return nil, nil, err
	}
	t, err := h.app.SetPhase(ctx, h.tok, owner, ledger, in.Handle, app.PhaseInput{Phase: in.Phase, Reason: in.Reason, Force: in.Force, ClaimID: in.ClaimID})
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
	ClaimID  string `json:"claim_id,omitempty" jsonschema:"Required while a lease is live. From claim_task or next_task."`
}

func (h *host) closeTask(ctx context.Context, _ *mcp.CallToolRequest, in closeIn) (*mcp.CallToolResult, map[string]any, error) {
	owner, ledger, err := h.scope(ctx, in.Owner, in.Ledger)
	if err != nil {
		return nil, nil, err
	}
	t, err := h.app.Close(ctx, h.tok, owner, ledger, in.Handle, in.Evidence, in.ClaimID)
	if err != nil {
		return nil, nil, err
	}
	return nil, taskMap(t), nil
}

func (h *host) verify(ctx context.Context, _ *mcp.CallToolRequest, in handleIn) (*mcp.CallToolResult, map[string]any, error) {
	owner, ledger, err := h.scope(ctx, in.Owner, in.Ledger)
	if err != nil {
		return nil, nil, err
	}
	t, err := h.app.Verify(ctx, h.tok, owner, ledger, in.Handle)
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
	ClaimID    string `json:"claim_id,omitempty" jsonschema:"From claim_task or next_task. Required for this chat's live lease."`
}

func (h *host) heartbeat(ctx context.Context, _ *mcp.CallToolRequest, in handleIn) (*mcp.CallToolResult, map[string]any, error) {
	owner, ledger, err := h.scope(ctx, in.Owner, in.Ledger)
	if err != nil {
		return nil, nil, err
	}
	var ttl time.Duration
	if in.TTLSeconds > 0 {
		ttl = time.Duration(in.TTLSeconds) * time.Second
	}
	t, err := h.app.Heartbeat(ctx, h.tok, owner, ledger, in.Handle, ttl, in.ClaimID)
	if err != nil {
		return nil, nil, err
	}
	return nil, taskMap(t), nil
}

func (h *host) release(ctx context.Context, _ *mcp.CallToolRequest, in handleIn) (*mcp.CallToolResult, map[string]any, error) {
	owner, ledger, err := h.scope(ctx, in.Owner, in.Ledger)
	if err != nil {
		return nil, nil, err
	}
	t, err := h.app.Release(ctx, h.tok, owner, ledger, in.Handle, in.ClaimID)
	if err != nil {
		return nil, nil, err
	}
	return nil, taskMap(t), nil
}

type reviewURLIn struct{}

func (h *host) reviewURL(ctx context.Context, _ *mcp.CallToolRequest, _ reviewURLIn) (*mcp.CallToolResult, map[string]any, error) {
	u, exp, err := h.app.MintReviewURL(ctx, h.tok)
	if err != nil {
		return nil, nil, err
	}
	return nil, map[string]any{
		"url":                u,
		"expires_in_seconds": exp,
		"note":               "Open this URL in a browser for the human. It works once. Do not paste the bearer token. Do not put the URL in a task note.",
	}, nil
}

func (h *host) readLive(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
	owner, ledger, err := h.scope(ctx, "", "")
	if err != nil {
		return nil, err
	}
	l, tasks, err := h.app.List(ctx, h.tok, owner, ledger, app.ListQuery{})
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
	checks := make([]map[string]any, 0, len(t.Checks))
	for i, c := range t.Checks {
		checks = append(checks, map[string]any{"n": i + 1, "body": c.Body, "done": c.Done})
	}
	notes := make([]map[string]any, 0, len(t.Notes))
	for _, n := range t.Notes {
		notes = append(notes, map[string]any{
			"actor": n.Actor,
			"body":  n.Body,
			"at":    n.CreatedAt.UTC().Format(time.RFC3339),
		})
	}
	m := map[string]any{
		"handle":     t.Handle,
		"title":      t.Title,
		"body":       t.Body,
		"phase":      t.Phase,
		"size":       t.Size,
		"pushed":     t.Pushed,
		"claimed_by": t.ClaimedBy,
		"evidence":   t.Evidence,
		"ref":        t.Ref,
		"checks":     checks,
		"tags":       append([]string{}, t.Tags...),
		"notes":      notes,
	}
	if t.VerifiedAt != nil {
		m["verified_at"] = t.VerifiedAt.UTC().Format(time.RFC3339)
	}
	if t.ClosedAt != nil {
		m["closed_at"] = t.ClosedAt.UTC().Format(time.RFC3339)
	}
	if t.ClaimID != "" {
		m["claim_id"] = t.ClaimID
	}
	return m
}
