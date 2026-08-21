package app_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/markedo-org/ledger/internal/app"
	"github.com/markedo-org/ledger/internal/types"
)

// tenant stands up an owner the way a hosting provider would, and hands back
// the operator token and the owner's own admin token.
func tenant(t *testing.T) (*app.App, types.Token, types.Token) {
	t.Helper()
	a, _ := boot(t)
	a.SetOperatorToken("lgr_operator_scope")
	ctx := context.Background()

	op, err := a.Auth(ctx, "lgr_operator_scope")
	if err != nil {
		t.Fatal(err)
	}
	created, err := a.CreateOwner(ctx, op, app.CreateOwnerInput{
		Slug: "acme", Ledger: "inbox", Actor: "ada", MaxLedgers: 5,
	})
	if err != nil || created.Token == nil {
		t.Fatalf("provision acme: %+v %v", created, err)
	}
	ada, err := a.Auth(ctx, created.Token.Plain)
	if err != nil {
		t.Fatal(err)
	}
	return a, op, ada
}

// The operator token belongs to whoever runs the machine. It sells capacity;
// it is not a member of anyone's team. It used to satisfy every ownership and
// admin check in the codebase, which made "operator" mean root over every
// tenant's tasks on the box.
func TestOperatorCannotReachTenantWork(t *testing.T) {
	a, op, ada := tenant(t)
	ctx := context.Background()

	if _, _, err := a.Create(ctx, ada, "acme", "inbox", app.CreateInput{
		Title: "Customer work", IdempotencyKey: "k1",
	}); err != nil {
		t.Fatal(err)
	}

	t.Run("cannot read the board", func(t *testing.T) {
		if _, _, _, err := a.List(ctx, op, "acme", "inbox", app.ListQuery{}); !errors.Is(err, app.ErrForbidden) {
			t.Fatalf("operator listed a tenant's tasks, got %v", err)
		}
	})

	t.Run("cannot read one task", func(t *testing.T) {
		if _, err := a.Get(ctx, op, "acme", "inbox", "T-001"); !errors.Is(err, app.ErrForbidden) {
			t.Fatalf("operator read a tenant's task, got %v", err)
		}
	})

	t.Run("cannot add work", func(t *testing.T) {
		if _, _, err := a.Create(ctx, op, "acme", "inbox", app.CreateInput{
			Title: "Not yours", IdempotencyKey: "k2",
		}); !errors.Is(err, app.ErrForbidden) {
			t.Fatalf("operator wrote into a tenant's ledger, got %v", err)
		}
	})

	t.Run("cannot take a lease", func(t *testing.T) {
		if _, err := a.Claim(ctx, op, "acme", "inbox", "T-001", app.ClaimInput{TTL: time.Minute}); !errors.Is(err, app.ErrForbidden) {
			t.Fatalf("operator claimed a tenant's task, got %v", err)
		}
	})

	t.Run("cannot empty the ledger", func(t *testing.T) {
		if _, _, err := a.ResetLedger(ctx, op, "acme", "inbox", "acme/inbox"); !errors.Is(err, app.ErrForbidden) {
			t.Fatalf("operator reset a tenant's ledger, got %v", err)
		}
	})

	t.Run("cannot change ledger settings", func(t *testing.T) {
		title := "Renamed by the host"
		if _, err := a.PatchLedger(ctx, op, "acme", "inbox", &title, nil, nil); !errors.Is(err, app.ErrForbidden) {
			t.Fatalf("operator edited a tenant's ledger, got %v", err)
		}
	})

	t.Run("cannot see or kill their tokens", func(t *testing.T) {
		if _, err := a.ListTokens(ctx, op, "acme"); !errors.Is(err, app.ErrForbidden) {
			t.Fatalf("operator listed a tenant's tokens, got %v", err)
		}
	})
}

// The one that decides whether any of the above is real. An operator that can
// mint itself an admin token for an existing owner still holds root over every
// tenant; it just takes a second call to get there.
func TestOperatorCannotMintItselfAdminOverATenant(t *testing.T) {
	a, op, _ := tenant(t)

	_, err := a.CreateToken(context.Background(), op, "acme", app.CreateTokenInput{
		Actor: "host", Role: types.RoleAdmin,
	})
	if !errors.Is(err, app.ErrForbidden) {
		t.Fatalf("operator minted an admin token for an existing owner, got %v", err)
	}
}

// And the narrowing has to leave a hosting provider able to do its job, or we
// have only moved the problem into a support queue.
func TestOperatorCanStillProvision(t *testing.T) {
	a, op, _ := tenant(t)
	ctx := context.Background()

	if _, err := a.CreateLedger(ctx, op, "acme", app.CreateLedgerInput{Slug: "second", Title: "Second"}); err != nil {
		t.Fatalf("operator could not add a ledger it just sold: %v", err)
	}
	if _, err := a.CreateToken(ctx, op, "acme", app.CreateTokenInput{
		Actor: "bob", Ledger: "inbox", Role: types.RoleWrite,
	}); err != nil {
		t.Fatalf("operator could not hand a tenant a write token: %v", err)
	}
	if _, err := a.SetMaxLedgers(ctx, op, "acme", 2); err != nil {
		t.Fatalf("operator could not change the meter: %v", err)
	}
	if _, _, err := a.GetOwner(ctx, op, "acme"); err != nil {
		t.Fatalf("operator could not read the owner it provisions: %v", err)
	}
	if _, err := a.ListOwners(ctx, op); err != nil {
		t.Fatalf("operator could not list owners: %v", err)
	}
}

// An owner is born with exactly one admin token, and that is the act the
// operator exists to perform. Barring the operator from minting admin tokens
// must not break the moment a customer signs up.
func TestOwnerIsBornWithAnAdminTokenAndItWorks(t *testing.T) {
	a, _, ada := tenant(t)
	ctx := context.Background()

	if ada.Role != types.RoleAdmin {
		t.Fatalf("the token handed over at signup has role %q, want admin", ada.Role)
	}
	if _, err := a.CreateToken(ctx, ada, "acme", app.CreateTokenInput{
		Actor: "carol", Role: types.RoleAdmin,
	}); err != nil {
		t.Fatalf("an owner admin cannot grow its own team: %v", err)
	}
}
