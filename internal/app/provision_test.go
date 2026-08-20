package app_test

import (
	"context"
	"errors"
	"testing"

	"github.com/markedo-org/ledger/internal/app"
	"github.com/markedo-org/ledger/internal/types"
)

func TestOperatorProvisionAndFreeze(t *testing.T) {
	a, tok := boot(t)
	a.SetOperatorToken("lgr_operator_test")
	ctx := context.Background()
	op, err := a.Auth(ctx, "lgr_operator_test")
	if err != nil || !op.IsOperator() {
		t.Fatalf("operator auth %v %+v", err, op)
	}
	if _, err := a.CreateOwner(ctx, tok, app.CreateOwnerInput{Slug: "acme"}); err == nil {
		t.Fatal("owner admin must not create owners")
	}
	if _, err := a.SetMaxLedgers(ctx, tok, "markedo", 2); err == nil {
		t.Fatal("owner admin must not set max_ledgers")
	}

	created, err := a.CreateOwner(ctx, op, app.CreateOwnerInput{
		Slug: "acme", Ledger: "inbox", Actor: "ada",
	})
	if err != nil || created.Owner.MaxLedgers != 1 || created.Ledger == nil || created.Token == nil {
		t.Fatalf("create owner %+v %v", created, err)
	}
	ada, err := a.Auth(ctx, created.Token.Plain)
	if err != nil || ada.Role != types.RoleAdmin || ada.OwnerSlug != "acme" || ada.LedgerSlug != "" {
		t.Fatalf("minted admin %+v %v", ada, err)
	}
	if created.WriteToken == nil {
		t.Fatal("expected first-ledger write token")
	}
	write, err := a.Auth(ctx, created.WriteToken.Plain)
	if err != nil || write.Role != types.RoleWrite || write.OwnerSlug != "acme" || write.LedgerSlug != "inbox" {
		t.Fatalf("minted write %+v %v", write, err)
	}
	if _, err := a.CreateLedger(ctx, ada, "acme", app.CreateLedgerInput{Slug: "extra"}); err == nil {
		t.Fatal("expected cap at 1")
	}

	if _, err := a.SetMaxLedgers(ctx, op, "acme", 2); err != nil {
		t.Fatal(err)
	}
	if _, err := a.CreateLedger(ctx, op, "acme", app.CreateLedgerInput{Slug: "extra"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := a.Create(ctx, ada, "acme", "extra", app.CreateInput{Title: "On extra", IdempotencyKey: "e0"}); err != nil {
		t.Fatal(err)
	}
	held, err := a.Claim(ctx, ada, "acme", "extra", "T-001", app.ClaimInput{})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := a.SetMaxLedgers(ctx, op, "acme", 1); err != nil {
		t.Fatal(err)
	}
	if _, _, err := a.Create(ctx, ada, "acme", "extra", app.CreateInput{Title: "Nope", IdempotencyKey: "x1"}); !errors.Is(err, app.ErrPolicy) {
		t.Fatalf("frozen write %v", err)
	}
	if _, err := a.Heartbeat(ctx, ada, "acme", "extra", "T-001", 0, held.ClaimID); !errors.Is(err, app.ErrPolicy) {
		t.Fatalf("frozen heartbeat %v", err)
	}
	if _, err := a.Release(ctx, ada, "acme", "extra", "T-001", held.ClaimID); err != nil {
		t.Fatalf("release on frozen %v", err)
	}
	if _, _, err := a.Create(ctx, ada, "acme", "inbox", app.CreateInput{Title: "Oldest stays", IdempotencyKey: "i1"}); err != nil {
		t.Fatalf("oldest writable %v", err)
	}

	if _, err := a.SetMaxLedgers(ctx, op, "acme", 2); err != nil {
		t.Fatal(err)
	}
	if _, _, err := a.Create(ctx, ada, "acme", "extra", app.CreateInput{Title: "Unfrozen", IdempotencyKey: "e1"}); err != nil {
		t.Fatalf("unfreeze %v", err)
	}

	if _, err := a.SetMaxLedgers(ctx, op, "acme", 0); err != nil {
		t.Fatal(err)
	}
	third, err := a.CreateLedger(ctx, ada, "acme", app.CreateLedgerInput{Slug: "third"})
	if err != nil || third.Slug != "third" {
		t.Fatalf("unlimited cap %v %+v", err, third)
	}

	if _, err := a.CreateOwner(ctx, op, app.CreateOwnerInput{Slug: "admin"}); err == nil {
		t.Fatal("reserved slug")
	}
}

// An operator session reaches the pages that are outside any tenancy, and
// stops at the door of one. The board renders from ListPublic and is gated on
// Covers alone, so this is the only thing standing between a host session and
// every customer's tasks.
func TestOperatorSessionStopsAtTheTenantDoor(t *testing.T) {
	a, _ := boot(t)
	a.SetOperatorToken("lgr_operator_test")
	sess, _, err := a.SessionFromAPIToken(context.Background(), "lgr_operator_test")
	if err != nil || !sess.IsOperator() {
		t.Fatalf("operator session %+v %v", sess, err)
	}
	if !sess.Covers("", "") {
		t.Fatal("operator cannot reach the owner list it provisions")
	}
	if sess.Covers("acme", "inbox") {
		t.Fatal("operator session can open a tenant's board")
	}
	if sess.Covers("acme", "") {
		t.Fatal("operator session can open a tenant's ledger index")
	}
}
