// Command smokemcp runs the v1.0 MCP work loop against a live server.
// Used by scripts/smoke.sh. Same sequence as internal/workloop tests.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/markedo-org/ledger/internal/workloop"
)

func main() {
	url := flag.String("url", "", "MCP URL, for example http://127.0.0.1:18787/mcp")
	token := flag.String("token", "", "bearer token")
	key := flag.String("key", "smoke-mcp-1", "create idempotency key")
	flag.Parse()
	if *url == "" || *token == "" {
		fmt.Fprintln(os.Stderr, "usage: smokemcp -url URL -token TOKEN [-key KEY]")
		os.Exit(2)
	}
	ctx := context.Background()
	session, err := workloop.Connect(ctx, *url, *token)
	if err != nil {
		fmt.Fprintln(os.Stderr, "smokemcp: connect:", err)
		os.Exit(1)
	}
	defer session.Close()
	if err := workloop.Run(ctx, session, *key); err != nil {
		fmt.Fprintln(os.Stderr, "smokemcp:", err)
		os.Exit(1)
	}
	fmt.Println("smokemcp: ok")
}
