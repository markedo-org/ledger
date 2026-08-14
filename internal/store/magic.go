package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"time"

	"github.com/google/uuid"
	"github.com/markedo-org/ledger/internal/types"
)

const MagicTTL = 15 * time.Minute

func NewMagicCode() (string, error) {
	var b [24]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return "lgl_" + hex.EncodeToString(b[:]), nil
}

func (s *Store) TokenByEmail(ctx context.Context, email string) (types.Token, error) {
	var t types.Token
	var ledger sql.NullString
	var ledgerSlug sql.NullString
	var created string
	err := s.db.QueryRowContext(ctx, `
		SELECT t.id, t.actor, t.owner_id, t.ledger_id, t.role, t.created_at, o.slug, l.slug
		FROM tokens t
		JOIN owners o ON o.id = t.owner_id
		LEFT JOIN ledgers l ON l.id = t.ledger_id
		WHERE t.email = ?`, email).
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

func (s *Store) CreateMagicLink(ctx context.Context, tokenID string) (string, error) {
	plain, err := NewMagicCode()
	if err != nil {
		return "", err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	t := now()
	id := uuid.NewString()
	if _, err := s.db.ExecContext(ctx, `DELETE FROM magic_links WHERE token_id = ?`, tokenID); err != nil {
		return "", err
	}
	if _, err := s.db.ExecContext(ctx, `INSERT INTO magic_links(id, code_hash, token_id, expires_at, created_at) VALUES (?,?,?,?,?)`,
		id, HashToken(plain), tokenID, fmtTime(t.Add(MagicTTL)), fmtTime(t)); err != nil {
		return "", err
	}
	return plain, nil
}

func (s *Store) ConsumeMagicLink(ctx context.Context, plain string) (types.Token, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var tokenID, exp string
	err := s.db.QueryRowContext(ctx, `SELECT token_id, expires_at FROM magic_links WHERE code_hash = ?`, HashToken(plain)).
		Scan(&tokenID, &exp)
	if err != nil {
		return types.Token{}, err
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM magic_links WHERE code_hash = ?`, HashToken(plain)); err != nil {
		return types.Token{}, err
	}
	if !parseTime(exp).After(time.Now().UTC()) {
		return types.Token{}, sql.ErrNoRows
	}
	var t types.Token
	var ledger sql.NullString
	var ledgerSlug sql.NullString
	var created string
	err = s.db.QueryRowContext(ctx, `
		SELECT t.id, t.actor, t.owner_id, t.ledger_id, t.role, t.created_at, o.slug, l.slug
		FROM tokens t
		JOIN owners o ON o.id = t.owner_id
		LEFT JOIN ledgers l ON l.id = t.ledger_id
		WHERE t.id = ?`, tokenID).
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

func (s *Store) ReapMagicLinks(ctx context.Context) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	res, err := s.db.ExecContext(ctx, `DELETE FROM magic_links WHERE expires_at < ?`, fmtTime(now()))
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}
