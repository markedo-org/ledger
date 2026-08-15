package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/markedo-org/ledger/internal/cliconfig"
)

func isolateEnv(t *testing.T) {
	t.Helper()
	t.Setenv("LEDGER_URL", "")
	t.Setenv("LEDGER_TOKEN", "")
	t.Setenv("LEDGER_PROFILE", "")
}

func TestInitWritesConfigAndMCP(t *testing.T) {
	isolateEnv(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("LEDGER_CONFIG", filepath.Join(home, ".ledger", "config"))
	t.Setenv("LEDGER_BOOT_TOKEN", "lgr_init_test")
	wd := t.TempDir()
	t.Chdir(wd)

	if code := Init([]string{"--owner", "acme", "--ledger", "inbox", "--actor", "ada", "--db", "t.db"}); code != 0 {
		t.Fatalf("init %d", code)
	}
	if _, err := os.Stat("t.db"); err != nil {
		t.Fatal(err)
	}
	f, err := cliconfig.Load(filepath.Join(home, ".ledger", "config"))
	if err != nil {
		t.Fatal(err)
	}
	p, ok := f.Get("default")
	if !ok || p.Owner != "acme" || p.Ledger != "inbox" || p.Token != "lgr_init_test" {
		t.Fatalf("config %+v", p)
	}
	if code := Init([]string{"--owner", "acme", "--ledger", "inbox", "--actor", "ada", "--db", "t.db"}); code == 0 {
		t.Fatal("second init should fail")
	}
}

func TestMCPPrintJSON(t *testing.T) {
	isolateEnv(t)
	home := t.TempDir()
	path := filepath.Join(home, "config")
	t.Setenv("LEDGER_CONFIG", path)
	f := cliconfig.File{Profiles: map[string]cliconfig.Profile{}}
	f.Put("default", cliconfig.Profile{URL: "http://127.0.0.1:8080", Token: "lgr_x", Owner: "acme", Ledger: "inbox"})
	if err := cliconfig.Save(path, f); err != nil {
		t.Fatal(err)
	}
	if code := MCP([]string{"print"}); code != 0 {
		t.Fatalf("mcp print %d", code)
	}
}

func TestOwnerCreateAgainstHTTP(t *testing.T) {
	isolateEnv(t)
	var saw string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		saw = r.Method + " " + r.URL.Path
		if r.Header.Get("Authorization") != "Bearer lgr_op" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"slug":"acme","token":"lgr_once"}`))
	}))
	t.Cleanup(srv.Close)
	path := filepath.Join(t.TempDir(), "config")
	t.Setenv("LEDGER_CONFIG", path)
	f := cliconfig.File{Profiles: map[string]cliconfig.Profile{}}
	f.Put("default", cliconfig.Profile{URL: srv.URL, Token: "lgr_op"})
	if err := cliconfig.Save(path, f); err != nil {
		t.Fatal(err)
	}
	if code := Owner([]string{"create", "--slug", "acme", "--ledger", "inbox", "--actor", "ada"}); code != 0 {
		t.Fatalf("owner create %d", code)
	}
	if saw != "POST /v1/owners" {
		t.Fatalf("saw %q", saw)
	}
}

func TestWriteCursorMerges(t *testing.T) {
	wd := t.TempDir()
	t.Chdir(wd)
	if err := os.MkdirAll(".cursor", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(".cursor/mcp.json", []byte(`{"mcpServers":{"other":{"url":"http://x"}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(mcpObject("task-ledger-admin", "http://127.0.0.1:8080", "lgr_z"))
	if err := writeCursorMCP(raw); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(".cursor/mcp.json")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if !strings.Contains(s, "other") || !strings.Contains(s, "task-ledger-admin") {
		t.Fatalf("merge %s", s)
	}
}
