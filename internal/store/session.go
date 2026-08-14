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

func NewSessionToken() (string, error) {
	var b [24]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return "lgs_" + hex.EncodeToString(b[:]), nil
}

func (s *Store) CreateSession(ctx context.Context, actor, githubID, login, ownerSlug, ledgerSlug, role string, ttl time.Duration) (types.Session, string, error) {
	plain, err := NewSessionToken()
	if err != nil {
		return types.Session{}, "", err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	t := now()
	sess := types.Session{
		ID:          uuid.NewString(),
		Actor:       actor,
		GitHubID:    githubID,
		GitHubLogin: login,
		OwnerSlug:   ownerSlug,
		LedgerSlug:  ledgerSlug,
		Role:        role,
		ExpiresAt:   t.Add(ttl),
		CreatedAt:   t,
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO sessions(id, token_hash, actor, github_id, github_login, owner_slug, ledger_slug, role, expires_at, created_at) VALUES (?,?,?,?,?,?,?,?,?,?)`,
		sess.ID, HashToken(plain), actor, githubID, login, ownerSlug, ledgerSlug, role, fmtTime(sess.ExpiresAt), fmtTime(sess.CreatedAt))
	return sess, plain, err
}

func (s *Store) LookupSession(ctx context.Context, plain string) (types.Session, error) {
	var sess types.Session
	var exp, created string
	err := s.db.QueryRowContext(ctx, `SELECT id, actor, github_id, github_login, owner_slug, ledger_slug, role, expires_at, created_at FROM sessions WHERE token_hash = ?`,
		HashToken(plain)).Scan(&sess.ID, &sess.Actor, &sess.GitHubID, &sess.GitHubLogin, &sess.OwnerSlug, &sess.LedgerSlug, &sess.Role, &exp, &created)
	if err != nil {
		return sess, err
	}
	sess.ExpiresAt = parseTime(exp)
	sess.CreatedAt = parseTime(created)
	if !sess.ExpiresAt.After(time.Now().UTC()) {
		return types.Session{}, sql.ErrNoRows
	}
	if sess.Actor == "" {
		sess.Actor = sess.GitHubLogin
	}
	return sess, nil
}

func (s *Store) DeleteSession(ctx context.Context, plain string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash = ?`, HashToken(plain))
	return err
}

func (s *Store) ReapSessions(ctx context.Context) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	res, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at < ?`, fmtTime(now()))
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

func (s *Store) OwnerBySlug(ctx context.Context, slug string) (types.Owner, error) {
	var o types.Owner
	var created string
	err := s.db.QueryRowContext(ctx, `SELECT id, slug, max_ledgers, created_at FROM owners WHERE slug = ?`, slug).
		Scan(&o.ID, &o.Slug, &o.MaxLedgers, &created)
	if err != nil {
		return o, err
	}
	o.CreatedAt = parseTime(created)
	return o, nil
}
