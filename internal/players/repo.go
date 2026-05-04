package players

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

type Scope struct {
	Type string // "server" or "cluster"
	ID   int64
}

type Player struct {
	SteamID       string
	LastKnownName string
	FirstSeen     time.Time
	LastSeen      time.Time
	TotalMinutes  int
	Notes         string
	IsWatched     bool
}

// Session is one play-time interval
type Session struct {
	ID       int64
	SteamID  string
	ServerID int64
	JoinedAt time.Time
	LeftAt   time.Time // zero = still online
}

type Ban struct {
	ID       int64
	SteamID  string
	Scope    Scope
	Reason   string
	BannedAt time.Time
	BannedBy string
}

type WhitelistEntry struct {
	ID      int64
	SteamID string
	Scope   Scope
	AddedAt time.Time
}

type Repo struct {
	db *sql.DB
}

func NewRepo(db *sql.DB) *Repo { return &Repo{db: db} }

var ErrNotFound = errors.New("player record not found")

// UpsertSeen records that steamID was observed under name at observedAt.
// Inserts a new players row if needed
func (r *Repo) UpsertSeen(ctx context.Context, steamID, name string, observedAt time.Time) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var prevName string
	err = tx.QueryRowContext(ctx, `SELECT last_known_name FROM players WHERE steam_id = ?`, steamID).Scan(&prevName)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO players(steam_id, last_known_name, first_seen, last_seen)
			VALUES(?,?,?,?)`,
			steamID, name, observedAt, observedAt); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO player_name_history(steam_id, name, observed_at) VALUES(?,?,?)`,
			steamID, name, observedAt); err != nil {
			return err
		}
	case err != nil:
		return err
	default:
		if _, err := tx.ExecContext(ctx, `
			UPDATE players SET last_known_name = ?, last_seen = ? WHERE steam_id = ?`,
			name, observedAt, steamID); err != nil {
			return err
		}
		if prevName != name {
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO player_name_history(steam_id, name, observed_at) VALUES(?,?,?)`,
				steamID, name, observedAt); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

func (r *Repo) StartSession(ctx context.Context, steamID string, serverID int64, joinedAt time.Time) (int64, error) {
	res, err := r.db.ExecContext(ctx, `
		INSERT INTO player_sessions(steam_id, server_id, joined_at) VALUES(?,?,?)`,
		steamID, serverID, joinedAt)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (r *Repo) EndSession(ctx context.Context, sessionID int64, leftAt time.Time) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var (
		steamID  string
		joinedAt time.Time
		alreadyEnded sql.NullTime
	)
	err = tx.QueryRowContext(ctx,
		`SELECT steam_id, joined_at, left_at FROM player_sessions WHERE id = ?`, sessionID,
	).Scan(&steamID, &joinedAt, &alreadyEnded)
	if err != nil {
		return fmt.Errorf("load session: %w", err)
	}
	if alreadyEnded.Valid {
		return nil
	}
	if leftAt.Before(joinedAt) {
		leftAt = joinedAt
	}
	minutes := int(leftAt.Sub(joinedAt).Minutes())
	if minutes < 0 {
		minutes = 0
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE player_sessions SET left_at = ? WHERE id = ?`, leftAt, sessionID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE players SET total_minutes = total_minutes + ? WHERE steam_id = ?`, minutes, steamID); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *Repo) ListOpenSessions(ctx context.Context, serverID int64) ([]Session, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, steam_id, server_id, joined_at FROM player_sessions
		 WHERE server_id = ? AND left_at IS NULL ORDER BY joined_at`, serverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Session
	for rows.Next() {
		var s Session
		if err := rows.Scan(&s.ID, &s.SteamID, &s.ServerID, &s.JoinedAt); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *Repo) GetPlayer(ctx context.Context, steamID string) (Player, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT steam_id, last_known_name, first_seen, last_seen, total_minutes, notes, is_watched
		 FROM players WHERE steam_id = ?`, steamID)
	return scanPlayer(row)
}

func (r *Repo) ListPlayers(ctx context.Context) ([]Player, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT steam_id, last_known_name, first_seen, last_seen, total_minutes, notes, is_watched
		 FROM players ORDER BY last_seen DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Player
	for rows.Next() {
		p, err := scanPlayer(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *Repo) SetNotes(ctx context.Context, steamID, notes string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE players SET notes = ? WHERE steam_id = ?`, notes, steamID)
	return err
}

func (r *Repo) SetWatched(ctx context.Context, steamID string, watched bool) error {
	v := 0
	if watched {
		v = 1
	}
	_, err := r.db.ExecContext(ctx, `UPDATE players SET is_watched = ? WHERE steam_id = ?`, v, steamID)
	return err
}

func (r *Repo) AddBan(ctx context.Context, b Ban) (int64, error) {
	res, err := r.db.ExecContext(ctx, `
		INSERT INTO bans(steam_id, scope_type, scope_id, reason, banned_at, banned_by)
		VALUES(?,?,?,?,?,?)
		ON CONFLICT(steam_id, scope_type, scope_id) DO UPDATE SET
			reason = excluded.reason, banned_at = excluded.banned_at, banned_by = excluded.banned_by`,
		b.SteamID, b.Scope.Type, b.Scope.ID, b.Reason, nonZeroTime(b.BannedAt), b.BannedBy)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (r *Repo) RemoveBan(ctx context.Context, steamID string, scope Scope) (bool, error) {
	res, err := r.db.ExecContext(ctx,
		`DELETE FROM bans WHERE steam_id = ? AND scope_type = ? AND scope_id = ?`,
		steamID, scope.Type, scope.ID)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

func (r *Repo) ListBans(ctx context.Context, scope Scope) ([]Ban, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, steam_id, scope_type, scope_id, reason, banned_at, banned_by
		 FROM bans WHERE scope_type = ? AND scope_id = ? ORDER BY banned_at DESC`,
		scope.Type, scope.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Ban
	for rows.Next() {
		var b Ban
		if err := rows.Scan(&b.ID, &b.SteamID, &b.Scope.Type, &b.Scope.ID, &b.Reason, &b.BannedAt, &b.BannedBy); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func (r *Repo) AddWhitelist(ctx context.Context, e WhitelistEntry) (int64, error) {
	res, err := r.db.ExecContext(ctx, `
		INSERT INTO whitelist(steam_id, scope_type, scope_id, added_at) VALUES(?,?,?,?)
		ON CONFLICT(steam_id, scope_type, scope_id) DO NOTHING`,
		e.SteamID, e.Scope.Type, e.Scope.ID, nonZeroTime(e.AddedAt))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (r *Repo) RemoveWhitelist(ctx context.Context, steamID string, scope Scope) (bool, error) {
	res, err := r.db.ExecContext(ctx,
		`DELETE FROM whitelist WHERE steam_id = ? AND scope_type = ? AND scope_id = ?`,
		steamID, scope.Type, scope.ID)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

func (r *Repo) ListWhitelist(ctx context.Context, scope Scope) ([]WhitelistEntry, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, steam_id, scope_type, scope_id, added_at FROM whitelist
		 WHERE scope_type = ? AND scope_id = ? ORDER BY added_at`,
		scope.Type, scope.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []WhitelistEntry
	for rows.Next() {
		var e WhitelistEntry
		if err := rows.Scan(&e.ID, &e.SteamID, &e.Scope.Type, &e.Scope.ID, &e.AddedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

type rowScanner interface {
	Scan(...any) error
}

func scanPlayer(row rowScanner) (Player, error) {
	var (
		p          Player
		notes      sql.NullString
		watchedInt int
	)
	err := row.Scan(&p.SteamID, &p.LastKnownName, &p.FirstSeen, &p.LastSeen, &p.TotalMinutes, &notes, &watchedInt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Player{}, ErrNotFound
		}
		return Player{}, err
	}
	p.Notes = strings.TrimSpace(notes.String)
	p.IsWatched = watchedInt != 0
	return p, nil
}

func nonZeroTime(t time.Time) time.Time {
	if t.IsZero() {
		return time.Now().UTC()
	}
	return t
}
