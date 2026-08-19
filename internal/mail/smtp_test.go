package mail

import "testing"

func TestEnvelopeFromDropsTheDisplayName(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"hello@task-ledger.com", "hello@task-ledger.com"},
		{"Task Ledger <hello@task-ledger.com>", "hello@task-ledger.com"},
		{"<hello@task-ledger.com>", "hello@task-ledger.com"},
		{"  hello@task-ledger.com  ", "hello@task-ledger.com"},
		{"resend", ""},
		{"", ""},
	} {
		if got := envelopeFrom(tc.in); got != tc.want {
			t.Errorf("envelopeFrom(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestFromEnvRefusesAFromThatIsNotAnAddress(t *testing.T) {
	t.Setenv("LEDGER_SMTP_HOST", "smtp.resend.com")
	t.Setenv("LEDGER_SMTP_USER", "resend")
	t.Setenv("LEDGER_SMTP_PASS", "secret")
	t.Setenv("LEDGER_SMTP_FROM", "")

	// With no From the user is the fallback, and "resend" is a username, not an
	// address. Better to report mail as off than to fail at every send.
	if s := FromEnv(); s != nil {
		t.Fatalf("expected nil sender, got From=%q", s.From)
	}

	t.Setenv("LEDGER_SMTP_FROM", "Task Ledger <hello@task-ledger.com>")
	s := FromEnv()
	if s == nil {
		t.Fatal("expected a configured sender")
	}
	if !s.Enabled() {
		t.Fatal("expected the sender to be enabled")
	}
	if s.Port != 465 {
		t.Errorf("port = %d, want 465", s.Port)
	}
}
