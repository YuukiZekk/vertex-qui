// Copyright (c) 2026, vertex-qui contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package brush

import (
	"context"
	"fmt"
	"time"

	qbt "github.com/autobrr/go-qbittorrent"

	"github.com/autobrr/qui/internal/brush/store"
)

// reservationTTL bounds how long a torrent brush sent counts as reserved space
// while qBittorrent does not list it yet. After that it is assumed lost.
const reservationTTL = 15 * time.Minute

// spaceReport is the committed-space arithmetic for one instance. Committed is the
// space left once every incomplete torrent and every reservation has finished.
type spaceReport struct {
	Torrents  int
	Free      int64
	Left      int64
	Reserved  int64
	Committed int64
}

// committedSpace counts what qBittorrent still has to write (amount_left of every
// torrent) and what brush sent but qBittorrent does not list yet. qBittorrent
// allocates lazily, so its free space alone would admit a burst of torrents that
// later fills the disk, which is how the original Vertex filled servers.
func committedSpace(free int64, torrents []qbt.Torrent, reservations []store.Reservation) spaceReport {
	r := spaceReport{Torrents: len(torrents), Free: free}
	listed := make(map[string]struct{}, len(torrents))
	for _, t := range torrents {
		listed[t.Hash] = struct{}{}
		r.Left += t.AmountLeft
	}
	for _, res := range reservations {
		if _, ok := listed[res.InfoHash]; !ok {
			r.Reserved += res.SizeBytes
		}
	}
	r.Committed = r.Free - r.Left - r.Reserved
	return r
}

func (s *Service) instanceSpace(ctx context.Context, instanceID int) (spaceReport, error) {
	torrents, err := s.qui.Torrents(ctx, instanceID)
	if err != nil {
		return spaceReport{}, fmt.Errorf("read torrents: %w", err)
	}
	free, err := s.qui.FreeSpace(ctx, instanceID)
	if err != nil {
		return spaceReport{}, fmt.Errorf("read free space: %w", err)
	}
	reservations, err := s.store.Reservations(ctx, instanceID, s.now().Add(-reservationTTL))
	if err != nil {
		return spaceReport{}, err
	}
	return committedSpace(free, torrents, reservations), nil
}
