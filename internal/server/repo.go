package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"asamanager/internal/settings"
)

type Repo struct {
	db *sql.DB
}

func NewRepo(db *sql.DB) *Repo { return &Repo{db: db} }

var (
	ErrNotFound      = errors.New("server not found")
	ErrNameDuplicate = errors.New("server name already exists in cluster")
)

func (r *Repo) Create(ctx context.Context, s Server) (Server, error) {
	overridesJSON, err := settings.EncodeValues(s.SettingOverrides)
	if err != nil {
		return Server{}, err
	}
	modsJSON, err := encodeMods(s.ActiveMods)
	if err != nil {
		return Server{}, err
	}
	if s.Status == "" {
		s.Status = StatusStopped
	}
	res, err := r.db.ExecContext(ctx, `
		INSERT INTO servers(
			cluster_id, name, map, install_dir,
			port, query_port, rcon_port, rcon_password, rcon_enabled,
			server_password, max_players,
			settings_overrides_json, active_mods_json, active_event,
			anticheat_enabled, status
		) VALUES (?,?,?,?, ?,?,?,?,?, ?,?, ?,?,?, ?,?)`,
		nullableClusterID(s.ClusterID), s.Name, s.Map, s.InstallDir,
		s.Ports.Game, s.Ports.Query, s.Ports.RCON, s.RCONPassword, boolToInt(s.RCONEnabled),
		s.ServerPassword, s.MaxPlayers,
		overridesJSON, modsJSON, s.ActiveEvent,
		boolToInt(s.AnticheatEnabled), string(s.Status))
	if err != nil {
		if isUniqueViolation(err) {
			return Server{}, fmt.Errorf("%w: %q", ErrNameDuplicate, s.Name)
		}
		return Server{}, fmt.Errorf("insert server: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Server{}, err
	}
	return r.Get(ctx, id)
}

func (r *Repo) Get(ctx context.Context, id int64) (Server, error) {
	row := r.db.QueryRowContext(ctx, baseSelect+` WHERE id = ?`, id)
	return scanServer(row)
}

func (r *Repo) ListByCluster(ctx context.Context, clusterID int64) ([]Server, error) {
	rows, err := r.db.QueryContext(ctx, baseSelect+` WHERE cluster_id = ? ORDER BY id`, clusterID)
	if err != nil {
		return nil, err
	}
	return scanServers(rows)
}

func (r *Repo) ListStandalone(ctx context.Context) ([]Server, error) {
	rows, err := r.db.QueryContext(ctx, baseSelect+` WHERE cluster_id IS NULL ORDER BY id`)
	if err != nil {
		return nil, err
	}
	return scanServers(rows)
}

func (r *Repo) ListAll(ctx context.Context) ([]Server, error) {
	rows, err := r.db.QueryContext(ctx, baseSelect+` ORDER BY cluster_id, id`)
	if err != nil {
		return nil, err
	}
	return scanServers(rows)
}

func (r *Repo) Update(ctx context.Context, s Server) error {
	overridesJSON, err := settings.EncodeValues(s.SettingOverrides)
	if err != nil {
		return err
	}
	modsJSON, err := encodeMods(s.ActiveMods)
	if err != nil {
		return err
	}
	res, err := r.db.ExecContext(ctx, `
		UPDATE servers SET
			cluster_id = ?, name = ?, map = ?, install_dir = ?,
			port = ?, query_port = ?, rcon_port = ?, rcon_password = ?, rcon_enabled = ?,
			server_password = ?, max_players = ?,
			settings_overrides_json = ?, active_mods_json = ?, active_event = ?,
			anticheat_enabled = ?, status = ?
		WHERE id = ?`,
		nullableClusterID(s.ClusterID), s.Name, s.Map, s.InstallDir,
		s.Ports.Game, s.Ports.Query, s.Ports.RCON, s.RCONPassword, boolToInt(s.RCONEnabled),
		s.ServerPassword, s.MaxPlayers,
		overridesJSON, modsJSON, s.ActiveEvent,
		boolToInt(s.AnticheatEnabled), string(s.Status),
		s.ID)
	if err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("%w: %q", ErrNameDuplicate, s.Name)
		}
		return fmt.Errorf("update server: %w", err)
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
	res, err := r.db.ExecContext(ctx, `DELETE FROM servers WHERE id = ?`, id)
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

func (r *Repo) UpdateStatus(ctx context.Context, id int64, st Status) error {
	res, err := r.db.ExecContext(ctx, `UPDATE servers SET status = ? WHERE id = ?`, string(st), id)
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

func (r *Repo) CollectAllPorts(ctx context.Context) ([]PortTriple, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT port, query_port, rcon_port FROM servers`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []PortTriple
	for rows.Next() {
		var p PortTriple
		if err := rows.Scan(&p.Game, &p.Query, &p.RCON); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *Repo) SuggestPortsFromDB(ctx context.Context) (PortTriple, error) {
	existing, err := r.CollectAllPorts(ctx)
	if err != nil {
		return PortTriple{}, err
	}
	return SuggestPorts(existing), nil
}

const baseSelect = `SELECT id, cluster_id, name, map, install_dir,
	port, query_port, rcon_port, rcon_password, rcon_enabled,
	server_password, max_players,
	settings_overrides_json, active_mods_json, active_event,
	anticheat_enabled, status, created_at FROM servers`

type rowScanner interface {
	Scan(...any) error
}

func scanServer(row rowScanner) (Server, error) {
	var (
		s             Server
		clusterID     sql.NullInt64
		overridesJSON string
		modsJSON      string
		anticheatInt  int
		rconEnInt     int
		statusStr     string
		createdAt     time.Time
	)
	err := row.Scan(
		&s.ID, &clusterID, &s.Name, &s.Map, &s.InstallDir,
		&s.Ports.Game, &s.Ports.Query, &s.Ports.RCON, &s.RCONPassword, &rconEnInt,
		&s.ServerPassword, &s.MaxPlayers,
		&overridesJSON, &modsJSON, &s.ActiveEvent,
		&anticheatInt, &statusStr, &createdAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Server{}, ErrNotFound
		}
		return Server{}, err
	}
	if clusterID.Valid {
		s.ClusterID = clusterID.Int64
	}
	s.AnticheatEnabled = anticheatInt != 0
	s.RCONEnabled = rconEnInt != 0
	s.Status = Status(statusStr)
	s.CreatedAt = createdAt

	overrides, err := settings.DecodeValues(overridesJSON)
	if err != nil {
		return Server{}, fmt.Errorf("decode overrides: %w", err)
	}
	s.SettingOverrides = overrides

	mods, err := decodeMods(modsJSON)
	if err != nil {
		return Server{}, fmt.Errorf("decode mods: %w", err)
	}
	s.ActiveMods = mods
	return s, nil
}

func scanServers(rows *sql.Rows) ([]Server, error) {
	defer rows.Close()
	var out []Server
	for rows.Next() {
		s, err := scanServer(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func nullableClusterID(id int64) any {
	if id == 0 {
		return nil
	}
	return id
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func encodeMods(mods []ModRef) (string, error) {
	if len(mods) == 0 {
		return "[]", nil
	}
	b, err := json.Marshal(mods)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func decodeMods(s string) ([]ModRef, error) {
	if s == "" || s == "null" {
		return nil, nil
	}
	var out []ModRef
	if err := json.Unmarshal([]byte(s), &out); err != nil {
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
