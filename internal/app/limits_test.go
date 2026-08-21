package app_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/markedo-org/ledger/internal/app"
	"github.com/markedo-org/ledger/internal/store"
	"github.com/markedo-org/ledger/internal/types"
)

func ledgerForLimits(t *testing.T) (*app.App, types.Token) {
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

// Nothing bounded any of this, so one authenticated write token could put a
// hundred megabyte title in SQLite and leave the board rendering it.
func TestOversizedTextIsRefused(t *testing.T) {
	a, tok := ledgerForLimits(t)
	ctx := context.Background()
	long := func(n int) string { return strings.Repeat("a", n) }

	cases := []struct {
		name string
		in   app.CreateInput
	}{
		{"title", app.CreateInput{Title: long(app.MaxTitle + 1), IdempotencyKey: "k1"}},
		{"body", app.CreateInput{Title: "ok", Body: long(app.MaxBody + 1), IdempotencyKey: "k2"}},
		{"ref", app.CreateInput{Title: "ok", Ref: long(app.MaxRef + 1), IdempotencyKey: "k3"}},
		{"idempotency key", app.CreateInput{Title: "ok", IdempotencyKey: long(app.MaxIdempotencyKey + 1)}},
		{"check body", app.CreateInput{Title: "ok", IdempotencyKey: "k4", Checks: []string{long(app.MaxCheckBody + 1)}}},
		{"check count", app.CreateInput{Title: "ok", IdempotencyKey: "k5", Checks: make([]string, app.MaxChecksPerTask+1)}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, _, err := a.Create(ctx, tok, "markedo", "meta", c.in); err == nil {
				t.Fatalf("an oversized %s was accepted", c.name)
			}
		})
	}
}

// The limits are generous on purpose: a real task must never meet one.
func TestOrdinaryTextGoesThrough(t *testing.T) {
	a, tok := ledgerForLimits(t)
	ctx := context.Background()

	task, _, err := a.Create(ctx, tok, "markedo", "meta", app.CreateInput{
		Title:          "Give the deploy a gate",
		Body:           strings.Repeat("A paragraph about why. ", 200),
		IdempotencyKey: "ordinary",
		Checks:         []string{"one", "two"},
	})
	if err != nil {
		t.Fatalf("an ordinary task was refused: %v", err)
	}
	if _, err := a.AddNote(ctx, tok, "markedo", "meta", task.Handle, strings.Repeat("note. ", 100)); err != nil {
		t.Fatalf("an ordinary note was refused: %v", err)
	}
}

func TestOversizedNoteEvidenceAndReasonAreRefused(t *testing.T) {
	a, tok := ledgerForLimits(t)
	ctx := context.Background()
	long := func(n int) string { return strings.Repeat("a", n) }

	task, _, err := a.Create(ctx, tok, "markedo", "meta", app.CreateInput{
		Title: "Anything", IdempotencyKey: "limits", Phase: "NOW",
	})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := a.AddNote(ctx, tok, "markedo", "meta", task.Handle, long(app.MaxNoteBody+1)); err == nil {
		t.Fatal("an oversized note was accepted")
	}
	if _, err := a.Close(ctx, tok, "markedo", "meta", task.Handle, long(app.MaxEvidence+1), ""); err == nil {
		t.Fatal("oversized evidence was accepted")
	}
	if _, err := a.SetPhase(ctx, tok, "markedo", "meta", task.Handle, app.PhaseInput{
		Phase: "LATER", Reason: long(app.MaxReason + 1),
	}); err == nil {
		t.Fatal("an oversized reason was accepted")
	}
}

// A board longer than one response says so, rather than quietly stopping short
// and letting an agent plan work off a list with a hole in it.
func TestALongBoardSaysItWasCut(t *testing.T) {
	a, tok := ledgerForLimits(t)
	ctx := context.Background()

	for i := 0; i < app.MaxListRows+5; i++ {
		if _, _, err := a.Create(ctx, tok, "markedo", "meta", app.CreateInput{
			Title:          "task",
			Phase:          "NOW",
			IdempotencyKey: "bulk-" + strings.Repeat("x", i%3) + itoa(i),
		}); err != nil {
			t.Fatal(err)
		}
	}

	_, tasks, truncated, err := a.List(ctx, tok, "markedo", "meta", app.ListQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != app.MaxListRows {
		t.Fatalf("got %d tasks, want the ceiling of %d", len(tasks), app.MaxListRows)
	}
	if !truncated {
		t.Fatal("the board was cut and the response did not say so")
	}
}

func TestAShortBoardIsNotMarkedCut(t *testing.T) {
	a, tok := ledgerForLimits(t)
	ctx := context.Background()

	if _, _, err := a.Create(ctx, tok, "markedo", "meta", app.CreateInput{
		Title: "one", IdempotencyKey: "short",
	}); err != nil {
		t.Fatal(err)
	}
	_, tasks, truncated, err := a.List(ctx, tok, "markedo", "meta", app.ListQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 1 || truncated {
		t.Fatalf("got %d tasks, truncated=%v", len(tasks), truncated)
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
