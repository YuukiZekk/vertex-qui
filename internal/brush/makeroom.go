// Copyright (c) 2026, vertex-qui contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package brush

import (
	"context"
	"math"
	"slices"
	"time"

	"github.com/autobrr/qui/internal/brush/site"
	"github.com/autobrr/qui/internal/brush/store"
)

// roomPlan is what one instance would delete to take a new torrent.
type roomPlan struct {
	candidate candidate
	picks     []torrentValue
	freed     int64
	// loss is what the picks were predicted to upload over the next horizon.
	loss int64
}

// planRoom finds, on one instance refused for lack of space, the torrents to delete
// so the new torrent fits above the floor: ones at least minKeep old that earn
// clearly less per GiB than a new torrent does there, lowest value first. It
// returns false when they do not free enough.
func planRoom(c candidate, size int64, yield float64, torrents []torrentValue) (roomPlan, bool) {
	shortfall := size + c.Floor - c.Committed
	limit := yield * reapMargin
	var eligible []torrentValue
	for _, tv := range torrents {
		if tv.Density != nil && tv.AgeSeconds >= int64(minKeep/time.Second) && *tv.Density < limit {
			eligible = append(eligible, tv)
		}
	}
	slices.SortFunc(eligible, func(a, b torrentValue) int {
		switch {
		case *a.Density < *b.Density:
			return -1
		case *a.Density > *b.Density:
			return 1
		}
		return 0
	})
	p := roomPlan{candidate: c}
	for _, tv := range eligible {
		if p.freed >= shortfall {
			break
		}
		p.picks = append(p.picks, tv)
		p.freed += tv.SizeBytes
		if tv.Predicted != nil {
			p.loss += *tv.Predicted
		}
	}
	return p, p.freed >= shortfall && shortfall > 0
}

// makeRoom is admission's last try when no instance has the space for a new
// torrent: on the instance where it costs the least predicted upload, it deletes
// torrents worth clearly less than the new one, then takes the torrent there. Only
// instances refused for space are considered, and nothing happens without the
// history that says what a new torrent is worth. It returns the candidate the
// torrent can now go to.
func (s *Service) makeRoom(ctx context.Context, cands []candidate, tf site.TorrentFile) (candidate, bool) {
	now := s.now()
	yields, err := s.store.NewTorrentYield(ctx, now.Add(-yieldWindow), newTorrentAge)
	if err != nil {
		s.log.Error().Err(err).Msg("read new torrent yield for making room")
		return candidate{}, false
	}
	byInstance := make(map[int][]torrentValue)
	for _, tv := range s.values.values(now) {
		byInstance[tv.InstanceID] = append(byInstance[tv.InstanceID], tv)
	}
	var best *roomPlan
	for _, c := range cands {
		if c.Reason != reasonLowSpace {
			continue
		}
		// A torrent is arriving now, so the replacement is certain: its value is the
		// whole yield.
		r := replacementFor(c.InstanceID, yields, math.Inf(1))
		if r.Yield == nil {
			continue
		}
		plan, ok := planRoom(c, tf.Size, *r.Yield, byInstance[c.InstanceID])
		if ok && (best == nil || plan.loss < best.loss) {
			best = &plan
		}
	}
	if best == nil {
		return candidate{}, false
	}
	st := reapState{InstanceID: best.candidate.InstanceID, Mode: store.ReapMakeRoom, Committed: best.candidate.Committed,
		FloorBytes: best.candidate.Floor, Shortfall: tf.Size + best.candidate.Floor - best.candidate.Committed}
	freed := s.deletePicks(ctx, st, best.picks)
	c := best.candidate
	// qBittorrent reports the freed disk only at its next sync, so the deleted
	// torrents are counted back here.
	c.Committed += freed
	if c.Committed-tf.Size < c.Floor {
		return candidate{}, false
	}
	c.Reason, c.Detail = "", "made room"
	c.SpaceCost = float64(tf.Size) / float64(c.Committed-c.Floor)
	c.Cost = c.SpaceCost + c.UploadCost
	s.log.Info().Int("instance", c.InstanceID).Int("deleted", len(best.picks)).Int64("freed", freed).
		Msg("made room for a new torrent")
	return c, true
}
