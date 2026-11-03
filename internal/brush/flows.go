// Copyright (c) 2026, vertex-qui contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package brush

import (
	"cmp"
	"context"
	"net/http"
	"slices"
	"strings"
	"time"

	qbt "github.com/autobrr/go-qbittorrent"

	"github.com/autobrr/qui/internal/brush/store"
)

const (
	// flowInterval is how often the peers of the busiest torrents are read.
	flowInterval = 15 * time.Second
	// flowTorrents is how many of an instance's busiest torrents have their peers
	// read each time; every read is one qBittorrent request.
	flowTorrents = 15
	// flowMinSpeed is how fast, in bytes per second, a torrent must move for its
	// peers to be read.
	flowMinSpeed = 1 << 10
	// flowPeersShown caps the peers the flow table lists.
	flowPeersShown = 300
	// flowFlushEvery is how often the per-country totals are written.
	flowFlushEvery = 5 * time.Minute
)

// flowPeer is one connection moving data now.
type flowPeer struct {
	InstanceID int    `json:"instanceId"`
	Hash       string `json:"hash"`
	Torrent    string `json:"torrent"`
	SiteID     int64  `json:"siteId"`
	IP         string `json:"ip"`
	Port       int    `json:"port"`
	Client     string `json:"client"`
	// Country is the peer's lower-case ISO code, empty when qBittorrent named none.
	Country       string  `json:"country"`
	UploadSpeed   int64   `json:"uploadSpeed"`
	DownloadSpeed int64   `json:"downloadSpeed"`
	Progress      float64 `json:"progress"`
}

type flowInstance struct {
	ID      int    `json:"id"`
	Name    string `json:"name"`
	Country string `json:"country"`
	// Brush is true when an enabled feed may add torrents to the instance; the
	// others only seed.
	Brush bool `json:"brush"`
	// UploadSpeed and DownloadSpeed are the whole instance's, not only the torrents
	// whose peers were read.
	UploadSpeed   int64 `json:"uploadSpeed"`
	DownloadSpeed int64 `json:"downloadSpeed"`
	// Connections counts the peers moving data among the torrents read.
	Connections int `json:"connections"`
}

type flowSnapshot struct {
	SampledAt int64          `json:"sampledAt"`
	Instances []flowInstance `json:"instances"`
	Peers     []flowPeer     `json:"peers"`
}

type peerKey struct {
	site     int64
	instance int
	country  string
}

// peerMeter adds up what each instance moves with the peers of every country, per
// site, from the change in each connection's counters between reads. The first
// read of a connection only sets its starting point. Only the flow goroutine uses it.
type peerMeter struct {
	// last holds each connection's counters by instance, torrent and address.
	last      map[int]map[string]map[string]torrentCounters
	pending   map[peerKey]*store.PeerTransfer
	day       string
	flushedAt time.Time
}

func newPeerMeter() *peerMeter {
	return &peerMeter{last: make(map[int]map[string]map[string]torrentCounters), pending: make(map[peerKey]*store.PeerTransfer)}
}

// observe records one read of a torrent's peers.
func (m *peerMeter) observe(instanceID int, hash string, site int64, peers map[string]qbt.TorrentPeer) {
	byHash := m.last[instanceID]
	if byHash == nil {
		byHash = make(map[string]map[string]torrentCounters)
		m.last[instanceID] = byHash
	}
	prev := byHash[hash]
	next := make(map[string]torrentCounters, len(peers))
	for addr, p := range peers {
		next[addr] = torrentCounters{up: p.Uploaded, down: p.Downloaded}
		before, seen := prev[addr]
		if !seen {
			continue
		}
		up, down := max(0, p.Uploaded-before.up), max(0, p.Downloaded-before.down)
		if up == 0 && down == 0 {
			continue
		}
		key := peerKey{site: site, instance: instanceID, country: strings.ToLower(p.CountryCode)}
		t := m.pending[key]
		if t == nil {
			t = &store.PeerTransfer{SiteID: key.site, InstanceID: key.instance, CountryCode: key.country}
			m.pending[key] = t
		}
		t.Uploaded += up
		t.Downloaded += down
	}
	byHash[hash] = next
}

// forget drops the torrents an instance no longer lists. A torrent still listed
// keeps its connections between reads, so one that drops out of the busiest and
// comes back adds what it moved meanwhile.
func (m *peerMeter) forget(instanceID int, torrents []qbt.Torrent) {
	byHash := m.last[instanceID]
	if len(byHash) == 0 {
		return
	}
	listed := make(map[string]bool, len(torrents))
	for _, t := range torrents {
		listed[t.Hash] = true
	}
	for hash := range byHash {
		if !listed[hash] {
			delete(byHash, hash)
		}
	}
}

// busiest returns the torrents moving at least flowMinSpeed, fastest first, at
// most flowTorrents of them.
func busiest(torrents []qbt.Torrent) []qbt.Torrent {
	out := make([]qbt.Torrent, 0, flowTorrents)
	for _, t := range torrents {
		if t.UpSpeed+t.DlSpeed >= flowMinSpeed {
			out = append(out, t)
		}
	}
	slices.SortFunc(out, func(a, b qbt.Torrent) int { return cmp.Compare(b.UpSpeed+b.DlSpeed, a.UpSpeed+a.DlSpeed) })
	return out[:min(len(out), flowTorrents)]
}

func (s *Service) runFlowSampler(ctx context.Context) {
	ticker := time.NewTicker(flowInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.sampleFlows(ctx)
		}
	}
}

// sampleFlows reads the peers of every instance's busiest torrents, keeps what is
// moving now for the flow map and adds what moved to the per-country totals.
func (s *Service) sampleFlows(ctx context.Context) {
	now := s.now().Local()
	s.flushFlows(ctx, now)
	instances, err := s.qui.Instances(ctx)
	if err != nil {
		s.log.Error().Err(err).Msg("list instances for flows")
		return
	}
	configs, err := s.store.InstanceConfigs(ctx)
	if err != nil {
		s.log.Error().Err(err).Msg("read brush instances for flows")
		return
	}
	sites, err := s.store.ListSites(ctx)
	if err != nil {
		s.log.Error().Err(err).Msg("read brush sites for flows")
		return
	}
	owners, err := s.store.BrushTorrentSites(ctx)
	if err != nil {
		s.log.Error().Err(err).Msg("read brush torrent sites for flows")
		return
	}
	feeds, err := s.store.ListFeeds(ctx)
	if err != nil {
		s.log.Error().Err(err).Msg("read brush feeds for flows")
		return
	}
	brushing := make(map[int]bool)
	for _, f := range feeds {
		if f.Enabled {
			for _, id := range f.InstanceIDs {
				brushing[id] = true
			}
		}
	}
	matcher := newSiteMatcher(sites)
	m := s.flowMeter
	m.day = now.Format(time.DateOnly)

	snap := flowSnapshot{SampledAt: now.Unix(), Instances: []flowInstance{}, Peers: []flowPeer{}}
	for _, inst := range instances {
		if !inst.Active {
			continue
		}
		torrents, err := s.qui.Torrents(ctx, inst.ID)
		if err != nil {
			continue
		}
		fi := flowInstance{ID: inst.ID, Name: inst.Name, Country: configs[inst.ID].CountryCode, Brush: brushing[inst.ID]}
		if tr, ok, err := s.qui.Transfer(ctx, inst.ID); err == nil && ok {
			fi.UploadSpeed, fi.DownloadSpeed = tr.ULSpeed, tr.DLSpeed
		}
		for _, t := range busiest(torrents) {
			peers, err := s.qui.Peers(ctx, inst.ID, t.Hash)
			if err != nil {
				continue
			}
			site := siteOf(owners[inst.ID], matcher, nil, t)
			m.observe(inst.ID, t.Hash, site, peers)
			for _, p := range peers {
				if p.UpSpeed == 0 && p.DownSpeed == 0 {
					continue
				}
				fi.Connections++
				snap.Peers = append(snap.Peers, flowPeer{InstanceID: inst.ID, Hash: t.Hash, Torrent: t.Name, SiteID: site,
					IP: p.IP, Port: p.Port, Client: p.Client, Country: strings.ToLower(p.CountryCode),
					UploadSpeed: p.UpSpeed, DownloadSpeed: p.DownSpeed, Progress: p.Progress})
			}
		}
		m.forget(inst.ID, torrents)
		snap.Instances = append(snap.Instances, fi)
	}
	slices.SortFunc(snap.Peers, func(a, b flowPeer) int {
		return cmp.Compare(b.UploadSpeed+b.DownloadSpeed, a.UploadSpeed+a.DownloadSpeed)
	})
	snap.Peers = snap.Peers[:min(len(snap.Peers), flowPeersShown)]
	slices.SortFunc(snap.Instances, func(a, b flowInstance) int { return cmp.Compare(a.ID, b.ID) })

	s.flowMu.Lock()
	s.flow = snap
	s.flowMu.Unlock()
}

// flushFlows writes the pending per-country totals at most once per flowFlushEvery,
// or at once when the day changed, so a day's traffic is never added to the next.
func (s *Service) flushFlows(ctx context.Context, now time.Time) {
	m := s.flowMeter
	if len(m.pending) == 0 || (m.day == now.Format(time.DateOnly) && now.Sub(m.flushedAt) < flowFlushEvery) {
		return
	}
	add := make([]store.PeerTransfer, 0, len(m.pending))
	for _, t := range m.pending {
		add = append(add, *t)
	}
	if err := s.store.AddPeerTransfer(ctx, m.day, add); err != nil {
		s.log.Error().Err(err).Msg("store brush peer totals")
		return
	}
	m.pending, m.flushedAt = make(map[peerKey]*store.PeerTransfer), now
}

// handleFlows returns the latest read of the connections moving data now.
func (s *Service) handleFlows(w http.ResponseWriter, _ *http.Request) {
	s.flowMu.RLock()
	snap := s.flow
	s.flowMu.RUnlock()
	if snap.Instances == nil {
		snap.Instances, snap.Peers = []flowInstance{}, []flowPeer{}
	}
	writeJSON(w, http.StatusOK, snap)
}
