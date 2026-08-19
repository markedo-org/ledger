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

// A database written before token revocation has idx_tokens_email unique over
// every token. Opening it must narrow the index to live tokens, otherwise a
// revoked token keeps its address and the replacement can never carry it.
func TestMigrateTokenEmailIndexNarrowsToLiveTokens(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old-tokens.sqlite")
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	for _, stmt := range []string{
		`CREATE TABLE owners (id TEXT PRIMARY KEY, slug TEXT NOT NULL UNIQUE, max_ledgers INTEGER NOT NULL DEFAULT 1, created_at TEXT NOT NULL)`,
		`CREATE TABLE ledgers (id TEXT PRIMARY KEY, owner_id TEXT NOT NULL, slug TEXT NOT NULL, title TEXT NOT NULL DEFAULT '', created_at TEXT NOT NULL)`,
		`CREATE TABLE tokens (
  id TEXT PRIMARY KEY,
  token_hash TEXT NOT NULL UNIQUE,
  actor TEXT NOT NULL,
  owner_id TEXT NOT NULL REFERENCES owners(id),
  ledger_id TEXT REFERENCES ledgers(id),
  role TEXT NOT NULL DEFAULT 'write',
  email TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL
)`,
		`CREATE UNIQUE INDEX idx_tokens_email ON tokens(email) WHERE email != ''`,
		`INSERT INTO owners(id, slug, max_ledgers, created_at) VALUES ('o1','markedo',1,'2026-01-01T00:00:00Z')`,
		`INSERT INTO tokens(id, token_hash, actor, owner_id, role, email, created_at)
			VALUES ('t1','hash-1','maria','o1','admin','lg@example.com','2026-01-01T00:00:00Z')`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("seed %s: %v", stmt, err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	s, err := store.Open(path)
	if err != nil {
		t.Fatalf("open a pre-revocation database: %v", err)
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
	if err := db.QueryRow(`SELECT sql FROM sqlite_master WHERE type = 'index' AND name = 'idx_tokens_email'`).Scan(&ddl); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.ReplaceAll(ddl, " ", ""), "revoked_at=''") {
		t.Fatalf("index was not narrowed: %s", ddl)
	}
	var revoked string
	if err := db.QueryRow(`SELECT revoked_at FROM tokens WHERE id = 't1'`).Scan(&revoked); err != nil {
		t.Fatalf("existing token lost in migration: %v", err)
	}
	if revoked != "" {
		t.Fatalf("an existing token came back revoked: %q", revoked)
	}
	if _, err := db.Exec(`INSERT INTO tokens(id, token_hash, actor, owner_id, role, email, created_at, revoked_at)
		VALUES ('t2','hash-2','maria','o1','admin','lg@example.com','2026-01-02T00:00:00Z','')`); err == nil {
		t.Fatal("two live tokens on one address must still collide")
	}
	if _, err := db.Exec(`UPDATE tokens SET revoked_at = '2026-01-02T00:00:00Z' WHERE id = 't1'`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO tokens(id, token_hash, actor, owner_id, role, email, created_at, revoked_at)
		VALUES ('t2','hash-2','maria','o1','admin','lg@example.com','2026-01-02T00:00:00Z','')`); err != nil {
		t.Fatalf("the address should be free once the old token is revoked: %v", err)
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
