package app_test

import (
	"context"
	"errors"
	"testing"

	"github.com/markedo-org/ledger/internal/app"
)

func TestReadTokenSeesOneLedgerAndCannotWrite(t *testing.T) {
	a, admin := boot(t)
	ctx := context.Background()
	if _, err := a.CreateLedger(ctx, admin, "markedo", app.CreateLedgerInput{Slug: "other", Title: "Other"}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.CreateToken(ctx, admin, "markedo", app.CreateTokenInput{Actor: "reader", Role: "read"}); err == nil {
		t.Fatal("owner-wide read token was accepted")
	}
	issued, err := a.CreateToken(ctx, admin, "markedo", app.CreateTokenInput{Actor: "reader", Role: "read", Ledger: "meta"})
	if err != nil {
		t.Fatal(err)
	}
	if issued.Token.Role != "read" || issued.Token.LedgerSlug != "meta" {
		t.Fatalf("token bound wrong: %+v", issued.Token)
	}
	created, _, err := a.Create(ctx, admin, "markedo", "meta", app.CreateInput{Title: "Visible", IdempotencyKey: "read-1"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.Get(ctx, issued.Token, "markedo", "meta", created.Handle); err != nil {
		t.Fatal(err)
	}
	ledgers, err := a.ListLedgers(ctx, issued.Token, "markedo")
	if err != nil {
		t.Fatal(err)
	}
	if len(ledgers) != 1 || ledgers[0].Slug != "meta" {
		t.Fatalf("read token saw %v", ledgers)
	}
	if _, _, _, err := a.List(ctx, issued.Token, "markedo", "other", app.ListQuery{}); !errors.Is(err, app.ErrForbidden) {
		t.Fatalf("sibling ledger: %v", err)
	}
	if _, _, err := a.Create(ctx, issued.Token, "markedo", "meta", app.CreateInput{Title: "Nope", IdempotencyKey: "read-2"}); !errors.Is(err, app.ErrForbidden) {
		t.Fatalf("create: %v", err)
	}
	if _, err := a.AddNote(ctx, issued.Token, "markedo", "meta", created.Handle, "nope"); !errors.Is(err, app.ErrForbidden) {
		t.Fatalf("note: %v", err)
	}
}
