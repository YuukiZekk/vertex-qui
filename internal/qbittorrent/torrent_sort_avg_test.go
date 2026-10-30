// Copyright (c) 2026, vertex-qui contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package qbittorrent

import (
	"testing"

	qbt "github.com/autobrr/go-qbittorrent"
	"github.com/stretchr/testify/require"
)

func TestSortTorrentsByAverageUploadSpeed(t *testing.T) {
	t.Parallel()

	sm := NewSyncManager(nil, nil)
	torrents := []qbt.Torrent{
		{Hash: "never", Uploaded: 0, TimeActive: 0},
		{Hash: "slow", Uploaded: 1000, TimeActive: 100},  // 10 B/s
		{Hash: "fast", Uploaded: 9000, TimeActive: 30},   // 300 B/s
		{Hash: "mid", Uploaded: 50000, TimeActive: 1000}, // 50 B/s, more uploaded than fast
	}
	hashes := func() []string {
		out := make([]string, len(torrents))
		for i, tor := range torrents {
			out[i] = tor.Hash
		}
		return out
	}

	sm.sortTorrents(torrents, "up_speed_avg", true, false, nil)
	require.Equal(t, []string{"fast", "mid", "slow", "never"}, hashes())

	sm.sortTorrents(torrents, "up_speed_avg", false, false, nil)
	require.Equal(t, []string{"slow", "mid", "fast", "never"}, hashes(), "a torrent never active stays last")
}
