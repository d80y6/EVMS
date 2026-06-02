package common

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jmoiron/sqlx"
)

type Migrator struct {
	db     *sqlx.DB
	logger *slog.Logger
	dir    string
}

func NewMigrator(db *sqlx.DB, dir string, logger *slog.Logger) *Migrator {
	return &Migrator{db: db, logger: logger, dir: dir}
}

func (m *Migrator) Run() error {
	if err := m.ensureMigrationsTable(); err != nil {
		return err
	}

	files, err := m.discoverMigrations(".up.sql")
	if err != nil {
		return err
	}

	for _, f := range files {
		applied, err := m.isApplied(f)
		if err != nil {
			return err
		}
		if applied {
			continue
		}

		if err := m.applyMigration(f, ".up.sql", "INSERT INTO schema_migrations (version) VALUES ($1)"); err != nil {
			return err
		}
	}

	return nil
}

func (m *Migrator) Rollback(steps int) error {
	if err := m.ensureMigrationsTable(); err != nil {
		return err
	}

	applied, err := m.getAppliedMigrations()
	if err != nil {
		return err
	}

	if steps > 0 && steps < len(applied) {
		applied = applied[len(applied)-steps:]
	}

	for _, f := range applied {
		if err := m.applyMigration(f, ".down.sql", "DELETE FROM schema_migrations WHERE version = $1"); err != nil {
			return err
		}
		m.logger.Info("Migration rolled back", "version", f)
	}

	return nil
}

func (m *Migrator) ensureMigrationsTable() error {
	_, err := m.db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version TEXT PRIMARY KEY,
		applied_at TIMESTAMPTZ DEFAULT NOW()
	)`)
	if err != nil {
		return fmt.Errorf("create migrations table: %w", err)
	}
	return nil
}

func (m *Migrator) discoverMigrations(suffix string) ([]string, error) {
	entries, err := os.ReadDir(m.dir)
	if err != nil {
		return nil, fmt.Errorf("read migrations dir: %w", err)
	}

	var files []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), suffix) {
			files = append(files, e.Name())
		}
	}
	sort.Strings(files)

	// Deduplicate by removing the suffix to get base names
	seen := make(map[string]bool)
	var unique []string
	for _, f := range files {
		base := strings.TrimSuffix(f, suffix)
		if !seen[base] {
			seen[base] = true
			unique = append(unique, f)
		}
	}

	return unique, nil
}

func (m *Migrator) isApplied(version string) (bool, error) {
	base := strings.TrimSuffix(version, ".up.sql")
	var count int
	err := m.db.Get(&count, "SELECT COUNT(*) FROM schema_migrations WHERE version = $1", base)
	if err != nil {
		return false, fmt.Errorf("check migration %s: %w", version, err)
	}
	return count > 0, nil
}

func (m *Migrator) getAppliedMigrations() ([]string, error) {
	var versions []string
	err := m.db.Select(&versions, "SELECT version FROM schema_migrations ORDER BY applied_at ASC")
	if err != nil {
		return nil, fmt.Errorf("list applied migrations: %w", err)
	}
	return versions, nil
}

func (m *Migrator) applyMigration(fileName, suffix, recordSQL string) error {
	base := strings.TrimSuffix(fileName, suffix)
	sourceFile := fileName

	// For rollback, construct the .down.sql filename
	if strings.HasSuffix(fileName, ".down.sql") {
		sourceFile = fileName
	} else {
		sourceFile = base + suffix
	}

	path := filepath.Join(m.dir, sourceFile)
	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) && strings.HasSuffix(sourceFile, ".down.sql") {
			m.logger.Warn("No rollback SQL found, skipping", "version", base)
			// Still record the rollback in schema_migrations
			tx, err := m.db.Begin()
			if err != nil {
				return fmt.Errorf("begin tx for rollback %s: %w", base, err)
			}
			if _, err := tx.Exec(recordSQL, base); err != nil {
				tx.Rollback()
				return fmt.Errorf("record rollback %s: %w", base, err)
			}
			return tx.Commit()
		}
		return fmt.Errorf("read migration %s: %w", sourceFile, err)
	}

	tx, err := m.db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx for %s: %w", sourceFile, err)
	}

	if _, err := tx.Exec(string(content)); err != nil {
		tx.Rollback()
		return fmt.Errorf("apply migration %s: %w", sourceFile, err)
	}

	if _, err := tx.Exec(recordSQL, base); err != nil {
		tx.Rollback()
		return fmt.Errorf("record migration %s: %w", base, err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migration %s: %w", sourceFile, err)
	}

	m.logger.Info("Migration applied", "version", base)
	return nil
}
