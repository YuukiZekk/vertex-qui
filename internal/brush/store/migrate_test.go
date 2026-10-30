// Copyright (c) 2026, vertex-qui contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package store

import (
	"path"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/autobrr/qui/internal/database"
	"github.com/autobrr/qui/internal/testutil/testdb"
)

func TestMigrateIsIdempotentAndEnabledDefaultsOff(t *testing.T) {
	for name, open := range map[string]func(*testing.T) *database.DB{
		"sqlite":   func(t *testing.T) *database.DB { return testdb.NewMigratedSQLite(t, t.Name()) },
		"postgres": func(t *testing.T) *database.DB { return testdb.NewMigratedPostgres(t, t.Name()) },
	} {
		t.Run(name, func(t *testing.T) {
			db := open(t)
			ctx := t.Context()

			require.NoError(t, Migrate(ctx, db))
			require.NoError(t, Migrate(ctx, db))

			var applied int
			require.NoError(t, db.QueryRowContext(ctx, "SELECT COUNT(*) FROM brush_schema_migrations").Scan(&applied))
			files, err := migrationFiles(name)
			require.NoError(t, err)
			require.Equal(t, len(files), applied)

			s := New(db)
			enabled, err := s.Enabled(ctx)
			require.NoError(t, err)
			require.False(t, enabled)

			_, err = db.ExecContext(ctx, "INSERT INTO brush_settings (key, value) VALUES (?, ?)", keyEnabled, "true")
			require.NoError(t, err)
			enabled, err = s.Enabled(ctx)
			require.NoError(t, err)
			require.True(t, enabled)
		})
	}
}

func TestMigrationFilesMatchAcrossDialects(t *testing.T) {
	sqlite, err := migrationFiles("sqlite")
	require.NoError(t, err)
	postgres, err := migrationFiles("postgres")
	require.NoError(t, err)
	require.NotEmpty(t, sqlite)

	names := func(files []string) []string {
		out := make([]string, len(files))
		for i, f := range files {
			out[i] = path.Base(f)
		}
		return out
	}
	require.Equal(t, names(sqlite), names(postgres))
}
