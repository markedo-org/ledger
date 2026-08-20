package app_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/markedo-org/ledger/internal/app"
	"github.com/markedo-org/ledger/internal/store"
	"github.com/markedo-org/ledger/internal/types"
)

func verifyFixture(t *testing.T) (*app.App, types.Token) {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	plain, err := store.NewToken()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Bootstrap(context.Background(), "markedo", "meta", "maria", plain); err != nil {
		t.Fatal(err)
	}
	a := app.New(s)
	tok, err := a.Auth(context.Background(), plain)
	if err != nil {
		t.Fatal(err)
	}
	return a, tok
}

func closedTask(t *testing.T, a *app.App, tok types.Token, actor string) types.Task {
	t.Helper()
	ctx := context.Background()
	worker := tok
	worker.Actor = actor
	task, _, err := a.Create(ctx, worker, "markedo", "meta", app.CreateInput{
		Title:          "Ship the slice",
		Phase:          "NOW",
		IdempotencyKey: "verify-" + actor,
	})
	if err != nil {
		t.Fatal(err)
	}
	done, err := a.Close(ctx, worker, "markedo", "meta", task.Handle, "shipped", "")
	if err != nil {
		t.Fatal(err)
	}
	return done
}

// Verify is the product's quality gate and it recorded nothing on the task
// about who passed it, so an agent closing its own work and verifying it a
// second later was indistinguishable from a second pair of eyes.
func TestTheTaskRemembersWhoClosedAndWhoVerified(t *testing.T) {
	a, tok := verifyFixture(t)
	ctx := context.Background()

	done := closedTask(t, a, tok, "maria")
	if done.ClosedBy != "maria" {
		t.Fatalf("closed_by = %q, want maria", done.ClosedBy)
	}

	reviewer := tok
	reviewer.Actor = "petra"
	out, err := a.Verify(ctx, reviewer, "markedo", "meta", done.Handle)
	if err != nil {
		t.Fatal(err)
	}
	if out.VerifiedBy != "petra" {
		t.Fatalf("verified_by = %q, want petra", out.VerifiedBy)
	}
	if out.VerifiedBy == out.ClosedBy {
		t.Fatal("a second pair of eyes should not read as self-verification")
	}
}

// Self-verification stays allowed. A ledger with one agent on it would
// otherwise never reach verified, which is the situation most of ours are in.
// The point is that it is now visible rather than forbidden.
func TestVerifyingYourOwnWorkIsAllowedAndVisible(t *testing.T) {
	a, tok := verifyFixture(t)
	ctx := context.Background()

	done := closedTask(t, a, tok, "maria")
	out, err := a.Verify(ctx, tok, "markedo", "meta", done.Handle)
	if err != nil {
		t.Fatalf("a solo agent could not verify its own work: %v", err)
	}
	if out.VerifiedBy != out.ClosedBy {
		t.Fatalf("closed by %q, verified by %q: the board cannot tell these are the same actor",
			out.ClosedBy, out.VerifiedBy)
	}
}
