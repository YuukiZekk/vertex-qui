// Copyright (c) 2026, vertex-qui contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package brush

import (
	"testing"
	"time"

	qbt "github.com/autobrr/go-qbittorrent"
	"github.com/stretchr/testify/require"

	"github.com/autobrr/qui/internal/brush/model"
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

// In preview the reaper records what it would delete, counts the rounds a pick
// stays picked, and measures what the torrent uploads for one horizon after.
func TestReapPreviewRecordsAndMeasures(t *testing.T) {
	ctx := t.Context()
	db := testdb.NewMigratedSQLite(t, t.Name())
	cipher, err := models.NewCredentialCipher(make([]byte, 32))
	require.NoError(t, err)
	qb := &fakeQB{free: map[int]int64{1: 14 * gib}}
	svc, err := New(ctx, db, cipher, qb)
	require.NoError(t, err)
	require.NoError(t, svc.store.SetFloor(ctx, 1, 20*gib))

	start := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	added := start.Add(-2 * time.Hour)
	at := start
	svc.now = func() time.Time { return at }
	// slow uploads 1 MiB/s and fast 10 MiB/s; fresh arrives later and has no value yet.
	read := func() {
		el := int64(at.Sub(start).Seconds())
		torrents := []qbt.Torrent{
			brushTorrent("slow", el*(1<<20), added),
			brushTorrent("fast", el*(10<<20), added),
		}
		if at.After(start.Add(model.RateWindow / 2)) {
			torrents = append(torrents, brushTorrent("fresh", 0, at))
		}
		svc.values.observe(1, at, torrents)
	}
	for ; !at.After(start.Add(model.RateWindow)); at = at.Add(sampleInterval) {
		read()
	}
	at = at.Add(-sampleInterval)

	// 6 GiB below the floor: slow then fast, 4 GiB each.
	svc.reap(ctx)
	state := svc.reaper.states[1]
	require.Equal(t, reapEmergency, state.Mode)
	require.Equal(t, 6*gib, state.Shortfall)
	require.Equal(t, 3, state.Torrents)
	require.Equal(t, 1, state.Unknown)
	require.Len(t, state.Picks, 2)
	require.Equal(t, "slow", state.Picks[0].Hash)
	require.Equal(t, "fast", state.Picks[1].Hash)
	first := at

	at = at.Add(reapInterval)
	read()
	svc.reap(ctx)
	reaps, err := svc.store.RecentReaps(ctx, 10)
	require.NoError(t, err)
	require.Len(t, reaps, 2, "a pick still inside its horizon is not recorded twice")
	for _, r := range reaps {
		require.Equal(t, 2, r.Rounds)
		require.Equal(t, store.ReapEmergency, r.Reason)
		require.False(t, r.Executed)
	}

	// Space comes back; the picks are still measured until their horizon ends.
	qb.free[1] = 100 * gib
	for at = at.Add(sampleInterval); !at.After(first.Add(model.Horizon)); at = at.Add(sampleInterval) {
		read()
		if at.Sub(first)%reapInterval == 0 {
			svc.reap(ctx)
		}
	}
	require.Equal(t, reapOK, svc.reaper.states[1].Mode)
	require.Empty(t, svc.reaper.open, "every pick closed after its horizon")
	reaps, err = svc.store.RecentReaps(ctx, 10)
	require.NoError(t, err)
	require.Len(t, reaps, 2)
	for _, r := range reaps {
		rate := int64(1 << 20)
		if r.InfoHash == "fast" {
			rate = 10 << 20
		}
		require.Equal(t, int64(model.Horizon/time.Second)*rate, r.UploadedLast-r.UploadedFirst, r.InfoHash)
		require.Equal(t, 2, r.Rounds)
		require.False(t, r.Gone)
	}
}
