// Package backup creates and restores zip archives of SavedArks, the shared cluster directory, and the server INI files.
package backup

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Scope identifies what a backup row belongs to.
type Scope struct {
	Type string // "server" or "cluster"
	ID   int64
}

// Backup is one persisted backup row pointing at a zip on disk.
type Backup struct {
	ID        int64
	Scope     Scope
	Path      string // absolute path to the .zip
	SizeBytes int64
	Kind      string // "manual" / "scheduled" / "pre_restore"
	CreatedAt time.Time
}

type Repo struct{ db *sql.DB }

func NewRepo(db *sql.DB) *Repo { return &Repo{db: db} }

var ErrNotFound = errors.New("backup not found")

func (r *Repo) Create(ctx context.Context, b Backup) (Backup, error) {
	var (
		res sql.Result
		err error
	)
	if b.CreatedAt.IsZero() {
		res, err = r.db.ExecContext(ctx,
			`INSERT INTO backups(scope_type, scope_id, path, size_bytes, kind) VALUES(?,?,?,?,?)`,
			b.Scope.Type, b.Scope.ID, b.Path, b.SizeBytes, b.Kind)
	} else {
		res, err = r.db.ExecContext(ctx,
			`INSERT INTO backups(scope_type, scope_id, path, size_bytes, kind, created_at) VALUES(?,?,?,?,?,?)`,
			b.Scope.Type, b.Scope.ID, b.Path, b.SizeBytes, b.Kind, b.CreatedAt)
	}
	if err != nil {
		return Backup{}, fmt.Errorf("insert backup: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Backup{}, err
	}
	out, err := r.Get(ctx, id)
	if err != nil {
		return Backup{}, err
	}
	return out, nil
}

func (r *Repo) Get(ctx context.Context, id int64) (Backup, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, scope_type, scope_id, path, size_bytes, kind, created_at
		 FROM backups WHERE id = ?`, id)
	return scanBackup(row)
}

func (r *Repo) List(ctx context.Context, scope Scope) ([]Backup, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, scope_type, scope_id, path, size_bytes, kind, created_at
		 FROM backups WHERE scope_type = ? AND scope_id = ?
		 ORDER BY created_at DESC, id DESC`, scope.Type, scope.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Backup
	for rows.Next() {
		b, err := scanBackup(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func (r *Repo) ListAll(ctx context.Context) ([]Backup, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, scope_type, scope_id, path, size_bytes, kind, created_at
		 FROM backups ORDER BY created_at DESC, id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Backup
	for rows.Next() {
		b, err := scanBackup(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func (r *Repo) Delete(ctx context.Context, id int64) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM backups WHERE id = ?`, id)
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

func scanBackup(row rowScanner) (Backup, error) {
	var b Backup
	err := row.Scan(&b.ID, &b.Scope.Type, &b.Scope.ID, &b.Path, &b.SizeBytes, &b.Kind, &b.CreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Backup{}, ErrNotFound
		}
		return Backup{}, err
	}
	return b, nil
}
