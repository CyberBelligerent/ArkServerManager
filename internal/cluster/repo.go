package cluster

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"asamanager/internal/server"
	"asamanager/internal/settings"
)

type Repo struct {
	db *sql.DB
}

func NewRepo(db *sql.DB) *Repo { return &Repo{db: db} }

var ErrNotFound = errors.New("cluster not found")

const clusterSelectColumns = `id, name, cluster_id, cluster_dir, settings_json, custom_lines_json, active_event, active_mods_json, created_at`

func (r *Repo) Create(ctx context.Context, c Cluster) (Cluster, error) {
	settingsJSON, err := settings.EncodeValues(c.Settings)
	if err != nil {
		return Cluster{}, err
	}
	customJSON, err := encodeCustomLines(c.CustomLines)
	if err != nil {
		return Cluster{}, err
	}
	modsJSON, err := encodeMods(c.ActiveMods)
	if err != nil {
		return Cluster{}, err
	}
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO clusters(name, cluster_id, cluster_dir, settings_json, custom_lines_json, active_event, active_mods_json) VALUES(?,?,?,?,?,?,?)`,
		c.Name, c.ClusterID, c.ClusterDir, settingsJSON, customJSON, c.ActiveEvent, modsJSON)
	if err != nil {
		if isUniqueViolation(err) {
			return Cluster{}, fmt.Errorf("%w: %q", ErrClusterIDDuplicate, c.ClusterID)
		}
		return Cluster{}, fmt.Errorf("insert cluster: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Cluster{}, err
	}
	out, err := r.Get(ctx, id)
	if err != nil {
		return Cluster{}, err
	}
	return out, nil
}

func (r *Repo) Get(ctx context.Context, id int64) (Cluster, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT `+clusterSelectColumns+` FROM clusters WHERE id = ?`, id)
	return scanCluster(row)
}

func (r *Repo) GetByClusterID(ctx context.Context, clusterID string) (Cluster, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT `+clusterSelectColumns+` FROM clusters WHERE cluster_id = ?`, clusterID)
	return scanCluster(row)
}

func (r *Repo) List(ctx context.Context) ([]Cluster, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+clusterSelectColumns+` FROM clusters ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Cluster
	for rows.Next() {
		c, err := scanCluster(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (r *Repo) ListClusterIDs(ctx context.Context) ([]string, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT cluster_id FROM clusters ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *Repo) Update(ctx context.Context, c Cluster) error {
	settingsJSON, err := settings.EncodeValues(c.Settings)
	if err != nil {
		return err
	}
	customJSON, err := encodeCustomLines(c.CustomLines)
	if err != nil {
		return err
	}
	modsJSON, err := encodeMods(c.ActiveMods)
	if err != nil {
		return err
	}
	res, err := r.db.ExecContext(ctx,
		`UPDATE clusters SET name = ?, cluster_id = ?, cluster_dir = ?, settings_json = ?, custom_lines_json = ?, active_event = ?, active_mods_json = ? WHERE id = ?`,
		c.Name, c.ClusterID, c.ClusterDir, settingsJSON, customJSON, c.ActiveEvent, modsJSON, c.ID)
	if err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("%w: %q", ErrClusterIDDuplicate, c.ClusterID)
		}
		return fmt.Errorf("update cluster: %w", err)
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
	res, err := r.db.ExecContext(ctx, `DELETE FROM clusters WHERE id = ?`, id)
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

func scanCluster(row rowScanner) (Cluster, error) {
	var (
		c            Cluster
		settingsJSON string
		customJSON   string
		modsJSON     string
		createdAt    time.Time
	)
	err := row.Scan(&c.ID, &c.Name, &c.ClusterID, &c.ClusterDir, &settingsJSON, &customJSON, &c.ActiveEvent, &modsJSON, &createdAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Cluster{}, ErrNotFound
		}
		return Cluster{}, err
	}
	c.CreatedAt = createdAt
	values, err := settings.DecodeValues(settingsJSON)
	if err != nil {
		return Cluster{}, fmt.Errorf("decode settings_json: %w", err)
	}
	c.Settings = values
	lines, err := decodeCustomLines(customJSON)
	if err != nil {
		return Cluster{}, fmt.Errorf("decode custom_lines_json: %w", err)
	}
	c.CustomLines = lines
	mods, err := decodeMods(modsJSON)
	if err != nil {
		return Cluster{}, fmt.Errorf("decode active_mods_json: %w", err)
	}
	c.ActiveMods = mods
	return c, nil
}

func encodeMods(mods []server.ModRef) (string, error) {
	if len(mods) == 0 {
		return "[]", nil
	}
	b, err := json.Marshal(mods)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func decodeMods(raw string) ([]server.ModRef, error) {
	if raw == "" || raw == "null" || raw == "[]" {
		return nil, nil
	}
	var out []server.ModRef
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, err
	}
	return out, nil
}

func encodeCustomLines(lines []CustomINIEntry) (string, error) {
	if len(lines) == 0 {
		return "[]", nil
	}
	b, err := json.Marshal(lines)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func decodeCustomLines(raw string) ([]CustomINIEntry, error) {
	if raw == "" || raw == "null" || raw == "[]" {
		return nil, nil
	}
	var out []CustomINIEntry
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, err
	}
	return out, nil
}

func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "UNIQUE constraint failed")
}
