package cli

import (
	"fmt"
	"os"
	"strings"
)

func Main(args []string, version, commit, date string) int {
	if len(args) < 2 || strings.HasPrefix(args[1], "-") {
		return Serve(args[1:], version, commit, date)
	}
	switch args[1] {
	case "serve":
		return Serve(args[2:], version, commit, date)
	case "init":
		return Init(args[2:])
	case "mcp":
		return MCP(args[2:])
	case "config":
		return Config(args[2:])
	case "owner":
		return Owner(args[2:])
	case "ledger":
		return Ledger(args[2:])
	case "token":
		return Token(args[2:])
	case "skill":
		return Skill(args[2:])
	case "version", "-version", "--version":
		fmt.Printf("ledger %s (%s %s)\n", version, commit, date)
		return 0
	case "help", "-h", "--help":
		fmt.Print(usage)
		return 0
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n%s", args[1], usage)
		return 2
	}
}

const usage = `ledger is the task-ledger server and provision CLI.

Serve (default, also: ledger serve):
  ledger [-listen 127.0.0.1:8080] [-db ledger.sqlite]

Local first owner:
  ledger init --owner acme --ledger inbox --actor ada
  ledger serve

Config (~/.ledger/config, or $LEDGER_CONFIG):
  ledger config path
  ledger config show [--profile name]
  ledger config set [--profile name] url|token|owner|ledger <value>

MCP snippet from the current profile:
  ledger mcp print [--profile name] [--name task-ledger-admin] [--write-cursor]

Agent skill (prints the install command; does not run it):
  ledger skill

Against a running server (same commands for local, self-host, hosted):
  ledger owner create --slug acme --ledger inbox --actor ada
  ledger owner list
  ledger owner set-max --owner acme --max-ledgers 2
  ledger ledger create --slug jobs
  ledger ledger list
  ledger token mint --actor bot --ledger jobs --role write

Profiles: --profile or $LEDGER_PROFILE. $LEDGER_URL and $LEDGER_TOKEN override.
`
