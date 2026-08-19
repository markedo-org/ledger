package app

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/markedo-org/ledger/internal/mail"
	"github.com/markedo-org/ledger/internal/store"
	"github.com/markedo-org/ledger/internal/types"
)

const (
	DefaultLease       = 30 * time.Minute
	MaxLease           = 24 * time.Hour
	MaxDeferral        = 3
	MaxSeriesPerLedger = 5
	SessionTTL         = 7 * 24 * time.Hour
)

var (
	ErrUnauthorized = errors.New("unauthorized")
	ErrForbidden    = errors.New("forbidden")
	ErrNotFound     = errors.New("not found")
	ErrConflict     = errors.New("conflict")
	ErrInvalid      = errors.New("invalid")
	ErrPolicy       = errors.New("policy")
)

type App struct {
	Store                *store.Store
	operator             string
	Mail                 mail.Sender
	PublicURL            string
	Version              string
	magic                *magicGate
	ArchiveDoneAfterDays int
	PurgeDoneAfterDays   int
}

func New(s *store.Store) *App {
	return &App{
		Store:                s,
		ArchiveDoneAfterDays: DefaultArchiveDoneAfterDays,
		PurgeDoneAfterDays:   DefaultPurgeDoneAfterDays,
	}
}

func (a *App) SetOperatorToken(plain string) {
	a.operator = strings.TrimSpace(plain)
}

func (a *App) OperatorConfigured() bool { return a.operator != "" }

func (a *App) Auth(ctx context.Context, token string) (types.Token, error) {
	if token == "" {
		return types.Token{}, ErrUnauthorized
	}
	if a.matchOperator(token) {
		return types.Token{Actor: "operator", Role: types.RoleOperator}, nil
	}
	t, err := a.Store.LookupToken(ctx, token)
	if err == sql.ErrNoRows {
		return types.Token{}, ErrUnauthorized
	}
	return t, err
}

func (a *App) matchOperator(plain string) bool {
	want := a.operator
	if want == "" {
		return false
	}
	if len(plain) != len(want) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(plain), []byte(want)) == 1
}

func (a *App) Ledger(ctx context.Context, tok types.Token, owner, ledger string) (types.Ledger, error) {
	l, err := a.Store.ResolveLedger(ctx, owner, ledger)
	if err == sql.ErrNoRows {
		return l, ErrNotFound
	}
	if err != nil {
		return l, err
	}
	if tok.IsOperator() {
		return l, nil
	}
	if tok.OwnerID != l.OwnerID {
		return l, ErrForbidden
	}
	if tok.LedgerID != "" && tok.LedgerID != l.ID {
		return l, ErrForbidden
	}
	return l, nil
}

type CreateInput struct {
	Prefix         string
	Title          string
	Body           string
	Phase          string
	Size           string
	Ref            string
	IdempotencyKey string
	Checks         []string
	Tags           []string
}

func (a *App) Create(ctx context.Context, tok types.Token, owner, ledger string, in CreateInput) (types.Task, bool, error) {
	l, err := a.Ledger(ctx, tok, owner, ledger)
	if err != nil {
		return types.Task{}, false, err
	}
	if err := a.requireWritable(ctx, l); err != nil {
		return types.Task{}, false, err
	}
	title := strings.TrimSpace(in.Title)
	if title == "" {
		return types.Task{}, false, fmt.Errorf("%w: title required", ErrInvalid)
	}
	key := strings.TrimSpace(in.IdempotencyKey)
	if key == "" {
		return types.Task{}, false, fmt.Errorf("%w: idempotency_key required", ErrInvalid)
	}
	prefix := strings.ToUpper(strings.TrimSpace(in.Prefix))
	if prefix == "" {
		prefix = "T"
	}
	if !types.ValidPrefix(prefix) {
		return types.Task{}, false, fmt.Errorf("%w: series prefix must be one letter", ErrInvalid)
	}
	phase := types.PhaseNOW
	if in.Phase != "" {
		p, ok := types.ParsePhase(in.Phase)
		if !ok || p == types.PhaseDONE {
			return types.Task{}, false, fmt.Errorf("%w: bad phase", ErrInvalid)
		}
		phase = p
	}
	size, ok := types.ParseSize(in.Size)
	if !ok {
		return types.Task{}, false, fmt.Errorf("%w: bad size", ErrInvalid)
	}
	tags, err := types.NormalizeTags(in.Tags)
	if err != nil {
		return types.Task{}, false, fmt.Errorf("%w: %s", ErrInvalid, err.Error())
	}
	task, replay, err := a.Store.CreateTask(ctx, store.CreateTaskParams{
		LedgerID:       l.ID,
		Prefix:         prefix,
		Title:          title,
		Body:           strings.TrimSpace(in.Body),
		Phase:          phase,
		Size:           size,
		Ref:            strings.TrimSpace(in.Ref),
		Actor:          tok.Actor,
		IdempotencyKey: key,
		Checks:         in.Checks,
		Tags:           tags,
	})
	return task, replay, err
}

func (a *App) Get(ctx context.Context, tok types.Token, owner, ledger, handle string) (types.Task, error) {
	l, err := a.Ledger(ctx, tok, owner, ledger)
	if err != nil {
		return types.Task{}, err
	}
	t, err := a.Store.GetTask(ctx, l.ID, handle)
	if err == sql.ErrNoRows {
		return t, ErrNotFound
	}
	return t, err
}

func (a *App) List(ctx context.Context, tok types.Token, owner, ledger string, q ListQuery) (types.Ledger, []types.Task, error) {
	l, err := a.Ledger(ctx, tok, owner, ledger)
	if err != nil {
		return l, nil, err
	}
	tasks, err := a.Store.ListTasks(ctx, l.ID, a.taskList(l, q))
	return l, tasks, err
}

func (a *App) PublicLedger(ctx context.Context, owner, ledger string) (types.Ledger, error) {
	l, err := a.Store.ResolveLedger(ctx, owner, ledger)
	if err == sql.ErrNoRows {
		return l, ErrNotFound
	}
	return l, err
}

// ListPublic is the HTML/markdown read path. Bind to localhost in v1.
func (a *App) ListPublic(ctx context.Context, owner, ledger string, q ListQuery) (types.Ledger, []types.Task, error) {
	l, err := a.Store.ResolveLedger(ctx, owner, ledger)
	if err == sql.ErrNoRows {
		return l, nil, ErrNotFound
	}
	if err != nil {
		return l, nil, err
	}
	tasks, err := a.Store.ListTasks(ctx, l.ID, a.taskList(l, q))
	return l, tasks, err
}

func (a *App) GetPublic(ctx context.Context, owner, ledger, handle string) (types.Ledger, types.Task, error) {
	l, err := a.Store.ResolveLedger(ctx, owner, ledger)
	if err == sql.ErrNoRows {
		return l, types.Task{}, ErrNotFound
	}
	if err != nil {
		return l, types.Task{}, err
	}
	t, err := a.Store.GetTask(ctx, l.ID, handle)
	if err == sql.ErrNoRows {
		return l, t, ErrNotFound
	}
	return l, t, err
}

type ClaimInput struct {
	TTL     time.Duration
	Steal   bool
	Reason  string
	ClaimID string
}

func leaseLive(t types.Task, now time.Time) bool {
	return t.ClaimedBy != "" && t.ClaimedUntil != nil && t.ClaimedUntil.After(now)
}

func leaseHeldElsewhere(t types.Task, claimID string) error {
	if t.ClaimSecretHash == "" {
		return nil
	}
	if store.ClaimIDOK(t.ClaimSecretHash, claimID) {
		return nil
	}
	return fmt.Errorf("%w: another session holds this lease", ErrConflict)
}

func (a *App) Claim(ctx context.Context, tok types.Token, owner, ledger, handle string, in ClaimInput) (types.Task, error) {
	t, err := a.loadForWrite(ctx, tok, owner, ledger, handle)
	if err != nil {
		return t, err
	}
	if t.Phase == types.PhaseDONE {
		return t, fmt.Errorf("%w: cannot claim a closed task", ErrInvalid)
	}
	ttl := in.TTL
	if ttl <= 0 {
		ttl = DefaultLease
	}
	if ttl > MaxLease {
		return t, fmt.Errorf("%w: lease longer than 24h", ErrInvalid)
	}
	now := time.Now().UTC()
	live := leaseLive(t, now)
	// A steal is the explicit, audited override, and it applies to your own
	// actor name too. A chat that was compacted, or a second session, has lost
	// the claim_id but still owns the work; without this the task is locked
	// until the lease runs out and nobody can even release it.
	steal := in.Steal && live
	if steal && strings.TrimSpace(in.Reason) == "" {
		return t, fmt.Errorf("%w: steal requires a reason", ErrInvalid)
	}
	if live && !steal {
		if t.ClaimedBy != tok.Actor {
			return t, fmt.Errorf("%w: held by %s until %s", ErrConflict, t.ClaimedBy, t.ClaimedUntil.Format(time.RFC3339))
		}
		if err := leaseHeldElsewhere(t, in.ClaimID); err != nil {
			return t, err
		}
	}
	until := now.Add(ttl)
	actor := tok.Actor
	kind := "claim"
	payload := map[string]any{"ttl_seconds": int(ttl.Seconds())}
	if steal {
		kind = "steal"
		payload["reason"] = in.Reason
		payload["from"] = t.ClaimedBy
	}
	plain := in.ClaimID
	var hash *string
	mint := !live || steal || (live && t.ClaimSecretHash == "")
	if mint {
		plain, err = store.NewClaimID()
		if err != nil {
			return t, err
		}
		h := store.HashToken(plain)
		hash = &h
	}
	out, err := a.Store.Mutate(ctx, t.ID, tok.Actor, kind, payload, store.MutateTask{
		ClaimedBy:       &actor,
		ClaimedUntil:    &until,
		ClaimSecretHash: hash,
	})
	if errors.Is(err, store.ErrConflict) {
		return out, ErrConflict
	}
	if err != nil {
		return out, err
	}
	out.ClaimID = plain
	return out, nil
}

func (a *App) Heartbeat(ctx context.Context, tok types.Token, owner, ledger, handle string, ttl time.Duration, claimID string) (types.Task, error) {
	t, err := a.loadForWrite(ctx, tok, owner, ledger, handle)
	if err != nil {
		return t, err
	}
	now := time.Now().UTC()
	if t.ClaimedBy != tok.Actor || t.ClaimedUntil == nil || !t.ClaimedUntil.After(now) {
		return t, fmt.Errorf("%w: you do not hold this claim", ErrConflict)
	}
	if err := leaseHeldElsewhere(t, claimID); err != nil {
		return t, err
	}
	if ttl <= 0 {
		ttl = DefaultLease
	}
	if ttl > MaxLease {
		return t, fmt.Errorf("%w: lease longer than 24h", ErrInvalid)
	}
	until := now.Add(ttl)
	out, err := a.Store.Mutate(ctx, t.ID, tok.Actor, "heartbeat", map[string]any{"ttl_seconds": int(ttl.Seconds())}, store.MutateTask{
		ClaimedUntil: &until,
	})
	if err != nil {
		return out, err
	}
	if t.ClaimSecretHash != "" {
		out.ClaimID = claimID
	}
	return out, nil
}

func (a *App) Release(ctx context.Context, tok types.Token, owner, ledger, handle, claimID string) (types.Task, error) {
	t, err := a.Get(ctx, tok, owner, ledger, handle)
	if err != nil {
		return t, err
	}
	now := time.Now().UTC()
	live := leaseLive(t, now)
	// Admin and operator override any live lease, including one standing under
	// their own actor name. Requiring the claim_id there left an admin unable
	// to clear a lease they could already have cleared for anyone else.
	if live && tok.Role != types.RoleAdmin && !tok.IsOperator() {
		if t.ClaimedBy != tok.Actor {
			return t, fmt.Errorf("%w: held by %s", ErrConflict, t.ClaimedBy)
		}
		if err := leaseHeldElsewhere(t, claimID); err != nil {
			return t, err
		}
	}
	return a.Store.Mutate(ctx, t.ID, tok.Actor, "release", map[string]string{}, store.MutateTask{ClearClaim: true})
}

type PhaseInput struct {
	Phase   string
	Reason  string
	Force   bool
	ClaimID string
}

func (a *App) SetPhase(ctx context.Context, tok types.Token, owner, ledger, handle string, in PhaseInput) (types.Task, error) {
	t, err := a.loadForWrite(ctx, tok, owner, ledger, handle)
	if err != nil {
		return t, err
	}
	phase, ok := types.ParsePhase(in.Phase)
	if !ok || phase == types.PhaseDONE {
		return t, fmt.Errorf("%w: use close to finish a task", ErrInvalid)
	}
	if phase == t.Phase {
		return t, nil
	}
	if leaseLive(t, time.Now().UTC()) {
		if err := leaseHeldElsewhere(t, in.ClaimID); err != nil {
			return t, err
		}
	}
	pushed := t.Pushed
	if phase.Rank() > t.Phase.Rank() && phase != types.PhaseDONE {
		if strings.TrimSpace(in.Reason) == "" {
			return t, fmt.Errorf("%w: deferral requires a reason", ErrInvalid)
		}
		pushed++
		if pushed > MaxDeferral && !in.Force {
			return t, fmt.Errorf("%w: pushed %d, resolve it (do, delete, or park with a trigger)", ErrPolicy, pushed)
		}
	}
	payload := map[string]any{"from": t.Phase, "to": phase, "reason": in.Reason, "pushed": pushed}
	return a.Store.Mutate(ctx, t.ID, tok.Actor, "phase", payload, store.MutateTask{
		Phase:  &phase,
		Pushed: &pushed,
	})
}

func (a *App) SetCheck(ctx context.Context, tok types.Token, owner, ledger, handle string, n int, body string, done bool, claimID string) (types.Task, error) {
	t, err := a.loadForWrite(ctx, tok, owner, ledger, handle)
	if err != nil {
		return t, err
	}
	if t.Phase == types.PhaseDONE {
		return t, fmt.Errorf("%w: cannot change checks on a closed task", ErrInvalid)
	}
	if leaseLive(t, time.Now().UTC()) {
		if err := leaseHeldElsewhere(t, claimID); err != nil {
			return t, err
		}
	}
	var id string
	if n > 0 {
		if n > len(t.Checks) {
			return t, fmt.Errorf("%w: check %d not found", ErrNotFound, n)
		}
		id = t.Checks[n-1].ID
	} else {
		body = strings.TrimSpace(body)
		if body == "" {
			return t, fmt.Errorf("%w: n or body required", ErrInvalid)
		}
		var matches []types.Check
		for _, c := range t.Checks {
			if c.Body == body {
				matches = append(matches, c)
			}
		}
		if len(matches) == 0 {
			return t, fmt.Errorf("%w: check %q not found", ErrNotFound, body)
		}
		if len(matches) > 1 {
			return t, fmt.Errorf("%w: check %q is not unique, use n", ErrInvalid, body)
		}
		id = matches[0].ID
	}
	return a.Store.SetCheck(ctx, t.ID, tok.Actor, id, done)
}

func (a *App) SetTags(ctx context.Context, tok types.Token, owner, ledger, handle string, raw []string, claimID string) (types.Task, error) {
	t, err := a.loadForWrite(ctx, tok, owner, ledger, handle)
	if err != nil {
		return t, err
	}
	if leaseLive(t, time.Now().UTC()) {
		if err := leaseHeldElsewhere(t, claimID); err != nil {
			return t, err
		}
	}
	tags, err := types.NormalizeTags(raw)
	if err != nil {
		return t, fmt.Errorf("%w: %s", ErrInvalid, err.Error())
	}
	return a.Store.SetTags(ctx, t.ID, tok.Actor, tags)
}

func (a *App) ListLedgerTags(ctx context.Context, owner, ledger string) ([]string, error) {
	l, err := a.Store.ResolveLedger(ctx, owner, ledger)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return a.Store.ListLedgerTags(ctx, l.ID)
}

func (a *App) AddNote(ctx context.Context, tok types.Token, owner, ledger, handle, body string) (types.Note, error) {
	t, err := a.loadForWrite(ctx, tok, owner, ledger, handle)
	if err != nil {
		return types.Note{}, err
	}
	body = strings.TrimSpace(body)
	if body == "" {
		return types.Note{}, fmt.Errorf("%w: note body required", ErrInvalid)
	}
	return a.Store.AddNote(ctx, t.ID, tok.Actor, body)
}

func (a *App) Close(ctx context.Context, tok types.Token, owner, ledger, handle, evidence, claimID string) (types.Task, error) {
	t, err := a.loadForWrite(ctx, tok, owner, ledger, handle)
	if err != nil {
		return t, err
	}
	if leaseLive(t, time.Now().UTC()) {
		if err := leaseHeldElsewhere(t, claimID); err != nil {
			return t, err
		}
	}
	evidence = strings.TrimSpace(evidence)
	if evidence == "" {
		return t, fmt.Errorf("%w: closing requires evidence", ErrInvalid)
	}
	for _, c := range t.Checks {
		if !c.Done {
			return t, fmt.Errorf("%w: unticked check: %s", ErrPolicy, c.Body)
		}
	}
	phase := types.PhaseDONE
	now := time.Now().UTC()
	return a.Store.Mutate(ctx, t.ID, tok.Actor, "close", map[string]string{"evidence": evidence}, store.MutateTask{
		Phase:      &phase,
		Evidence:   &evidence,
		ClosedAt:   &now,
		ClearClaim: true,
	})
}

func (a *App) Verify(ctx context.Context, tok types.Token, owner, ledger, handle string) (types.Task, error) {
	t, err := a.loadForWrite(ctx, tok, owner, ledger, handle)
	if err != nil {
		return t, err
	}
	now := time.Now().UTC()
	return a.Store.Mutate(ctx, t.ID, tok.Actor, "verify", map[string]string{}, store.MutateTask{VerifiedAt: &now})
}

func (a *App) Next(ctx context.Context, tok types.Token, owner, ledger, prefix string, ttl time.Duration) (types.Task, error) {
	_, tasks, err := a.List(ctx, tok, owner, ledger, ListQuery{})
	if err != nil {
		return types.Task{}, err
	}
	now := time.Now().UTC()
	prefix = strings.ToUpper(prefix)
	for _, t := range tasks {
		if t.Phase != types.PhaseNOW {
			continue
		}
		if prefix != "" && !strings.HasPrefix(t.Handle, prefix+"-") {
			continue
		}
		live := t.ClaimedBy != "" && t.ClaimedUntil != nil && t.ClaimedUntil.After(now)
		if live {
			continue
		}
		if len(t.DependsOn) > 0 {
			continue // v1: skip blocked; full DAG check later
		}
		return a.Claim(ctx, tok, owner, ledger, t.Handle, ClaimInput{TTL: ttl})
	}
	return types.Task{}, fmt.Errorf("%w: no eligible task", ErrNotFound)
}

func (a *App) Reap(ctx context.Context) (int, error) {
	n, err := a.Store.Reap(ctx)
	if err != nil {
		return n, err
	}
	s, err := a.Store.ReapSessions(ctx)
	if err != nil {
		return n + s, err
	}
	m, err := a.Store.ReapMagicLinks(ctx)
	if err != nil {
		return n + s + m, err
	}
	r, err := a.Store.ReapReviewLinks(ctx)
	if err != nil {
		return n + s + m + r, err
	}
	p, err := a.purgeDone(ctx)
	return n + s + m + r + p, err
}

func (a *App) ListLedgersPublic(ctx context.Context, ownerSlug string) ([]LedgerInfo, error) {
	_, ledgers, err := a.PublicOwner(ctx, ownerSlug)
	return ledgers, err
}

func (a *App) PublicOwner(ctx context.Context, ownerSlug string) (types.Owner, []LedgerInfo, error) {
	o, err := a.Store.OwnerBySlug(ctx, ownerSlug)
	if err == sql.ErrNoRows {
		return types.Owner{}, nil, ErrNotFound
	}
	if err != nil {
		return types.Owner{}, nil, err
	}
	infos, err := a.ledgersWithFreeze(ctx, o)
	return o, infos, err
}

func (a *App) CreateSession(ctx context.Context, actor, githubID, login, ownerSlug, ledgerSlug, role, tokenID string) (types.Session, string, error) {
	if actor == "" {
		actor = login
	}
	return a.Store.CreateSession(ctx, actor, githubID, login, ownerSlug, ledgerSlug, role, tokenID, SessionTTL)
}

func (a *App) SessionFromAPIToken(ctx context.Context, apiToken string) (types.Session, string, error) {
	tok, err := a.Auth(ctx, strings.TrimSpace(apiToken))
	if err != nil {
		return types.Session{}, "", err
	}
	return a.CreateSession(ctx, tok.Actor, "", "", tok.OwnerSlug, tok.LedgerSlug, sessionRole(tok), tok.ID)
}

// sessionRole carries the token's own role onto the HTML session. Without it
// every non-operator sign-in looked alike and the board treated a write token
// as an owner admin.
func sessionRole(tok types.Token) string {
	if tok.IsOperator() {
		return types.RoleOperator
	}
	if tok.Role == "" {
		return types.RoleWrite
	}
	return tok.Role
}

func (a *App) Session(ctx context.Context, plain string) (types.Session, error) {
	if plain == "" {
		return types.Session{}, ErrUnauthorized
	}
	s, err := a.Store.LookupSession(ctx, plain)
	if err == sql.ErrNoRows {
		return types.Session{}, ErrUnauthorized
	}
	return s, err
}

func (a *App) DeleteSession(ctx context.Context, plain string) error {
	return a.Store.DeleteSession(ctx, plain)
}

func (a *App) requireOwner(tok types.Token, ownerSlug string) error {
	if tok.IsOperator() {
		return nil
	}
	if tok.OwnerSlug != ownerSlug {
		return ErrForbidden
	}
	return nil
}

func (a *App) requireAdmin(tok types.Token) error {
	if tok.IsOperator() || tok.Role == types.RoleAdmin {
		return nil
	}
	return fmt.Errorf("%w: admin role required", ErrForbidden)
}

func (a *App) requireOperator(tok types.Token) error {
	if !tok.IsOperator() {
		return fmt.Errorf("%w: operator token required", ErrForbidden)
	}
	return nil
}

func (a *App) resolveOwner(ctx context.Context, tok types.Token, ownerSlug string) (types.Owner, error) {
	if err := a.requireOwner(tok, ownerSlug); err != nil {
		return types.Owner{}, err
	}
	if tok.IsOperator() {
		o, err := a.Store.OwnerBySlug(ctx, ownerSlug)
		if err == sql.ErrNoRows {
			return types.Owner{}, ErrNotFound
		}
		return o, err
	}
	return types.Owner{ID: tok.OwnerID, Slug: tok.OwnerSlug}, nil
}

func (a *App) ListLedgers(ctx context.Context, tok types.Token, ownerSlug string) ([]types.Ledger, error) {
	o, err := a.resolveOwner(ctx, tok, ownerSlug)
	if err != nil {
		return nil, err
	}
	return a.Store.ListLedgers(ctx, o.ID)
}

type CreateLedgerInput struct {
	Slug  string
	Title string
}

func (a *App) CreateLedger(ctx context.Context, tok types.Token, ownerSlug string, in CreateLedgerInput) (types.Ledger, error) {
	if err := a.requireAdmin(tok); err != nil {
		return types.Ledger{}, err
	}
	o, err := a.resolveOwner(ctx, tok, ownerSlug)
	if err != nil {
		return types.Ledger{}, err
	}
	slug := strings.ToLower(strings.TrimSpace(in.Slug))
	if !types.ValidSlug(slug) {
		return types.Ledger{}, fmt.Errorf("%w: ledger slug must be lowercase letters, digits, hyphens", ErrInvalid)
	}
	title := strings.TrimSpace(in.Title)
	l, err := a.Store.CreateLedger(ctx, o.ID, slug, title, tok.Actor)
	if errors.Is(err, store.ErrLedgerCap) {
		return l, fmt.Errorf("%w: max_ledgers reached", ErrPolicy)
	}
	if errors.Is(err, store.ErrConflict) {
		return l, fmt.Errorf("%w: ledger %s already exists", ErrConflict, slug)
	}
	return l, err
}

// ProjectActor is who to mint a ledger-bound write token for after create_ledger.
// An explicit actor wins. Owner admin defaults to the creating token's actor.
// Operator does not default: they must pass an actor or we only suggest a mint.
func ProjectActor(tok types.Token, requested string) string {
	if a := strings.ToLower(strings.TrimSpace(requested)); a != "" {
		return a
	}
	if tok.IsOperator() {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(tok.Actor))
}

func (a *App) MintProjectWrite(ctx context.Context, tok types.Token, ownerSlug, ledgerSlug, actor string) (*IssuedToken, error) {
	if strings.TrimSpace(actor) == "" {
		return nil, nil
	}
	issued, err := a.CreateToken(ctx, tok, ownerSlug, CreateTokenInput{
		Actor:  actor,
		Ledger: ledgerSlug,
		Role:   types.RoleWrite,
	})
	if err != nil {
		return nil, err
	}
	return &issued, nil
}

func (a *App) AgentMCPConfig(serverName, tokenPlain string) map[string]any {
	origin := strings.TrimRight(strings.TrimSpace(a.PublicURL), "/")
	url := "<your-origin>/mcp"
	if origin != "" {
		url = origin + "/mcp"
	}
	if strings.TrimSpace(tokenPlain) == "" {
		tokenPlain = "<ledger-write-token>"
	}
	if strings.TrimSpace(serverName) == "" {
		serverName = "task-ledger"
	}
	return map[string]any{
		"mcpServers": map[string]any{
			serverName: map[string]any{
				"url": url,
				"headers": map[string]any{
					"Authorization": "Bearer " + tokenPlain,
				},
			},
		},
	}
}

func (a *App) LedgerCreatedView(l types.Ledger, issued *IssuedToken) map[string]any {
	server := "task-ledger-" + l.Slug
	plain := ""
	out := map[string]any{
		"owner": l.OwnerSlug,
		"slug":  l.Slug,
		"title": l.Title,
	}
	if issued != nil {
		plain = issued.Plain
		out["token"] = issued.Plain
		out["actor"] = issued.Token.Actor
		out["role"] = issued.Token.Role
		out["note"] = "Ledger-bound write token, shown once. Put it in the MCP server named for this project. Keep the owner admin token in its own server."
	} else {
		out["note"] = "No token minted. create_token with ledger=" + l.Slug + " and role write, then use the mcp object."
	}
	out["mcp"] = a.AgentMCPConfig(server, plain)
	return out
}

type CreateTokenInput struct {
	Actor  string
	Ledger string // empty: owner-scoped
	Role   string
	Email  string
}

type IssuedToken struct {
	Token types.Token
	Plain string
}

func (a *App) CreateToken(ctx context.Context, tok types.Token, ownerSlug string, in CreateTokenInput) (IssuedToken, error) {
	if err := a.requireAdmin(tok); err != nil {
		return IssuedToken{}, err
	}
	o, err := a.resolveOwner(ctx, tok, ownerSlug)
	if err != nil {
		return IssuedToken{}, err
	}
	actor := strings.ToLower(strings.TrimSpace(in.Actor))
	if !types.ValidActor(actor) {
		return IssuedToken{}, fmt.Errorf("%w: actor must be lowercase letters, digits, underscore or hyphen", ErrInvalid)
	}
	role := strings.ToLower(strings.TrimSpace(in.Role))
	if role == "" {
		role = types.RoleWrite
	}
	if role != types.RoleWrite && role != types.RoleAdmin {
		return IssuedToken{}, fmt.Errorf("%w: role must be write or admin", ErrInvalid)
	}
	var ledgerID string
	if strings.TrimSpace(in.Ledger) != "" {
		l, err := a.Store.ResolveLedger(ctx, ownerSlug, strings.TrimSpace(in.Ledger))
		if err == sql.ErrNoRows {
			return IssuedToken{}, ErrNotFound
		}
		if err != nil {
			return IssuedToken{}, err
		}
		if l.OwnerID != o.ID {
			return IssuedToken{}, ErrForbidden
		}
		ledgerID = l.ID
	}
	plain, err := store.NewToken()
	if err != nil {
		return IssuedToken{}, err
	}
	email, err := NormalizeEmail(in.Email)
	if err != nil {
		return IssuedToken{}, err
	}
	issued, err := a.Store.CreateToken(ctx, o.ID, actor, ledgerID, role, email, plain)
	if errors.Is(err, store.ErrConflict) {
		return IssuedToken{}, fmt.Errorf("%w: email already bound to a token", ErrConflict)
	}
	return IssuedToken{Token: issued, Plain: plain}, nil
}

func (a *App) ListTokens(ctx context.Context, tok types.Token, ownerSlug string) ([]types.TokenInfo, error) {
	if err := a.requireAdmin(tok); err != nil {
		return nil, err
	}
	o, err := a.resolveOwner(ctx, tok, ownerSlug)
	if err != nil {
		return nil, err
	}
	return a.Store.ListTokens(ctx, o.ID)
}

// RevokeToken kills a bearer token for good. It refuses to revoke the token
// making the request, which is not squeamishness: a token cannot be un-revoked
// and the plaintext is gone, so revoking the one in your own hand locks you out
// of the owner entirely. Minting the replacement first and revoking with that
// is the only order that cannot strand you.
func (a *App) RevokeToken(ctx context.Context, tok types.Token, ownerSlug, tokenID string) (types.TokenInfo, error) {
	if err := a.requireAdmin(tok); err != nil {
		return types.TokenInfo{}, err
	}
	o, err := a.resolveOwner(ctx, tok, ownerSlug)
	if err != nil {
		return types.TokenInfo{}, err
	}
	tokenID = strings.TrimSpace(tokenID)
	if tokenID == "" {
		return types.TokenInfo{}, fmt.Errorf("%w: token id is required", ErrInvalid)
	}
	if tokenID == tok.ID {
		return types.TokenInfo{}, fmt.Errorf("%w: that is the token you are using. Mint the replacement first, then revoke this one with the new token", ErrInvalid)
	}
	info, err := a.Store.RevokeToken(ctx, o.ID, tokenID)
	if err == sql.ErrNoRows {
		return types.TokenInfo{}, ErrNotFound
	}
	return info, err
}
