// Copyright (c) 2026, vertex-qui contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package brush

import (
	"context"
	"time"

	qbt "github.com/autobrr/go-qbittorrent"
)

const (
	// defaultFloorShare is the share of an instance's disk a new instance keeps free.
	defaultFloorShare = 0.10
	floorCheckEvery   = time.Minute
)

// estimateTotalSpace is the instance's disk as qBittorrent sees it: the free space
// plus what its torrents have written. qBittorrent does not report the disk size,
// and qui can only read it for instances with filesystem access, so files outside
// qBittorrent make this, and the floor from it, somewhat low.
func estimateTotalSpace(free int64, torrents []qbt.Torrent) int64 {
	total := free
	for _, t := range torrents {
		total += t.Completed
	}
	return total
}

// defaultFloor is defaultFloorShare of the estimated disk, rounded up to a whole GiB.
func defaultFloor(free int64, torrents []qbt.Torrent) int64 {
	floor := int64(float64(estimateTotalSpace(free, torrents)) * defaultFloorShare)
	return (floor + 1<<30 - 1) / (1 << 30) * (1 << 30)
}

// ensureFloors gives every active instance that has no safety floor yet the
// default one, at most once per floorCheckEvery. A floor the user set is never
// changed. An instance whose free space is not known yet is tried again later.
func (s *Service) ensureFloors(ctx context.Context, now time.Time) {
	if now.Sub(s.lastFloorCheck) < floorCheckEvery {
		return
	}
	s.lastFloorCheck = now
	configs, err := s.store.InstanceConfigs(ctx)
	if err != nil {
		s.log.Error().Err(err).Msg("read brush instances for default floors")
		return
	}
	instances, err := s.qui.Instances(ctx)
	if err != nil {
		return
	}
	for _, inst := range instances {
		if _, ok := configs[inst.ID]; ok || !inst.Active {
			continue
		}
		free, err := s.qui.FreeSpace(ctx, inst.ID)
		if err != nil || free <= 0 {
			continue
		}
		torrents, err := s.qui.Torrents(ctx, inst.ID)
		if err != nil {
			continue
		}
		floor := defaultFloor(free, torrents)
		if err := s.store.SetFloor(ctx, inst.ID, floor); err != nil {
			s.log.Error().Err(err).Int("instance", inst.ID).Msg("set default safety floor")
			continue
		}
		s.log.Info().Int("instance", inst.ID).Int64("floor", floor).
			Int64("estimatedTotal", estimateTotalSpace(free, torrents)).Msg("set default safety floor")
	}
}
