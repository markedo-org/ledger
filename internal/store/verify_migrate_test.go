package store_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/markedo-org/ledger/internal/store"
	_ "modernc.org/sqlite"
)

// The columns behind "who closed this and who verified it" arrive by ALTER on
// databases that already hold a customer's tasks, and every test that opens a
// fresh store gets them from the schema instead, so the upgrade path is the
// one thing none of them exercise. This rehearses it: build a database with a
// task in it, take the columns away so it looks like the old one, and open it
// again the way a deploy would.
func TestTasksSurviveGainingTheVerifierColumns(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.db")

	s, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	plain, err := store.NewToken()
	if err != nil {
		t.Fatal(err)
	}
	boot, err := s.Bootstrap(context.Background(), "markedo", "meta", "maria", plain)
	if err != nil {
		t.Fatal(err)
	}
	task, _, err := s.CreateTask(context.Background(), store.CreateTaskParams{
		LedgerID:       boot.Ledger.ID,
		Prefix:         boot.Series.Prefix,
		Actor:          "maria",
		Title:          "Work that predates the columns",
		Phase:          "NOW",
		IdempotencyKey: "old-work",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	for _, col := range []string{"verified_by", "closed_by"} {
		if _, err := db.Exec(`ALTER TABLE tasks DROP COLUMN ` + col); err != nil {
			t.Fatalf("could not undo %s to make an old database: %v", col, err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	s2, err := store.Open(path)
	if err != nil {
		t.Fatalf("opening a database from before the columns existed: %v", err)
	}
	defer func() { _ = s2.Close() }()

	// Reading the task is where a missing column would surface, because the
	// select names every one of them.
	got, err := s2.GetTask(context.Background(), task.LedgerID, task.Handle)
	if err != nil {
		t.Fatalf("a task written before the upgrade is unreadable after it: %v", err)
	}
	if got.Title != task.Title {
		t.Fatalf("got %q, want %q", got.Title, task.Title)
	}
	if got.VerifiedBy != "" || got.ClosedBy != "" {
		t.Fatalf("old work should carry no verifier, got closed_by %q verified_by %q", got.ClosedBy, got.VerifiedBy)
	}
}
