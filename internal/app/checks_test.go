package app_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/markedo-org/ledger/internal/app"
	"github.com/markedo-org/ledger/internal/store"
	"github.com/markedo-org/ledger/internal/types"
)

func taskWithChecks(t *testing.T) (*app.App, types.Token, types.Task) {
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
	task, _, err := a.Create(context.Background(), tok, "markedo", "meta", app.CreateInput{
		Title:          "Ship the slice",
		Phase:          "NOW",
		IdempotencyKey: "checks",
		Checks:         []string{"one", "two", "three", "four", "five", "six"},
	})
	if err != nil {
		t.Fatal(err)
	}
	return a, tok, task
}

// Finishing a task with six boxes meant six calls, each reloading the task and
// re-checking the lease, and an agent that stopped halfway left it in a state
// nobody asked for.
func TestSeveralBoxesGoDownInOneCall(t *testing.T) {
	a, tok, task := taskWithChecks(t)

	out, err := a.SetChecks(context.Background(), tok, "markedo", "meta", task.Handle,
		[]int{1, 2, 3, 4, 5, 6}, "", true, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range out.Checks {
		if !c.Done {
			t.Fatalf("check %q was left unticked by the batch", c.Body)
		}
	}
	// One change, one version bump, so a holder of a stale read is told once.
	if out.Version != task.Version+1 {
		t.Fatalf("version went %d -> %d, want a single bump for one batch", task.Version, out.Version)
	}
}

// A bad index in the middle must not leave the earlier boxes ticked. Half a
// change is worse than none, because nothing says which half happened.
func TestABadIndexTicksNothing(t *testing.T) {
	a, tok, task := taskWithChecks(t)

	_, err := a.SetChecks(context.Background(), tok, "markedo", "meta", task.Handle,
		[]int{1, 2, 99}, "", true, "")
	if err == nil {
		t.Fatal("a check that does not exist should be refused")
	}

	got, err := a.Get(context.Background(), tok, "markedo", "meta", task.Handle)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range got.Checks {
		if c.Done {
			t.Fatalf("check %q was ticked by a batch that failed", c.Body)
		}
	}
}

// The lease is checked once for the batch, and it still has to hold.
func TestABatchStillNeedsTheClaimID(t *testing.T) {
	a, tok, task := taskWithChecks(t)
	ctx := context.Background()

	held, err := a.Claim(ctx, tok, "markedo", "meta", task.Handle, app.ClaimInput{TTL: 30 * time.Minute})
	if err != nil {
		t.Fatal(err)
	}

	other := tok
	other.Actor = "petra"
	if _, err := a.SetChecks(ctx, other, "markedo", "meta", task.Handle, []int{1, 2}, "", true, ""); err == nil {
		t.Fatal("a batch went through on someone else's live lease without the claim_id")
	}
	if _, err := a.SetChecks(ctx, tok, "markedo", "meta", task.Handle, []int{1, 2}, "", true, held.ClaimID); err != nil {
		t.Fatalf("the lease holder was refused its own task: %v", err)
	}
}
