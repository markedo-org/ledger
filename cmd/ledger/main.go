package main

import (
	"os"

	"github.com/markedo-org/ledger/internal/cli"
)

var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

func main() {
	os.Exit(cli.Main(os.Args, version, commit, date))
}
