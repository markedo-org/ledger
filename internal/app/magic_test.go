package app_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/markedo-org/ledger/internal/app"
	"github.com/markedo-org/ledger/internal/types"
)

type fakeMail struct {
	mu      sync.Mutex
	n       int
	to      string
	subject string
	body    string
}

func (f *fakeMail) Enabled() bool { return true }

func (f *fakeMail) Send(_ context.Context, to, subject, body string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.n++
	f.to, f.subject, f.body = to, subject, body
	return nil
}

func TestMagicLinkOffUntilSMTP(t *testing.T) {
	a, _ := boot(t)
	err := a.RequestMagicLink(context.Background(), "lg@forsberg.eu")
	if !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("want not found while mail is off, got %v", err)
	}
}

func TestMagicLinkUnknownEmailSilent(t *testing.T) {
	a, _ := boot(t)
	mailer := &fakeMail{}
	a.Mail = mailer
	if err := a.RequestMagicLink(context.Background(), "nobody@example.com"); err != nil {
		t.Fatal(err)
	}
	if mailer.n != 0 {
		t.Fatalf("sent %d messages for unknown email", mailer.n)
	}
}

func TestMagicLinkRoundTrip(t *testing.T) {
	a, tok := boot(t)
	ctx := context.Background()
	mailer := &fakeMail{}
	a.Mail = mailer
	a.PublicURL = "http://example.test"
	issued, err := a.CreateToken(ctx, tok, "markedo", app.CreateTokenInput{
		Actor: "lg",
		Role:  types.RoleWrite,
		Email: "LG@Forsberg.EU",
	})
	if err != nil {
		t.Fatal(err)
	}
	if issued.Token.Email != "lg@forsberg.eu" {
		t.Fatalf("email %q", issued.Token.Email)
	}
	if err := a.RequestMagicLink(ctx, "lg@forsberg.eu"); err != nil {
		t.Fatal(err)
	}
	if mailer.n != 1 || mailer.to != "lg@forsberg.eu" {
		t.Fatalf("send n=%d to=%s", mailer.n, mailer.to)
	}
	if strings.Contains(mailer.body, issued.Plain) {
		t.Fatal("API token leaked into the email")
	}
	i := strings.Index(mailer.body, "lgl_")
	if i < 0 {
		t.Fatalf("no magic code in body: %s", mailer.body)
	}
	code := strings.Fields(mailer.body[i:])[0]
	if !strings.Contains(mailer.body, "http://example.test/login/email?code="+code) {
		t.Fatalf("link missing: %s", mailer.body)
	}
	sess, _, err := a.ConsumeMagicLink(ctx, code)
	if err != nil {
		t.Fatal(err)
	}
	if sess.Actor != "lg" {
		t.Fatalf("actor %s", sess.Actor)
	}
	if _, _, err := a.ConsumeMagicLink(ctx, code); !errors.Is(err, app.ErrUnauthorized) {
		t.Fatalf("reuse want unauthorized, got %v", err)
	}
}

func TestMagicLinkDuplicateEmail(t *testing.T) {
	a, tok := boot(t)
	ctx := context.Background()
	if _, err := a.CreateToken(ctx, tok, "markedo", app.CreateTokenInput{Actor: "one", Email: "shared@example.com"}); err != nil {
		t.Fatal(err)
	}
	_, err := a.CreateToken(ctx, tok, "markedo", app.CreateTokenInput{Actor: "two", Email: "shared@example.com"})
	if !errors.Is(err, app.ErrConflict) {
		t.Fatalf("want conflict, got %v", err)
	}
}

func TestMagicLinkRateLimit(t *testing.T) {
	a, tok := boot(t)
	ctx := context.Background()
	mailer := &fakeMail{}
	a.Mail = mailer
	if _, err := a.CreateToken(ctx, tok, "markedo", app.CreateTokenInput{Actor: "lg", Email: "lg@forsberg.eu"}); err != nil {
		t.Fatal(err)
	}
	if err := a.RequestMagicLink(ctx, "lg@forsberg.eu"); err != nil {
		t.Fatal(err)
	}
	if err := a.RequestMagicLink(ctx, "lg@forsberg.eu"); err != nil {
		t.Fatal(err)
	}
	if mailer.n != 1 {
		t.Fatalf("rate limit: sent %d", mailer.n)
	}
}

func TestMagicLinkInvalidEmail(t *testing.T) {
	a, _ := boot(t)
	a.Mail = &fakeMail{}
	err := a.RequestMagicLink(context.Background(), "not-an-email")
	if !errors.Is(err, app.ErrInvalid) {
		t.Fatalf("want invalid, got %v", err)
	}
}
