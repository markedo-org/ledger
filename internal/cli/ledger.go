package cli

import (
	"flag"
	"fmt"
	"os"
)

func Ledger(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: ledger ledger create|list|set|reset")
		return 2
	}
	switch args[0] {
	case "create":
		return ledgerCreate(args[1:])
	case "list":
		return ledgerList(args[1:])
	case "set":
		return ledgerSet(args[1:])
	case "reset":
		return ledgerReset(args[1:])
	default:
		fmt.Fprintln(os.Stderr, "usage: ledger ledger create|list|set|reset")
		return 2
	}
}

func ledgerCreate(args []string) int {
	fs := flag.NewFlagSet("ledger create", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	profile := fs.String("profile", "", "config profile (owner admin)")
	owner := fs.String("owner", "", "owner slug (default from profile)")
	slug := fs.String("slug", "", "ledger slug")
	title := fs.String("title", "", "display title")
	actor := fs.String("actor", "", "project write-token actor (owner admin defaults to the token actor)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *slug == "" {
		fmt.Fprintln(os.Stderr, "ledger create: --slug is required")
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
		fmt.Fprintln(os.Stderr, "ledger create: --owner is required")
		return 2
	}
	body := map[string]any{"slug": *slug}
	if *title != "" {
		body["title"] = *title
	}
	if *actor != "" {
		body["actor"] = *actor
	}
	out, err := c.do("POST", "/v1/"+*owner+"/ledgers", body)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return printJSON(out)
}

func ledgerList(args []string) int {
	fs := flag.NewFlagSet("ledger list", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	profile := fs.String("profile", "", "config profile")
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
		fmt.Fprintln(os.Stderr, "ledger list: --owner is required")
		return 2
	}
	out, err := c.do("GET", "/v1/"+*owner+"/ledgers", nil)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return printJSON(out)
}

func ledgerSet(args []string) int {
	fs := flag.NewFlagSet("ledger set", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	profile := fs.String("profile", "", "config profile (owner admin)")
	owner := fs.String("owner", "", "owner slug (default from profile)")
	slug := fs.String("slug", "", "ledger slug")
	title := fs.String("title", "", "display title")
	archive := fs.Int("archive-done-after-days", -1, "days before DONE leaves the default list (0 = never)")
	purge := fs.Int("purge-done-after-days", -1, "days before DONE is deleted (0 = never)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *slug == "" {
		fmt.Fprintln(os.Stderr, "ledger set: --slug is required")
		return 2
	}
	setTitle := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "title" {
			setTitle = true
		}
	})
	if *archive < 0 && *purge < 0 && !setTitle {
		fmt.Fprintln(os.Stderr, "ledger set: --title, --archive-done-after-days, or --purge-done-after-days is required")
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
		fmt.Fprintln(os.Stderr, "ledger set: --owner is required")
		return 2
	}
	body := map[string]any{}
	if setTitle {
		body["title"] = *title
	}
	if *archive >= 0 {
		body["archive_done_after_days"] = *archive
	}
	if *purge >= 0 {
		body["purge_done_after_days"] = *purge
	}
	out, err := c.do("PATCH", "/v1/"+*owner+"/ledgers/"+*slug, body)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return printJSON(out)
}

func ledgerReset(args []string) int {
	fs := flag.NewFlagSet("ledger reset", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	profile := fs.String("profile", "", "config profile (owner admin or operator)")
	owner := fs.String("owner", "", "owner slug (default from profile)")
	slug := fs.String("slug", "", "ledger slug")
	confirm := fs.String("confirm", "", "must be owner/ledger")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *slug == "" {
		fmt.Fprintln(os.Stderr, "ledger reset: --slug is required")
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
		fmt.Fprintln(os.Stderr, "ledger reset: --owner is required")
		return 2
	}
	if *confirm == "" {
		fmt.Fprintln(os.Stderr, "ledger reset: --confirm owner/ledger is required")
		return 2
	}
	out, err := c.do("POST", "/v1/"+*owner+"/"+*slug+"/reset", map[string]any{"confirm": *confirm})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return printJSON(out)
}
