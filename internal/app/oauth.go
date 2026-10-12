package app

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/markedo-org/ledger/internal/store"
	"github.com/markedo-org/ledger/internal/types"
)

const (
	OAuthAccessTTL  = time.Hour
	OAuthCodeTTL    = 5 * time.Minute
	OAuthPendingTTL = 10 * time.Minute
	// OAuthActor is the name on the read token a ChatGPT grant mints. Revoke
	// the token with this actor to disconnect the client. The plaintext of
	// that token is discarded. ChatGPT only ever sees access tokens.
	OAuthActor = "chatgpt"
	OAuthScope = "ledger:read offline_access"
)

func (a *App) Issuer() string {
	return strings.TrimRight(strings.TrimSpace(a.PublicURL), "/")
}

func (a *App) OAuthResource() string {
	iss := a.Issuer()
	if iss == "" {
		return ""
	}
	return iss + "/mcp"
}

func (a *App) OAuthResourceMetadataURL() string {
	iss := a.Issuer()
	if iss == "" {
		return ""
	}
	return iss + "/.well-known/oauth-protected-resource/mcp"
}

func (a *App) WWWAuthenticate() string {
	const realm = `Bearer realm="task-ledger"`
	meta := a.OAuthResourceMetadataURL()
	if meta == "" {
		return realm
	}
	return realm + `, resource_metadata="` + meta + `"`
}

type OAuthStart struct {
	ClientID      string
	RedirectURI   string
	CodeChallenge string
	Resource      string
	Scope         string
	State         string
}

func (a *App) BeginOAuth(ctx context.Context, in OAuthStart) (string, error) {
	if a.OAuthResource() == "" {
		return "", fmt.Errorf("%w: LEDGER_PUBLIC_URL is not set", ErrInvalid)
	}
	if in.Resource != a.OAuthResource() {
		return "", fmt.Errorf("%w: resource", ErrInvalid)
	}
	if !validPKCE(in.CodeChallenge) {
		return "", fmt.Errorf("%w: code_challenge", ErrInvalid)
	}
	scope, err := normalizeScope(in.Scope)
	if err != nil {
		return "", err
	}
	if len(in.State) > 512 || len(in.ClientID) > 512 || len(in.RedirectURI) > 512 {
		return "", fmt.Errorf("%w: parameter too long", ErrInvalid)
	}
	id, err := store.NewPrefixedSecret("lgp_")
	if err != nil {
		return "", err
	}
	err = a.Store.InsertOAuthPending(ctx, store.OAuthPending{
		ID: id, ClientID: in.ClientID, RedirectURI: in.RedirectURI,
		CodeChallenge: in.CodeChallenge, Resource: in.Resource,
		Scope: scope, State: in.State, ExpiresAt: time.Now().UTC().Add(OAuthPendingTTL),
	})
	return id, err
}

// ConsentOAuth mints a one-time code for a single ledger. The caller must
// already have established that tok is that owner's admin. The read token
// itself is minted later, when the code is exchanged, so an abandoned screen
// does not leave a credential behind.
func (a *App) ConsentOAuth(ctx context.Context, tok types.Token, pendingID, ledgerSlug string) (string, error) {
	if tok.IsOperator() || tok.Role != types.RoleAdmin {
		return "", fmt.Errorf("%w: an owner admin token has to approve a reader", ErrForbidden)
	}
	p, err := a.Store.OAuthPending(ctx, pendingID, time.Now().UTC())
	if err == sql.ErrNoRows {
		return "", ErrUnauthorized
	}
	if err != nil {
		return "", err
	}
	ledgerSlug = strings.TrimSpace(ledgerSlug)
	if tok.LedgerSlug != "" && tok.LedgerSlug != ledgerSlug {
		return "", ErrForbidden
	}
	l, err := a.Store.ResolveLedger(ctx, tok.OwnerSlug, ledgerSlug)
	if err == sql.ErrNoRows {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	if l.OwnerID != tok.OwnerID {
		return "", ErrForbidden
	}
	code, err := store.NewPrefixedSecret("lgc_")
	if err != nil {
		return "", err
	}
	if err := a.Store.InsertOAuthCode(ctx, code, store.OAuthCode{
		ClientID: p.ClientID, RedirectURI: p.RedirectURI, CodeChallenge: p.CodeChallenge,
		Resource: p.Resource, OwnerID: l.OwnerID, LedgerID: l.ID, Actor: OAuthActor,
		State: p.State, ExpiresAt: time.Now().UTC().Add(OAuthCodeTTL),
	}); err != nil {
		return "", err
	}
	if err := a.Store.DeleteOAuthPending(ctx, pendingID); err != nil {
		return "", err
	}
	return a.oauthRedirect(p.RedirectURI, code, p.State, "")
}

func (a *App) DenyOAuth(ctx context.Context, pendingID string) (string, error) {
	p, err := a.Store.OAuthPending(ctx, pendingID, time.Now().UTC())
	if err == sql.ErrNoRows {
		return "", ErrUnauthorized
	}
	if err != nil {
		return "", err
	}
	if err := a.Store.DeleteOAuthPending(ctx, pendingID); err != nil {
		return "", err
	}
	return a.oauthRedirect(p.RedirectURI, "", p.State, "access_denied")
}

type OAuthTokens struct {
	Access  string
	Refresh string
	Expires int
	Scope   string
}

func (a *App) ExchangeOAuthCode(ctx context.Context, code, verifier, clientID, redirectURI, resource string) (OAuthTokens, error) {
	c, err := a.Store.OAuthCode(ctx, strings.TrimSpace(code), time.Now().UTC())
	if err != nil {
		return OAuthTokens{}, ErrUnauthorized
	}
	if subtle.ConstantTimeCompare([]byte(c.ClientID), []byte(clientID)) != 1 ||
		subtle.ConstantTimeCompare([]byte(c.RedirectURI), []byte(redirectURI)) != 1 ||
		c.Resource != resource || resource != a.OAuthResource() {
		return OAuthTokens{}, ErrUnauthorized
	}
	if !pkceS256(verifier, c.CodeChallenge) {
		return OAuthTokens{}, ErrUnauthorized
	}
	return a.issueOAuthPair(func(readPlain, accessPlain, refreshPlain string, exp time.Time) error {
		_, err := a.Store.CompleteOAuthGrant(ctx, code, readPlain, accessPlain, refreshPlain, exp)
		return err
	})
}

func (a *App) RefreshOAuth(ctx context.Context, refresh, clientID, resource string) (OAuthTokens, error) {
	boundClient, boundResource, err := a.Store.OAuthRefreshBound(ctx, strings.TrimSpace(refresh))
	if err != nil {
		return OAuthTokens{}, ErrUnauthorized
	}
	if subtle.ConstantTimeCompare([]byte(boundClient), []byte(clientID)) != 1 || boundResource != resource || resource != a.OAuthResource() {
		return OAuthTokens{}, ErrUnauthorized
	}
	return a.issueOAuthPair(func(_, accessPlain, refreshPlain string, exp time.Time) error {
		return a.Store.RotateOAuthRefresh(ctx, refresh, refreshPlain, accessPlain, exp)
	})
}

func (a *App) issueOAuthPair(save func(readPlain, accessPlain, refreshPlain string, exp time.Time) error) (OAuthTokens, error) {
	readPlain, err := store.NewToken()
	if err != nil {
		return OAuthTokens{}, err
	}
	accessPlain, err := store.NewPrefixedSecret("lga_")
	if err != nil {
		return OAuthTokens{}, err
	}
	refreshPlain, err := store.NewPrefixedSecret("lgf_")
	if err != nil {
		return OAuthTokens{}, err
	}
	exp := time.Now().UTC().Add(OAuthAccessTTL)
	if err := save(readPlain, accessPlain, refreshPlain, exp); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return OAuthTokens{}, ErrUnauthorized
		}
		return OAuthTokens{}, err
	}
	// readPlain is the ledger token. It is hashed into tokens and then
	// dropped. Nothing in this function returns it.
	return OAuthTokens{Access: accessPlain, Refresh: refreshPlain, Expires: int(OAuthAccessTTL.Seconds()), Scope: OAuthScope}, nil
}

func (a *App) oauthRedirect(redirectURI, code, state, errCode string) (string, error) {
	u, err := url.Parse(redirectURI)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return "", fmt.Errorf("%w: redirect_uri", ErrInvalid)
	}
	q := u.Query()
	if errCode != "" {
		q.Set("error", errCode)
	} else {
		q.Set("code", code)
	}
	if state != "" {
		q.Set("state", state)
	}
	q.Set("iss", a.Issuer())
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func normalizeScope(scope string) (string, error) {
	scope = strings.TrimSpace(scope)
	if scope == "" {
		return OAuthScope, nil
	}
	for _, part := range strings.Fields(scope) {
		if part != "ledger:read" && part != "offline_access" {
			return "", fmt.Errorf("%w: scope", ErrInvalid)
		}
	}
	return OAuthScope, nil
}

func validPKCE(s string) bool {
	if len(s) < 43 || len(s) > 128 {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '.', r == '_', r == '~':
		default:
			return false
		}
	}
	return true
}

func pkceS256(verifier, challenge string) bool {
	if !validPKCE(verifier) {
		return false
	}
	sum := sha256.Sum256([]byte(verifier))
	got := base64.RawURLEncoding.EncodeToString(sum[:])
	if len(got) != len(challenge) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(challenge)) == 1
}
