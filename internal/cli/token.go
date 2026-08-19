package cli

import (
	"flag"
	"fmt"
	"os"
)

func Token(args []string) int {
	if len(args) == 0 {
		return tokenUsage()
	}
	switch args[0] {
	case "mint":
		return tokenMint(args[1:])
	case "list":
		return tokenList(args[1:])
	case "revoke":
		return tokenRevoke(args[1:])
	default:
		return tokenUsage()
	}
}

func tokenUsage() int {
	fmt.Fprintln(os.Stderr, "usage:")
	fmt.Fprintln(os.Stderr, "  ledger token mint --actor name [--ledger slug] [--role write] [--email you@example.com]")
	fmt.Fprintln(os.Stderr, "  ledger token list [--owner slug]")
	fmt.Fprintln(os.Stderr, "  ledger token revoke --id <token id> [--owner slug]")
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "To rotate: mint the replacement, put it in every config that held the old")
	fmt.Fprintln(os.Stderr, "one, then revoke the old id using the new token. Revoking cannot be undone")
	fmt.Fprintln(os.Stderr, "and a token cannot revoke itself.")
	return 2
}

func tokenList(args []string) int {
	fs := flag.NewFlagSet("token list", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	profile := fs.String("profile", "", "config profile (owner admin)")
	owner := fs.String("owner", "", "owner slug (default from profile)")
	if err := fs.Parse(args); err != nil {
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
		fmt.Fprintln(os.Stderr, "token list: --owner is required")
		return 2
	}
	out, err := c.do("GET", "/v1/"+*owner+"/tokens", nil)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return printJSON(out)
}

func tokenRevoke(args []string) int {
	fs := flag.NewFlagSet("token revoke", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	profile := fs.String("profile", "", "config profile (owner admin)")
	owner := fs.String("owner", "", "owner slug (default from profile)")
	id := fs.String("id", "", "token id from ledger token list")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *id == "" {
		fmt.Fprintln(os.Stderr, "token revoke: --id is required (see ledger token list)")
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
		fmt.Fprintln(os.Stderr, "token revoke: --owner is required")
		return 2
	}
	out, err := c.do("DELETE", "/v1/"+*owner+"/tokens/"+*id, nil)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return printJSON(out)
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
