// Copyright (c) 2026, vertex-qui contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package brush

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	qbt "github.com/autobrr/go-qbittorrent"
	"github.com/stretchr/testify/require"

	"github.com/autobrr/qui/internal/brush/store"
	"github.com/autobrr/qui/internal/models"
	"github.com/autobrr/qui/internal/testutil/testdb"
)

// racePoll runs a baseline poll, then a poll that lists the given torrents as new,
// and waits until every new entry is decided.
func racePoll(t *testing.T, qb *fakeQB, floors map[int]int64, capacity, siteLimit int64, feedInstance int, torrents ...fakeTorrent) []store.Entry {
	t.Helper()
	ctx := t.Context()
	db := testdb.NewMigratedSQLite(t, t.Name())
	cipher, err := models.NewCredentialCipher(make([]byte, 32))
	require.NoError(t, err)
	svc, err := New(ctx, db, cipher, qb)
	require.NoError(t, err)

	fs := &fakeSite{files: map[int][]byte{}}
	srv := httptest.NewServer(fs)
	t.Cleanup(srv.Close)

	siteID, err := svc.store.CreateSite(ctx, store.Site{Name: "pt", BaseURL: srv.URL, Cookie: "c=1", TorrentUploadLimit: siteLimit})
	require.NoError(t, err)
	feedID, err := svc.store.CreateFeed(ctx, store.Feed{SiteID: siteID, Name: "all", URL: srv.URL + "/rss.php",
		IntervalSeconds: 60, MinSizeBytes: gib, MaxSizeBytes: 100 * gib, FreeOnly: true, WindowSeconds: 600,
		InstanceID: feedInstance, Enabled: true})
	require.NoError(t, err)
	for id, floor := range floors {
		require.NoError(t, svc.store.SetFloor(ctx, id, floor))
		require.NoError(t, svc.store.SetUploadCapacity(ctx, id, capacity))
	}
	feed, err := svc.store.GetFeed(ctx, feedID)
	require.NoError(t, err)

	svc.poll(ctx, feed)
	fs.set(t, torrents...)
	svc.poll(ctx, feed)

	var entries []store.Entry
	require.Eventually(t, func() bool {
		entries, err = svc.store.RecentEntries(ctx, 50)
		require.NoError(t, err)
		for _, e := range entries {
			if e.Status == store.StatusPending || e.Status == store.StatusAdding {
				return false
			}
		}
		return len(entries) == len(torrents)
	}, 10*time.Second, 50*time.Millisecond)
	return entries
}

func freshTorrents(n int, size int64) []fakeTorrent {
	now := time.Now()
	out := make([]fakeTorrent, n)
	for i := range out {
		out[i] = fakeTorrent{id: i + 1, published: now.Add(-time.Duration(i+1) * time.Second), size: size, free: true}
	}
	return out
}

// A burst on two idle instances must not all land on one, even when one has far more
// space: each torrent sent counts as expected upload on its instance for the next one.
func TestAutoPlacementSpreadsBurstAcrossInstances(t *testing.T) {
	qb := &fakeQB{free: map[int]int64{1: 1000 * gib, 2: 500 * gib}}
	entries := racePoll(t, qb, map[int]int64{1: 50 * gib, 2: 50 * gib}, 250<<20, 0, 0, freshTorrents(4, 10*gib)...)

	for _, e := range entries {
		require.Equal(t, store.StatusAdded, e.Status, e.Reason)
		var p placement
		require.NoError(t, json.Unmarshal(e.Placement, &p))
		require.Equal(t, e.InstanceID, p.Chosen)
		require.Len(t, p.Candidates, 2)
	}
	require.Equal(t, map[int]int{1: 2, 2: 2}, qb.addedTo())
}

// A site's per-torrent upload limit goes on every torrent, and caps how much upload
// a fresh torrent is expected to take: with a low limit the burst no longer looks
// like it fills the roomier instance, so it all goes there.
func TestSiteUploadLimitCapsFreshPullAndIsApplied(t *testing.T) {
	qb := &fakeQB{free: map[int]int64{1: 1000 * gib, 2: 500 * gib}}
	entries := racePoll(t, qb, map[int]int64{1: 50 * gib, 2: 50 * gib}, 250<<20, 512<<10, 0, freshTorrents(4, 10*gib)...)

	for _, e := range entries {
		require.Equal(t, store.StatusAdded, e.Status, e.Reason)
	}
	require.Equal(t, map[int]int{1: 4}, qb.addedTo())
	require.Equal(t, []int64{512 << 10, 512 << 10, 512 << 10, 512 << 10}, qb.upLimits)
}

func TestFreshPull(t *testing.T) {
	fresh := []store.Reservation{{UploadLimit: 0}, {UploadLimit: 10 << 20}, {UploadLimit: 1 << 30}}
	// 25% of 400 MiB/s is 100 MiB/s: unlimited and 1 GiB/s take the share, 10 MiB/s its limit.
	require.Equal(t, int64(210<<20), freshPull(fresh, 400<<20))
}

// Without an upload capacity the instance with more spare space above its floor wins,
// and a torrent that fits nowhere is refused for low space.
func TestAutoPlacementFollowsSpareSpace(t *testing.T) {
	qb := &fakeQB{free: map[int]int64{1: 100 * gib, 2: 300 * gib}}
	entries := racePoll(t, qb, map[int]int64{1: 50 * gib, 2: 50 * gib}, 0, 0, 0,
		fakeTorrent{id: 1, published: time.Now().Add(-time.Second), size: 40 * gib, free: true})
	require.Equal(t, store.StatusAdded, entries[0].Status, entries[0].Reason)
	require.Equal(t, 2, entries[0].InstanceID)

	qb = &fakeQB{free: map[int]int64{1: 60 * gib, 2: 70 * gib}}
	entries = racePoll(t, qb, map[int]int64{1: 50 * gib, 2: 50 * gib}, 0, 0, 0,
		fakeTorrent{id: 1, published: time.Now().Add(-time.Second), size: 40 * gib, free: true})
	require.Equal(t, store.StatusRejected, entries[0].Status)
	require.True(t, strings.HasPrefix(entries[0].Reason, reasonLowSpace), entries[0].Reason)
	require.Zero(t, qb.addCount())
}

// A feed fixed to one instance is refused when that instance is full, never sent to
// another one with room.
func TestFixedInstanceIsNotRerouted(t *testing.T) {
	qb := &fakeQB{free: map[int]int64{1: 60 * gib, 2: 500 * gib}}
	entries := racePoll(t, qb, map[int]int64{1: 50 * gib, 2: 50 * gib}, 0, 0, 1,
		fakeTorrent{id: 1, published: time.Now().Add(-time.Second), size: 20 * gib, free: true})
	require.Equal(t, store.StatusRejected, entries[0].Status)
	require.True(t, strings.HasPrefix(entries[0].Reason, reasonLowSpace), entries[0].Reason)
	require.Zero(t, qb.addCount())
}

func TestUploadMeterAveragesCumulativeUpload(t *testing.T) {
	m := newUploadMeter()
	start := time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)
	_, ok := m.rate(1, start)
	require.False(t, ok, "no samples yet")

	m.observe(1, start, []qbt.Torrent{{Hash: "a", Uploaded: 0}, {Hash: "b", Uploaded: 100}})
	_, ok = m.rate(1, start)
	require.False(t, ok, "one read is not a rate")

	// a uploads 100 MiB in 10 s; b disappears and must not count as negative upload;
	// c is new and has no earlier value to compare against.
	m.observe(1, start.Add(10*time.Second), []qbt.Torrent{{Hash: "a", Uploaded: 100 << 20}, {Hash: "c", Uploaded: 1 << 30}})
	rate, ok := m.rate(1, start.Add(10*time.Second))
	require.True(t, ok)
	require.Equal(t, int64(10<<20), rate)

	m.observe(1, start.Add(20*time.Second), []qbt.Torrent{{Hash: "a", Uploaded: 100 << 20}, {Hash: "c", Uploaded: 1<<30 + 300<<20}})
	rate, _ = m.rate(1, start.Add(20*time.Second))
	require.Equal(t, int64(20<<20), rate, "400 MiB over 20 s")

	_, ok = m.rate(1, start.Add(20*time.Second+2*uploadWindow))
	require.False(t, ok, "a stale meter does not describe the instance now")
}
