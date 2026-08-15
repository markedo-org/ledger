package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/markedo-org/ledger/internal/store"
	"github.com/markedo-org/ledger/internal/types"
)

var reservedOwnerSlugs = map[string]bool{
	"admin":  true,
	"auth":   true,
	"health": true,
	"login":  true,
	"logout": true,
	"mcp":    true,
	"owners": true,
	"static": true,
	"v1":     true,
}

type CreateOwnerInput struct {
	Slug       string
	MaxLedgers int
	Ledger     string
	Title      string
	Actor      string
	Email      string
}

type CreatedOwner struct {
	Owner      types.Owner
	Ledger     *types.Ledger
	Token      *IssuedToken // owner-scoped admin
	WriteToken *IssuedToken // ledger-bound write for the first ledger
}

func (a *App) CreateOwner(ctx context.Context, tok types.Token, in CreateOwnerInput) (CreatedOwner, error) {
	if err := a.requireOperator(tok); err != nil {
		return CreatedOwner{}, err
	}
	slug := strings.ToLower(strings.TrimSpace(in.Slug))
	if !types.ValidSlug(slug) {
		return CreatedOwner{}, fmt.Errorf("%w: owner slug must be lowercase letters, digits, hyphens", ErrInvalid)
	}
	if reservedOwnerSlugs[slug] {
		return CreatedOwner{}, fmt.Errorf("%w: owner slug %s is reserved", ErrInvalid, slug)
	}
	max := in.MaxLedgers
	if max == 0 {
		max = 1
	}
	if max < 0 {
		return CreatedOwner{}, fmt.Errorf("%w: max_ledgers must be 0 (unlimited) or at least 1", ErrInvalid)
	}
	o, err := a.Store.CreateOwner(ctx, slug, max)
	if errors.Is(err, store.ErrConflict) {
		return CreatedOwner{}, fmt.Errorf("%w: owner %s already exists", ErrConflict, slug)
	}
	if err != nil {
		return CreatedOwner{}, err
	}
	out := CreatedOwner{Owner: o}
	ledgerSlug := strings.ToLower(strings.TrimSpace(in.Ledger))
	if ledgerSlug == "" {
		return out, nil
	}
	l, err := a.CreateLedger(ctx, tok, slug, CreateLedgerInput{Slug: ledgerSlug, Title: in.Title})
	if err != nil {
		return out, err
	}
	out.Ledger = &l
	actor := strings.ToLower(strings.TrimSpace(in.Actor))
	if actor == "" {
		return out, nil
	}
	issued, err := a.CreateToken(ctx, tok, slug, CreateTokenInput{Actor: actor, Role: types.RoleAdmin, Email: in.Email})
	if err != nil {
		return out, err
	}
	out.Token = &issued
	write, err := a.MintProjectWrite(ctx, tok, slug, ledgerSlug, actor)
	if err != nil {
		return out, err
	}
	out.WriteToken = write
	return out, nil
}

func (a *App) SetMaxLedgers(ctx context.Context, tok types.Token, ownerSlug string, n int) (types.Owner, error) {
	if err := a.requireOperator(tok); err != nil {
		return types.Owner{}, err
	}
	if n < 0 {
		return types.Owner{}, fmt.Errorf("%w: max_ledgers must be 0 (unlimited) or at least 1", ErrInvalid)
	}
	o, err := a.Store.OwnerBySlug(ctx, ownerSlug)
	if err == sql.ErrNoRows {
		return types.Owner{}, ErrNotFound
	}
	if err != nil {
		return types.Owner{}, err
	}
	if err := a.Store.SetMaxLedgers(ctx, o.ID, n); err != nil {
		return types.Owner{}, err
	}
	o.MaxLedgers = n
	return o, nil
}

func (a *App) GetOwner(ctx context.Context, tok types.Token, ownerSlug string) (types.Owner, []LedgerInfo, error) {
	o, err := a.resolveOwner(ctx, tok, ownerSlug)
	if err != nil {
		return types.Owner{}, nil, err
	}
	if tok.IsOperator() {
		full, err := a.Store.OwnerBySlug(ctx, ownerSlug)
		if err == sql.ErrNoRows {
			return types.Owner{}, nil, ErrNotFound
		}
		if err != nil {
			return types.Owner{}, nil, err
		}
		o = full
	} else {
		full, err := a.Store.OwnerBySlug(ctx, ownerSlug)
		if err != nil && err != sql.ErrNoRows {
			return types.Owner{}, nil, err
		}
		if err == nil {
			o = full
		}
	}
	infos, err := a.ledgersWithFreeze(ctx, o)
	return o, infos, err
}

func (a *App) ListOwners(ctx context.Context, tok types.Token) ([]types.Owner, error) {
	if err := a.requireOperator(tok); err != nil {
		return nil, err
	}
	return a.Store.ListOwners(ctx)
}

// OwnersForSession is the HTML home list. An owner-bound session sees that
// owner. GitHub allowlist and operator see every owner.
func (a *App) OwnersForSession(ctx context.Context, sess types.Session) ([]types.Owner, error) {
	if sess.OwnerSlug != "" && !sess.IsOperator() {
		o, err := a.Store.OwnerBySlug(ctx, sess.OwnerSlug)
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		if err != nil {
			return nil, err
		}
		return []types.Owner{o}, nil
	}
	return a.Store.ListOwners(ctx)
}

type LedgerInfo struct {
	types.Ledger
	Frozen bool
}

func (a *App) ledgersWithFreeze(ctx context.Context, o types.Owner) ([]LedgerInfo, error) {
	ledgers, err := a.Store.ListLedgers(ctx, o.ID)
	if err != nil {
		return nil, err
	}
	out := make([]LedgerInfo, 0, len(ledgers))
	for i, l := range ledgers {
		frozen := o.MaxLedgers > 0 && i >= o.MaxLedgers
		out = append(out, LedgerInfo{Ledger: l, Frozen: frozen})
	}
	return out, nil
}

func (a *App) LedgerFrozen(ctx context.Context, owner, ledger string) (bool, error) {
	l, err := a.Store.ResolveLedger(ctx, owner, ledger)
	if err == sql.ErrNoRows {
		return false, ErrNotFound
	}
	if err != nil {
		return false, err
	}
	ok, err := a.Store.LedgerWritable(ctx, l.ID)
	if err != nil {
		return false, err
	}
	return !ok, nil
}

func (a *App) requireWritable(ctx context.Context, l types.Ledger) error {
	ok, err := a.Store.LedgerWritable(ctx, l.ID)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("%w: ledger over max_ledgers", ErrPolicy)
	}
	return nil
}

func (a *App) loadForWrite(ctx context.Context, tok types.Token, owner, ledger, handle string) (types.Task, error) {
	t, err := a.Get(ctx, tok, owner, ledger, handle)
	if err != nil {
		return t, err
	}
	l, err := a.Ledger(ctx, tok, owner, ledger)
	if err != nil {
		return t, err
	}
	if err := a.requireWritable(ctx, l); err != nil {
		return t, err
	}
	return t, nil
}
