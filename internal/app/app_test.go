package app_test

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
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

	if held.ClaimID == "" || !strings.HasPrefix(held.ClaimID, "clm_") {
		t.Fatalf("claim_id %q", held.ClaimID)
	}
	got, err := a.Get(ctx, tok, "markedo", "meta", "T-001")
	if err != nil || got.ClaimID != "" {
		t.Fatalf("get must omit claim_id: %+v %v", got, err)
	}

	if _, err = a.Close(ctx, tok, "markedo", "meta", "T-001", "", held.ClaimID); err == nil {
		t.Fatal("close without evidence")
	}
	closed, err := a.Close(ctx, tok, "markedo", "meta", "T-001", "tests pass", held.ClaimID)
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
	if _, err := a.Close(ctx, tok, "markedo", "meta", task.Handle, "nope", ""); err == nil {
		t.Fatal("close with unticked checks")
	}
	ticked, err := a.SetCheck(ctx, tok, "markedo", "meta", task.Handle, 1, "", true)
	if err != nil || !ticked.Checks[0].Done || ticked.Checks[1].Done {
		t.Fatalf("tick 1: %v %+v", err, ticked.Checks)
	}
	if _, err := a.SetCheck(ctx, tok, "markedo", "meta", task.Handle, 0, "HTML", true); err != nil {
		t.Fatal(err)
	}
	closed, err := a.Close(ctx, tok, "markedo", "meta", task.Handle, "both ticked", "")
	if err != nil || closed.Phase != types.PhaseDONE {
		t.Fatalf("close %v %+v", err, closed)
	}
}

func TestTagsCreateListReplace(t *testing.T) {
	a, tok := boot(t)
	ctx := context.Background()
	if _, _, err := a.Create(ctx, tok, "markedo", "meta", app.CreateInput{
		Title: "No tags", IdempotencyKey: "tag-0",
	}); err != nil {
		t.Fatal(err)
	}
	task, _, err := a.Create(ctx, tok, "markedo", "meta", app.CreateInput{
		Title:          "Ledger work",
		IdempotencyKey: "tag-1",
		Tags:           []string{"Ledger", "site", "ledger"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(task.Tags) != 2 || task.Tags[0] != "ledger" || task.Tags[1] != "site" {
		t.Fatalf("create tags %v", task.Tags)
	}
	if _, _, err := a.Create(ctx, tok, "markedo", "meta", app.CreateInput{
		Title:          "Too many",
		IdempotencyKey: "tag-4",
		Tags:           []string{"a", "b", "c", "d"},
	}); err == nil {
		t.Fatal("want max 3")
	}
	_, listed, err := a.List(ctx, tok, "markedo", "meta", app.ListQuery{Tag: "ledger"})
	if err != nil || len(listed) != 1 || listed[0].Handle != task.Handle {
		t.Fatalf("filter %v %+v", err, listed)
	}
	cleared, err := a.SetTags(ctx, tok, "markedo", "meta", task.Handle, nil)
	if err != nil || len(cleared.Tags) != 0 {
		t.Fatalf("clear %v %v", err, cleared.Tags)
	}
	got, err := a.ListLedgerTags(ctx, "markedo", "meta")
	if err != nil || len(got) != 0 {
		t.Fatalf("ledger tags after clear %v %v", err, got)
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

func TestClaimIDIsolatesSameActorSessions(t *testing.T) {
	a, tok := boot(t)
	ctx := context.Background()
	if _, _, err := a.Create(ctx, tok, "markedo", "meta", app.CreateInput{Title: "Lease", IdempotencyKey: "clm-1"}); err != nil {
		t.Fatal(err)
	}
	held, err := a.Claim(ctx, tok, "markedo", "meta", "T-001", app.ClaimInput{TTL: time.Minute})
	if err != nil || !strings.HasPrefix(held.ClaimID, "clm_") {
		t.Fatalf("claim %v %+v", err, held)
	}
	if _, err := a.Claim(ctx, tok, "markedo", "meta", "T-001", app.ClaimInput{TTL: time.Minute}); !errors.Is(err, app.ErrConflict) {
		t.Fatalf("re-claim without id: %v", err)
	}
	again, err := a.Claim(ctx, tok, "markedo", "meta", "T-001", app.ClaimInput{TTL: time.Minute, ClaimID: held.ClaimID})
	if err != nil || again.ClaimID != held.ClaimID {
		t.Fatalf("re-claim with id: %v %q", err, again.ClaimID)
	}
	if _, err := a.Heartbeat(ctx, tok, "markedo", "meta", "T-001", 0, ""); !errors.Is(err, app.ErrConflict) {
		t.Fatalf("heartbeat without id: %v", err)
	}
	beat, err := a.Heartbeat(ctx, tok, "markedo", "meta", "T-001", 0, held.ClaimID)
	if err != nil || beat.ClaimID != held.ClaimID {
		t.Fatalf("heartbeat %v %q", err, beat.ClaimID)
	}
	if _, err := a.SetPhase(ctx, tok, "markedo", "meta", "T-001", app.PhaseInput{Phase: "NEXT", Reason: "later"}); !errors.Is(err, app.ErrConflict) {
		t.Fatalf("phase without id: %v", err)
	}
	if _, err := a.Close(ctx, tok, "markedo", "meta", "T-001", "nope", ""); !errors.Is(err, app.ErrConflict) {
		t.Fatalf("close without id: %v", err)
	}
	if _, err := a.AddNote(ctx, tok, "markedo", "meta", "T-001", "open notes"); err != nil {
		t.Fatalf("note without id: %v", err)
	}
	got, err := a.Get(ctx, tok, "markedo", "meta", "T-001")
	if err != nil || got.ClaimID != "" || got.ClaimSecretHash == "" {
		t.Fatalf("get must hide plaintext and keep hash: %+v %v", got, err)
	}

	other, err := a.CreateToken(ctx, tok, "markedo", app.CreateTokenInput{Actor: "batty", Role: "write"})
	if err != nil {
		t.Fatal(err)
	}
	batty, err := a.Auth(ctx, other.Plain)
	if err != nil {
		t.Fatal(err)
	}
	stolen, err := a.Claim(ctx, batty, "markedo", "meta", "T-001", app.ClaimInput{Steal: true, Reason: "take it"})
	if err != nil || stolen.ClaimID == "" || stolen.ClaimID == held.ClaimID {
		t.Fatalf("steal %v %q", err, stolen.ClaimID)
	}
	if _, err := a.Heartbeat(ctx, tok, "markedo", "meta", "T-001", 0, held.ClaimID); !errors.Is(err, app.ErrConflict) {
		t.Fatalf("old id after steal: %v", err)
	}
	if _, err := a.Release(ctx, batty, "markedo", "meta", "T-001", stolen.ClaimID); err != nil {
		t.Fatal(err)
	}

	legacy, err := a.Claim(ctx, tok, "markedo", "meta", "T-001", app.ClaimInput{TTL: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	empty := ""
	if _, err := a.Store.Mutate(ctx, legacy.ID, tok.Actor, "claim", map[string]string{}, store.MutateTask{ClaimSecretHash: &empty}); err != nil {
		t.Fatal(err)
	}
	upgraded, err := a.Claim(ctx, tok, "markedo", "meta", "T-001", app.ClaimInput{TTL: time.Minute})
	if err != nil || upgraded.ClaimID == "" {
		t.Fatalf("legacy re-claim %v %q", err, upgraded.ClaimID)
	}
}
