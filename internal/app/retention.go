package app

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/markedo-org/ledger/internal/store"
	"github.com/markedo-org/ledger/internal/types"
)

const (
	DefaultArchiveDoneAfterDays = 7
	DefaultPurgeDoneAfterDays   = 0
)

type ListQuery struct {
	DoneOnly bool
}

func ValidateRetention(archiveDays, purgeDays int) error {
	if archiveDays < 0 || purgeDays < 0 {
		return fmt.Errorf("%w: archive_done_after_days and purge_done_after_days must be >= 0", ErrInvalid)
	}
	if archiveDays == 0 && purgeDays > 0 {
		return fmt.Errorf("%w: purge_done_after_days requires archive_done_after_days > 0", ErrInvalid)
	}
	if purgeDays > 0 && purgeDays < archiveDays {
		return fmt.Errorf("%w: purge_done_after_days must be 0 or >= archive_done_after_days", ErrInvalid)
	}
	return nil
}

func RetentionFromEnv() (archiveDays, purgeDays int, err error) {
	archiveDays, err = envInt("LEDGER_ARCHIVE_DONE_AFTER_DAYS", DefaultArchiveDoneAfterDays)
	if err != nil {
		return 0, 0, err
	}
	purgeDays, err = envInt("LEDGER_PURGE_DONE_AFTER_DAYS", DefaultPurgeDoneAfterDays)
	if err != nil {
		return 0, 0, err
	}
	if err := ValidateRetention(archiveDays, purgeDays); err != nil {
		return 0, 0, err
	}
	return archiveDays, purgeDays, nil
}

func envInt(key string, fallback int) (int, error) {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", key, err)
	}
	return n, nil
}

func (a *App) retentionDays(l types.Ledger) (archiveDays, purgeDays int) {
	archiveDays = a.ArchiveDoneAfterDays
	purgeDays = a.PurgeDoneAfterDays
	if l.ArchiveDoneAfterDays != nil {
		archiveDays = *l.ArchiveDoneAfterDays
	}
	if l.PurgeDoneAfterDays != nil {
		purgeDays = *l.PurgeDoneAfterDays
	}
	return archiveDays, purgeDays
}

func (a *App) EffectiveRetention(l types.Ledger) (archiveDays, purgeDays int) {
	return a.retentionDays(l)
}

func (a *App) taskList(l types.Ledger, q ListQuery) store.TaskList {
	out := store.TaskList{DoneOnly: q.DoneOnly}
	if q.DoneOnly {
		return out
	}
	archiveDays, _ := a.retentionDays(l)
	if archiveDays > 0 {
		t := time.Now().UTC().AddDate(0, 0, -archiveDays)
		out.ArchiveBefore = &t
	}
	return out
}

func (a *App) SetLedgerRetention(ctx context.Context, tok types.Token, owner, ledger string, archiveDays, purgeDays *int) (types.Ledger, error) {
	if err := a.requireAdmin(tok); err != nil {
		return types.Ledger{}, err
	}
	l, err := a.Ledger(ctx, tok, owner, ledger)
	if err != nil {
		return l, err
	}
	nextArchive, nextPurge := a.retentionDays(l)
	if archiveDays != nil {
		nextArchive = *archiveDays
	}
	if purgeDays != nil {
		nextPurge = *purgeDays
	}
	if err := ValidateRetention(nextArchive, nextPurge); err != nil {
		return l, err
	}
	if err := a.Store.SetLedgerRetention(ctx, l.ID, archiveDays, purgeDays); err != nil {
		return l, err
	}
	return a.Store.ResolveLedger(ctx, owner, ledger)
}

func (a *App) purgeDone(ctx context.Context) (int, error) {
	ledgers, err := a.Store.ListAllLedgers(ctx)
	if err != nil {
		return 0, err
	}
	n := 0
	now := time.Now().UTC()
	for _, l := range ledgers {
		_, purgeDays := a.retentionDays(l)
		if purgeDays <= 0 {
			continue
		}
		before := now.AddDate(0, 0, -purgeDays)
		p, err := a.Store.PurgeDoneBefore(ctx, l.ID, before)
		if err != nil {
			return n, err
		}
		n += p
	}
	return n, nil
}
