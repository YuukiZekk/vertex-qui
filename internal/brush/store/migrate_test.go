// Copyright (c) 2026, vertex-qui contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package store

import (
	"path"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/autobrr/qui/internal/database"
	"github.com/autobrr/qui/internal/models"
	"github.com/autobrr/qui/internal/testutil/testdb"
)

func dialects() map[string]func(*testing.T) *database.DB {
	return map[string]func(*testing.T) *database.DB{
		"sqlite":   func(t *testing.T) *database.DB { return testdb.NewMigratedSQLite(t, t.Name()) },
		"postgres": func(t *testing.T) *database.DB { return testdb.NewMigratedPostgres(t, t.Name()) },
	}
}

func newTestStore(t *testing.T, db *database.DB) *Store {
	t.Helper()
	require.NoError(t, Migrate(t.Context(), db))
	cipher, err := models.NewCredentialCipher(make([]byte, 32))
	require.NoError(t, err)
	return New(db, cipher)
}

func TestMigrateIsIdempotentAndEnabledDefaultsOff(t *testing.T) {
	for name, open := range dialects() {
		t.Run(name, func(t *testing.T) {
			db := open(t)
			ctx := t.Context()
			s := newTestStore(t, db)
			require.NoError(t, Migrate(ctx, db))

			var applied int
			require.NoError(t, db.QueryRowContext(ctx, "SELECT COUNT(*) FROM brush_schema_migrations").Scan(&applied))
			files, err := migrationFiles(name)
			require.NoError(t, err)
			require.Equal(t, len(files), applied)

			enabled, err := s.Enabled(ctx)
			require.NoError(t, err)
			require.False(t, enabled)

			require.NoError(t, s.SetEnabled(ctx, true))
			require.NoError(t, s.SetEnabled(ctx, true))
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

func TestSecretsAreSealedAndKeptOnBlankUpdate(t *testing.T) {
	for name, open := range dialects() {
		t.Run(name, func(t *testing.T) {
			db := open(t)
			ctx := t.Context()
			s := newTestStore(t, db)

			siteID, err := s.CreateSite(ctx, Site{Name: "pt", BaseURL: "https://pt.example", Cookie: "c_secure_pass=abc"})
			require.NoError(t, err)
			feedID, err := s.CreateFeed(ctx, Feed{SiteID: siteID, Name: "all", URL: "https://pt.example/rss?passkey=secret",
				IntervalSeconds: 15, WindowSeconds: 300, InstanceID: 1, Enabled: true})
			require.NoError(t, err)

			var cookie, feedURL string
			require.NoError(t, db.QueryRowContext(ctx, "SELECT cookie_encrypted FROM brush_sites WHERE id = ?", siteID).Scan(&cookie))
			require.NoError(t, db.QueryRowContext(ctx, "SELECT url_encrypted FROM brush_feeds WHERE id = ?", feedID).Scan(&feedURL))
			require.False(t, strings.Contains(cookie, "abc"))
			require.False(t, strings.Contains(feedURL, "secret"))

			require.NoError(t, s.UpdateSite(ctx, Site{ID: siteID, Name: "pt2", BaseURL: "https://pt.example"}))
			require.NoError(t, s.UpdateFeed(ctx, Feed{ID: feedID, SiteID: siteID, Name: "all2", IntervalSeconds: 20,
				WindowSeconds: 300, InstanceID: 1}))

			site, err := s.GetSite(ctx, siteID)
			require.NoError(t, err)
			require.Equal(t, "c_secure_pass=abc", site.Cookie)
			feed, err := s.GetFeed(ctx, feedID)
			require.NoError(t, err)
			require.Equal(t, "https://pt.example/rss?passkey=secret", feed.URL)
			require.Equal(t, "all2", feed.Name)
			require.False(t, feed.Enabled)
		})
	}
}

func TestEntriesDedupeAndReservations(t *testing.T) {
	for name, open := range dialects() {
		t.Run(name, func(t *testing.T) {
			db := open(t)
			ctx := t.Context()
			s := newTestStore(t, db)

			siteID, err := s.CreateSite(ctx, Site{Name: "pt", BaseURL: "https://pt.example"})
			require.NoError(t, err)
			feedA, err := s.CreateFeed(ctx, Feed{SiteID: siteID, Name: "a", URL: "https://pt.example/a", IntervalSeconds: 15, WindowSeconds: 300, InstanceID: 1})
			require.NoError(t, err)
			feedB, err := s.CreateFeed(ctx, Feed{SiteID: siteID, Name: "b", URL: "https://pt.example/b", IntervalSeconds: 15, WindowSeconds: 300, InstanceID: 1})
			require.NoError(t, err)

			idA, inserted, err := s.InsertEntry(ctx, Entry{FeedID: feedA, GUID: "g1", Title: "t", Status: StatusPending})
			require.NoError(t, err)
			require.True(t, inserted)
			_, inserted, err = s.InsertEntry(ctx, Entry{FeedID: feedA, GUID: "g1", Title: "t", Status: StatusPending})
			require.NoError(t, err)
			require.False(t, inserted, "a feed sees each guid once")
			idB, _, err := s.InsertEntry(ctx, Entry{FeedID: feedB, GUID: "g1", Title: "t", Status: StatusPending})
			require.NoError(t, err)

			require.NoError(t, s.Claim(ctx, idA, "hash1", 10, 1))
			taken, err := s.HashTaken(ctx, "hash1", 1)
			require.NoError(t, err)
			require.True(t, taken)
			require.Error(t, s.Claim(ctx, idB, "hash1", 10, 1), "the unique index stops a second add of the same torrent to one instance")

			res, err := s.Reservations(ctx, 1, time.Now().Add(-time.Minute))
			require.NoError(t, err)
			require.Equal(t, []Reservation{{InfoHash: "hash1", SizeBytes: 10}}, res)

			require.NoError(t, s.MarkInterruptedAdds(ctx))
			res, err = s.Reservations(ctx, 1, time.Now().Add(-time.Minute))
			require.NoError(t, err)
			require.Empty(t, res)
			entries, err := s.RecentEntries(ctx, 10)
			require.NoError(t, err)
			require.Equal(t, StatusUnknown, entries[len(entries)-1].Status)
		})
	}
}
