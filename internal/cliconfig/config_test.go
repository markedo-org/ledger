package cliconfig

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config")
	f := File{Profiles: map[string]Profile{}}
	f.Put("default", Profile{URL: "http://127.0.0.1:8080", Token: "lgr_a", Owner: "acme", Ledger: "inbox"})
	f.Put("hosted", Profile{URL: "https://task-ledger.com", Token: "lgr_b", Owner: "acme"})
	if err := Save(path, f); err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != 0o600 {
		t.Fatalf("mode %o", st.Mode().Perm())
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	d, ok := got.Get("default")
	if !ok || d.Owner != "acme" || d.Token != "lgr_a" {
		t.Fatalf("default %+v", d)
	}
	h, ok := got.Get("hosted")
	if !ok || h.URL != "https://task-ledger.com" {
		t.Fatalf("hosted %+v", h)
	}
}

func TestResolveEnvOverride(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config")
	t.Setenv("LEDGER_CONFIG", path)
	t.Setenv("LEDGER_URL", "http://override")
	t.Setenv("LEDGER_TOKEN", "lgr_env")
	f := File{Profiles: map[string]Profile{}}
	f.Put("default", Profile{URL: "http://old", Token: "lgr_old", Owner: "acme"})
	if err := Save(path, f); err != nil {
		t.Fatal(err)
	}
	_, p, err := Resolve("")
	if err != nil || p.URL != "http://override" || p.Token != "lgr_env" || p.Owner != "acme" {
		t.Fatalf("%+v %v", p, err)
	}
}
