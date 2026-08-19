package app_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/markedo-org/ledger/internal/app"
	"github.com/markedo-org/ledger/internal/types"
)

// A revoked token must stop working everywhere, not just on the route that
// revoked it. This is the whole point of the feature.
func TestRevokedTokenCannotAuthenticate(t *testing.T) {
	a, admin := boot(t)
	ctx := context.Background()
	issued, err := a.CreateToken(ctx, admin, "markedo", app.CreateTokenInput{Actor: "ada", Role: "write", Ledger: "meta"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.Auth(ctx, issued.Plain); err != nil {
		t.Fatalf("the token should work before it is revoked: %v", err)
	}
	info, err := a.RevokeToken(ctx, admin, "markedo", issued.Token.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !info.Revoked() {
		t.Fatal("revoke did not stamp revoked_at")
	}
	if _, err := a.Auth(ctx, issued.Plain); err == nil {
		t.Fatal("a revoked token still authenticates")
	}
}

// Revoking twice is a retry, not an error.
func TestRevokeIsIdempotent(t *testing.T) {
	a, admin := boot(t)
	ctx := context.Background()
	issued, err := a.CreateToken(ctx, admin, "markedo", app.CreateTokenInput{Actor: "ada", Role: "write"})
	if err != nil {
		t.Fatal(err)
	}
	first, err := a.RevokeToken(ctx, admin, "markedo", issued.Token.ID)
	if err != nil {
		t.Fatal(err)
	}
	second, err := a.RevokeToken(ctx, admin, "markedo", issued.Token.ID)
	if err != nil {
		t.Fatalf("second revoke: %v", err)
	}
	if second.RevokedAt == nil || !second.RevokedAt.Equal(*first.RevokedAt) {
		t.Fatal("a repeat revoke moved the timestamp")
	}
}

// Revoking the token in your own hand cannot be undone and cannot be recovered,
// so the server refuses and points at the safe order instead.
func TestTokenCannotRevokeItself(t *testing.T) {
	a, admin := boot(t)
	ctx := context.Background()
	_, err := a.RevokeToken(ctx, admin, "markedo", admin.ID)
	if !errors.Is(err, app.ErrInvalid) {
		t.Fatalf("expected a refusal, got %v", err)
	}
	if _, err := a.Auth(ctx, ""); err == nil {
		t.Fatal("sanity: empty token should not authenticate")
	}
}

// The rotation this was built for: mint the replacement, revoke the old one
// with the new token, and the old one is dead while the new one works.
func TestRotateAnAdminToken(t *testing.T) {
	a, old := boot(t)
	ctx := context.Background()
	replacement, err := a.CreateToken(ctx, old, "markedo", app.CreateTokenInput{Actor: old.Actor, Role: "admin"})
	if err != nil {
		t.Fatal(err)
	}
	fresh, err := a.Auth(ctx, replacement.Plain)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.RevokeToken(ctx, fresh, "markedo", old.ID); err != nil {
		t.Fatalf("the new token must be able to revoke the old one: %v", err)
	}
	if _, err := a.Auth(ctx, replacement.Plain); err != nil {
		t.Fatalf("the replacement stopped working: %v", err)
	}
	list, err := a.ListTokens(ctx, fresh, "markedo")
	if err != nil {
		t.Fatal(err)
	}
	var live, revoked int
	for _, ti := range list {
		if ti.Revoked() {
			revoked++
		} else {
			live++
		}
	}
	if live != 1 || revoked != 1 {
		t.Fatalf("expected one live and one revoked token, got %d and %d", live, revoked)
	}
}

// An email is unique across live tokens only, otherwise a revoked token would
// hold its address hostage and the replacement could never carry it.
func TestRevokingFreesTheEmailForTheReplacement(t *testing.T) {
	a, admin := boot(t)
	ctx := context.Background()
	first, err := a.CreateToken(ctx, admin, "markedo", app.CreateTokenInput{Actor: "ada", Role: "write", Email: "ada@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.CreateToken(ctx, admin, "markedo", app.CreateTokenInput{Actor: "ada", Role: "write", Email: "ada@example.com"}); err == nil {
		t.Fatal("two live tokens must not share an address")
	}
	if _, err := a.RevokeToken(ctx, admin, "markedo", first.Token.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := a.CreateToken(ctx, admin, "markedo", app.CreateTokenInput{Actor: "ada", Role: "write", Email: "ada@example.com"}); err != nil {
		t.Fatalf("the address should be free once the old token is revoked: %v", err)
	}
}

// A revoked token must not be reachable by the side doors either: the magic
// link by email, and any session or link already outstanding.
func TestRevokeClosesSessionsAndLinks(t *testing.T) {
	a, admin := boot(t)
	ctx := context.Background()
	issued, err := a.CreateToken(ctx, admin, "markedo", app.CreateTokenInput{Actor: "ada", Role: "write", Ledger: "meta", Email: "ada@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	ada, err := a.Auth(ctx, issued.Plain)
	if err != nil {
		t.Fatal(err)
	}
	_, cookie, err := a.SessionFromAPIToken(ctx, issued.Plain)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.Session(ctx, cookie); err != nil {
		t.Fatalf("the session should be live before the revoke: %v", err)
	}
	reviewURL, _, err := a.MintReviewURL(ctx, ada)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.RevokeToken(ctx, admin, "markedo", issued.Token.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Session(ctx, cookie); err == nil {
		t.Fatal("the HTML session outlived the token it was signed in with")
	}
	code := reviewURL[strings.LastIndex(reviewURL, "=")+1:]
	if _, _, err := a.ConsumeReviewLink(ctx, code); err == nil {
		t.Fatal("an outstanding review link outlived the revoke")
	}
	if _, err := a.Store.TokenByEmail(ctx, "ada@example.com"); err == nil {
		t.Fatal("a magic link by email still resolves to the revoked token")
	}
}

// A write token has no business listing or killing the owner's tokens.
func TestWriteTokenCannotListOrRevoke(t *testing.T) {
	a, admin := boot(t)
	ctx := context.Background()
	issued, err := a.CreateToken(ctx, admin, "markedo", app.CreateTokenInput{Actor: "ada", Role: "write", Ledger: "meta"})
	if err != nil {
		t.Fatal(err)
	}
	ada, err := a.Auth(ctx, issued.Plain)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.ListTokens(ctx, ada, "markedo"); !errors.Is(err, app.ErrForbidden) {
		t.Fatalf("write token listed tokens: %v", err)
	}
	if _, err := a.RevokeToken(ctx, ada, "markedo", admin.ID); !errors.Is(err, app.ErrForbidden) {
		t.Fatalf("write token revoked a token: %v", err)
	}
	if ada.Role != types.RoleWrite {
		t.Fatalf("sanity: expected a write token, got %q", ada.Role)
	}
}
