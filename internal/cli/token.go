package cli

import (
	"flag"
	"fmt"
	"os"
)

func Token(args []string) int {
	if len(args) == 0 || args[0] != "mint" {
		fmt.Fprintln(os.Stderr, "usage: ledger token mint --actor name [--ledger slug] [--role write]")
		return 2
	}
	return tokenMint(args[1:])
}

func tokenMint(args []string) int {
	fs := flag.NewFlagSet("token mint", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	profile := fs.String("profile", "", "config profile (owner admin)")
	owner := fs.String("owner", "", "owner slug (default from profile)")
	actor := fs.String("actor", "", "actor name")
	ledger := fs.String("ledger", "", "bind to this ledger (omit for owner-scoped)")
	role := fs.String("role", "write", "write or admin")
	email := fs.String("email", "", "optional email for magic-link")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *actor == "" {
		fmt.Fprintln(os.Stderr, "token mint: --actor is required")
		return 2
	}
	c, err := newAPI(*profile)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if *owner == "" {
		*owner = c.owner
	}
	if *owner == "" {
		fmt.Fprintln(os.Stderr, "token mint: --owner is required")
		return 2
	}
	body := map[string]any{"actor": *actor, "role": *role}
	if *ledger != "" {
		body["ledger"] = *ledger
	}
	if *email != "" {
		body["email"] = *email
	}
	out, err := c.do("POST", "/v1/"+*owner+"/tokens", body)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return printJSON(out)
}
