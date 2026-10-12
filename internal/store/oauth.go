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

// NewPrefixedSecret is a bearer-shaped random string. OAuth codes and access
// tokens use their own prefixes so a glance can tell them from a ledger token.
func NewPrefixedSecret(prefix string) (string, error) {
	var b [24]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return prefix + hex.EncodeToString(b[:]), nil
}

type OAuthPending struct {
	ID            string
	ClientID      string
	RedirectURI   string
	CodeChallenge string
	Resource      string
	Scope         string
	State         string
	ExpiresAt     time.Time
}

func (s *Store) InsertOAuthPending(ctx context.Context, p OAuthPending) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO oauth_pending(id, client_id, redirect_uri, code_challenge, resource, scope, state, expires_at, created_at)
		VALUES (?,?,?,?,?,?,?,?,?)`,
		p.ID, p.ClientID, p.RedirectURI, p.CodeChallenge, p.Resource, p.Scope, p.State,
		fmtTime(p.ExpiresAt), fmtTime(now()))
	return err
}

func (s *Store) OAuthPending(ctx context.Context, id string, at time.Time) (OAuthPending, error) {
	var p OAuthPending
	var exp string
	err := s.db.QueryRowContext(ctx, `
		SELECT id, client_id, redirect_uri, code_challenge, resource, scope, state, expires_at
		FROM oauth_pending WHERE id = ?`, id).
		Scan(&p.ID, &p.ClientID, &p.RedirectURI, &p.CodeChallenge, &p.Resource, &p.Scope, &p.State, &exp)
	if err != nil {
		return p, err
	}
	p.ExpiresAt = parseTime(exp)
	if !p.ExpiresAt.After(at) {
		return OAuthPending{}, sql.ErrNoRows
	}
	return p, nil
}

func (s *Store) DeleteOAuthPending(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.ExecContext(ctx, `DELETE FROM oauth_pending WHERE id = ?`, id)
	return err
}

type OAuthCode struct {
	ClientID      string
	RedirectURI   string
	CodeChallenge string
	Resource      string
	OwnerID       string
	LedgerID      string
	Actor         string
	State         string
	ExpiresAt     time.Time
	Used          bool
}

func (s *Store) InsertOAuthCode(ctx context.Context, plain string, c OAuthCode) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO oauth_codes(code_hash, client_id, redirect_uri, code_challenge, resource, owner_id, ledger_id, actor, expires_at, state)
		VALUES (?,?,?,?,?,?,?,?,?,?)`,
		HashToken(plain), c.ClientID, c.RedirectURI, c.CodeChallenge, c.Resource,
		c.OwnerID, c.LedgerID, c.Actor, fmtTime(c.ExpiresAt), c.State)
	return err
}

func (s *Store) OAuthCode(ctx context.Context, plain string, at time.Time) (OAuthCode, error) {
	var c OAuthCode
	var exp, used string
	err := s.db.QueryRowContext(ctx, `
		SELECT client_id, redirect_uri, code_challenge, resource, owner_id, ledger_id, actor, expires_at, used_at, state
		FROM oauth_codes WHERE code_hash = ?`, HashToken(plain)).
		Scan(&c.ClientID, &c.RedirectURI, &c.CodeChallenge, &c.Resource, &c.OwnerID, &c.LedgerID, &c.Actor, &exp, &used, &c.State)
	if err != nil {
		return c, err
	}
	c.ExpiresAt = parseTime(exp)
	c.Used = used != ""
	if c.Used || !c.ExpiresAt.After(at) {
		return OAuthCode{}, sql.ErrNoRows
	}
	return c, nil
}

// CompleteOAuthGrant mints the ledger-bound read token, stores the refresh
// and the first access token, and burns the authorization code. One
// transaction, so a replayed code cannot mint a second grant.
func (s *Store) CompleteOAuthGrant(ctx context.Context, codePlain string, readPlain, accessPlain, refreshPlain string, accessExpires time.Time) (types.Token, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var tok types.Token
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		var ownerID, ledgerID, actor, resource, clientID, used, exp string
		err := tx.QueryRowContext(ctx, `
			SELECT owner_id, ledger_id, actor, resource, client_id, used_at, expires_at
			FROM oauth_codes WHERE code_hash = ?`, HashToken(codePlain)).
			Scan(&ownerID, &ledgerID, &actor, &resource, &clientID, &used, &exp)
		if err != nil {
			return err
		}
		if used != "" || !parseTime(exp).After(now()) {
			return sql.ErrNoRows
		}
		res, err := tx.ExecContext(ctx, `UPDATE oauth_codes SET used_at = ? WHERE code_hash = ? AND used_at = ''`,
			fmtTime(now()), HashToken(codePlain))
		if err != nil {
			return err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if n != 1 {
			return sql.ErrNoRows
		}
		tok, err = insertTokenTx(ctx, tx, ownerID, actor, ledgerID, types.RoleRead, readPlain)
		if err != nil {
			return err
		}
		grantID := uuid.NewString()
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO oauth_grants(id, token_id, client_id, resource, refresh_hash, created_at)
			VALUES (?,?,?,?,?,?)`,
			grantID, tok.ID, clientID, resource, HashToken(refreshPlain), fmtTime(now())); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `
			INSERT INTO oauth_access(token_hash, grant_id, expires_at) VALUES (?,?,?)`,
			HashToken(accessPlain), grantID, fmtTime(accessExpires))
		return err
	})
	return tok, err
}

func insertTokenTx(ctx context.Context, tx *sql.Tx, ownerID, actor, ledgerID, role, plain string) (types.Token, error) {
	var ownerSlug, ledgerSlug string
	if err := tx.QueryRowContext(ctx, `SELECT slug FROM owners WHERE id = ?`, ownerID).Scan(&ownerSlug); err != nil {
		return types.Token{}, err
	}
	if err := tx.QueryRowContext(ctx, `SELECT slug FROM ledgers WHERE id = ? AND owner_id = ?`, ledgerID, ownerID).Scan(&ledgerSlug); err != nil {
		return types.Token{}, err
	}
	id := uuid.NewString()
	t := now()
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO tokens(id, token_hash, actor, owner_id, ledger_id, role, email, created_at, revoked_at)
		VALUES (?,?,?,?,?,?,?,?,?)`,
		id, HashToken(plain), actor, ownerID, ledgerID, role, "", fmtTime(t), ""); err != nil {
		return types.Token{}, err
	}
	return types.Token{
		ID: id, Actor: actor, OwnerID: ownerID, OwnerSlug: ownerSlug,
		LedgerID: ledgerID, LedgerSlug: ledgerSlug, Role: role, CreatedAt: t,
	}, nil
}

type oauthRefreshRow struct {
	GrantID  string
	TokenID  string
	ClientID string
	Resource string
	Revoked  string
}

func (s *Store) RotateOAuthRefresh(ctx context.Context, oldRefresh, newRefresh, accessPlain string, accessExpires time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.withTx(ctx, func(tx *sql.Tx) error {
		var row oauthRefreshRow
		err := tx.QueryRowContext(ctx, `
			SELECT g.id, g.token_id, g.client_id, g.resource, t.revoked_at
			FROM oauth_grants g
			JOIN tokens t ON t.id = g.token_id
			WHERE g.refresh_hash = ?`, HashToken(oldRefresh)).
			Scan(&row.GrantID, &row.TokenID, &row.ClientID, &row.Resource, &row.Revoked)
		if err != nil {
			return err
		}
		if row.Revoked != "" {
			return sql.ErrNoRows
		}
		res, err := tx.ExecContext(ctx, `UPDATE oauth_grants SET refresh_hash = ? WHERE id = ? AND refresh_hash = ?`,
			HashToken(newRefresh), row.GrantID, HashToken(oldRefresh))
		if err != nil {
			return err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if n != 1 {
			return sql.ErrNoRows
		}
		_, err = tx.ExecContext(ctx, `
			INSERT INTO oauth_access(token_hash, grant_id, expires_at) VALUES (?,?,?)`,
			HashToken(accessPlain), row.GrantID, fmtTime(accessExpires))
		return err
	})
}

// OAuthRefreshBound reports the client and resource on a live refresh token.
// The caller checks they match the request before rotating.
func (s *Store) OAuthRefreshBound(ctx context.Context, refresh string) (clientID, resource string, err error) {
	var revoked string
	err = s.db.QueryRowContext(ctx, `
		SELECT g.client_id, g.resource, t.revoked_at
		FROM oauth_grants g
		JOIN tokens t ON t.id = g.token_id
		WHERE g.refresh_hash = ?`, HashToken(refresh)).Scan(&clientID, &resource, &revoked)
	if err != nil {
		return "", "", err
	}
	if revoked != "" {
		return "", "", sql.ErrNoRows
	}
	return clientID, resource, nil
}

func (s *Store) LookupOAuthAccess(ctx context.Context, plain string, at time.Time) (types.Token, error) {
	var t types.Token
	var ledger sql.NullString
	var ledgerSlug sql.NullString
	var created string
	err := s.db.QueryRowContext(ctx, `
		SELECT t.id, t.actor, t.owner_id, t.ledger_id, t.role, t.created_at, o.slug, l.slug
		FROM oauth_access a
		JOIN oauth_grants g ON g.id = a.grant_id
		JOIN tokens t ON t.id = g.token_id
		JOIN owners o ON o.id = t.owner_id
		LEFT JOIN ledgers l ON l.id = t.ledger_id
		WHERE a.token_hash = ? AND a.expires_at > ? AND t.revoked_at = ''`,
		HashToken(plain), fmtTime(at)).
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
