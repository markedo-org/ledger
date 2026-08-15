package cli

import (
	"flag"
	"fmt"
	"os"

	"github.com/markedo-org/ledger/internal/cliconfig"
)

func Config(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: ledger config path|show|set")
		return 2
	}
	switch args[0] {
	case "path":
		p, err := cliconfig.Path()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		fmt.Println(p)
		return 0
	case "show":
		return configShow(args[1:])
	case "set":
		return configSet(args[1:])
	default:
		fmt.Fprintln(os.Stderr, "usage: ledger config path|show|set")
		return 2
	}
}

func configShow(args []string) int {
	fs := flag.NewFlagSet("config show", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	profile := fs.String("profile", "", "profile to show")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	name, p, err := cliconfig.Resolve(*profile)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	path, _ := cliconfig.Path()
	fmt.Printf("file: %s\n", path)
	fmt.Printf("profile: %s\n", name)
	fmt.Printf("url: %s\n", p.URL)
	if p.Token != "" {
		fmt.Println("token: set")
	} else {
		fmt.Println("token: (empty)")
	}
	fmt.Printf("owner: %s\n", p.Owner)
	fmt.Printf("ledger: %s\n", p.Ledger)
	return 0
}

func configSet(args []string) int {
	fs := flag.NewFlagSet("config set", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	profile := fs.String("profile", "default", "profile to update")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	rest := fs.Args()
	if len(rest) != 2 {
		fmt.Fprintln(os.Stderr, "usage: ledger config set [--profile name] <url|token|owner|ledger> <value>")
		return 2
	}
	key, val := rest[0], rest[1]
	path, err := cliconfig.Path()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	f, err := cliconfig.Load(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	p, _ := f.Get(*profile)
	switch key {
	case "url":
		p.URL = val
	case "token":
		p.Token = val
	case "owner":
		p.Owner = val
	case "ledger":
		p.Ledger = val
	default:
		fmt.Fprintln(os.Stderr, "config set: key must be url, token, owner, or ledger")
		return 2
	}
	f.Put(*profile, p)
	if err := cliconfig.Save(path, f); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Printf("set [%s] %s\n", *profile, key)
	return 0
}
