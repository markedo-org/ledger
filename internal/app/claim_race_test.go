package app_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/markedo-org/ledger/internal/app"
	"github.com/markedo-org/ledger/internal/types"
)

// writers mints n write tokens on the shared ledger, each with its own actor
// name, so a race is between agents rather than between sessions of one agent.
func writers(t *testing.T, a *app.App, admin types.Token, n int) []types.Token {
	t.Helper()
	ctx := context.Background()
	out := make([]types.Token, 0, n)
	for i := 0; i < n; i++ {
		issued, err := a.CreateToken(ctx, admin, "markedo", app.CreateTokenInput{
			Actor:  string(rune('a'+i)) + "-agent",
			Ledger: "meta",
			Role:   "write",
		})
		if err != nil {
			t.Fatal(err)
		}
		tok, err := a.Auth(ctx, issued.Plain)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, tok)
	}
	return out
}

// The lease is the whole promise: one agent owns a task at a time. Claim reads
// the task, decides nobody holds it, then writes, and those three steps are
// not one step. Eight agents reading the same unclaimed task all used to
// decide it was free, all mint a claim_id and all come away believing they own
// the work.
func TestOnlyOneAgentWinsAContestedClaim(t *testing.T) {
	a, admin := boot(t)
	ctx := context.Background()
	if _, _, err := a.Create(ctx, admin, "markedo", "meta", app.CreateInput{
		Title: "Contested", IdempotencyKey: "race-1", Phase: "NOW",
	}); err != nil {
		t.Fatal(err)
	}

	const agents = 8
	toks := writers(t, a, admin, agents)

	start := make(chan struct{})
	var wg sync.WaitGroup
	won := make([]types.Task, agents)
	errs := make([]error, agents)

	for i := 0; i < agents; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			won[i], errs[i] = a.Claim(ctx, toks[i], "markedo", "meta", "T-001", app.ClaimInput{TTL: time.Minute})
		}(i)
	}
	close(start)
	wg.Wait()

	winners := 0
	var winner types.Task
	for i, err := range errs {
		switch {
		case err == nil:
			winners++
			winner = won[i]
		case errors.Is(err, app.ErrConflict):
		default:
			t.Fatalf("agent %d: unexpected error %v", i, err)
		}
	}
	if winners != 1 {
		t.Fatalf("%d agents were told they hold the lease, want exactly 1", winners)
	}
	if winner.ClaimID == "" {
		t.Fatal("the winner was not given a claim_id")
	}

	// And the winner really holds it: the lease it was handed is the one the
	// task is carrying, so it can go on working.
	if _, err := a.Heartbeat(ctx, toks[indexOf(errs)], "markedo", "meta", "T-001", time.Minute, winner.ClaimID); err != nil {
		t.Fatalf("the winner cannot heartbeat its own lease: %v", err)
	}
}

func indexOf(errs []error) int {
	for i, err := range errs {
		if err == nil {
			return i
		}
	}
	return -1
}

// Asking for the next task should hand back a task. Losing a race for one
// candidate is not a reason to answer "no eligible task" when others are
// waiting, so two agents asking at the same moment get one each.
func TestConcurrentNextGivesEachAgentADifferentTask(t *testing.T) {
	a, admin := boot(t)
	ctx := context.Background()
	for i, title := range []string{"First", "Second"} {
		if _, _, err := a.Create(ctx, admin, "markedo", "meta", app.CreateInput{
			Title: title, IdempotencyKey: "race-next-" + string(rune('a'+i)), Phase: "NOW",
		}); err != nil {
			t.Fatal(err)
		}
	}

	toks := writers(t, a, admin, 2)
	start := make(chan struct{})
	var wg sync.WaitGroup
	got := make([]types.Task, 2)
	errs := make([]error, 2)

	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			got[i], errs[i] = a.Next(ctx, toks[i], "markedo", "meta", "", time.Minute)
		}(i)
	}
	close(start)
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("agent %d asked for work and was refused: %v", i, err)
		}
	}
	if got[0].Handle == got[1].Handle {
		t.Fatalf("both agents were handed %s", got[0].Handle)
	}
}

// Ticking a check changes the task, so a claim decided from a read taken
// before that tick is working from a stale picture and must not land.
func TestATickedCheckInvalidatesAStaleRead(t *testing.T) {
	a, admin := boot(t)
	ctx := context.Background()
	if _, _, err := a.Create(ctx, admin, "markedo", "meta", app.CreateInput{
		Title: "Versioned", IdempotencyKey: "race-2", Checks: []string{"one"},
	}); err != nil {
		t.Fatal(err)
	}

	before, err := a.Get(ctx, admin, "markedo", "meta", "T-001")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.SetCheck(ctx, admin, "markedo", "meta", "T-001", 1, "", true, ""); err != nil {
		t.Fatal(err)
	}
	after, err := a.Get(ctx, admin, "markedo", "meta", "T-001")
	if err != nil {
		t.Fatal(err)
	}
	if after.Version <= before.Version {
		t.Fatalf("version stayed at %d after a check was ticked", after.Version)
	}

	// Same for tags.
	if _, err := a.SetTags(ctx, admin, "markedo", "meta", "T-001", []string{"ledger"}, ""); err != nil {
		t.Fatal(err)
	}
	tagged, err := a.Get(ctx, admin, "markedo", "meta", "T-001")
	if err != nil {
		t.Fatal(err)
	}
	if tagged.Version <= after.Version {
		t.Fatalf("version stayed at %d after tags changed", tagged.Version)
	}
}
