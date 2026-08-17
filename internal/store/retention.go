package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/markedo-org/ledger/internal/types"
)

const ledgerCols = `l.id, l.owner_id, o.slug, l.slug, l.title, l.created_at, l.archive_done_after_days, l.purge_done_after_days`

type TaskList struct {
	DoneOnly      bool
	ArchiveBefore *time.Time // hide DONE closed before this; nil keeps all DONE
}

func scanLedger(sc interface{ Scan(dest ...any) error }) (types.Ledger, error) {
	var l types.Ledger
	var created string
	var archive, purge sql.NullInt64
	if err := sc.Scan(&l.ID, &l.OwnerID, &l.OwnerSlug, &l.Slug, &l.Title, &created, &archive, &purge); err != nil {
		return l, err
	}
	l.CreatedAt = parseTime(created)
	if archive.Valid {
		n := int(archive.Int64)
		l.ArchiveDoneAfterDays = &n
	}
	if purge.Valid {
		n := int(purge.Int64)
		l.PurgeDoneAfterDays = &n
	}
	return l, nil
}

func (s *Store) ListAllLedgers(ctx context.Context) ([]types.Ledger, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+ledgerCols+`
		FROM ledgers l JOIN owners o ON o.id = l.owner_id
		ORDER BY l.created_at ASC, l.rowid ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]types.Ledger, 0)
	for rows.Next() {
		l, err := scanLedger(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

func (s *Store) SetLedgerTitle(ctx context.Context, ledgerID, title string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	res, err := s.db.ExecContext(ctx, `UPDATE ledgers SET title = ? WHERE id = ?`, title, ledgerID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) SetLedgerRetention(ctx context.Context, ledgerID string, archive, purge *int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	res, err := s.db.ExecContext(ctx, `
		UPDATE ledgers SET
			archive_done_after_days = COALESCE(?, archive_done_after_days),
			purge_done_after_days = COALESCE(?, purge_done_after_days)
		WHERE id = ?`, nullInt(archive), nullInt(purge), ledgerID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) SetClosedAt(ctx context.Context, ledgerID, handle string, at time.Time) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE tasks SET closed_at = ? WHERE ledger_id = ? AND handle = ? AND phase = 'DONE'`,
		fmtTime(at), ledgerID, handle)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) PurgeDoneBefore(ctx context.Context, ledgerID string, before time.Time) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, `
			SELECT id, handle FROM tasks
			WHERE ledger_id = ? AND phase = 'DONE' AND closed_at IS NOT NULL AND closed_at < ?`,
			ledgerID, fmtTime(before))
		if err != nil {
			return err
		}
		type row struct{ id, handle string }
		var dead []row
		for rows.Next() {
			var r row
			if err := rows.Scan(&r.id, &r.handle); err != nil {
				_ = rows.Close()
				return err
			}
			dead = append(dead, r)
		}
		if err := rows.Close(); err != nil {
			return err
		}
		if err := rows.Err(); err != nil {
			return err
		}
		t := now()
		for _, r := range dead {
			if _, err := tx.ExecContext(ctx, `DELETE FROM notes WHERE task_id = ?`, r.id); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `DELETE FROM checks WHERE task_id = ?`, r.id); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `DELETE FROM deps WHERE task_id = ? OR depends_on = ?`, r.id, r.id); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `DELETE FROM idempotency WHERE task_id = ?`, r.id); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `DELETE FROM events WHERE task_id = ?`, r.id); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `DELETE FROM tasks WHERE id = ?`, r.id); err != nil {
				return err
			}
			payload, _ := json.Marshal(map[string]string{"handle": r.handle})
			if _, err := tx.ExecContext(ctx, `INSERT INTO events(ledger_id, task_id, actor, kind, payload, created_at) VALUES (?,?,?,?,?,?)`,
				ledgerID, nil, "reaper", "purge", string(payload), fmtTime(t)); err != nil {
				return err
			}
			n++
		}
		return nil
	})
	return n, err
}

func nullInt(p *int) any {
	if p == nil {
		return nil
	}
	return *p
}
