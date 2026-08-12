package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/markedo-org/ledger/internal/store"
	"github.com/markedo-org/ledger/internal/types"
)

const (
	DefaultLease       = 30 * time.Minute
	MaxLease           = 24 * time.Hour
	MaxDeferral        = 3
	MaxSeriesPerLedger = 5
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
	Store *store.Store
}

func New(s *store.Store) *App { return &App{Store: s} }

func (a *App) Auth(ctx context.Context, token string) (types.Token, error) {
	if token == "" {
		return types.Token{}, ErrUnauthorized
	}
	t, err := a.Store.LookupToken(ctx, token)
	if err == sql.ErrNoRows {
		return types.Token{}, ErrUnauthorized
	}
	return t, err
}

func (a *App) Ledger(ctx context.Context, tok types.Token, owner, ledger string) (types.Ledger, error) {
	l, err := a.Store.ResolveLedger(ctx, owner, ledger)
	if err == sql.ErrNoRows {
		return l, ErrNotFound
	}
	if err != nil {
		return l, err
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
}

func (a *App) Create(ctx context.Context, tok types.Token, owner, ledger string, in CreateInput) (types.Task, bool, error) {
	l, err := a.Ledger(ctx, tok, owner, ledger)
	if err != nil {
		return types.Task{}, false, err
	}
	title := strings.TrimSpace(in.Title)
	if title == "" {
		return types.Task{}, false, fmt.Errorf("%w: title required", ErrInvalid)
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
	task, replay, err := a.Store.CreateTask(ctx, store.CreateTaskParams{
		LedgerID:       l.ID,
		Prefix:         prefix,
		Title:          title,
		Body:           strings.TrimSpace(in.Body),
		Phase:          phase,
		Size:           size,
		Ref:            strings.TrimSpace(in.Ref),
		Actor:          tok.Actor,
		IdempotencyKey: strings.TrimSpace(in.IdempotencyKey),
		Checks:         in.Checks,
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

func (a *App) List(ctx context.Context, tok types.Token, owner, ledger string) (types.Ledger, []types.Task, error) {
	l, err := a.Ledger(ctx, tok, owner, ledger)
	if err != nil {
		return l, nil, err
	}
	tasks, err := a.Store.ListTasks(ctx, l.ID)
	return l, tasks, err
}

// ListPublic is the HTML/markdown read path. Bind to localhost in v1.
func (a *App) ListPublic(ctx context.Context, owner, ledger string) (types.Ledger, []types.Task, error) {
	l, err := a.Store.ResolveLedger(ctx, owner, ledger)
	if err == sql.ErrNoRows {
		return l, nil, ErrNotFound
	}
	if err != nil {
		return l, nil, err
	}
	tasks, err := a.Store.ListTasks(ctx, l.ID)
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
	TTL    time.Duration
	Steal  bool
	Reason string
}

func (a *App) Claim(ctx context.Context, tok types.Token, owner, ledger, handle string, in ClaimInput) (types.Task, error) {
	t, err := a.Get(ctx, tok, owner, ledger, handle)
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
	live := t.ClaimedBy != "" && t.ClaimedUntil != nil && t.ClaimedUntil.After(now)
	if live && t.ClaimedBy != tok.Actor {
		if !in.Steal {
			return t, fmt.Errorf("%w: held by %s until %s", ErrConflict, t.ClaimedBy, t.ClaimedUntil.Format(time.RFC3339))
		}
		if strings.TrimSpace(in.Reason) == "" {
			return t, fmt.Errorf("%w: steal requires a reason", ErrInvalid)
		}
	}
	until := now.Add(ttl)
	actor := tok.Actor
	kind := "claim"
	payload := map[string]any{"ttl_seconds": int(ttl.Seconds())}
	if in.Steal && live && t.ClaimedBy != tok.Actor {
		kind = "steal"
		payload["reason"] = in.Reason
		payload["from"] = t.ClaimedBy
	}
	out, err := a.Store.Mutate(ctx, t.ID, tok.Actor, kind, payload, store.MutateTask{
		ClaimedBy:    &actor,
		ClaimedUntil: &until,
	})
	if errors.Is(err, store.ErrConflict) {
		return out, ErrConflict
	}
	return out, err
}

func (a *App) Heartbeat(ctx context.Context, tok types.Token, owner, ledger, handle string, ttl time.Duration) (types.Task, error) {
	t, err := a.Get(ctx, tok, owner, ledger, handle)
	if err != nil {
		return t, err
	}
	now := time.Now().UTC()
	if t.ClaimedBy != tok.Actor || t.ClaimedUntil == nil || !t.ClaimedUntil.After(now) {
		return t, fmt.Errorf("%w: you do not hold this claim", ErrConflict)
	}
	if ttl <= 0 {
		ttl = DefaultLease
	}
	if ttl > MaxLease {
		return t, fmt.Errorf("%w: lease longer than 24h", ErrInvalid)
	}
	until := now.Add(ttl)
	return a.Store.Mutate(ctx, t.ID, tok.Actor, "heartbeat", map[string]any{"ttl_seconds": int(ttl.Seconds())}, store.MutateTask{
		ClaimedUntil: &until,
	})
}

func (a *App) Release(ctx context.Context, tok types.Token, owner, ledger, handle string) (types.Task, error) {
	t, err := a.Get(ctx, tok, owner, ledger, handle)
	if err != nil {
		return t, err
	}
	if t.ClaimedBy != "" && t.ClaimedBy != tok.Actor && tok.Role != "admin" {
		return t, fmt.Errorf("%w: held by %s", ErrConflict, t.ClaimedBy)
	}
	return a.Store.Mutate(ctx, t.ID, tok.Actor, "release", map[string]string{}, store.MutateTask{ClearClaim: true})
}

type PhaseInput struct {
	Phase  string
	Reason string
	Force  bool
}

func (a *App) SetPhase(ctx context.Context, tok types.Token, owner, ledger, handle string, in PhaseInput) (types.Task, error) {
	t, err := a.Get(ctx, tok, owner, ledger, handle)
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

func (a *App) AddNote(ctx context.Context, tok types.Token, owner, ledger, handle, body string) (types.Note, error) {
	t, err := a.Get(ctx, tok, owner, ledger, handle)
	if err != nil {
		return types.Note{}, err
	}
	body = strings.TrimSpace(body)
	if body == "" {
		return types.Note{}, fmt.Errorf("%w: note body required", ErrInvalid)
	}
	return a.Store.AddNote(ctx, t.ID, tok.Actor, body)
}

func (a *App) Close(ctx context.Context, tok types.Token, owner, ledger, handle, evidence string) (types.Task, error) {
	t, err := a.Get(ctx, tok, owner, ledger, handle)
	if err != nil {
		return t, err
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
	t, err := a.Get(ctx, tok, owner, ledger, handle)
	if err != nil {
		return t, err
	}
	now := time.Now().UTC()
	return a.Store.Mutate(ctx, t.ID, tok.Actor, "verify", map[string]string{}, store.MutateTask{VerifiedAt: &now})
}

func (a *App) Next(ctx context.Context, tok types.Token, owner, ledger, prefix string, ttl time.Duration) (types.Task, error) {
	_, tasks, err := a.List(ctx, tok, owner, ledger)
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
	return a.Store.Reap(ctx)
}
