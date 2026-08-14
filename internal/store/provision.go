package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/markedo-org/ledger/internal/types"
)

var ErrLedgerCap = errors.New("ledger cap")

func (s *Store) CountLedgers(ctx context.Context, ownerID string) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM ledgers WHERE owner_id = ?`, ownerID).Scan(&n)
	return n, err
}

func (s *Store) ListLedgers(ctx context.Context, ownerID string) ([]types.Ledger, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT l.id, l.owner_id, o.slug, l.slug, l.title, l.created_at
		FROM ledgers l JOIN owners o ON o.id = l.owner_id
		WHERE l.owner_id = ?
		ORDER BY l.created_at ASC, l.rowid ASC`, ownerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]types.Ledger, 0)
	for rows.Next() {
		var l types.Ledger
		var created string
		if err := rows.Scan(&l.ID, &l.OwnerID, &l.OwnerSlug, &l.Slug, &l.Title, &created); err != nil {
			return nil, err
		}
		l.CreatedAt = parseTime(created)
		out = append(out, l)
	}
	return out, rows.Err()
}

func (s *Store) SetMaxLedgers(ctx context.Context, ownerID string, n int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	res, err := s.db.ExecContext(ctx, `UPDATE owners SET max_ledgers = ? WHERE id = ?`, n, ownerID)
	if err != nil {
		return err
	}
	got, _ := res.RowsAffected()
	if got == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) CreateOwner(ctx context.Context, slug string, max int) (types.Owner, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out types.Owner
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		var exists int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM owners WHERE slug = ?`, slug).Scan(&exists); err != nil {
			return err
		}
		if exists > 0 {
			return ErrConflict
		}
		t := now()
		id := uuid.NewString()
		if _, err := tx.ExecContext(ctx, `INSERT INTO owners(id, slug, max_ledgers, created_at) VALUES (?,?,?,?)`,
			id, slug, max, fmtTime(t)); err != nil {
			return err
		}
		out = types.Owner{ID: id, Slug: slug, MaxLedgers: max, CreatedAt: t}
		return nil
	})
	return out, err
}

func (s *Store) ListOwners(ctx context.Context) ([]types.Owner, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, slug, max_ledgers, created_at FROM owners ORDER BY created_at ASC, rowid ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]types.Owner, 0)
	for rows.Next() {
		var o types.Owner
		var created string
		if err := rows.Scan(&o.ID, &o.Slug, &o.MaxLedgers, &created); err != nil {
			return nil, err
		}
		o.CreatedAt = parseTime(created)
		out = append(out, o)
	}
	return out, rows.Err()
}

// LedgerWritable is true when this ledger is among the owner's oldest
// max_ledgers (or the cap is 0, unlimited). Newest extra ledgers freeze first.
func (s *Store) LedgerWritable(ctx context.Context, ledgerID string) (bool, error) {
	var ownerID string
	var max int
	if err := s.db.QueryRowContext(ctx, `
		SELECT o.id, o.max_ledgers
		FROM ledgers l JOIN owners o ON o.id = l.owner_id
		WHERE l.id = ?`, ledgerID).Scan(&ownerID, &max); err != nil {
		return false, err
	}
	if max == 0 {
		return true, nil
	}
	if max < 1 {
		max = 1
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id FROM ledgers WHERE owner_id = ? ORDER BY created_at ASC, rowid ASC`, ownerID)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	i := 0
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return false, err
		}
		if id == ledgerID {
			return i < max, nil
		}
		i++
	}
	if err := rows.Err(); err != nil {
		return false, err
	}
	return false, sql.ErrNoRows
}

func (s *Store) CreateLedger(ctx context.Context, ownerID, slug, title, actor string) (types.Ledger, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out types.Ledger
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		var max int
		var ownerSlug string
		if err := tx.QueryRowContext(ctx, `SELECT slug, max_ledgers FROM owners WHERE id = ?`, ownerID).
			Scan(&ownerSlug, &max); err != nil {
			return err
		}
		var n int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM ledgers WHERE owner_id = ?`, ownerID).Scan(&n); err != nil {
			return err
		}
		if max > 0 && n >= max {
			return ErrLedgerCap
		}
		var exists int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM ledgers WHERE owner_id = ? AND slug = ?`, ownerID, slug).
			Scan(&exists); err != nil {
			return err
		}
		if exists > 0 {
			return ErrConflict
		}
		t := now()
		lid := uuid.NewString()
		sid := uuid.NewString()
		if title == "" {
			title = slug
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO ledgers(id, owner_id, slug, title, created_at) VALUES (?,?,?,?,?)`,
			lid, ownerID, slug, title, fmtTime(t)); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO series(id, ledger_id, prefix, next_n) VALUES (?,?,?,?)`,
			sid, lid, "T", 1); err != nil {
			return err
		}
		payload, _ := json.Marshal(map[string]string{"slug": slug, "title": title})
		if _, err := tx.ExecContext(ctx, `INSERT INTO events(ledger_id, task_id, actor, kind, payload, created_at) VALUES (?,?,?,?,?,?)`,
			lid, nil, actor, "ledger", string(payload), fmtTime(t)); err != nil {
			return err
		}
		out = types.Ledger{ID: lid, OwnerID: ownerID, OwnerSlug: ownerSlug, Slug: slug, Title: title, CreatedAt: t}
		return nil
	})
	return out, err
}

func (s *Store) CreateToken(ctx context.Context, ownerID, actor, ledgerID, role, email, plain string) (types.Token, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var tok types.Token
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		var ownerSlug string
		if err := tx.QueryRowContext(ctx, `SELECT slug FROM owners WHERE id = ?`, ownerID).Scan(&ownerSlug); err != nil {
			return err
		}
		var ledgerSlug string
		if ledgerID != "" {
			var lidOwner string
			if err := tx.QueryRowContext(ctx, `SELECT owner_id, slug FROM ledgers WHERE id = ?`, ledgerID).
				Scan(&lidOwner, &ledgerSlug); err != nil {
				return err
			}
			if lidOwner != ownerID {
				return ErrConflict
			}
		}
		if email != "" {
			var taken int
			if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM tokens WHERE email = ?`, email).Scan(&taken); err != nil {
				return err
			}
			if taken > 0 {
				return ErrConflict
			}
		}
		t := now()
		id := uuid.NewString()
		var ledger any
		if ledgerID == "" {
			ledger = nil
		} else {
			ledger = ledgerID
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO tokens(id, token_hash, actor, owner_id, ledger_id, role, email, created_at) VALUES (?,?,?,?,?,?,?,?)`,
			id, HashToken(plain), actor, ownerID, ledger, role, email, fmtTime(t)); err != nil {
			return err
		}
		if ledgerID != "" {
			payload, _ := json.Marshal(map[string]string{"actor": actor, "role": role})
			if _, err := tx.ExecContext(ctx, `INSERT INTO events(ledger_id, task_id, actor, kind, payload, created_at) VALUES (?,?,?,?,?,?)`,
				ledgerID, nil, actor, "token", string(payload), fmtTime(t)); err != nil {
				return err
			}
		}
		tok = types.Token{
			ID:         id,
			Actor:      actor,
			OwnerID:    ownerID,
			OwnerSlug:  ownerSlug,
			LedgerID:   ledgerID,
			LedgerSlug: ledgerSlug,
			Role:       role,
			Email:      email,
			CreatedAt:  t,
		}
		return nil
	})
	return tok, err
}
