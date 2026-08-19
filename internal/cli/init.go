package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/markedo-org/ledger/internal/cliconfig"
	"github.com/markedo-org/ledger/internal/store"
	"github.com/markedo-org/ledger/internal/types"
)

func Init(args []string) int {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	owner := fs.String("owner", "", "owner slug (required)")
	ledger := fs.String("ledger", "", "first ledger slug (required)")
	actor := fs.String("actor", "", "admin actor (required)")
	dbPath := fs.String("db", "ledger.sqlite", "sqlite path")
	listen := fs.String("listen", "127.0.0.1:8080", "address written into the config URL")
	profile := fs.String("profile", "default", "config profile name")
	projectDir := fs.String("project-dir", "", "repo root or .cursor dir; write mcp.json there")
	writeCursor := fs.Bool("write-cursor", false, "write ./.cursor/mcp.json even if .cursor is missing")
	noWriteCursor := fs.Bool("no-write-cursor", false, "never write mcp.json")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if !types.ValidSlug(*owner) || !types.ValidSlug(*ledger) || !types.ValidActor(*actor) {
		fmt.Fprintln(os.Stderr, "init: --owner, --ledger, and --actor are required (lowercase slugs)")
		return 2
	}

	st, err := store.Open(*dbPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer st.Close()

	token := strings.TrimSpace(os.Getenv("LEDGER_BOOT_TOKEN"))
	if token == "" {
		token, err = store.NewToken()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
	}
	res, err := st.Bootstrap(context.Background(), *owner, *ledger, *actor, token)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if !res.Created {
		fmt.Fprintln(os.Stderr, "init: database already initialised; use ledger serve")
		return 1
	}

	url := publicURL(*listen)
	cfgPath, err := cliconfig.Path()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	f, err := cliconfig.Load(cfgPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	f.Put(*profile, cliconfig.Profile{
		URL:    url,
		Token:  res.Token,
		Owner:  *owner,
		Ledger: *ledger,
	})
	if err := cliconfig.Save(cfgPath, f); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	mcp := mcpObject("task-ledger-admin", url, res.Token)
	raw, err := json.MarshalIndent(mcp, "", "  ")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	wrote, err := maybeWriteCursor(*projectDir, *writeCursor, *noWriteCursor, raw)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	fmt.Printf("booted %s/%s actor=%s\n", *owner, *ledger, *actor)
	fmt.Printf("boot token (save this, it is not stored in plaintext): %s\n", res.Token)
	fmt.Printf("config: %s [%s]\n", cfgPath, *profile)
	fmt.Printf("database: %s\n", *dbPath)
	fmt.Println()
	fmt.Println("Start the server:")
	fmt.Printf("  ledger serve -listen %s -db %s\n", *listen, *dbPath)
	fmt.Println()
	fmt.Println("Owner admin MCP (paste into .cursor/mcp.json):")
	fmt.Println(string(raw))
	if wrote != "" {
		fmt.Println("wrote " + wrote)
	}
	fmt.Println()
	fmt.Println("This token is owner admin. For a project-only agent, start the server")
	fmt.Println("and run: ledger token mint --actor <name> --ledger " + *ledger + " --role write")
	fmt.Println()
	fmt.Println("Install the agent skill:")
	fmt.Println("  " + SkillInstall)
	return 0
}

func publicURL(listen string) string {
	listen = strings.TrimSpace(listen)
	if strings.HasPrefix(listen, "http://") || strings.HasPrefix(listen, "https://") {
		return strings.TrimRight(listen, "/")
	}
	if strings.HasPrefix(listen, ":") {
		return "http://127.0.0.1" + listen
	}
	return "http://" + listen
}

func mcpObject(name, origin, token string) map[string]any {
	return map[string]any{
		"mcpServers": map[string]any{
			name: map[string]any{
				"url": strings.TrimRight(origin, "/") + "/mcp",
				"headers": map[string]any{
					"Authorization": "Bearer " + token,
				},
			},
		},
	}
}
