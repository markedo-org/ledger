package cli

import (
	"flag"
	"fmt"
	"os"
	"strconv"
)

func Owner(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: ledger owner create|list|set-max")
		return 2
	}
	switch args[0] {
	case "create":
		return ownerCreate(args[1:])
	case "list":
		return ownerList(args[1:])
	case "set-max":
		return ownerSetMax(args[1:])
	default:
		fmt.Fprintln(os.Stderr, "usage: ledger owner create|list|set-max")
		return 2
	}
}

func ownerCreate(args []string) int {
	fs := flag.NewFlagSet("owner create", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	profile := fs.String("profile", "", "config profile (operator token)")
	slug := fs.String("slug", "", "owner slug")
	ledger := fs.String("ledger", "", "first ledger slug")
	title := fs.String("title", "", "first ledger title")
	actor := fs.String("actor", "", "admin actor to mint")
	email := fs.String("email", "", "optional email on the minted token")
	max := fs.Int("max-ledgers", 1, "max_ledgers (0 is unlimited)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *slug == "" {
		fmt.Fprintln(os.Stderr, "owner create: --slug is required")
		return 2
	}
	c, err := newAPI(*profile)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	body := map[string]any{"slug": *slug, "max_ledgers": *max}
	if *ledger != "" {
		body["ledger"] = *ledger
	}
	if *title != "" {
		body["title"] = *title
	}
	if *actor != "" {
		body["actor"] = *actor
	}
	if *email != "" {
		body["email"] = *email
	}
	out, err := c.do("POST", "/v1/owners", body)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return printJSON(out)
}

func ownerList(args []string) int {
	fs := flag.NewFlagSet("owner list", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	profile := fs.String("profile", "", "config profile (operator token)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	c, err := newAPI(*profile)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	out, err := c.do("GET", "/v1/owners", nil)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return printJSON(out)
}

func ownerSetMax(args []string) int {
	fs := flag.NewFlagSet("owner set-max", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	profile := fs.String("profile", "", "config profile (operator token)")
	owner := fs.String("owner", "", "owner slug")
	n := fs.String("max-ledgers", "", "new cap (0 is unlimited)")
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
	if *owner == "" || *n == "" {
		fmt.Fprintln(os.Stderr, "owner set-max: --owner and --max-ledgers are required")
		return 2
	}
	max, err := strconv.Atoi(*n)
	if err != nil {
		fmt.Fprintln(os.Stderr, "owner set-max: --max-ledgers must be an integer")
		return 2
	}
	out, err := c.do("PATCH", "/v1/owners/"+*owner, map[string]any{"max_ledgers": max})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return printJSON(out)
}
