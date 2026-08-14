package web

import (
	"testing"

	"github.com/markedo-org/ledger/internal/types"
)

func TestHomePath(t *testing.T) {
	cases := []struct {
		name string
		sess types.Session
		next string
		want string
	}{
		{"explicit next", types.Session{OwnerSlug: "acme"}, "/acme/jobs", "/acme/jobs"},
		{"operator", types.Session{Role: types.RoleOperator}, "", "/admin"},
		{"owner and ledger", types.Session{OwnerSlug: "acme", LedgerSlug: "jobs"}, "", "/acme/jobs"},
		{"owner only", types.Session{OwnerSlug: "acme"}, "", "/acme"},
		{"github allowlist", types.Session{GitHubLogin: "lgforsberg"}, "", "/admin"},
		{"reject open redirect", types.Session{GitHubLogin: "lgforsberg"}, "https://evil.example", "/admin"},
		{"admin next without operator", types.Session{OwnerSlug: "acme"}, "/admin", "/acme"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := homePath(tc.sess, tc.next)
			if got != tc.want {
				t.Fatalf("got %s want %s", got, tc.want)
			}
		})
	}
}
