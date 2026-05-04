package webhook

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

type Scope struct {
	Type string		// global, cluster, server
	ID   int64
}

type Webhook struct {
	ID        int64
	Name      string
	URL       string
	Scope     Scope
	EventMask []string          // event names to receive; "*" = all
	Templates map[string]string // custom template (overrides defaults)
	Enabled   bool
}

type Repo struct{ db *sql.DB }

func NewRepo(db *sql.DB) *Repo { return &Repo{db: db} }

var ErrNotFound = errors.New("webhook not found")

// Create inserts w and returns the row with ID populated.
func (r *Repo) Create(ctx context.Context, w Webhook) (Webhook, error) {
	maskJSON, err := json.Marshal(w.EventMask)
	if err != nil {
		return Webhook{}, err
	}
	tplJSON, err := json.Marshal(w.Templates)
	if err != nil {
		return Webhook{}, err
	}
	scopeID := scopeNullable(w.Scope)
	res, err := r.db.ExecContext(ctx, `
		INSERT INTO discord_webhooks(name, url, scope_type, scope_id, event_mask, template_overrides_json, enabled)
		VALUES(?,?,?,?,?,?,?)`,
		w.Name, w.URL, w.Scope.Type, scopeID, string(maskJSON), string(tplJSON), boolToInt(w.Enabled))
	if err != nil {
		return Webhook{}, fmt.Errorf("insert webhook: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Webhook{}, err
	}
	w.ID = id
	return w, nil
}

func (r *Repo) Get(ctx context.Context, id int64) (Webhook, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, name, url, scope_type, scope_id, event_mask, template_overrides_json, enabled
		 FROM discord_webhooks WHERE id = ?`, id)
	return scanWebhook(row)
}

func (r *Repo) List(ctx context.Context) ([]Webhook, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, name, url, scope_type, scope_id, event_mask, template_overrides_json, enabled
		 FROM discord_webhooks ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Webhook
	for rows.Next() {
		w, err := scanWebhook(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

func (r *Repo) Update(ctx context.Context, w Webhook) error {
	maskJSON, err := json.Marshal(w.EventMask)
	if err != nil {
		return err
	}
	tplJSON, err := json.Marshal(w.Templates)
	if err != nil {
		return err
	}
	res, err := r.db.ExecContext(ctx, `
		UPDATE discord_webhooks SET
			name = ?, url = ?, scope_type = ?, scope_id = ?,
			event_mask = ?, template_overrides_json = ?, enabled = ?
		WHERE id = ?`,
		w.Name, w.URL, w.Scope.Type, scopeNullable(w.Scope),
		string(maskJSON), string(tplJSON), boolToInt(w.Enabled), w.ID)
	if err != nil {
		return fmt.Errorf("update webhook: %w", err)
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

func (r *Repo) Delete(ctx context.Context, id int64) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM discord_webhooks WHERE id = ?`, id)
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

type rowScanner interface {
	Scan(...any) error
}

func scanWebhook(row rowScanner) (Webhook, error) {
	var (
		w        Webhook
		scopeID  sql.NullInt64
		maskJSON string
		tplJSON  string
		enabled  int
	)
	err := row.Scan(&w.ID, &w.Name, &w.URL, &w.Scope.Type, &scopeID, &maskJSON, &tplJSON, &enabled)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Webhook{}, ErrNotFound
		}
		return Webhook{}, err
	}
	if scopeID.Valid {
		w.Scope.ID = scopeID.Int64
	}
	w.Enabled = enabled != 0
	if maskJSON != "" && maskJSON != "null" {
		if err := json.Unmarshal([]byte(maskJSON), &w.EventMask); err != nil {
			return Webhook{}, fmt.Errorf("decode event_mask: %w", err)
		}
	}
	if tplJSON != "" && tplJSON != "null" && tplJSON != "{}" {
		if err := json.Unmarshal([]byte(tplJSON), &w.Templates); err != nil {
			return Webhook{}, fmt.Errorf("decode template_overrides: %w", err)
		}
	}
	return w, nil
}

func scopeNullable(s Scope) any {
	if s.Type == "global" {
		return nil
	}
	return s.ID
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
