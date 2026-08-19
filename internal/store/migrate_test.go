package store_test

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"github.com/markedo-org/ledger/internal/store"
	_ "modernc.org/sqlite"
)

// A database written before 0.17.0 has idempotency keyed globally. Opening it
// must rebuild the table on (ledger_id, key) without losing a row.
func TestMigrateIdempotencyScopeRebuildsOldTable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.sqlite")
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE idempotency (
  key TEXT PRIMARY KEY,
  ledger_id TEXT NOT NULL,
  task_id TEXT NOT NULL,
  created_at TEXT NOT NULL
)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO idempotency(key, ledger_id, task_id, created_at)
		VALUES ('setup-1', 'ledger-a', 'task-a', '2026-01-01T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	s, err := store.Open(path)
	if err != nil {
		t.Fatalf("open with the old idempotency table: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	db, err = sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var ddl string
	if err := db.QueryRow(`SELECT sql FROM sqlite_master WHERE type = 'table' AND name = 'idempotency'`).Scan(&ddl); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.ReplaceAll(ddl, " ", ""), "PRIMARYKEY(ledger_id,key)") {
		t.Fatalf("idempotency was not rebuilt: %s", ddl)
	}
	var taskID string
	if err := db.QueryRow(`SELECT task_id FROM idempotency WHERE key = 'setup-1'`).Scan(&taskID); err != nil {
		t.Fatalf("row lost in migration: %v", err)
	}
	if taskID != "task-a" {
		t.Fatalf("task_id %q", taskID)
	}
	if _, err := db.Exec(`INSERT INTO idempotency(key, ledger_id, task_id, created_at)
		VALUES ('setup-1', 'ledger-b', 'task-b', '2026-01-01T00:00:00Z')`); err != nil {
		t.Fatalf("same key on a second ledger still refused: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO idempotency(key, ledger_id, task_id, created_at)
		VALUES ('setup-1', 'ledger-a', 'task-c', '2026-01-01T00:00:00Z')`); err == nil {
		t.Fatal("the same key on the same ledger must still collide")
	}
}

// Opening a database twice must be a no-op the second time.
func TestMigrateIdempotencyScopeIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fresh.sqlite")
	for i := 0; i < 2; i++ {
		s, err := store.Open(path)
		if err != nil {
			t.Fatalf("open %d: %v", i, err)
		}
		if err := s.Close(); err != nil {
			t.Fatal(err)
		}
	}
}
