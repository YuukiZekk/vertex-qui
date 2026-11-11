// Copyright (c) 2026, vertex-qui contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package brush

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/autobrr/qui/internal/brush/store"
)

func TestCalibrationFactors(t *testing.T) {
	got := calibrationFactors(map[store.CalibrationKey]store.Calibration{
		{FeedID: 1, Band: 0}: {Count: 400, Predicted: 1000, Actual: 70},
		{FeedID: 1, Band: 1}: {Count: minCalibrationChecks - 1, Predicted: 1000, Actual: 10}, // too few checks
		{FeedID: 2, Band: 0}: {Count: 100, Predicted: 1000, Actual: 5000},                    // capped
		{FeedID: 3, Band: 0}: {Count: 100, Predicted: 0, Actual: 10},                         // nothing predicted
	})
	require.Len(t, got, 2)
	require.InDelta(t, 0.07, got[store.CalibrationKey{FeedID: 1, Band: 0}], 1e-9)
	require.InDelta(t, maxCalibration, got[store.CalibrationKey{FeedID: 2, Band: 0}], 1e-9)

	require.Equal(t, 0, calibrationBand(59*60))
	require.Equal(t, 1, calibrationBand(3600))
	require.Equal(t, 2, calibrationBand(6*3600))
}

// A feed's torrents are scaled by their age band's factor; torrents from other feeds,
// bands or instances, and ones without a prediction, keep the model's.
func TestCalibrate(t *testing.T) {
	tv := func(instance int, hash string, age time.Duration, predicted int64) torrentValue {
		v := torrentValue{InstanceID: instance, Hash: hash, SizeBytes: gib, AgeSeconds: int64(age / time.Second)}
		if predicted > 0 {
			d := float64(predicted)
			v.Predicted, v.Density = &predicted, &d
		}
		return v
	}
	torrents := []torrentValue{
		tv(1, "COLA", 20*time.Minute, 1000), // hash case ignored
		tv(1, "cola-old", 2*time.Hour, 1000),
		tv(1, "rainbow", 20*time.Minute, 1000),
		tv(2, "cola", 20*time.Minute, 1000),
		tv(1, "manual", 20*time.Minute, 1000),
		tv(1, "unread", 20*time.Minute, 0),
	}
	added := map[store.AddedTorrent]int64{
		{InstanceID: 1, Hash: "cola"}: 3, {InstanceID: 1, Hash: "cola-old"}: 3, {InstanceID: 1, Hash: "rainbow"}: 1,
		{InstanceID: 1, Hash: "unread"}: 3,
	}
	factors := map[store.CalibrationKey]float64{{FeedID: 3, Band: 0}: 0.05, {FeedID: 1, Band: 0}: 1.2}
	calibrate(torrents, added, factors)

	got := map[string]int64{}
	for _, v := range torrents {
		if v.Predicted != nil {
			got[v.Hash] = *v.Predicted
			require.InDelta(t, float64(*v.Predicted), *v.Density, 1e-6)
		}
	}
	require.Equal(t, map[string]int64{"COLA": 50, "cola-old": 1000, "rainbow": 1200, "cola": 1000, "manual": 1000}, got)
	require.Nil(t, torrents[5].Predicted)
}

// A young torrent may be replaced for its value only once it is complete and idle.
func TestReplaceable(t *testing.T) {
	young := func(complete bool, rate *int64) torrentValue {
		return torrentValue{AgeSeconds: int64(10 * time.Minute / time.Second), Complete: complete, Rate: rate}
	}
	require.True(t, replaceable(torrentValue{AgeSeconds: int64(minKeep / time.Second)}))
	require.True(t, replaceable(young(true, ptr(int64(0)))))
	require.False(t, replaceable(young(true, ptr(int64(idleRate)))), "still uploading")
	require.False(t, replaceable(young(false, ptr(int64(0)))), "still downloading")
	require.False(t, replaceable(young(true, nil)), "no full window of readings yet")

	// When space is needed, the idle young torrent goes before a busy old one.
	idle := young(true, ptr(int64(0)))
	idle.Hash, idle.SizeBytes, idle.Density = "idle", 10*gib, ptr(0.0)
	busy := valued("busy", 10*gib, 2*time.Hour, ptr(500.0))
	_, _, picks, _ := pickReaps(40*gib, 20*gib, 30*gib, needRounds, ptr(1000.0), []torrentValue{busy, idle})
	require.Equal(t, []string{"idle"}, hashes(picks))
}
