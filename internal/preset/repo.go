package preset

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

type Repo struct{ db *sql.DB }

func NewRepo(db *sql.DB) *Repo { return &Repo{db: db} }

var ErrNotFound = errors.New("preset not found")

var ErrNameDuplicate = errors.New("preset name already exists in this scope")

const cols = `id, scope_type, scope_id, name, description, payload_json, created_at`

func (r *Repo) Create(ctx context.Context, p Preset) (Preset, error) {
	body, err := MarshalPayload(p.Payload)
	if err != nil {
		return Preset{}, fmt.Errorf("encode payload: %w", err)
	}
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO settings_presets(scope_type, scope_id, name, description, payload_json) VALUES(?,?,?,?,?)`,
		p.ScopeType, p.ScopeID, p.Name, p.Description, body)
	if err != nil {
		if isUniqueViolation(err) {
			return Preset{}, fmt.Errorf("%w: %q", ErrNameDuplicate, p.Name)
		}
		return Preset{}, fmt.Errorf("insert preset: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Preset{}, err
	}
	return r.Get(ctx, id)
}

func (r *Repo) Get(ctx context.Context, id int64) (Preset, error) {
	row := r.db.QueryRowContext(ctx, `SELECT `+cols+` FROM settings_presets WHERE id = ?`, id)
	return scan(row)
}

func (r *Repo) List(ctx context.Context, scopeType string, scopeID int64) ([]Preset, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+cols+` FROM settings_presets WHERE scope_type = ? AND scope_id = ? ORDER BY name`,
		scopeType, scopeID)
	if err != nil {
		return nil, err
	}
	return scanAll(rows)
}

func (r *Repo) Update(ctx context.Context, p Preset) error {
	body, err := MarshalPayload(p.Payload)
	if err != nil {
		return fmt.Errorf("encode payload: %w", err)
	}
	res, err := r.db.ExecContext(ctx,
		`UPDATE settings_presets SET name = ?, description = ?, payload_json = ? WHERE id = ?`,
		p.Name, p.Description, body, p.ID)
	if err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("%w: %q", ErrNameDuplicate, p.Name)
		}
		return fmt.Errorf("update preset: %w", err)
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
	res, err := r.db.ExecContext(ctx, `DELETE FROM settings_presets WHERE id = ?`, id)
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

func scan(row rowScanner) (Preset, error) {
	var (
		p           Preset
		body        string
		createdAt   time.Time
	)
	err := row.Scan(&p.ID, &p.ScopeType, &p.ScopeID, &p.Name, &p.Description, &body, &createdAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Preset{}, ErrNotFound
		}
		return Preset{}, err
	}
	p.CreatedAt = createdAt
	payload, err := UnmarshalPayload(body)
	if err != nil {
		return Preset{}, fmt.Errorf("decode payload: %w", err)
	}
	p.Payload = payload
	return p, nil
}

func scanAll(rows *sql.Rows) ([]Preset, error) {
	defer rows.Close()
	var out []Preset
	for rows.Next() {
		p, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "UNIQUE constraint failed")
}
