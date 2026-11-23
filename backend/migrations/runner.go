package migrations

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"strings"

	"github.com/logoes0/peeriodic.git/logger"
)

//go:embed *.sql
var MigrationFS embed.FS

type Runner struct{ db *sql.DB }

func NewRunner(db *sql.DB) *Runner { return &Runner{db: db} }

// ApplyAll serializes concurrent startup migrations and commits schema plus tracking together.
func (r *Runner) ApplyAll(ctx context.Context) error {
	entries, err := fs.ReadDir(MigrationFS, ".")
	if err != nil {
		return err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(1819301)`); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (version VARCHAR(255) PRIMARY KEY, applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW())`); err != nil {
		return err
	}
	// ReadDir returns filenames in sorted order.
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		filename := entry.Name()
		var applied bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version = $1)`, filename).Scan(&applied); err != nil {
			return err
		}
		if applied {
			continue
		}
		content, err := MigrationFS.ReadFile(filename)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, string(content)); err != nil {
			return fmt.Errorf("migration %s: %w", filename, err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations (version) VALUES ($1)`, filename); err != nil {
			return err
		}
		logger.Info("Applied migration", "version", filename)
	}
	return tx.Commit()
}
