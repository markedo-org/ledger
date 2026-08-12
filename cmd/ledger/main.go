package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
)

var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

func main() {
	listen := flag.String("listen", "127.0.0.1:8080", "listen address")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Printf("ledger %s (%s %s)\n", version, commit, date)
		os.Exit(0)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", health)

	log.Printf("ledger %s listening on %s", version, *listen)
	if err := http.ListenAndServe(*listen, mux); err != nil {
		log.Fatal(err)
	}
}

func health(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"ok":true}`))
}
