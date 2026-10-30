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

func brushTorrent(hash string, uploaded int64, addedOn time.Time) qbt.Torrent {
	return qbt.Torrent{Hash: hash, Name: hash, Tags: "other, " + Tag, Size: 4 << 30, Uploaded: uploaded, AddedOn: addedOn.Unix()}
}

// A torrent's value needs a full window of readings, uses the change in cumulative
// upload over it, and ignores torrents brush did not add.
func TestValueTrackerPredictsFromAFullWindow(t *testing.T) {
	v := newValueTracker()
	start := time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)
	added := start.Add(-time.Hour)
	other := qbt.Torrent{Hash: "x", Tags: "vertex-qui-old", Uploaded: 1 << 40}

	var at time.Time
	for i := 0; at.Before(start.Add(model.RateWindow)); i++ {
		at = start.Add(time.Duration(i) * sampleInterval)
		// 10 MiB/s steady.
		v.observe(1, at, []qbt.Torrent{brushTorrent("a", int64(at.Sub(start).Seconds())*(10<<20), added), other})
		if at.Before(start.Add(model.RateWindow - sampleInterval)) {
			vals := v.values(at)
			require.Len(t, vals, 1, "only brush's torrents")
			require.Nil(t, vals[0].Predicted, "not a full window yet at %s", at.Sub(start))
		}
	}
	vals := v.values(at)
	require.Len(t, vals, 1)
	require.NotNil(t, vals[0].Rate)
	require.Equal(t, int64(10<<20), *vals[0].Rate)
	want := model.Predict(model.Input{Age: at.Sub(added), Rate: 10 << 20}).Bytes
	require.Equal(t, want, *vals[0].Predicted)
	require.InDelta(t, float64(want)/4, *vals[0].Density, 1, "per GiB of a 4 GiB torrent")

	// Readings older than the window are dropped but one stays as its start.
	v.mu.Lock()
	points := v.torrents[1]["a"].points
	v.mu.Unlock()
	require.False(t, points[0].at.After(at.Add(-model.RateWindow)))
	require.True(t, points[1].at.After(at.Add(-model.RateWindow)))
}

// Stored predictions are checked once their horizon ends: against the torrent's
// upload when it is still there, as gone when the instance no longer lists it, and
// given up when the instance cannot be read for too long.
func TestPredictionsAreRecordedAndResolved(t *testing.T) {
	ctx := t.Context()
	db := testdb.NewMigratedSQLite(t, t.Name())
	cipher, err := models.NewCredentialCipher(make([]byte, 32))
	require.NoError(t, err)
	svc, err := New(ctx, db, cipher, &fakeQB{})
	require.NoError(t, err)

	start := time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)
	added := start.Add(-time.Hour)
	uploaded := func(at time.Time) int64 { return int64(at.Sub(start).Seconds()) * (10 << 20) }
	read := func(at time.Time, readTwo bool, torrents ...qbt.Torrent) {
		svc.values.observe(1, at, torrents)
		if readTwo {
			svc.values.observe(2, at, []qbt.Torrent{brushTorrent("c", uploaded(at), added)})
		}
		svc.resolvePredictions(ctx, at)
		svc.recordPredictions(ctx, at)
	}

	at := start
	for ; !at.After(start.Add(model.RateWindow)); at = at.Add(sampleInterval) {
		read(at, true, brushTorrent("a", uploaded(at), added), brushTorrent("b", uploaded(at), added))
	}
	made := at.Add(-sampleInterval)
	acc, err := svc.store.PredictionAccuracy(ctx, start)
	require.NoError(t, err)
	require.Zero(t, acc.Count, "nothing is due yet")

	// b disappears, and instance 2 is no longer read.
	end := made.Add(model.Horizon)
	for ; !at.After(end); at = at.Add(sampleInterval) {
		read(at, false, brushTorrent("a", uploaded(at), added))
	}
	var pending int
	require.NoError(t, db.QueryRowContext(ctx, "SELECT COUNT(*) FROM brush_predictions WHERE info_hash = 'c' AND resolved_at = 0").Scan(&pending))
	require.Equal(t, 1, pending, "c waits for its instance")

	acc, err = svc.store.PredictionAccuracy(ctx, start)
	require.NoError(t, err)
	require.Equal(t, int64(1), acc.Count, "a resolved")
	require.Equal(t, int64(1), acc.Gone, "b gone")
	recent, err := svc.store.RecentResolved(ctx, 10)
	require.NoError(t, err)
	require.Len(t, recent, 1)
	require.Equal(t, "a", recent[0].InfoHash)
	require.Equal(t, store.OutcomeResolved, recent[0].Outcome)
	require.Equal(t, uploaded(time.Unix(recent[0].ResolvedAt, 0))-uploaded(made), recent[0].Actual)
	require.Equal(t, model.Predict(model.Input{Age: made.Sub(added), Rate: 10 << 20}).Bytes, recent[0].Predicted)
	require.GreaterOrEqual(t, recent[0].ActualSeconds, int64(model.Horizon/time.Second))

	for ; !at.After(end.Add(lostAfter + sampleInterval)); at = at.Add(sampleInterval) {
		read(at, false)
	}
	var outcome string
	require.NoError(t, db.QueryRowContext(ctx, "SELECT outcome FROM brush_predictions WHERE info_hash = 'c'").Scan(&outcome))
	require.Equal(t, store.OutcomeLost, outcome)
}
