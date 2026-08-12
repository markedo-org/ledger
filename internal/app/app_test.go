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

func boot(t *testing.T) (*app.App, types.Token) {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	tok, err := store.NewToken()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Bootstrap(context.Background(), "markedo", "meta", "maria", tok); err != nil {
		t.Fatal(err)
	}
	a := app.New(s)
	auth, err := a.Auth(context.Background(), tok)
	if err != nil {
		t.Fatal(err)
	}
	return a, auth
}

func TestCreateClaimClose(t *testing.T) {
	a, tok := boot(t)
	ctx := context.Background()
	task, replay, err := a.Create(ctx, tok, "markedo", "meta", app.CreateInput{
		Title:          "First",
		IdempotencyKey: "k1",
	})
	if err != nil || replay {
		t.Fatalf("create: %v replay=%v", err, replay)
	}
	if task.Handle != "T-001" {
		t.Fatalf("handle %s", task.Handle)
	}
	again, replay, err := a.Create(ctx, tok, "markedo", "meta", app.CreateInput{
		Title:          "First",
		IdempotencyKey: "k1",
	})
	if err != nil || !replay || again.Handle != "T-001" {
		t.Fatalf("idempotency: %+v replay=%v err=%v", again, replay, err)
	}
	second, _, err := a.Create(ctx, tok, "markedo", "meta", app.CreateInput{Title: "Second"})
	if err != nil || second.Handle != "T-002" {
		t.Fatalf("second %s %v", second.Handle, err)
	}

	held, err := a.Claim(ctx, tok, "markedo", "meta", "T-001", app.ClaimInput{TTL: time.Minute})
	if err != nil || held.ClaimedBy != "maria" {
		t.Fatalf("claim %v %+v", err, held)
	}

	if _, err = a.Close(ctx, tok, "markedo", "meta", "T-001", ""); err == nil {
		t.Fatal("close without evidence")
	}
	closed, err := a.Close(ctx, tok, "markedo", "meta", "T-001", "tests pass")
	if err != nil || closed.Phase != types.PhaseDONE {
		t.Fatalf("close %v %+v", err, closed)
	}
}

func TestDeferralPolicy(t *testing.T) {
	a, tok := boot(t)
	ctx := context.Background()
	if _, _, err := a.Create(ctx, tok, "markedo", "meta", app.CreateInput{Title: "Slippery"}); err != nil {
		t.Fatal(err)
	}
	steps := []string{"NEXT", "LATER", "GATED"}
	for i, phase := range steps {
		if _, err := a.SetPhase(ctx, tok, "markedo", "meta", "T-001", app.PhaseInput{Phase: phase, Reason: "not now"}); err != nil {
			t.Fatalf("defer %d: %v", i, err)
		}
	}
	if _, err := a.SetPhase(ctx, tok, "markedo", "meta", "T-001", app.PhaseInput{Phase: "PARKED", Reason: "again"}); err == nil {
		t.Fatal("expected policy block on fourth deferral")
	}
}

func TestNextSkipsClaimed(t *testing.T) {
	a, tok := boot(t)
	ctx := context.Background()
	if _, _, err := a.Create(ctx, tok, "markedo", "meta", app.CreateInput{Title: "A"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := a.Create(ctx, tok, "markedo", "meta", app.CreateInput{Title: "B"}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Claim(ctx, tok, "markedo", "meta", "T-001", app.ClaimInput{}); err != nil {
		t.Fatal(err)
	}
	n, err := a.Next(ctx, tok, "markedo", "meta", "T", 0)
	if err != nil || n.Handle != "T-002" {
		t.Fatalf("next %s %v", n.Handle, err)
	}
}
