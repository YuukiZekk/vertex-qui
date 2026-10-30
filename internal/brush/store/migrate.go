// Copyright (c) 2026, vertex-qui contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

// Package store holds the brush tables. They use their own migration ledger
// (brush_schema_migrations) so brush migrations never take a number in qui's
// sequential migrations and an upstream merge cannot collide with them.
package store

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"time"

	"github.com/autobrr/qui/internal/dbinterface"
)

//go:embed migrations
var migrationsFS embed.FS

const createLedger = `CREATE TABLE IF NOT EXISTS brush_schema_migrations (
	filename TEXT PRIMARY KEY,
	applied_at BIGINT NOT NULL
)`

// Migrate applies the pending brush migrations for the database's dialect, one
// transaction per file. Each file holds a single statement so both drivers run it.
func Migrate(ctx context.Context, db dbinterface.Querier) error {
	if _, err := db.ExecContext(ctx, createLedger); err != nil {
		return fmt.Errorf("create brush migration ledger: %w", err)
	}

	files, err := migrationFiles(dbinterface.DialectOf(db))
	if err != nil {
		return err
	}
	for _, file := range files {
		if err := applyMigration(ctx, db, file); err != nil {
			return err
		}
	}
	return nil
}

func migrationFiles(dialect string) ([]string, error) {
	dir := path.Join("migrations", dialect)
	entries, err := fs.ReadDir(migrationsFS, dir)
	if err != nil {
		return nil, fmt.Errorf("read brush migrations for %s: %w", dialect, err)
	}
	files := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() && path.Ext(entry.Name()) == ".sql" {
			files = append(files, path.Join(dir, entry.Name()))
		}
	}
	sort.Strings(files)
	return files, nil
}

func applyMigration(ctx context.Context, db dbinterface.Querier, file string) error {
	name := path.Base(file)
	var applied int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM brush_schema_migrations WHERE filename = ?", name).Scan(&applied); err != nil {
		return fmt.Errorf("check brush migration %s: %w", name, err)
	}
	if applied > 0 {
		return nil
	}

	content, err := migrationsFS.ReadFile(file)
	if err != nil {
		return fmt.Errorf("read brush migration %s: %w", name, err)
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin brush migration %s: %w", name, err)
	}
	if _, err := tx.ExecContext(ctx, string(content)); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("apply brush migration %s: %w", name, err)
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO brush_schema_migrations (filename, applied_at) VALUES (?, ?)", name, time.Now().Unix()); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("record brush migration %s: %w", name, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit brush migration %s: %w", name, err)
	}
	return nil
}
