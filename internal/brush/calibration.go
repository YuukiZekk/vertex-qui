// Copyright (c) 2026, vertex-qui contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package brush

import (
	"context"
	"strings"
	"time"

	"github.com/autobrr/qui/internal/brush/store"
)

const (
	// calibrationWindow is how far back checked predictions tell how far each feed's
	// torrents fall short of, or beat, the model. A feed whose swarms die once
	// everyone has the free download uploads far less after the first hour than its
	// speed promises, while slow sites keep up.
	calibrationWindow = 7 * 24 * time.Hour
	// minCalibrationChecks is how many checked predictions a feed's age band needs
	// before it is calibrated; fewer leave the model's prediction as it is.
	minCalibrationChecks = 30
	// maxCalibration caps a band that beat the model, so a few lucky torrents do not
	// make a feed untouchable.
	maxCalibration = 2.0
	// feedMemory is how far back a torrent is traced to the feed that added it, the
	// same as predictions are kept.
	feedMemory = 14 * 24 * time.Hour
	// idleRate is the upload speed, over model.RateWindow, under which a complete
	// torrent counts as idle: nothing to protect while it is young.
	idleRate = 16 << 10
)

// calibrationBands are the ages where a torrent's age band ends. A feed's young
// torrents and its older ones miss the model differently.
var calibrationBands = []int64{int64(time.Hour / time.Second), int64(6 * time.Hour / time.Second)}

func calibrationBand(age int64) int {
	band := 0
	for _, b := range calibrationBands {
		if age >= b {
			band++
		}
	}
	return band
}

// calibrationFactors turns each feed's checked predictions per age band into what
// its torrents' predictions are multiplied by.
func calibrationFactors(sums map[store.CalibrationKey]store.Calibration) map[store.CalibrationKey]float64 {
	out := make(map[store.CalibrationKey]float64, len(sums))
	for k, c := range sums {
		if c.Count < minCalibrationChecks || c.Predicted <= 0 {
			continue
		}
		out[k] = min(maxCalibration, float64(c.Actual)/float64(c.Predicted))
	}
	return out
}

// calibrate scales the prediction and value of every torrent a calibrated feed added
// by how its feed's torrents of that age did against the model. Torrents brush did
// not add, or from bands without enough history, keep the model's prediction. The
// stored predictions stay the model's own, so calibration never feeds on itself.
func calibrate(torrents []torrentValue, added map[store.AddedTorrent]int64, factors map[store.CalibrationKey]float64) {
	if len(factors) == 0 {
		return
	}
	for i := range torrents {
		tv := &torrents[i]
		if tv.Predicted == nil {
			continue
		}
		feedID, ok := added[store.AddedTorrent{InstanceID: tv.InstanceID, Hash: strings.ToLower(tv.Hash)}]
		if !ok {
			continue
		}
		f, ok := factors[store.CalibrationKey{FeedID: feedID, Band: calibrationBand(tv.AgeSeconds)}]
		if !ok {
			continue
		}
		p := int64(float64(*tv.Predicted) * f)
		tv.Predicted = &p
		if tv.Density != nil {
			d := *tv.Density * f
			tv.Density = &d
		}
	}
}

// replaceable reports whether a torrent may be replaced for its value: it is old
// enough to have shown what it earns, or it is complete and already idle, which a
// young torrent that is done and uploads nothing will not grow out of.
func replaceable(tv torrentValue) bool {
	if tv.AgeSeconds >= int64(minKeep/time.Second) {
		return true
	}
	return tv.Complete && tv.Rate != nil && *tv.Rate < idleRate
}

// loadCalibration reads which feed added each torrent and how each feed's torrents
// did against the model. A failure leaves the model's predictions as they are.
func (s *Service) loadCalibration(ctx context.Context, now time.Time) (map[store.AddedTorrent]int64, map[store.CalibrationKey]float64) {
	added, err := s.store.AddedFeeds(ctx, now.Add(-feedMemory))
	if err != nil {
		s.log.Error().Err(err).Msg("read brush torrents' feeds")
		return nil, nil
	}
	sums, err := s.store.FeedCalibration(ctx, now.Add(-calibrationWindow), calibrationBands)
	if err != nil {
		s.log.Error().Err(err).Msg("read brush feed calibration")
		return added, nil
	}
	return added, calibrationFactors(sums)
}
