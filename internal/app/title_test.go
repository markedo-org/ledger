package app_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/markedo-org/ledger/internal/app"
)

// A title was written once at create and stood for ever. create_task is the one
// call an agent makes with nothing to review first, and a stale import put the
// wrong IDs into a whole board of titles, repairable only by editing SQLite by
// hand on the server.
func TestATitleCanBeCorrected(t *testing.T) {
	a, tok := ledgerForLimits(t)
	ctx := context.Background()

	task, _, err := a.Create(ctx, tok, "markedo", "meta", app.CreateInput{
		Title:          "T-042 Wire the importer",
		IdempotencyKey: "title-fix",
	})
	if err != nil {
		t.Fatal(err)
	}

	fixed, err := a.SetTitle(ctx, tok, "markedo", "meta", task.Handle, "Wire the importer", "")
	if err != nil {
		t.Fatal(err)
	}
	if fixed.Title != "Wire the importer" {
		t.Fatalf("title is %q", fixed.Title)
	}
	if fixed.Handle != task.Handle {
		t.Fatalf("the handle changed: %s -> %s", task.Handle, fixed.Handle)
	}
}

// The correction is a change on the record, not a quiet overwrite, so the
// title it replaced stays readable.
func TestTheOldTitleStaysInTheLog(t *testing.T) {
	a, tok := ledgerForLimits(t)
	ctx := context.Background()

	task, _, err := a.Create(ctx, tok, "markedo", "meta", app.CreateInput{
		Title:          "T-042 Wire the importer",
		IdempotencyKey: "title-log",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.SetTitle(ctx, tok, "markedo", "meta", task.Handle, "Wire the importer", ""); err != nil {
		t.Fatal(err)
	}

	events, err := a.Store.Events(ctx, task.LedgerID, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, e := range events {
		if e.Kind == "title" && strings.Contains(e.Payload, "T-042 Wire the importer") {
			found = true
		}
	}
	if !found {
		t.Fatalf("the title it replaced is not in the log: %+v", events)
	}
}

func TestAnEmptyOrOversizedTitleIsRefused(t *testing.T) {
	a, tok := ledgerForLimits(t)
	ctx := context.Background()

	task, _, err := a.Create(ctx, tok, "markedo", "meta", app.CreateInput{
		Title: "Something", IdempotencyKey: "title-bad",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.SetTitle(ctx, tok, "markedo", "meta", task.Handle, "   ", ""); err == nil {
		t.Fatal("an empty title was accepted")
	}
	if _, err := a.SetTitle(ctx, tok, "markedo", "meta", task.Handle, strings.Repeat("a", app.MaxTitle+1), ""); err == nil {
		t.Fatal("an oversized title was accepted")
	}
}

// Correcting a title is a write like any other, so a live lease governs it.
func TestCorrectingATitleRespectsTheLease(t *testing.T) {
	a, tok := ledgerForLimits(t)
	ctx := context.Background()

	task, _, err := a.Create(ctx, tok, "markedo", "meta", app.CreateInput{
		Title: "Something", IdempotencyKey: "title-lease", Phase: "NOW",
	})
	if err != nil {
		t.Fatal(err)
	}
	held, err := a.Claim(ctx, tok, "markedo", "meta", task.Handle, app.ClaimInput{TTL: 30 * time.Minute})
	if err != nil {
		t.Fatal(err)
	}

	other := tok
	other.Actor = "petra"
	if _, err := a.SetTitle(ctx, other, "markedo", "meta", task.Handle, "Taken", ""); err == nil {
		t.Fatal("a title changed under someone else's live lease without the claim_id")
	}
	if _, err := a.SetTitle(ctx, tok, "markedo", "meta", task.Handle, "Corrected", held.ClaimID); err != nil {
		t.Fatalf("the lease holder was refused: %v", err)
	}
}
