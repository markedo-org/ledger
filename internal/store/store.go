package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/markedo-org/ledger/internal/types"
	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
	mu sync.Mutex
}

func Open(path string) (*Store, error) {
	dsn := path
	if path != ":memory:" {
		dsn = "file:" + path + "?_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)&_pragma=busy_timeout(5000)"
	} else {
		dsn = "file:ledgermem?mode=memory&cache=shared&_pragma=foreign_keys(ON)"
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(schema); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("schema: %w", err)
	}
	if err := migrate(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	if _, err := db.Exec(`INSERT OR IGNORE INTO meta(key, value) VALUES ('schema', '1')`); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func migrate(db *sql.DB) error {
	for _, stmt := range []string{
		`ALTER TABLE sessions ADD COLUMN actor TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE sessions ADD COLUMN owner_slug TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE sessions ADD COLUMN ledger_slug TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE sessions ADD COLUMN role TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE tokens ADD COLUMN email TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE tokens ADD COLUMN revoked_at TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE sessions ADD COLUMN token_id TEXT NOT NULL DEFAULT ''`,
		`CREATE TABLE IF NOT EXISTS magic_links (
  id TEXT PRIMARY KEY,
  code_hash TEXT NOT NULL UNIQUE,
  token_id TEXT NOT NULL REFERENCES tokens(id),
  expires_at TEXT NOT NULL,
  created_at TEXT NOT NULL
)`,
		`CREATE TABLE IF NOT EXISTS review_links (
  id TEXT PRIMARY KEY,
  code_hash TEXT NOT NULL UNIQUE,
  token_id TEXT NOT NULL REFERENCES tokens(id),
  expires_at TEXT NOT NULL,
  created_at TEXT NOT NULL
)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_tokens_email ON tokens(email) WHERE email != '' AND revoked_at = ''`,
		`ALTER TABLE ledgers ADD COLUMN archive_done_after_days INTEGER`,
		`ALTER TABLE ledgers ADD COLUMN purge_done_after_days INTEGER`,
		`ALTER TABLE tasks ADD COLUMN claim_id_hash TEXT`,
		`ALTER TABLE tasks ADD COLUMN verified_by TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE tasks ADD COLUMN closed_by TEXT NOT NULL DEFAULT ''`,
		`CREATE TABLE IF NOT EXISTS task_tags (
  task_id TEXT NOT NULL REFERENCES tasks(id),
  slug TEXT NOT NULL,
  rank INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (task_id, slug)
)`,
	} {
		if _, err := db.Exec(stmt); err != nil && !strings.Contains(strings.ToLower(err.Error()), "duplicate column") {
			return fmt.Errorf("migrate: %s: %w", stmt, err)
		}
	}
	if err := migrateTokenEmailIndex(db); err != nil {
		return err
	}
	return migrateIdempotencyScope(db)
}

// The email index was unique across every token, so a revoked token kept its
// address hostage and the replacement could never be minted with it. Revoked
// rows are excluded, which is what makes rotating a token with an email on it
// possible at all.
func migrateTokenEmailIndex(db *sql.DB) error {
	var ddl string
	err := db.QueryRow(`SELECT sql FROM sqlite_master WHERE type = 'index' AND name = 'idx_tokens_email'`).Scan(&ddl)
	if err == sql.ErrNoRows {
		ddl = ""
	} else if err != nil {
		return err
	}
	if strings.Contains(strings.ReplaceAll(ddl, " ", ""), "revoked_at=''") {
		return nil
	}
	for _, stmt := range []string{
		`DROP INDEX IF EXISTS idx_tokens_email`,
		`CREATE UNIQUE INDEX idx_tokens_email ON tokens(email) WHERE email != '' AND revoked_at = ''`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("migrate token email index: %w", err)
		}
	}
	return nil
}

// The first schema made an idempotency key globally unique, but the replay
// lookup has always been scoped to one ledger. Two ledgers under one owner
// using the same key therefore missed the lookup and then collided on the
// primary key. Rebuild the table on (ledger_id, key).
func migrateIdempotencyScope(db *sql.DB) error {
	var ddl string
	err := db.QueryRow(`SELECT sql FROM sqlite_master WHERE type = 'table' AND name = 'idempotency'`).Scan(&ddl)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}
	if strings.Contains(strings.ReplaceAll(ddl, " ", ""), "PRIMARYKEY(ledger_id,key)") {
		return nil
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for _, stmt := range []string{
		`DROP TABLE IF EXISTS idempotency_rebuild`,
		`CREATE TABLE idempotency_rebuild (
  key TEXT NOT NULL,
  ledger_id TEXT NOT NULL,
  task_id TEXT NOT NULL,
  created_at TEXT NOT NULL,
  PRIMARY KEY (ledger_id, key)
)`,
		`INSERT OR IGNORE INTO idempotency_rebuild(key, ledger_id, task_id, created_at)
			SELECT key, ledger_id, task_id, created_at FROM idempotency`,
		`DROP TABLE idempotency`,
		`ALTER TABLE idempotency_rebuild RENAME TO idempotency`,
	} {
		if _, err := tx.Exec(stmt); err != nil {
			return fmt.Errorf("migrate idempotency scope: %w", err)
		}
	}
	return tx.Commit()
}

func now() time.Time { return time.Now().UTC().Truncate(time.Second) }

func fmtTime(t time.Time) string { return t.UTC().Format(time.RFC3339) }

func parseTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}
	}
	return t
}

func parseTimePtr(s sql.NullString) *time.Time {
	if !s.Valid || s.String == "" {
		return nil
	}
	t := parseTime(s.String)
	return &t
}

type BootstrapResult struct {
	Owner   types.Owner
	Ledger  types.Ledger
	Series  types.Series
	TokenID string
	Token   string
	Created bool
}

func (s *Store) Bootstrap(ctx context.Context, ownerSlug, ledgerSlug, actor, tokenPlain string) (BootstrapResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var out BootstrapResult
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		var n int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM owners`).Scan(&n); err != nil {
			return err
		}
		if n > 0 {
			out.Created = false
			return nil
		}
		t := now()
		oid := uuid.NewString()
		lid := uuid.NewString()
		sid := uuid.NewString()
		tid := uuid.NewString()
		if _, err := tx.ExecContext(ctx, `INSERT INTO owners(id, slug, max_ledgers, created_at) VALUES (?,?,?,?)`,
			oid, ownerSlug, 8, fmtTime(t)); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO ledgers(id, owner_id, slug, title, created_at) VALUES (?,?,?,?,?)`,
			lid, oid, ledgerSlug, ledgerSlug, fmtTime(t)); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO series(id, ledger_id, prefix, next_n) VALUES (?,?,?,?)`,
			sid, lid, "T", 1); err != nil {
			return err
		}
		hash := HashToken(tokenPlain)
		if _, err := tx.ExecContext(ctx, `INSERT INTO tokens(id, token_hash, actor, owner_id, ledger_id, role, created_at) VALUES (?,?,?,?,?,?,?)`,
			tid, hash, actor, oid, lid, "admin", fmtTime(t)); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO events(ledger_id, task_id, actor, kind, payload, created_at) VALUES (?,?,?,?,?,?)`,
			lid, nil, actor, "bootstrap", `{"owner":"`+ownerSlug+`","ledger":"`+ledgerSlug+`"}`, fmtTime(t)); err != nil {
			return err
		}
		out = BootstrapResult{
			Owner:   types.Owner{ID: oid, Slug: ownerSlug, MaxLedgers: 8, CreatedAt: t},
			Ledger:  types.Ledger{ID: lid, OwnerID: oid, OwnerSlug: ownerSlug, Slug: ledgerSlug, Title: ledgerSlug, CreatedAt: t},
			Series:  types.Series{ID: sid, LedgerID: lid, Prefix: "T", NextN: 1},
			TokenID: tid,
			Token:   tokenPlain,
			Created: true,
		}
		return nil
	})
	return out, err
}

func (s *Store) withTx(ctx context.Context, fn func(*sql.Tx) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

func (s *Store) LookupToken(ctx context.Context, tokenPlain string) (types.Token, error) {
	var t types.Token
	var ledger sql.NullString
	var ledgerSlug sql.NullString
	var created string
	err := s.db.QueryRowContext(ctx, `
		SELECT t.id, t.actor, t.owner_id, t.ledger_id, t.role, t.created_at, o.slug, l.slug
		FROM tokens t
		JOIN owners o ON o.id = t.owner_id
		LEFT JOIN ledgers l ON l.id = t.ledger_id
		WHERE t.token_hash = ? AND t.revoked_at = ''`, HashToken(tokenPlain)).
		Scan(&t.ID, &t.Actor, &t.OwnerID, &ledger, &t.Role, &created, &t.OwnerSlug, &ledgerSlug)
	if err != nil {
		return t, err
	}
	if ledger.Valid {
		t.LedgerID = ledger.String
	}
	if ledgerSlug.Valid {
		t.LedgerSlug = ledgerSlug.String
	}
	t.CreatedAt = parseTime(created)
	return t, nil
}

func (s *Store) ResolveLedger(ctx context.Context, ownerSlug, ledgerSlug string) (types.Ledger, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT `+ledgerCols+`
		FROM ledgers l JOIN owners o ON o.id = l.owner_id
		WHERE o.slug = ? AND l.slug = ?`, ownerSlug, ledgerSlug)
	return scanLedger(row)
}

func (s *Store) SeriesByPrefix(ctx context.Context, ledgerID, prefix string) (types.Series, error) {
	var ser types.Series
	err := s.db.QueryRowContext(ctx, `SELECT id, ledger_id, prefix, next_n FROM series WHERE ledger_id = ? AND prefix = ?`,
		ledgerID, prefix).Scan(&ser.ID, &ser.LedgerID, &ser.Prefix, &ser.NextN)
	return ser, err
}

type CreateTaskParams struct {
	LedgerID       string
	Prefix         string
	Title          string
	Body           string
	Phase          types.Phase
	Size           types.Size
	Ref            string
	Actor          string
	IdempotencyKey string
	Checks         []string
	Tags           []string
}

func (s *Store) CreateTask(ctx context.Context, p CreateTaskParams) (types.Task, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var task types.Task
	var replay bool
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		if p.IdempotencyKey != "" {
			var existing string
			err := tx.QueryRowContext(ctx, `SELECT task_id FROM idempotency WHERE key = ? AND ledger_id = ?`,
				p.IdempotencyKey, p.LedgerID).Scan(&existing)
			if err == nil {
				replay = true
				t, err := getTaskTx(ctx, tx, existing)
				if err != nil {
					return err
				}
				task = t
				return nil
			}
			if err != sql.ErrNoRows {
				return err
			}
		}

		var ser types.Series
		if err := tx.QueryRowContext(ctx, `SELECT id, ledger_id, prefix, next_n FROM series WHERE ledger_id = ? AND prefix = ?`,
			p.LedgerID, p.Prefix).Scan(&ser.ID, &ser.LedgerID, &ser.Prefix, &ser.NextN); err != nil {
			return fmt.Errorf("series %s: %w", p.Prefix, err)
		}
		n := ser.NextN
		if _, err := tx.ExecContext(ctx, `UPDATE series SET next_n = next_n + 1 WHERE id = ?`, ser.ID); err != nil {
			return err
		}
		t := now()
		id := uuid.NewString()
		handle := types.FormatHandle(ser.Prefix, n)
		var maxRank int
		_ = tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(rank), 0) FROM tasks WHERE ledger_id = ? AND phase = ?`,
			p.LedgerID, string(p.Phase)).Scan(&maxRank)
		if _, err := tx.ExecContext(ctx, `INSERT INTO tasks(
			id, ledger_id, series_id, n, handle, title, body, phase, size, rank, version, pushed,
			verified_at, since, ref, created_at, updated_at
		) VALUES (?,?,?,?,?,?,?,?,?,?,1,0,?,?,?,?,?)`,
			id, p.LedgerID, ser.ID, n, handle, p.Title, p.Body, string(p.Phase), string(p.Size), maxRank+10,
			nil, fmtTime(t), p.Ref, fmtTime(t), fmtTime(t)); err != nil {
			return err
		}
		for i, c := range p.Checks {
			if _, err := tx.ExecContext(ctx, `INSERT INTO checks(id, task_id, body, done, rank) VALUES (?,?,?,0,?)`,
				uuid.NewString(), id, c, i); err != nil {
				return err
			}
		}
		if err := replaceTagsTx(ctx, tx, id, p.Tags); err != nil {
			return err
		}
		payload, _ := json.Marshal(map[string]any{"handle": handle, "title": p.Title})
		if _, err := tx.ExecContext(ctx, `INSERT INTO events(ledger_id, task_id, actor, kind, payload, created_at) VALUES (?,?,?,?,?,?)`,
			p.LedgerID, id, p.Actor, "create", string(payload), fmtTime(t)); err != nil {
			return err
		}
		if p.IdempotencyKey != "" {
			if _, err := tx.ExecContext(ctx, `INSERT INTO idempotency(key, ledger_id, task_id, created_at) VALUES (?,?,?,?)`,
				p.IdempotencyKey, p.LedgerID, id, fmtTime(t)); err != nil {
				return err
			}
		}
		t2, err := getTaskTx(ctx, tx, id)
		if err != nil {
			return err
		}
		task = t2
		return nil
	})
	return task, replay, err
}

func (s *Store) GetTask(ctx context.Context, ledgerID, handle string) (types.Task, error) {
	prefix, n, err := types.ParseHandle(handle)
	if err != nil {
		return types.Task{}, err
	}
	canonical := types.FormatHandle(prefix, n)
	var id string
	err = s.db.QueryRowContext(ctx, `SELECT id FROM tasks WHERE ledger_id = ? AND handle = ?`, ledgerID, canonical).Scan(&id)
	if err != nil {
		return types.Task{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var task types.Task
	err = s.withTx(ctx, func(tx *sql.Tx) error {
		t, err := getTaskTx(ctx, tx, id)
		task = t
		return err
	})
	return task, err
}

func (s *Store) ListTasks(ctx context.Context, ledgerID string, q TaskList) ([]types.Task, error) {
	query := `SELECT id FROM tasks WHERE ledger_id = ?`
	args := []any{ledgerID}
	if q.DoneOnly {
		query += ` AND phase = 'DONE'`
	} else if q.ArchiveBefore != nil {
		query += ` AND (phase != 'DONE' OR closed_at IS NULL OR closed_at >= ?)`
		args = append(args, fmtTime(*q.ArchiveBefore))
	}
	if q.Tag != "" {
		query += ` AND EXISTS (SELECT 1 FROM task_tags g WHERE g.task_id = tasks.id AND g.slug = ?)`
		args = append(args, q.Tag)
	}
	query += ` ORDER BY
		CASE phase WHEN 'NOW' THEN 0 WHEN 'NEXT' THEN 1 WHEN 'LATER' THEN 2 WHEN 'GATED' THEN 3 WHEN 'PARKED' THEN 4 WHEN 'DONE' THEN 5 ELSE 9 END,
		rank ASC, n ASC`
	if q.Limit > 0 {
		query += ` LIMIT ?`
		args = append(args, q.Limit)
	}
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]types.Task, 0, len(ids))
	err = s.withTx(ctx, func(tx *sql.Tx) error {
		for _, id := range ids {
			t, err := getTaskTx(ctx, tx, id)
			if err != nil {
				return err
			}
			out = append(out, t)
		}
		return nil
	})
	return out, err
}

func getTaskTx(ctx context.Context, tx *sql.Tx, id string) (types.Task, error) {
	var t types.Task
	var phase, size string
	var verified, claimedUntil, closed, claimedBy, evidence, claimHash sql.NullString
	var since, created, updated string
	var signoff, third, decision, gateTime int
	err := tx.QueryRowContext(ctx, `SELECT id, ledger_id, series_id, n, handle, title, body, phase, size, rank, version, pushed,
		verified_at, verified_by, closed_by, since, ref, gate_signoff, gate_third, gate_decision, gate_time, claimed_by, claimed_until, claim_id_hash, evidence, closed_at, created_at, updated_at
		FROM tasks WHERE id = ?`, id).Scan(
		&t.ID, &t.LedgerID, &t.SeriesID, &t.N, &t.Handle, &t.Title, &t.Body, &phase, &size, &t.Rank, &t.Version, &t.Pushed,
		&verified, &t.VerifiedBy, &t.ClosedBy, &since, &t.Ref, &signoff, &third, &decision, &gateTime, &claimedBy, &claimedUntil, &claimHash, &evidence, &closed, &created, &updated)
	if err != nil {
		return t, err
	}
	t.Phase = types.Phase(phase)
	t.Size = types.Size(size)
	t.GateSignoff = signoff != 0
	t.GateThird = third != 0
	t.GateDecision = decision != 0
	t.GateTime = gateTime != 0
	t.VerifiedAt = parseTimePtr(verified)
	t.ClaimedUntil = parseTimePtr(claimedUntil)
	t.ClosedAt = parseTimePtr(closed)
	if claimedBy.Valid {
		t.ClaimedBy = claimedBy.String
	}
	if claimHash.Valid {
		t.ClaimSecretHash = claimHash.String
	}
	if evidence.Valid {
		t.Evidence = evidence.String
	}
	t.Since = parseTime(since)
	t.CreatedAt = parseTime(created)
	t.UpdatedAt = parseTime(updated)

	nRows, err := tx.QueryContext(ctx, `SELECT id, task_id, actor, body, created_at FROM notes WHERE task_id = ? ORDER BY created_at ASC`, id)
	if err != nil {
		return t, err
	}
	defer nRows.Close()
	for nRows.Next() {
		var n types.Note
		var c string
		if err := nRows.Scan(&n.ID, &n.TaskID, &n.Actor, &n.Body, &c); err != nil {
			return t, err
		}
		n.CreatedAt = parseTime(c)
		t.Notes = append(t.Notes, n)
	}

	cRows, err := tx.QueryContext(ctx, `SELECT id, task_id, body, done, rank FROM checks WHERE task_id = ? ORDER BY rank ASC`, id)
	if err != nil {
		return t, err
	}
	defer cRows.Close()
	for cRows.Next() {
		var c types.Check
		var done int
		if err := cRows.Scan(&c.ID, &c.TaskID, &c.Body, &done, &c.Rank); err != nil {
			return t, err
		}
		c.Done = done != 0
		t.Checks = append(t.Checks, c)
	}

	gRows, err := tx.QueryContext(ctx, `SELECT slug FROM task_tags WHERE task_id = ? ORDER BY rank ASC, slug ASC`, id)
	if err != nil {
		return t, err
	}
	defer gRows.Close()
	for gRows.Next() {
		var slug string
		if err := gRows.Scan(&slug); err != nil {
			return t, err
		}
		t.Tags = append(t.Tags, slug)
	}

	dRows, err := tx.QueryContext(ctx, `SELECT t.handle FROM deps d JOIN tasks t ON t.id = d.depends_on WHERE d.task_id = ?`, id)
	if err != nil {
		return t, err
	}
	defer dRows.Close()
	for dRows.Next() {
		var h string
		if err := dRows.Scan(&h); err != nil {
			return t, err
		}
		t.DependsOn = append(t.DependsOn, h)
	}
	return t, nil
}

type MutateTask struct {
	Title           *string
	Body            *string
	Phase           *types.Phase
	Pushed          *int
	Rank            *int
	ClaimedBy       *string
	ClaimedUntil    *time.Time
	ClaimSecretHash *string
	ClearClaim      bool
	Evidence        *string
	ClosedAt        *time.Time
	VerifiedAt      *time.Time
	VerifiedBy      *string
	ClosedBy        *string
	Version         int
}

func (s *Store) Mutate(ctx context.Context, taskID, actor, kind string, payload any, m MutateTask) (types.Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var task types.Task
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		t, err := getTaskTx(ctx, tx, taskID)
		if err != nil {
			return err
		}
		if m.Version != 0 && t.Version != m.Version {
			return ErrConflict
		}
		sets := []string{"version = version + 1", "updated_at = ?"}
		args := []any{fmtTime(now())}
		if m.Title != nil {
			sets = append(sets, "title = ?")
			args = append(args, *m.Title)
		}
		if m.Body != nil {
			sets = append(sets, "body = ?")
			args = append(args, *m.Body)
		}
		if m.Phase != nil {
			sets = append(sets, "phase = ?")
			args = append(args, string(*m.Phase))
		}
		if m.Pushed != nil {
			sets = append(sets, "pushed = ?")
			args = append(args, *m.Pushed)
		}
		if m.Rank != nil {
			sets = append(sets, "rank = ?")
			args = append(args, *m.Rank)
		}
		if m.ClearClaim {
			sets = append(sets, "claimed_by = NULL", "claimed_until = NULL", "claim_id_hash = NULL")
		} else {
			if m.ClaimedBy != nil {
				sets = append(sets, "claimed_by = ?")
				args = append(args, *m.ClaimedBy)
			}
			if m.ClaimedUntil != nil {
				sets = append(sets, "claimed_until = ?")
				args = append(args, fmtTime(*m.ClaimedUntil))
			}
			if m.ClaimSecretHash != nil {
				sets = append(sets, "claim_id_hash = ?")
				args = append(args, *m.ClaimSecretHash)
			}
		}
		if m.Evidence != nil {
			sets = append(sets, "evidence = ?")
			args = append(args, *m.Evidence)
		}
		if m.ClosedAt != nil {
			sets = append(sets, "closed_at = ?")
			args = append(args, fmtTime(*m.ClosedAt))
		}
		if m.VerifiedAt != nil {
			sets = append(sets, "verified_at = ?")
			args = append(args, fmtTime(*m.VerifiedAt))
		}
		if m.VerifiedBy != nil {
			sets = append(sets, "verified_by = ?")
			args = append(args, *m.VerifiedBy)
		}
		if m.ClosedBy != nil {
			sets = append(sets, "closed_by = ?")
			args = append(args, *m.ClosedBy)
		}
		args = append(args, taskID)
		q := `UPDATE tasks SET ` + strings.Join(sets, ", ") + ` WHERE id = ?`
		if _, err := tx.ExecContext(ctx, q, args...); err != nil {
			return err
		}
		b, _ := json.Marshal(payload)
		if _, err := tx.ExecContext(ctx, `INSERT INTO events(ledger_id, task_id, actor, kind, payload, created_at) VALUES (?,?,?,?,?,?)`,
			t.LedgerID, t.ID, actor, kind, string(b), fmtTime(now())); err != nil {
			return err
		}
		t2, err := getTaskTx(ctx, tx, taskID)
		task = t2
		return err
	})
	return task, err
}

func (s *Store) AddNote(ctx context.Context, taskID, actor, body string) (types.Note, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var note types.Note
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		t, err := getTaskTx(ctx, tx, taskID)
		if err != nil {
			return err
		}
		note = types.Note{ID: uuid.NewString(), TaskID: taskID, Actor: actor, Body: body, CreatedAt: now()}
		if _, err := tx.ExecContext(ctx, `INSERT INTO notes(id, task_id, actor, body, created_at) VALUES (?,?,?,?,?)`,
			note.ID, note.TaskID, note.Actor, note.Body, fmtTime(note.CreatedAt)); err != nil {
			return err
		}
		payload, _ := json.Marshal(map[string]string{"body": body})
		_, err = tx.ExecContext(ctx, `INSERT INTO events(ledger_id, task_id, actor, kind, payload, created_at) VALUES (?,?,?,?,?,?)`,
			t.LedgerID, t.ID, actor, "note", string(payload), fmtTime(note.CreatedAt))
		return err
	})
	return note, err
}

func (s *Store) SetCheck(ctx context.Context, taskID, actor, checkID string, done bool) (types.Task, error) {
	return s.SetChecks(ctx, taskID, actor, []string{checkID}, done)
}

// SetChecks sets several boxes in one transaction. Ticking six of them used to
// be six round trips, each one reloading the task and re-checking the lease,
// and a failure halfway left the task half ticked with no way to tell that was
// not what the agent meant.
func (s *Store) SetChecks(ctx context.Context, taskID, actor string, checkIDs []string, done bool) (types.Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var task types.Task
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		t, err := getTaskTx(ctx, tx, taskID)
		if err != nil {
			return err
		}
		doneInt := 0
		if done {
			doneInt = 1
		}
		touched := make([]map[string]any, 0, len(checkIDs))
		for _, checkID := range checkIDs {
			var found bool
			var body string
			var n int
			for _, c := range t.Checks {
				if c.ID == checkID {
					found = true
					body = c.Body
					n = c.Rank + 1
					break
				}
			}
			if !found {
				return fmt.Errorf("check not found")
			}
			if _, err := tx.ExecContext(ctx, `UPDATE checks SET done = ? WHERE id = ?`, doneInt, checkID); err != nil {
				return err
			}
			touched = append(touched, map[string]any{"n": n, "body": body, "done": done})
		}
		// Ticking a check changes the task, so it counts as a version: anyone
		// holding a stale read of this task is holding a stale read. One bump
		// for the batch, because the batch is one change.
		if _, err := tx.ExecContext(ctx, `UPDATE tasks SET version = version + 1, updated_at = ? WHERE id = ?`,
			fmtTime(now()), taskID); err != nil {
			return err
		}
		// One event per box either way, so the log reads the same whether the
		// agent sent them together or one at a time.
		for _, p := range touched {
			payload, _ := json.Marshal(p)
			if _, err := tx.ExecContext(ctx, `INSERT INTO events(ledger_id, task_id, actor, kind, payload, created_at) VALUES (?,?,?,?,?,?)`,
				t.LedgerID, t.ID, actor, "check", string(payload), fmtTime(now())); err != nil {
				return err
			}
		}
		t2, err := getTaskTx(ctx, tx, taskID)
		task = t2
		return err
	})
	return task, err
}

func (s *Store) Reap(ctx context.Context) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		t := now()
		rows, err := tx.QueryContext(ctx, `SELECT id, ledger_id, claimed_by FROM tasks WHERE claimed_until IS NOT NULL AND claimed_until < ?`, fmtTime(t))
		if err != nil {
			return err
		}
		defer rows.Close()
		type row struct{ id, ledger, actor string }
		var expired []row
		for rows.Next() {
			var r row
			if err := rows.Scan(&r.id, &r.ledger, &r.actor); err != nil {
				return err
			}
			expired = append(expired, r)
		}
		if err := rows.Close(); err != nil {
			return err
		}
		for _, r := range expired {
			if _, err := tx.ExecContext(ctx, `UPDATE tasks SET claimed_by = NULL, claimed_until = NULL, claim_id_hash = NULL, version = version + 1, updated_at = ? WHERE id = ?`,
				fmtTime(t), r.id); err != nil {
				return err
			}
			payload, _ := json.Marshal(map[string]string{"expired_actor": r.actor})
			if _, err := tx.ExecContext(ctx, `INSERT INTO events(ledger_id, task_id, actor, kind, payload, created_at) VALUES (?,?,?,?,?,?)`,
				r.ledger, r.id, "reaper", "claim_expired", string(payload), fmtTime(t)); err != nil {
				return err
			}
			n++
		}
		return nil
	})
	return n, err
}

func (s *Store) Events(ctx context.Context, ledgerID string, after int64, limit int) ([]types.Event, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, ledger_id, task_id, actor, kind, payload, created_at FROM events WHERE ledger_id = ? AND id > ? ORDER BY id ASC LIMIT ?`,
		ledgerID, after, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []types.Event
	for rows.Next() {
		var e types.Event
		var taskID sql.NullString
		var created string
		if err := rows.Scan(&e.ID, &e.LedgerID, &taskID, &e.Actor, &e.Kind, &e.Payload, &created); err != nil {
			return nil, err
		}
		if taskID.Valid {
			e.TaskID = taskID.String
		}
		e.CreatedAt = parseTime(created)
		out = append(out, e)
	}
	return out, rows.Err()
}
