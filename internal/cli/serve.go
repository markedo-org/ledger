package cli

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/markedo-org/ledger/internal/app"
	"github.com/markedo-org/ledger/internal/githuboauth"
	"github.com/markedo-org/ledger/internal/mail"
	"github.com/markedo-org/ledger/internal/store"
	"github.com/markedo-org/ledger/internal/web"
)

func Serve(args []string, version, commit, date string) int {
	fs := flag.NewFlagSet("ledger", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	listen := fs.String("listen", "127.0.0.1:8080", "listen address")
	dbPath := fs.String("db", "ledger.sqlite", "sqlite path")
	bootOwner := fs.String("boot-owner", "acme", "owner slug created on empty database")
	bootLedger := fs.String("boot-ledger", "inbox", "ledger slug created on empty database")
	bootActor := fs.String("boot-actor", "ada", "actor bound to the boot token")
	showVersion := fs.Bool("version", false, "print version and exit")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *showVersion {
		fmt.Printf("ledger %s (%s %s)\n", version, commit, date)
		return 0
	}

	st, err := store.Open(*dbPath)
	if err != nil {
		log.Print(err)
		return 1
	}
	defer st.Close()

	token := os.Getenv("LEDGER_BOOT_TOKEN")
	if token == "" {
		token, err = store.NewToken()
		if err != nil {
			log.Print(err)
			return 1
		}
	}
	res, err := st.Bootstrap(context.Background(), *bootOwner, *bootLedger, *bootActor, token)
	if err != nil {
		log.Print(err)
		return 1
	}
	if res.Created {
		log.Printf("booted %s/%s actor=%s", *bootOwner, *bootLedger, *bootActor)
		if os.Getenv("LEDGER_BOOT_TOKEN") == "" {
			path := *dbPath + ".boot-token"
			if err := os.WriteFile(path, []byte(res.Token+"\n"), 0o600); err != nil {
				log.Printf("could not write the boot token to %s: %v", path, err)
				return 1
			}
			log.Printf("boot token written to %s, read it once and delete it", path)
		}
	} else {
		log.Printf("database already initialised")
	}

	a := app.New(st)
	a.Version = version
	archiveDays, purgeDays, err := app.RetentionFromEnv()
	if err != nil {
		log.Print(err)
		return 1
	}
	a.ArchiveDoneAfterDays = archiveDays
	a.PurgeDoneAfterDays = purgeDays
	if archiveDays != app.DefaultArchiveDoneAfterDays || purgeDays != app.DefaultPurgeDoneAfterDays {
		log.Printf("retention archive_done_after_days=%d purge_done_after_days=%d", archiveDays, purgeDays)
	}
	a.SetOperatorToken(os.Getenv("LEDGER_OPERATOR_TOKEN"))
	if a.OperatorConfigured() {
		log.Printf("operator token enabled")
	}
	a.PublicURL = strings.TrimSpace(os.Getenv("LEDGER_PUBLIC_URL"))
	siteURL := strings.TrimSpace(os.Getenv("LEDGER_SITE_URL"))
	if m := mail.FromEnv(); m != nil {
		a.Mail = m
	}
	if a.MailEnabled() {
		log.Printf("magic-link email enabled")
	}
	srv, err := web.New(a)
	if err != nil {
		log.Print(err)
		return 1
	}
	srv.SiteURL = siteURL
	if srv.SiteURL != "" {
		log.Printf("site url %s", srv.SiteURL)
	}
	srv.Auth = authFromEnv()
	if srv.Auth.Enabled() {
		srv.GitHub = githuboauth.New(githuboauth.Config{
			ClientID:     srv.Auth.ClientID,
			ClientSecret: srv.Auth.ClientSecret,
			RedirectURI:  srv.Auth.CallbackURL,
		})
		log.Printf("github oauth enabled callback=%s allowlist=%d", srv.Auth.CallbackURL, len(srv.Auth.Allowlist))
	}
	if srv.Auth.RequireHTML {
		log.Printf("html auth required (token login)")
	}
	root, err := web.ParseRoot(os.Getenv("LEDGER_ROOT"), os.Getenv("LEDGER_ROOT_URL"), os.Getenv("LEDGER_ROOT_FILE"))
	if err != nil {
		log.Print(err)
		return 1
	}
	srv.Root = root
	if root.Mode != web.RootLogin {
		log.Printf("GET / mode=%s", root.Mode)
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

	log.Printf("ledger %s listening on http://%s", version, *listen)
	go func() {
		<-ctx.Done()
		log.Printf("shutting down")
		os.Exit(0)
	}()
	if err := http.ListenAndServe(*listen, srv.Handler()); err != nil {
		log.Print(err)
		return 1
	}
	return 0
}

func authFromEnv() web.AuthConfig {
	var allow []string
	for _, n := range strings.Split(os.Getenv("LEDGER_GITHUB_ALLOWLIST"), ",") {
		n = strings.TrimSpace(n)
		if n != "" {
			allow = append(allow, n)
		}
	}
	cb := strings.TrimSpace(os.Getenv("LEDGER_GITHUB_CALLBACK_URL"))
	if cb == "" {
		cb = "http://127.0.0.1:8080/auth/github/callback"
	}
	return web.AuthConfig{
		ClientID:     strings.TrimSpace(os.Getenv("LEDGER_GITHUB_CLIENT_ID")),
		ClientSecret: strings.TrimSpace(os.Getenv("LEDGER_GITHUB_CLIENT_SECRET")),
		CallbackURL:  cb,
		Allowlist:    allow,
		RequireHTML:  os.Getenv("LEDGER_HTML_AUTH") == "1",
		SecureCookie: os.Getenv("LEDGER_SECURE_COOKIES") == "1",
	}
}
