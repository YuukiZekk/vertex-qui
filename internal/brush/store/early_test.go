// Copyright (c) 2026, vertex-qui contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package store

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// A feed keeps its early check, and AddedFeeds finds the feed of each torrent brush
// added since a time, by instance and lower-cased hash.
func TestEarlyCheckFeedsAndAddedTorrents(t *testing.T) {
	for name, open := range dialects() {
		t.Run(name, func(t *testing.T) {
			db := open(t)
			ctx := t.Context()
			s := newTestStore(t, db)

			siteID, err := s.CreateSite(ctx, Site{Name: "pt", BaseURL: "https://pt.example"})
			require.NoError(t, err)
			feedID, err := s.CreateFeed(ctx, Feed{SiteID: siteID, Name: "a", URL: "https://pt.example/a", IntervalSeconds: 60,
				WindowSeconds: 300, InstanceIDs: []int{1}, Enabled: true, EarlyCheckSeconds: 900, EarlyMinRatio: 0.02})
			require.NoError(t, err)
			f, err := s.GetFeed(ctx, feedID)
			require.NoError(t, err)
			require.Equal(t, 900, f.EarlyCheckSeconds)
			require.InDelta(t, 0.02, f.EarlyMinRatio, 1e-9)

			f.EarlyCheckSeconds, f.EarlyMinRatio = 0, 0
			require.NoError(t, s.UpdateFeed(ctx, f))
			f, err = s.GetFeed(ctx, feedID)
			require.NoError(t, err)
			require.Zero(t, f.EarlyCheckSeconds)

			since := time.Now().Add(-time.Minute)
			added, _, err := s.InsertEntry(ctx, Entry{FeedID: feedID, GUID: "g1", Title: "t", Status: StatusPending})
			require.NoError(t, err)
			require.NoError(t, s.Claim(ctx, added, "ABCDEF", 10, 1))
			require.NoError(t, s.MarkAdded(ctx, added))
			pending, _, err := s.InsertEntry(ctx, Entry{FeedID: feedID, GUID: "g2", Title: "t", Status: StatusPending})
			require.NoError(t, err)
			require.NoError(t, s.Claim(ctx, pending, "123456", 10, 1))

			got, err := s.AddedFeeds(ctx, since)
			require.NoError(t, err)
			require.Equal(t, map[AddedTorrent]int64{{InstanceID: 1, Hash: "abcdef"}: feedID}, got)

			got, err = s.AddedFeeds(ctx, time.Now().Add(time.Hour))
			require.NoError(t, err)
			require.Empty(t, got)
		})
	}
}
