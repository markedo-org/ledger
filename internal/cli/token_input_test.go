package cli

import (
	"os"
	"path/filepath"
	"testing"
)

// A bearer token typed as a command argument is kept by the shell's history
// file and is visible in the process list while the command runs. Reading it
// from a file or stdin avoids both.
func TestATokenCanArriveWithoutTouchingTheCommandLine(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "token.txt")
	// Trailing newline included on purpose: that is what a file written by
	// `ledger token mint > token.txt` or an editor actually contains.
	if err := os.WriteFile(file, []byte("lgr_from_a_file\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := valueFrom("@" + file)
	if err != nil {
		t.Fatal(err)
	}
	if got != "lgr_from_a_file" {
		t.Fatalf("got %q, want the token without the newline", got)
	}
}

func TestATokenCanArriveOnStdin(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "pipe")
	if err := os.WriteFile(path, []byte("lgr_piped\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	old := os.Stdin
	os.Stdin = f
	defer func() { os.Stdin = old }()

	got, err := valueFrom("-")
	if err != nil {
		t.Fatal(err)
	}
	if got != "lgr_piped" {
		t.Fatalf("got %q, want the piped token", got)
	}
}

// An ordinary value still passes through untouched, including one that merely
// looks like a path.
func TestAPlainValueIsLeftAlone(t *testing.T) {
	got, err := valueFrom("https://task-ledger.com")
	if err != nil {
		t.Fatal(err)
	}
	if got != "https://task-ledger.com" {
		t.Fatalf("got %q", got)
	}
}
