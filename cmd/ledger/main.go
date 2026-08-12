package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/markedo-org/ledger/internal/app"
	"github.com/markedo-org/ledger/internal/store"
	"github.com/markedo-org/ledger/internal/web"
)

var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

func main() {
	listen := flag.String("listen", "127.0.0.1:8080", "listen address")
	dbPath := flag.String("db", "ledger.sqlite", "sqlite path")
	bootOwner := flag.String("boot-owner", "markedo", "owner slug created on empty database")
	bootLedger := flag.String("boot-ledger", "markedo-meta", "ledger slug created on empty database")
	bootActor := flag.String("boot-actor", "maria", "actor bound to the boot token")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Printf("ledger %s (%s %s)\n", version, commit, date)
		os.Exit(0)
	}

	st, err := store.Open(*dbPath)
	if err != nil {
		log.Fatal(err)
	}
	defer st.Close()

	token := os.Getenv("LEDGER_BOOT_TOKEN")
	if token == "" {
		token, err = store.NewToken()
		if err != nil {
			log.Fatal(err)
		}
	}
	res, err := st.Bootstrap(context.Background(), *bootOwner, *bootLedger, *bootActor, token)
	if err != nil {
		log.Fatal(err)
	}
	if res.Created {
		log.Printf("booted %s/%s actor=%s", *bootOwner, *bootLedger, *bootActor)
		log.Printf("boot token (save this, it is not stored in plaintext): %s", res.Token)
	} else {
		log.Printf("database already initialised")
	}

	a := app.New(st)
	srv, err := web.New(a)
	if err != nil {
		log.Fatal(err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		t := time.NewTicker(30 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if n, err := a.Reap(context.Background()); err != nil {
					log.Printf("reap: %v", err)
				} else if n > 0 {
					log.Printf("reaped %d expired claims", n)
				}
			}
		}
	}()

	engine := srv.Engine()
	log.Printf("ledger %s listening on http://%s", version, *listen)
	go func() {
		<-ctx.Done()
		log.Printf("shutting down")
		os.Exit(0)
	}()
	if err := engine.Run(*listen); err != nil {
		log.Fatal(err)
	}
}
