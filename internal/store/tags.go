package store

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/markedo-org/ledger/internal/types"
)

func replaceTagsTx(ctx context.Context, tx *sql.Tx, taskID string, tags []string) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM task_tags WHERE task_id = ?`, taskID); err != nil {
		return err
	}
	for i, slug := range tags {
		if _, err := tx.ExecContext(ctx, `INSERT INTO task_tags(task_id, slug, rank) VALUES (?,?,?)`,
			taskID, slug, i); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) SetTags(ctx context.Context, taskID, actor string, tags []string) (types.Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var task types.Task
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		t, err := getTaskTx(ctx, tx, taskID)
		if err != nil {
			return err
		}
		if err := replaceTagsTx(ctx, tx, taskID, tags); err != nil {
			return err
		}
		payload, _ := json.Marshal(map[string]any{"tags": tags})
		if _, err := tx.ExecContext(ctx, `INSERT INTO events(ledger_id, task_id, actor, kind, payload, created_at) VALUES (?,?,?,?,?,?)`,
			t.LedgerID, t.ID, actor, "tags", string(payload), fmtTime(now())); err != nil {
			return err
		}
		t2, err := getTaskTx(ctx, tx, taskID)
		task = t2
		return err
	})
	return task, err
}

func (s *Store) ListLedgerTags(ctx context.Context, ledgerID string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT DISTINCT g.slug
		FROM task_tags g
		JOIN tasks t ON t.id = g.task_id
		WHERE t.ledger_id = ?
		ORDER BY g.slug ASC`, ledgerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var slug string
		if err := rows.Scan(&slug); err != nil {
			return nil, err
		}
		out = append(out, slug)
	}
	return out, rows.Err()
}
