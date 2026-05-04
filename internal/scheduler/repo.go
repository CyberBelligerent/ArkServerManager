package scheduler

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

type Repo struct{ db *sql.DB }

func NewRepo(db *sql.DB) *Repo { return &Repo{db: db} }

var ErrNotFound = errors.New("scheduled task not found")

const taskColumns = `id, name, scope_type, scope_id, trigger_kind,
	trigger_payload_json, action_kind, action_payload_json,
	start_at, end_at, cron_expr, recurrence, missed_policy, status,
	last_fired_at, next_fire_at, created_at`

func (r *Repo) Create(ctx context.Context, t Task) (Task, error) {
	if t.MissedPolicy == "" {
		t.MissedPolicy = MissedSkip
	}
	if t.Status == "" {
		t.Status = StatusEnabled
	}
	if t.ActionPayload == nil {
		t.ActionPayload = []byte("{}")
	}
	res, err := r.db.ExecContext(ctx, `
		INSERT INTO scheduled_tasks(
			name, scope_type, scope_id, trigger_kind, trigger_payload_json,
			action_kind, action_payload_json, start_at, end_at, cron_expr,
			recurrence, missed_policy, status, last_fired_at, next_fire_at
		) VALUES(?,?,?,?,?, ?,?,?,?,?, ?,?,?,?,?)`,
		t.Name, t.ScopeType, nullableScopeID(t.ScopeType, t.ScopeID), t.TriggerKind, "{}",
		t.ActionKind, string(t.ActionPayload), nullableTime(t.StartAt), nullableTime(t.EndAt), nullableString(t.TriggerCron),
		"", t.MissedPolicy, t.Status, nullableTimePtr(t.LastFiredAt), nullableTimePtr(t.NextFireAt))
	if err != nil {
		return Task{}, fmt.Errorf("insert task: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Task{}, err
	}
	return r.Get(ctx, id)
}

func (r *Repo) Get(ctx context.Context, id int64) (Task, error) {
	row := r.db.QueryRowContext(ctx, `SELECT `+taskColumns+` FROM scheduled_tasks WHERE id = ?`, id)
	return scanTask(row)
}

func (r *Repo) List(ctx context.Context, scopeType string, scopeID int64) ([]Task, error) {
	var rows *sql.Rows
	var err error
	if scopeType == "global" {
		rows, err = r.db.QueryContext(ctx, `SELECT `+taskColumns+
			` FROM scheduled_tasks WHERE scope_type = 'global' ORDER BY id`)
	} else {
		rows, err = r.db.QueryContext(ctx, `SELECT `+taskColumns+
			` FROM scheduled_tasks WHERE scope_type = ? AND scope_id = ? ORDER BY id`,
			scopeType, scopeID)
	}
	if err != nil {
		return nil, err
	}
	return scanTasks(rows)
}

func (r *Repo) ListAll(ctx context.Context) ([]Task, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+taskColumns+` FROM scheduled_tasks ORDER BY id`)
	if err != nil {
		return nil, err
	}
	return scanTasks(rows)
}

func (r *Repo) ListEnabled(ctx context.Context) ([]Task, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+taskColumns+
		` FROM scheduled_tasks WHERE status = 'enabled' ORDER BY id`)
	if err != nil {
		return nil, err
	}
	return scanTasks(rows)
}

func (r *Repo) Update(ctx context.Context, t Task) error {
	if t.ActionPayload == nil {
		t.ActionPayload = []byte("{}")
	}
	res, err := r.db.ExecContext(ctx, `
		UPDATE scheduled_tasks SET
			name = ?, scope_type = ?, scope_id = ?, trigger_kind = ?,
			action_kind = ?, action_payload_json = ?,
			start_at = ?, end_at = ?, cron_expr = ?,
			missed_policy = ?, status = ?,
			last_fired_at = ?, next_fire_at = ?
		WHERE id = ?`,
		t.Name, t.ScopeType, nullableScopeID(t.ScopeType, t.ScopeID), t.TriggerKind,
		t.ActionKind, string(t.ActionPayload),
		nullableTime(t.StartAt), nullableTime(t.EndAt), nullableString(t.TriggerCron),
		t.MissedPolicy, t.Status,
		nullableTimePtr(t.LastFiredAt), nullableTimePtr(t.NextFireAt),
		t.ID)
	if err != nil {
		return fmt.Errorf("update task: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repo) SetStatus(ctx context.Context, id int64, status string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE scheduled_tasks SET status = ? WHERE id = ?`, status, id)
	return err
}

func (r *Repo) SetNextFire(ctx context.Context, id int64, lastFired, nextFire *time.Time) error {
	_, err := r.db.ExecContext(ctx, `UPDATE scheduled_tasks SET last_fired_at = ?, next_fire_at = ? WHERE id = ?`,
		nullableTimePtr(lastFired), nullableTimePtr(nextFire), id)
	return err
}

func (r *Repo) Delete(ctx context.Context, id int64) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM scheduled_tasks WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repo) RecordRun(ctx context.Context, taskID int64, firedAt time.Time, result, message string) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO scheduled_task_runs(task_id, fired_at, result, message)
		VALUES(?,?,?,?)`,
		taskID, firedAt, result, message)
	return err
}

func (r *Repo) ListRuns(ctx context.Context, taskID int64, limit int) ([]Run, error) {
	if limit <= 0 {
		limit = 25
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, task_id, fired_at, result, message
		FROM scheduled_task_runs WHERE task_id = ?
		ORDER BY fired_at DESC, id DESC LIMIT ?`, taskID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Run
	for rows.Next() {
		var r Run
		if err := rows.Scan(&r.ID, &r.TaskID, &r.FiredAt, &r.Result, &r.Message); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

type rowScanner interface {
	Scan(...any) error
}

func scanTask(row rowScanner) (Task, error) {
	var (
		t              Task
		scopeID        sql.NullInt64
		triggerPayload string
		actionPayload  string
		startAt, endAt sql.NullTime
		cronExpr       sql.NullString
		recurrence     string
		lastFired      sql.NullTime
		nextFire       sql.NullTime
	)
	err := row.Scan(
		&t.ID, &t.Name, &t.ScopeType, &scopeID, &t.TriggerKind,
		&triggerPayload, &t.ActionKind, &actionPayload,
		&startAt, &endAt, &cronExpr, &recurrence, &t.MissedPolicy, &t.Status,
		&lastFired, &nextFire, &t.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Task{}, ErrNotFound
		}
		return Task{}, err
	}
	if scopeID.Valid {
		t.ScopeID = scopeID.Int64
	}
	if startAt.Valid {
		t.StartAt = startAt.Time
	}
	if endAt.Valid {
		t.EndAt = endAt.Time
	}
	if cronExpr.Valid {
		t.TriggerCron = cronExpr.String
	}
	if lastFired.Valid {
		v := lastFired.Time
		t.LastFiredAt = &v
	}
	if nextFire.Valid {
		v := nextFire.Time
		t.NextFireAt = &v
	}
	t.ActionPayload = []byte(actionPayload)
	_ = triggerPayload
	_ = recurrence
	return t, nil
}

func scanTasks(rows *sql.Rows) ([]Task, error) {
	defer rows.Close()
	var out []Task
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func nullableScopeID(scopeType string, id int64) any {
	if scopeType == "global" {
		return nil
	}
	return id
}

func nullableTime(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t
}

func nullableTimePtr(t *time.Time) any {
	if t == nil || t.IsZero() {
		return nil
	}
	return *t
}

func nullableString(s string) any {
	if s == "" {
		return nil
	}
	return s
}
