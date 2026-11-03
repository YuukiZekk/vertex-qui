// Copyright (c) 2026, vertex-qui contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package brush

import (
	"testing"
	"time"

	qbt "github.com/autobrr/go-qbittorrent"
	"github.com/stretchr/testify/require"

	"github.com/autobrr/qui/internal/brush/quiadapter"
	"github.com/autobrr/qui/internal/brush/store"
	"github.com/autobrr/qui/internal/models"
	"github.com/autobrr/qui/internal/testutil/testdb"
)

func TestBusiest(t *testing.T) {
	var torrents []qbt.Torrent
	for i := range flowTorrents + 5 {
		torrents = append(torrents, qbt.Torrent{Hash: string(rune('a' + i)), UpSpeed: int64(i+1) << 20})
	}
	torrents = append(torrents, qbt.Torrent{Hash: "idle", UpSpeed: 10, DlSpeed: 10})
	picked := busiest(torrents)
	require.Len(t, picked, flowTorrents)
	require.Equal(t, string(rune('a'+flowTorrents+4)), picked[0].Hash, "fastest first")
	for _, p := range picked {
		require.NotEqual(t, "idle", p.Hash)
	}
}

// The flow sampler lists the connections moving data on each instance and adds
// what each connection moved since the last read to its site and country.
func TestSampleFlows(t *testing.T) {
	ctx := t.Context()
	db := testdb.NewMigratedSQLite(t, t.Name())
	cipher, err := models.NewCredentialCipher(make([]byte, 32))
	require.NoError(t, err)
	now := time.Date(2026, 11, 3, 12, 0, 0, 0, time.Local)
	busy := qbt.Torrent{Hash: "busy", Name: "Busy", Tracker: "https://tracker.pt.example/announce", UpSpeed: 5 << 20}
	quiet := qbt.Torrent{Hash: "quiet", Name: "Quiet"}
	qb := &fakeQB{
		free:     map[int]int64{1: 100 * gib},
		listed:   map[int][]qbt.Torrent{1: {busy, quiet}},
		transfer: map[int]quiadapter.Transfer{1: {ULSpeed: 6 << 20, DLSpeed: 1 << 20}},
		peers: map[int]map[string]map[string]qbt.TorrentPeer{1: {"busy": {
			"1.2.3.4:5000": {IP: "1.2.3.4", Port: 5000, CountryCode: "DE", UpSpeed: 4 << 20, Uploaded: 100},
			"5.6.7.8:6000": {IP: "5.6.7.8", Port: 6000, CountryCode: "jp", Uploaded: 50},
		}}},
	}
	svc, err := New(ctx, db, cipher, qb)
	require.NoError(t, err)
	svc.now = func() time.Time { return now }
	require.NoError(t, svc.store.SetFloor(ctx, 1, 20*gib))
	require.NoError(t, svc.store.SetCountry(ctx, 1, "nl"))
	siteID, err := svc.store.CreateSite(ctx, store.Site{Name: "pt", BaseURL: "https://pt.example"})
	require.NoError(t, err)
	_, err = svc.store.CreateFeed(ctx, store.Feed{SiteID: siteID, Name: "a", URL: "https://pt.example/a",
		IntervalSeconds: 60, WindowSeconds: 300, InstanceIDs: []int{1}, Enabled: true})
	require.NoError(t, err)

	svc.sampleFlows(ctx)
	snap := svc.flow
	require.Equal(t, []flowInstance{{ID: 1, Name: "vps-1", Country: "nl", Brush: true, UploadSpeed: 6 << 20, DownloadSpeed: 1 << 20, Connections: 1}}, snap.Instances)
	require.Len(t, snap.Peers, 1, "only connections moving data")
	require.Equal(t, flowPeer{InstanceID: 1, Hash: "busy", Torrent: "Busy", SiteID: siteID, IP: "1.2.3.4", Port: 5000,
		Country: "de", UploadSpeed: 4 << 20}, snap.Peers[0])
	require.Empty(t, svc.flowMeter.pending, "the first read only sets the starting point")

	peers := qb.peers[1]["busy"]
	peers["1.2.3.4:5000"] = qbt.TorrentPeer{IP: "1.2.3.4", CountryCode: "DE", UpSpeed: 1, Uploaded: 400, Downloaded: 7}
	peers["5.6.7.8:6000"] = qbt.TorrentPeer{IP: "5.6.7.8", CountryCode: "jp", Uploaded: 40}
	now = now.Add(flowInterval)
	svc.sampleFlows(ctx)
	// The next day's first read writes what the day before still had pending.
	now = now.Add(24 * time.Hour)
	svc.sampleFlows(ctx)

	got, err := svc.store.PeerTransfersSince(ctx, "2026-01-01")
	require.NoError(t, err)
	require.Equal(t, []store.PeerTransfer{{Day: "2026-11-03", SiteID: siteID, InstanceID: 1, CountryCode: "de", Uploaded: 300, Downloaded: 7}}, got,
		"a counter that went down adds nothing")
}
