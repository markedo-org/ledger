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

func TestBootstrapIgnoresDefaultOwnerOnExistingDB(t *testing.T) {
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
	again, err := s.Bootstrap(context.Background(), "acme", "inbox", "ada", tok)
	if err != nil || again.Created {
		t.Fatalf("existing db must not require default owner: %+v %v", again, err)
	}
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
	if task.VerifiedAt != nil {
		t.Fatal("create must not stamp verified_at")
	}
	again, replay, err := a.Create(ctx, tok, "markedo", "meta", app.CreateInput{
		Title:          "First",
		IdempotencyKey: "k1",
	})
	if err != nil || !replay || again.Handle != "T-001" {
		t.Fatalf("idempotency: %+v replay=%v err=%v", again, replay, err)
	}
	if _, _, err := a.Create(ctx, tok, "markedo", "meta", app.CreateInput{Title: "No key"}); err == nil {
		t.Fatal("create without idempotency_key")
	}
	second, _, err := a.Create(ctx, tok, "markedo", "meta", app.CreateInput{Title: "Second", IdempotencyKey: "k2"})
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

func TestCheckMustTickBeforeClose(t *testing.T) {
	a, tok := boot(t)
	ctx := context.Background()
	task, _, err := a.Create(ctx, tok, "markedo", "meta", app.CreateInput{
		Title:          "With checks",
		IdempotencyKey: "chk-1",
		Checks:         []string{"API", "HTML"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.Close(ctx, tok, "markedo", "meta", task.Handle, "nope"); err == nil {
		t.Fatal("close with unticked checks")
	}
	ticked, err := a.SetCheck(ctx, tok, "markedo", "meta", task.Handle, 1, "", true)
	if err != nil || !ticked.Checks[0].Done || ticked.Checks[1].Done {
		t.Fatalf("tick 1: %v %+v", err, ticked.Checks)
	}
	if _, err := a.SetCheck(ctx, tok, "markedo", "meta", task.Handle, 0, "HTML", true); err != nil {
		t.Fatal(err)
	}
	closed, err := a.Close(ctx, tok, "markedo", "meta", task.Handle, "both ticked")
	if err != nil || closed.Phase != types.PhaseDONE {
		t.Fatalf("close %v %+v", err, closed)
	}
}

func TestDeferralPolicy(t *testing.T) {
	a, tok := boot(t)
	ctx := context.Background()
	if _, _, err := a.Create(ctx, tok, "markedo", "meta", app.CreateInput{Title: "Slippery", IdempotencyKey: "def-1"}); err != nil {
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
	forced, err := a.SetPhase(ctx, tok, "markedo", "meta", "T-001", app.PhaseInput{Phase: "PARKED", Reason: "park it", Force: true})
	if err != nil || forced.Phase != types.PhasePARKED {
		t.Fatalf("force %v %+v", err, forced)
	}
}

func TestNextSkipsClaimed(t *testing.T) {
	a, tok := boot(t)
	ctx := context.Background()
	if _, _, err := a.Create(ctx, tok, "markedo", "meta", app.CreateInput{Title: "A", IdempotencyKey: "n1"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := a.Create(ctx, tok, "markedo", "meta", app.CreateInput{Title: "B", IdempotencyKey: "n2"}); err != nil {
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

func TestProvisionLedgersAndTokens(t *testing.T) {
	a, tok := boot(t)
	ctx := context.Background()
	if err := a.Store.SetMaxLedgers(ctx, tok.OwnerID, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := a.CreateLedger(ctx, tok, "markedo", app.CreateLedgerInput{Slug: "iq"}); err == nil {
		t.Fatal("expected cap")
	}
	if err := a.Store.SetMaxLedgers(ctx, tok.OwnerID, 2); err != nil {
		t.Fatal(err)
	}
	iq, err := a.CreateLedger(ctx, tok, "markedo", app.CreateLedgerInput{Slug: "iq", Title: "iQ"})
	if err != nil || iq.Slug != "iq" {
		t.Fatalf("create ledger %v %+v", err, iq)
	}
	if _, err := a.CreateLedger(ctx, tok, "markedo", app.CreateLedgerInput{Slug: "iq"}); err == nil {
		t.Fatal("expected duplicate")
	}
	issued, err := a.CreateToken(ctx, tok, "markedo", app.CreateTokenInput{Actor: "batty", Ledger: "iq", Role: "write"})
	if err != nil || issued.Plain == "" || issued.Token.LedgerSlug != "iq" {
		t.Fatalf("token %v %+v", err, issued)
	}
	batty, err := a.Auth(ctx, issued.Plain)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := a.Create(ctx, batty, "markedo", "iq", app.CreateInput{Title: "From iq", IdempotencyKey: "iq-1"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := a.Create(ctx, batty, "markedo", "meta", app.CreateInput{Title: "Nope", IdempotencyKey: "nope"}); err == nil {
		t.Fatal("ledger-bound token must not write another ledger")
	}
	if _, err := a.CreateLedger(ctx, batty, "markedo", app.CreateLedgerInput{Slug: "dispatch"}); err == nil {
		t.Fatal("write role must not create ledgers")
	}
	ownerTok, err := a.CreateToken(ctx, tok, "markedo", app.CreateTokenInput{Actor: "maria", Role: "admin"})
	if err != nil || ownerTok.Token.LedgerID != "" {
		t.Fatalf("owner token %v %+v", err, ownerTok)
	}
	wide, err := a.Auth(ctx, ownerTok.Plain)
	if err != nil {
		t.Fatal(err)
	}
	ledgers, err := a.ListLedgers(ctx, wide, "markedo")
	if err != nil || len(ledgers) != 2 {
		t.Fatalf("list %d %v", len(ledgers), err)
	}
}

func TestCreateLedgerMintsProjectTokenForOwnerAdmin(t *testing.T) {
	a, tok := boot(t)
	a.PublicURL = "https://task-ledger.example"
	ctx := context.Background()
	if err := a.Store.SetMaxLedgers(ctx, tok.OwnerID, 2); err != nil {
		t.Fatal(err)
	}
	l, err := a.CreateLedger(ctx, tok, "markedo", app.CreateLedgerInput{Slug: "jobs"})
	if err != nil {
		t.Fatal(err)
	}
	if got := app.ProjectActor(tok, ""); got != tok.Actor {
		t.Fatalf("project actor %q", got)
	}
	issued, err := a.MintProjectWrite(ctx, tok, "markedo", l.Slug, app.ProjectActor(tok, ""))
	if err != nil || issued == nil || issued.Token.LedgerSlug != "jobs" || issued.Token.Role != types.RoleWrite {
		t.Fatalf("mint %+v %v", issued, err)
	}
	view := a.LedgerCreatedView(l, issued)
	if view["token"] != issued.Plain {
		t.Fatalf("view token %v", view["token"])
	}
	mcp, _ := view["mcp"].(map[string]any)
	servers, _ := mcp["mcpServers"].(map[string]any)
	proj, _ := servers["task-ledger-jobs"].(map[string]any)
	if proj["url"] != "https://task-ledger.example/mcp" {
		t.Fatalf("mcp url %v", proj["url"])
	}
	op := types.Token{Actor: "operator", Role: types.RoleOperator}
	if app.ProjectActor(op, "") != "" {
		t.Fatal("operator must not default an actor")
	}
	if issued, err := a.MintProjectWrite(ctx, tok, "markedo", l.Slug, ""); err != nil || issued != nil {
		t.Fatalf("empty actor %v %v", issued, err)
	}
}
