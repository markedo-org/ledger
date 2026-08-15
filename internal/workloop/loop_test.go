package workloop_test

import (
	"context"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/markedo-org/ledger/internal/app"
	"github.com/markedo-org/ledger/internal/mcpserver"
	"github.com/markedo-org/ledger/internal/store"
	"github.com/markedo-org/ledger/internal/web"
	"github.com/markedo-org/ledger/internal/workloop"
)

func TestWorkLoopOnHandler(t *testing.T) {
	tok, url := bootMCP(t, false)
	ctx := context.Background()
	session, err := workloop.Connect(ctx, url, tok)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	if err := workloop.Run(ctx, session, "workloop-handler"); err != nil {
		t.Fatal(err)
	}
}

func TestWorkLoopThroughMux(t *testing.T) {
	tok, url := bootMCP(t, true)
	ctx := context.Background()
	session, err := workloop.Connect(ctx, url, tok)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	if err := workloop.Run(ctx, session, "workloop-mux"); err != nil {
		t.Fatal(err)
	}
}

func bootMCP(t *testing.T, throughMux bool) (token, url string) {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	tok, err := store.NewToken()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Bootstrap(context.Background(), "smoke", "inbox", "ada", tok); err != nil {
		t.Fatal(err)
	}
	a := app.New(s)
	if throughMux {
		srv, err := web.New(a)
		if err != nil {
			t.Fatal(err)
		}
		httpSrv := httptest.NewServer(srv.Handler())
		t.Cleanup(httpSrv.Close)
		return tok, httpSrv.URL + "/mcp"
	}
	httpSrv := httptest.NewServer(mcpserver.Handler(a))
	t.Cleanup(httpSrv.Close)
	return tok, httpSrv.URL
}
