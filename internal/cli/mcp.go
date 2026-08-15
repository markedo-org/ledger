package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/markedo-org/ledger/internal/cliconfig"
)

func MCP(args []string) int {
	if len(args) == 0 || args[0] == "print" {
		if len(args) > 0 && args[0] == "print" {
			args = args[1:]
		}
		return mcpPrint(args)
	}
	fmt.Fprintln(os.Stderr, "usage: ledger mcp print [--profile name] [--name task-ledger-admin] [--project-dir path] [--write-cursor] [--no-write-cursor]")
	return 2
}

func mcpPrint(args []string) int {
	fs := flag.NewFlagSet("mcp print", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	profile := fs.String("profile", "", "config profile")
	name := fs.String("name", "", "MCP server key (default task-ledger-admin, or task-ledger-<ledger>)")
	projectDir := fs.String("project-dir", "", "repo root or .cursor dir; write mcp.json there")
	writeCursor := fs.Bool("write-cursor", false, "write ./.cursor/mcp.json even if .cursor is missing")
	noWriteCursor := fs.Bool("no-write-cursor", false, "never write mcp.json")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	_, p, err := cliconfig.Resolve(*profile)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if p.URL == "" || p.Token == "" {
		fmt.Fprintln(os.Stderr, "mcp print: url and token missing; run ledger init or ledger config set")
		return 1
	}
	server := *name
	if server == "" {
		if p.Ledger != "" {
			server = "task-ledger-admin"
		} else {
			server = "task-ledger"
		}
	}
	raw, err := json.MarshalIndent(mcpObject(server, p.URL, p.Token), "", "  ")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Println(string(raw))
	wrote, err := maybeWriteCursor(*projectDir, *writeCursor, *noWriteCursor, raw)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if wrote != "" {
		fmt.Fprintln(os.Stderr, "wrote "+wrote)
	}
	return 0
}
