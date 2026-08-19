package store

import (
	"context"
	"database/sql"

	"github.com/markedo-org/ledger/internal/types"
)

const tokenInfoCols = `t.id, t.actor, o.slug, l.slug, t.role, t.email, t.created_at, t.revoked_at`

func scanTokenInfo(sc interface{ Scan(...any) error }) (types.TokenInfo, error) {
	var ti types.TokenInfo
	var ledgerSlug sql.NullString
	var created, revoked string
	if err := sc.Scan(&ti.ID, &ti.Actor, &ti.OwnerSlug, &ledgerSlug, &ti.Role, &ti.Email, &created, &revoked); err != nil {
		return ti, err
	}
	if ledgerSlug.Valid {
		ti.LedgerSlug = ledgerSlug.String
	}
	ti.CreatedAt = parseTime(created)
	if revoked != "" {
		t := parseTime(revoked)
		ti.RevokedAt = &t
	}
	return ti, nil
}

// ListTokens returns every token minted for an owner, live and revoked, newest
// first. It never returns the hash, so a listing cannot be used to authenticate.
func (s *Store) ListTokens(ctx context.Context, ownerID string) ([]types.TokenInfo, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+tokenInfoCols+`
		FROM tokens t
		JOIN owners o ON o.id = t.owner_id
		LEFT JOIN ledgers l ON l.id = t.ledger_id
		WHERE t.owner_id = ?
		ORDER BY t.revoked_at = '' DESC, t.created_at DESC`, ownerID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []types.TokenInfo{}
	for rows.Next() {
		ti, err := scanTokenInfo(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, ti)
	}
	return out, rows.Err()
}

func (s *Store) TokenInfoByID(ctx context.Context, ownerID, tokenID string) (types.TokenInfo, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT `+tokenInfoCols+`
		FROM tokens t
		JOIN owners o ON o.id = t.owner_id
		LEFT JOIN ledgers l ON l.id = t.ledger_id
		WHERE t.owner_id = ? AND t.id = ?`, ownerID, tokenID)
	return scanTokenInfo(row)
}

// RevokeToken kills a bearer token and everything standing on it: any HTML
// session signed in with it, and any magic or review link still outstanding.
// The row stays so the audit trail keeps who held what. Revoking twice is a
// no-op rather than an error, because a retry should not fail.
func (s *Store) RevokeToken(ctx context.Context, ownerID, tokenID string) (types.TokenInfo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out types.TokenInfo
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		var revoked string
		if err := tx.QueryRowContext(ctx, `SELECT revoked_at FROM tokens WHERE owner_id = ? AND id = ?`,
			ownerID, tokenID).Scan(&revoked); err != nil {
			return err
		}
		if revoked == "" {
			at := fmtTime(now())
			if _, err := tx.ExecContext(ctx, `UPDATE tokens SET revoked_at = ? WHERE id = ?`, at, tokenID); err != nil {
				return err
			}
		}
		for _, q := range []string{
			`DELETE FROM sessions WHERE token_id = ?`,
			`DELETE FROM magic_links WHERE token_id = ?`,
			`DELETE FROM review_links WHERE token_id = ?`,
		} {
			if _, err := tx.ExecContext(ctx, q, tokenID); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return out, err
	}
	return s.TokenInfoByID(ctx, ownerID, tokenID)
}
