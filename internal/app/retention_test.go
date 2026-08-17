package app_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/markedo-org/ledger/internal/app"
)

func TestRetentionFromEnv(t *testing.T) {
	t.Setenv("LEDGER_ARCHIVE_DONE_AFTER_DAYS", "")
	t.Setenv("LEDGER_PURGE_DONE_AFTER_DAYS", "")
	a, p, err := app.RetentionFromEnv()
	if err != nil || a != 7 || p != 0 {
		t.Fatalf("defaults %d %d %v", a, p, err)
	}
	t.Setenv("LEDGER_ARCHIVE_DONE_AFTER_DAYS", "14")
	t.Setenv("LEDGER_PURGE_DONE_AFTER_DAYS", "30")
	a, p, err = app.RetentionFromEnv()
	if err != nil || a != 14 || p != 30 {
		t.Fatalf("env %d %d %v", a, p, err)
	}
	t.Setenv("LEDGER_ARCHIVE_DONE_AFTER_DAYS", "0")
	t.Setenv("LEDGER_PURGE_DONE_AFTER_DAYS", "10")
	if _, _, err := app.RetentionFromEnv(); err == nil {
		t.Fatal("invalid env pair")
	}
}

func TestValidateRetention(t *testing.T) {
	if err := app.ValidateRetention(7, 0); err != nil {
		t.Fatal(err)
	}
	if err := app.ValidateRetention(0, 0); err != nil {
		t.Fatal(err)
	}
	if err := app.ValidateRetention(7, 7); err != nil {
		t.Fatal(err)
	}
	if err := app.ValidateRetention(7, 30); err != nil {
		t.Fatal(err)
	}
	if err := app.ValidateRetention(0, 30); err == nil {
		t.Fatal("purge with archive 0")
	}
	if err := app.ValidateRetention(14, 7); err == nil {
		t.Fatal("purge shorter than archive")
	}
	if err := app.ValidateRetention(-1, 0); err == nil {
		t.Fatal("negative archive")
	}
}

func TestListHidesOldDone(t *testing.T) {
	a, tok := boot(t)
	ctx := context.Background()
	if _, _, err := a.Create(ctx, tok, "markedo", "meta", app.CreateInput{
		Title: "Keep me", IdempotencyKey: "ret-1",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Close(ctx, tok, "markedo", "meta", "T-001", "done", ""); err != nil {
		t.Fatal(err)
	}
	l, err := a.Ledger(ctx, tok, "markedo", "meta")
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Store.SetClosedAt(ctx, l.ID, "T-001", time.Now().UTC().AddDate(0, 0, -8)); err != nil {
		t.Fatal(err)
	}

	_, board, err := a.List(ctx, tok, "markedo", "meta", app.ListQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if len(board) != 0 {
		t.Fatalf("default list should hide 8-day DONE, got %d", len(board))
	}
	_, archived, err := a.List(ctx, tok, "markedo", "meta", app.ListQuery{DoneOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(archived) != 1 || archived[0].Handle != "T-001" {
		t.Fatalf("archive %#v", archived)
	}
	got, err := a.Get(ctx, tok, "markedo", "meta", "T-001")
	if err != nil || got.Phase != "DONE" {
		t.Fatalf("get hidden DONE: %+v %v", got, err)
	}
}

func TestRecentDoneStaysOnBoard(t *testing.T) {
	a, tok := boot(t)
	ctx := context.Background()
	if _, _, err := a.Create(ctx, tok, "markedo", "meta", app.CreateInput{
		Title: "Fresh", IdempotencyKey: "ret-2",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Close(ctx, tok, "markedo", "meta", "T-001", "just now", ""); err != nil {
		t.Fatal(err)
	}
	_, board, err := a.List(ctx, tok, "markedo", "meta", app.ListQuery{})
	if err != nil || len(board) != 1 {
		t.Fatalf("recent DONE should stay on the board: %d %v", len(board), err)
	}
}

func TestArchiveZeroKeepsDone(t *testing.T) {
	a, tok := boot(t)
	a.ArchiveDoneAfterDays = 0
	ctx := context.Background()
	if _, _, err := a.Create(ctx, tok, "markedo", "meta", app.CreateInput{
		Title: "Old", IdempotencyKey: "ret-3",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Close(ctx, tok, "markedo", "meta", "T-001", "done", ""); err != nil {
		t.Fatal(err)
	}
	l, _ := a.Ledger(ctx, tok, "markedo", "meta")
	_ = a.Store.SetClosedAt(ctx, l.ID, "T-001", time.Now().UTC().AddDate(0, 0, -40))
	_, board, err := a.List(ctx, tok, "markedo", "meta", app.ListQuery{})
	if err != nil || len(board) != 1 {
		t.Fatalf("archive 0 should keep DONE: %d %v", len(board), err)
	}
}

func TestPurgeDeletesOldDone(t *testing.T) {
	a, tok := boot(t)
	a.ArchiveDoneAfterDays = 7
	a.PurgeDoneAfterDays = 14
	ctx := context.Background()
	if _, _, err := a.Create(ctx, tok, "markedo", "meta", app.CreateInput{
		Title: "Dust", IdempotencyKey: "ret-4",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Close(ctx, tok, "markedo", "meta", "T-001", "done", ""); err != nil {
		t.Fatal(err)
	}
	l, _ := a.Ledger(ctx, tok, "markedo", "meta")
	if err := a.Store.SetClosedAt(ctx, l.ID, "T-001", time.Now().UTC().AddDate(0, 0, -15)); err != nil {
		t.Fatal(err)
	}
	n, err := a.Reap(ctx)
	if err != nil || n < 1 {
		t.Fatalf("reap purged %d %v", n, err)
	}
	if _, err := a.Get(ctx, tok, "markedo", "meta", "T-001"); err == nil {
		t.Fatal("purged task still gettable")
	}
	_, archived, err := a.List(ctx, tok, "markedo", "meta", app.ListQuery{DoneOnly: true})
	if err != nil || len(archived) != 0 {
		t.Fatalf("archive after purge: %d %v", len(archived), err)
	}
}

func TestPurgeZeroNeverDeletes(t *testing.T) {
	a, tok := boot(t)
	ctx := context.Background()
	if _, _, err := a.Create(ctx, tok, "markedo", "meta", app.CreateInput{
		Title: "Keep", IdempotencyKey: "ret-5",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Close(ctx, tok, "markedo", "meta", "T-001", "done", ""); err != nil {
		t.Fatal(err)
	}
	l, _ := a.Ledger(ctx, tok, "markedo", "meta")
	_ = a.Store.SetClosedAt(ctx, l.ID, "T-001", time.Now().UTC().AddDate(0, 0, -400))
	if _, err := a.Reap(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Get(ctx, tok, "markedo", "meta", "T-001"); err != nil {
		t.Fatal("purge 0 deleted a task")
	}
}

func TestPatchLedgerTitle(t *testing.T) {
	a, tok := boot(t)
	ctx := context.Background()
	title := "Abuse manager"
	l, err := a.PatchLedger(ctx, tok, "markedo", "meta", &title, nil, nil)
	if err != nil || l.Title != title {
		t.Fatalf("title %#v %v", l.Title, err)
	}
	empty := ""
	l, err = a.PatchLedger(ctx, tok, "markedo", "meta", &empty, nil, nil)
	if err != nil || l.Title != "" {
		t.Fatalf("clear %#v %v", l.Title, err)
	}
	long := strings.Repeat("x", app.MaxLedgerTitle+1)
	if _, err := a.PatchLedger(ctx, tok, "markedo", "meta", &long, nil, nil); err == nil {
		t.Fatal("overlong title")
	}
}

func TestSetLedgerRetentionRejectsBadPair(t *testing.T) {
	a, tok := boot(t)
	ctx := context.Background()
	seven := 7
	three := 3
	if _, err := a.SetLedgerRetention(ctx, tok, "markedo", "meta", &seven, &three); err == nil {
		t.Fatal("purge < archive")
	}
	zero := 0
	thirty := 30
	if _, err := a.SetLedgerRetention(ctx, tok, "markedo", "meta", &zero, &thirty); err == nil {
		t.Fatal("purge with archive 0")
	}
	if _, err := a.SetLedgerRetention(ctx, tok, "markedo", "meta", &seven, &zero); err != nil {
		t.Fatal(err)
	}
	l, err := a.Ledger(ctx, tok, "markedo", "meta")
	if err != nil || l.ArchiveDoneAfterDays == nil || *l.ArchiveDoneAfterDays != 7 {
		t.Fatalf("stored %#v %v", l.ArchiveDoneAfterDays, err)
	}
}
