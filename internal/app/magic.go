package app

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/mail"
	"strings"
	"sync"
	"time"

	"github.com/markedo-org/ledger/internal/store"
	"github.com/markedo-org/ledger/internal/types"
)

func (a *App) MailEnabled() bool {
	return a.Mail != nil && a.Mail.Enabled()
}

func NormalizeEmail(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	addr, err := mail.ParseAddress(raw)
	if err != nil || addr.Address == "" || !strings.Contains(addr.Address, "@") {
		return "", fmt.Errorf("%w: invalid email", ErrInvalid)
	}
	return strings.ToLower(addr.Address), nil
}

func (a *App) RequestMagicLink(ctx context.Context, email string) error {
	if !a.MailEnabled() {
		return fmt.Errorf("%w: magic-link email is off until SMTP is set", ErrNotFound)
	}
	addr, err := NormalizeEmail(email)
	if err != nil {
		return err
	}
	if addr == "" {
		return fmt.Errorf("%w: email required", ErrInvalid)
	}
	if !a.allowMagic(addr) {
		return nil
	}
	tok, err := a.Store.TokenByEmail(ctx, addr)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}
	code, err := a.Store.CreateMagicLink(ctx, tok.ID)
	if err != nil {
		return err
	}
	base := strings.TrimRight(a.PublicURL, "/")
	if base == "" {
		base = "http://127.0.0.1:8080"
	}
	link := base + "/login/email?code=" + code
	body := "Sign in to task-ledger with this link. It expires in 15 minutes and works once.\n\n" + link + "\n\nIf you did not ask for this, ignore the message.\n"
	if err := a.Mail.Send(ctx, addr, "Your task-ledger sign-in link", body); err != nil {
		log.Printf("magic-link send: %v", err)
	}
	return nil
}

func (a *App) publicBase() string {
	base := strings.TrimRight(a.PublicURL, "/")
	if base == "" {
		return "http://127.0.0.1:8080"
	}
	return base
}

func (a *App) MintReviewURL(ctx context.Context, tok types.Token) (string, int, error) {
	if tok.ID == "" {
		return "", 0, fmt.Errorf("%w: review_url needs a minted owner or ledger token, not the operator secret", ErrForbidden)
	}
	code, err := a.Store.CreateReviewLink(ctx, tok.ID)
	if err != nil {
		return "", 0, err
	}
	return a.publicBase() + "/login/review?code=" + code, int(store.MagicTTL.Seconds()), nil
}

func (a *App) ConsumeReviewLink(ctx context.Context, code string) (types.Session, string, error) {
	code = strings.TrimSpace(code)
	if !strings.HasPrefix(code, "lgv_") {
		return types.Session{}, "", ErrUnauthorized
	}
	tok, err := a.Store.ConsumeReviewLink(ctx, code)
	if err == sql.ErrNoRows {
		return types.Session{}, "", ErrUnauthorized
	}
	if err != nil {
		return types.Session{}, "", err
	}
	return a.CreateSession(ctx, tok.Actor, "", "", tok.OwnerSlug, tok.LedgerSlug, sessionRole(tok), tok.ID)
}

func (a *App) ConsumeMagicLink(ctx context.Context, code string) (types.Session, string, error) {
	code = strings.TrimSpace(code)
	if code == "" {
		return types.Session{}, "", ErrUnauthorized
	}
	tok, err := a.Store.ConsumeMagicLink(ctx, code)
	if err == sql.ErrNoRows {
		return types.Session{}, "", ErrUnauthorized
	}
	if err != nil {
		return types.Session{}, "", err
	}
	return a.CreateSession(ctx, tok.Actor, "", "", tok.OwnerSlug, tok.LedgerSlug, sessionRole(tok), tok.ID)
}

type magicGate struct {
	mu   sync.Mutex
	last map[string]time.Time
}

func (a *App) allowMagic(email string) bool {
	if a.magic == nil {
		a.magic = &magicGate{last: map[string]time.Time{}}
	}
	g := a.magic
	g.mu.Lock()
	defer g.mu.Unlock()
	now := time.Now()
	if t, ok := g.last[email]; ok && now.Sub(t) < time.Minute {
		return false
	}
	g.last[email] = now
	return true
}
