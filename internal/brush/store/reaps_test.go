// Copyright (c) 2026, vertex-qui contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package store

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestReapsRoundTrip(t *testing.T) {
	for name, open := range dialects() {
		t.Run(name, func(t *testing.T) {
			db := open(t)
			ctx := t.Context()
			s := newTestStore(t, db)

			at := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
			density, replacement := 12.5, 40.0
			predicted := int64(1 << 20)
			_, err := s.InsertReap(ctx, Reap{InstanceID: 1, InfoHash: "a", Name: "A", SizeBytes: 4 << 30,
				Reason: ReapNeedSpace, DeletedAt: at.Unix(), AgeSeconds: 3600, Predicted: &predicted, Density: &density,
				Replacement: &replacement, Committed: 10, Need: 20, FloorBytes: 30, Uploaded: 100, Ratio: 1.5,
				FilesKept: true})
			require.NoError(t, err)
			// An emergency deletion of a torrent whose value is unknown stores no value.
			_, err = s.InsertReap(ctx, Reap{InstanceID: 1, InfoHash: "b", Name: "B", Reason: ReapEmergency,
				DeletedAt: at.Add(time.Minute).Unix()})
			require.NoError(t, err)
			// A row the preview wrote is not a deletion.
			_, err = db.ExecContext(ctx, `INSERT INTO brush_reaps (instance_id, info_hash, name, size_bytes, reason, first_at,
				last_at, rounds, age_seconds, committed, need, floor_bytes, uploaded_first, uploaded_last, measured_at)
				VALUES (1, 'p', 'P', 0, 'need_space', ?, 0, 1, 0, 0, 0, 0, 0, 0, 0)`, at.Add(2*time.Minute).Unix())
			require.NoError(t, err)

			reaps, err := s.RecentReaps(ctx, "", 10)
			require.NoError(t, err)
			require.Len(t, reaps, 2)
			require.Equal(t, "b", reaps[0].InfoHash, "newest first")
			require.Nil(t, reaps[0].Predicted)
			require.Nil(t, reaps[0].Density)
			require.Nil(t, reaps[0].Replacement)
			a := reaps[1]
			require.Equal(t, int64(100), a.Uploaded)
			require.InDelta(t, 1.5, a.Ratio, 1e-9)
			require.True(t, a.FilesKept)
			require.Equal(t, predicted, *a.Predicted)
			require.InDelta(t, density, *a.Density, 1e-9)
			require.InDelta(t, replacement, *a.Replacement, 1e-9)

			found, err := s.RecentReaps(ctx, " a ", 10)
			require.NoError(t, err)
			require.Len(t, found, 1, "search ignores case and surrounding spaces")
			require.Equal(t, "a", found[0].InfoHash)
			found, err = s.RecentReaps(ctx, "%", 10)
			require.NoError(t, err)
			require.Empty(t, found, "a LIKE wildcard in the search is taken literally")

			require.NoError(t, s.PruneReaps(ctx, at.Add(reapRetention+30*time.Second)))
			reaps, err = s.RecentReaps(ctx, "", 10)
			require.NoError(t, err)
			require.Len(t, reaps, 1, "only the older one is past retention")
		})
	}
}

// Arrivals count what passed a feed's filters, added or refused only for space, in
// the window; the feed's first entry ever bounds a young feed's span.
func TestArrivalsAndYield(t *testing.T) {
	for name, open := range dialects() {
		t.Run(name, func(t *testing.T) {
			db := open(t)
			ctx := t.Context()
			s := newTestStore(t, db)

			siteID, err := s.CreateSite(ctx, Site{Name: "pt", BaseURL: "https://pt.example"})
			require.NoError(t, err)
			feedID, err := s.CreateFeed(ctx, Feed{SiteID: siteID, Name: "a", URL: "https://pt.example/a",
				IntervalSeconds: 15, WindowSeconds: 300, InstanceIDs: []int{1}})
			require.NoError(t, err)
			idle, err := s.CreateFeed(ctx, Feed{SiteID: siteID, Name: "idle", URL: "https://pt.example/b",
				IntervalSeconds: 15, WindowSeconds: 300, InstanceIDs: []int{1}})
			require.NoError(t, err)

			now := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
			since := now.Add(-24 * time.Hour)
			insert := func(guid string, seen time.Time, size int64, status, reason string) {
				_, _, err := s.InsertEntry(ctx, Entry{FeedID: feedID, GUID: guid, Title: guid, SizeBytes: size,
					FirstSeenAt: seen.Unix(), Status: status, Reason: reason})
				require.NoError(t, err)
			}
			// Only one "added" row may lack a hash, so the old one is an interrupted add.
			insert("old", since.Add(-time.Hour), 1, StatusUnknown, "interrupted")
			insert("added", now.Add(-time.Hour), 10, StatusAdded, "")
			insert("space", now.Add(-time.Hour), 20, StatusRejected, "low_space: 1 < 2")
			insert("small", now.Add(-time.Hour), 40, StatusRejected, "size")
			insert("base", now.Add(-time.Hour), 80, StatusBaseline, "")

			arrivals, err := s.Arrivals(ctx, since)
			require.NoError(t, err)
			require.Equal(t, []Arrival{
				{FeedID: feedID, Count: 2, Bytes: 30, FirstSeen: since.Add(-time.Hour).Unix()},
				{FeedID: idle},
			}, arrivals)

			for i, p := range []Prediction{
				{InstanceID: 1, AgeSeconds: 900, SizeBytes: 1 << 30, Actual: 300, Outcome: OutcomeResolved},
				{InstanceID: 1, AgeSeconds: 900, SizeBytes: 1 << 30, Actual: 100, Outcome: OutcomeResolved},
				{InstanceID: 1, AgeSeconds: 7200, SizeBytes: 1 << 30, Actual: 999, Outcome: OutcomeResolved},
				{InstanceID: 1, AgeSeconds: 900, SizeBytes: 1 << 30, Outcome: OutcomeGone},
				{InstanceID: 2, AgeSeconds: 600, SizeBytes: 2 << 30, Actual: 50, Outcome: OutcomeResolved},
			} {
				p.InfoHash, p.MadeAt, p.HorizonSeconds = string(rune('a'+i)), now.Add(-time.Hour).Unix(), 1800
				require.NoError(t, s.InsertPrediction(ctx, p))
				var id int64
				require.NoError(t, db.QueryRowContext(ctx, "SELECT MAX(id) FROM brush_predictions").Scan(&id))
				require.NoError(t, s.ResolvePrediction(ctx, id, now, p.Actual, 1800, p.Outcome))
			}
			yields, err := s.NewTorrentYield(ctx, since, 30*time.Minute)
			require.NoError(t, err)
			require.Equal(t, map[int]Yield{1: {Count: 2, Actual: 400, Size: 2 << 30}, 2: {Count: 1, Actual: 50, Size: 2 << 30}}, yields)
		})
	}
}

func TestCountsSince(t *testing.T) {
	for name, open := range dialects() {
		t.Run(name, func(t *testing.T) {
			db := open(t)
			ctx := t.Context()
			s := newTestStore(t, db)
			siteID, err := s.CreateSite(ctx, Site{Name: "pt", BaseURL: "https://pt.example"})
			require.NoError(t, err)
			feed, err := s.CreateFeed(ctx, Feed{SiteID: siteID, Name: "a", URL: "https://pt.example/a", IntervalSeconds: 15, WindowSeconds: 300, InstanceIDs: []int{1}})
			require.NoError(t, err)
			midnight := time.Date(2026, 11, 3, 0, 0, 0, 0, time.UTC)
			for i, e := range []struct {
				status, reason string
				at             time.Time
			}{
				{StatusAdded, "", midnight.Add(time.Hour)},
				{StatusRejected, "low_space: 1 < 2", midnight.Add(time.Hour)},
				{StatusRejected, "window: 6m0s", midnight.Add(time.Hour)},
				{StatusRejected, "size", midnight.Add(time.Hour)},
				{StatusRejected, "low_space: 1 < 2", midnight.Add(-time.Hour)},
			} {
				_, _, err := s.InsertEntry(ctx, Entry{FeedID: feed, GUID: string(rune('a' + i)), Title: "t", Status: e.status, Reason: e.reason, DecidedAt: e.at.Unix()})
				require.NoError(t, err)
			}
			_, err = s.InsertReap(ctx, Reap{InstanceID: 1, InfoHash: "x", Name: "x", Reason: ReapNeedSpace, DeletedAt: midnight.Add(time.Hour).Unix()})
			require.NoError(t, err)

			c, err := s.CountsSince(ctx, midnight)
			require.NoError(t, err)
			require.Equal(t, TodayCounts{Added: 1, Deleted: 1, MissedSpace: 1, MissedWindow: 1}, c)
		})
	}
}
