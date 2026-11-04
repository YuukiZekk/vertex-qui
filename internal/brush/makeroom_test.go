// Copyright (c) 2026, vertex-qui contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package brush

import (
	"testing"
	"time"

	qbt "github.com/autobrr/go-qbittorrent"
	"github.com/stretchr/testify/require"

	"github.com/autobrr/qui/internal/brush/model"
	"github.com/autobrr/qui/internal/brush/site"
	"github.com/autobrr/qui/internal/brush/store"
	"github.com/autobrr/qui/internal/models"
	"github.com/autobrr/qui/internal/testutil/testdb"
)

func TestPlanRoom(t *testing.T) {
	c := candidate{InstanceID: 1, Reason: reasonLowSpace, Committed: 25 * gib, Floor: 20 * gib}
	torrents := []torrentValue{
		valued("busy", 4*gib, 2*time.Hour, ptr(900.0)),
		valued("young", 4*gib, 10*time.Minute, ptr(1.0)),
		valued("idle", 4*gib, 3*time.Hour, ptr(10.0)),
		valued("slow", 4*gib, time.Hour, ptr(50.0)),
	}
	// 10 GiB to place, 5 above the floor: idle and slow free 8.
	p, ok := planRoom(c, 10*gib, 1000, torrents)
	require.True(t, ok)
	require.Equal(t, []string{"idle", "slow"}, hashes(p.picks))
	require.Equal(t, 8*gib, p.freed)

	// Busy is worth more than 80% of a new torrent and young is protected: not enough.
	_, ok = planRoom(c, 20*gib, 1000, torrents)
	require.False(t, ok)
}

// When no instance has the space, admission deletes torrents worth clearly less
// than the new one on the instance that loses the least, and takes the torrent.
func TestAdmitMakesRoom(t *testing.T) {
	ctx := t.Context()
	db := testdb.NewMigratedSQLite(t, t.Name())
	cipher, err := models.NewCredentialCipher(make([]byte, 32))
	require.NoError(t, err)
	now := time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)
	idle := brushTorrent("idle", 0, now.Add(-3*time.Hour))
	idle.ContentPath = "/data/idle"
	qb := &fakeQB{free: map[int]int64{1: 25 * gib}, listed: map[int][]qbt.Torrent{1: {idle}}}
	svc, err := New(ctx, db, cipher, qb)
	require.NoError(t, err)
	svc.now = func() time.Time { return now }
	require.NoError(t, svc.store.SetFloor(ctx, 1, 20*gib))

	// New torrents here upload about 1 GiB per GiB over a horizon.
	for i := range minYieldChecks {
		require.NoError(t, svc.store.InsertPrediction(ctx, store.Prediction{InstanceID: 1, InfoHash: string(rune('a' + i)),
			MadeAt: now.Add(-time.Hour).Unix(), AgeSeconds: 600, SizeBytes: gib, HorizonSeconds: 1800}))
		var id int64
		require.NoError(t, db.QueryRowContext(ctx, "SELECT MAX(id) FROM brush_predictions").Scan(&id))
		require.NoError(t, svc.store.ResolvePrediction(ctx, id, now, gib, 1800, store.OutcomeResolved))
	}
	// idle has a full window of readings with no upload.
	for at := now.Add(-model.RateWindow); !at.After(now); at = at.Add(defaultReadInterval) {
		svc.values.observe(1, at, []qbt.Torrent{idle})
	}

	siteID, err := svc.store.CreateSite(ctx, store.Site{Name: "pt", BaseURL: "https://pt.example"})
	require.NoError(t, err)
	feed, err := svc.store.CreateFeed(ctx, store.Feed{SiteID: siteID, Name: "a", URL: "https://pt.example/a",
		IntervalSeconds: 60, WindowSeconds: 300, InstanceIDs: []int{1}})
	require.NoError(t, err)
	f, err := svc.store.GetFeed(ctx, feed)
	require.NoError(t, err)
	entry, _, err := svc.store.InsertEntry(ctx, store.Entry{FeedID: feed, GUID: "new", Title: "new", Status: store.StatusPending})
	require.NoError(t, err)

	// 8 GiB to place with 5 above the floor: deleting idle's 4 makes 9.
	instanceID, code, _ := svc.admit(ctx, f, entry, site.TorrentFile{InfoHash: "new", Size: 8 * gib})
	require.Empty(t, code)
	require.Equal(t, 1, instanceID)
	require.Equal(t, []fakeDeletion{{1, "idle", true}}, qb.deleted)
	require.Empty(t, qb.reannounced, "no announce wait while a torrent races")
	deletions, err := svc.store.RecentReaps(ctx, "", 10)
	require.NoError(t, err)
	require.Len(t, deletions, 1)
	require.Equal(t, store.ReapMakeRoom, deletions[0].Reason)
}
