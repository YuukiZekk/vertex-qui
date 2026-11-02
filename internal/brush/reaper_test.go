// Copyright (c) 2026, vertex-qui contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package brush

import (
	"testing"
	"time"

	qbt "github.com/autobrr/go-qbittorrent"
	"github.com/stretchr/testify/require"

	"github.com/autobrr/qui/internal/brush/store"
	"github.com/autobrr/qui/internal/models"
	"github.com/autobrr/qui/internal/testutil/testdb"
)

func valued(hash string, size int64, age time.Duration, density *float64) torrentValue {
	return torrentValue{Hash: hash, SizeBytes: size, AgeSeconds: int64(age / time.Second), Density: density}
}

func ptr[T any](v T) *T { return &v }

func hashes(picks []torrentValue) []string {
	out := make([]string, 0, len(picks))
	for _, p := range picks {
		out = append(out, p.Hash)
	}
	return out
}

func TestPickReaps(t *testing.T) {
	torrents := []torrentValue{
		valued("busy", 4*gib, 2*time.Hour, ptr(900.0)),
		valued("young", 4*gib, 10*time.Minute, ptr(1.0)),
		valued("unknown-new", 4*gib, 5*time.Minute, nil),
		valued("unknown-old", 4*gib, 20*time.Minute, nil),
		valued("idle", 4*gib, 3*time.Hour, ptr(10.0)),
		valued("slow", 4*gib, time.Hour, ptr(50.0)),
	}

	t.Run("plenty of space keeps everything", func(t *testing.T) {
		mode, shortfall, picks, _ := pickReaps(100*gib, 20*gib, 30*gib, 0, ptr(1000.0), torrents)
		require.Equal(t, reapOK, mode)
		require.Zero(t, shortfall)
		require.Empty(t, picks)
	})

	t.Run("a short dip is only watched", func(t *testing.T) {
		mode, _, picks, _ := pickReaps(40*gib, 20*gib, 30*gib, 1, ptr(1000.0), torrents)
		require.Equal(t, reapWatch, mode)
		require.Empty(t, picks)
	})

	t.Run("needed space takes the cheapest old torrents below the replacement", func(t *testing.T) {
		// Needs 10 GiB: idle and slow give 8, busy is worth more than 80% of 1000.
		mode, shortfall, picks, eligible := pickReaps(40*gib, 20*gib, 30*gib, needRounds, ptr(1000.0), torrents)
		require.Equal(t, reapNeedSpace, mode)
		require.Equal(t, 10*gib, shortfall)
		require.Equal(t, 2, eligible)
		require.Equal(t, []string{"idle", "slow"}, hashes(picks))

		// A cheap replacement protects slow too.
		_, _, picks, _ = pickReaps(40*gib, 20*gib, 30*gib, needRounds, ptr(40.0), torrents)
		require.Equal(t, []string{"idle"}, hashes(picks))

		// Stops once the shortfall is covered.
		_, _, picks, _ = pickReaps(48*gib, 20*gib, 30*gib, needRounds, ptr(1000.0), torrents)
		require.Equal(t, []string{"idle"}, hashes(picks))
	})

	t.Run("no replacement data deletes nothing for need", func(t *testing.T) {
		mode, _, picks, _ := pickReaps(40*gib, 20*gib, 30*gib, needRounds, nil, torrents)
		require.Equal(t, reapNeedSpace, mode)
		require.Empty(t, picks)
	})

	t.Run("below the floor everything is a candidate, unknown values last", func(t *testing.T) {
		mode, shortfall, picks, eligible := pickReaps(-2*gib, 20*gib, 30*gib, 0, nil, torrents)
		require.Equal(t, reapEmergency, mode)
		require.Equal(t, 22*gib, shortfall)
		require.Equal(t, len(torrents), eligible)
		require.Equal(t, []string{"young", "idle", "slow", "busy", "unknown-old", "unknown-new"}, hashes(picks))

		_, _, picks, _ = pickReaps(15*gib, 20*gib, 0, 0, nil, torrents)
		require.Equal(t, []string{"young", "idle"}, hashes(picks), "5 GiB short takes two 4 GiB torrents")
	})
}

func TestDemandAndReplacement(t *testing.T) {
	now := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	configs := map[int]store.InstanceConfig{1: {}, 2: {}}
	feeds := []store.Feed{
		{ID: 1, Enabled: true, InstanceIDs: []int{1, 2, 3}},
		{ID: 2, Enabled: true, InstanceIDs: []int{2}},
		{ID: 3, Enabled: false},
		{ID: 4, Enabled: true},
	}
	arrivals := []store.Arrival{
		// 48 torrents of 2 GiB a day, spread over instances 1 and 2 (3 has no floor).
		{FeedID: 1, Count: 48, Bytes: 96 * gib, FirstSeen: now.Add(-48 * time.Hour).Unix()},
		// Ten hours old: averaged over its ten hours.
		{FeedID: 2, Count: 10, Bytes: 10 * gib, FirstSeen: now.Add(-10 * time.Hour).Unix()},
		{FeedID: 3, Count: 100, Bytes: 100 * gib, FirstSeen: 1},
		// Too young to have a rate.
		{FeedID: 4, Count: 5, Bytes: 5 * gib, FirstSeen: now.Add(-30 * time.Minute).Unix()},
	}
	need, rate := demand(feeds, arrivals, configs, now)
	require.InDelta(t, float64(gib), float64(need[1]), 1e3, "2 GiB an hour, half of it, over half an hour")
	require.InDelta(t, float64(gib)+float64(gib)/2, float64(need[2]), 1e3)
	require.InDelta(t, 2.0/3600, rate[1], 1e-12)
	require.InDelta(t, 3.0/3600, rate[2], 1e-12)

	yields := map[int]store.Yield{
		1: {Count: 10, Actual: 10 * 200 << 20, Size: 10 * gib},
		2: {Count: 2, Actual: 100, Size: gib},
	}
	r := replacementFor(1, yields, rate[1])
	require.Equal(t, "instance", r.YieldFrom)
	require.InDelta(t, 200<<20, *r.Yield, 1)
	require.InDelta(t, 1-0.36787944, r.Probability, 1e-6, "one expected arrival in the lookahead")
	require.InDelta(t, *r.Yield*r.Probability, *r.Value, 1e-6)

	r = replacementFor(2, yields, rate[2])
	require.Equal(t, "all", r.YieldFrom, "too few checks on instance 2")
	require.InDelta(t, float64(10*200<<20+100)/11, *r.Yield, 1)

	r = replacementFor(1, map[int]store.Yield{1: {Count: 1, Actual: 1, Size: 1}}, rate[1])
	require.Nil(t, r.Value)
}

// The reaper deletes only while brushing is on. Below the floor it skips the
// announce and deletes the oldest unknown-value torrents first, keeps the files of
// one a cross-seed shares, and does not pick a deleted torrent again from a read
// that still lists it.
func TestReapDeletesBelowTheFloor(t *testing.T) {
	ctx := t.Context()
	db := testdb.NewMigratedSQLite(t, t.Name())
	cipher, err := models.NewCredentialCipher(make([]byte, 32))
	require.NoError(t, err)
	now := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	old := brushTorrent("old", 5<<30, now.Add(-3*time.Hour))
	old.ContentPath, old.Ratio = "/data/old", 1.25
	mid := brushTorrent("mid", 1<<30, now.Add(-2*time.Hour))
	mid.ContentPath = "/data/mid"
	young := brushTorrent("young", 0, now.Add(-time.Hour))
	young.ContentPath = "/data/young"
	xseed := qbt.Torrent{Hash: "xseed", Name: "old", ContentPath: "/data/old", Size: 4 << 30}
	qb := &fakeQB{free: map[int]int64{1: 14 * gib}, listed: map[int][]qbt.Torrent{1: {old, mid, young, xseed}}}
	svc, err := New(ctx, db, cipher, qb)
	require.NoError(t, err)
	svc.now = func() time.Time { return now }
	require.NoError(t, svc.store.SetFloor(ctx, 1, 20*gib))

	read := func() {
		listed, _ := qb.Torrents(ctx, 1)
		svc.values.observe(1, now, listed)
	}
	read()
	svc.reap(ctx)
	require.Empty(t, qb.deleted, "brushing is off")

	require.NoError(t, svc.store.SetEnabled(ctx, true))
	svc.reap(ctx)
	// 6 GiB short: old and mid, 4 GiB each.
	require.Equal(t, []fakeDeletion{{1, "old", false}, {1, "mid", true}}, qb.deleted)
	require.Empty(t, qb.reannounced, "an emergency does not wait for the announce")

	deletions, err := svc.store.RecentReaps(ctx, 10)
	require.NoError(t, err)
	require.Len(t, deletions, 2)
	byHash := map[string]store.Reap{}
	for _, d := range deletions {
		byHash[d.InfoHash] = d
		require.Equal(t, store.ReapEmergency, d.Reason)
		require.Equal(t, now.Unix(), d.DeletedAt)
	}
	require.True(t, byHash["old"].FilesKept)
	require.Equal(t, int64(5<<30), byHash["old"].Uploaded)
	require.InDelta(t, 1.25, byHash["old"].Ratio, 1e-9)
	require.False(t, byHash["mid"].FilesKept)

	// The value tracker has not read the instance again and still lists both.
	svc.reap(ctx)
	require.Len(t, qb.deleted, 2)
}

// Deleting for space announces the picks first.
func TestDeletePicksReannouncesFirst(t *testing.T) {
	ctx := t.Context()
	db := testdb.NewMigratedSQLite(t, t.Name())
	cipher, err := models.NewCredentialCipher(make([]byte, 32))
	require.NoError(t, err)
	now := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	a := brushTorrent("a", 1, now.Add(-time.Hour))
	qb := &fakeQB{free: map[int]int64{1: 100 * gib}, listed: map[int][]qbt.Torrent{1: {a}}}
	svc, err := New(ctx, db, cipher, qb)
	require.NoError(t, err)
	svc.now = func() time.Time { return now }
	old := reannounceWait
	reannounceWait = time.Millisecond
	defer func() { reannounceWait = old }()

	svc.deletePicks(ctx, reapState{InstanceID: 1, Mode: reapNeedSpace}, []torrentValue{{Hash: "a", SizeBytes: a.Size}, {Hash: "missing"}})
	require.Equal(t, []string{"a"}, qb.reannounced, "only torrents still listed")
	require.Equal(t, []fakeDeletion{{1, "a", true}}, qb.deleted)
}
