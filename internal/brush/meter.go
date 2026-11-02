// Copyright (c) 2026, vertex-qui contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package brush

import (
	"context"
	"sync"
	"time"

	qbt "github.com/autobrr/go-qbittorrent"
)

const (
	// sampleInterval is how often the meter reads each instance's torrents from qui's cache.
	sampleInterval = 10 * time.Second
	// uploadWindow is how far back an instance's average upload reaches.
	uploadWindow = time.Minute
)

type uploadSample struct {
	from, to time.Time
	bytes    int64
}

// uploadMeter averages each instance's upload from the change in every torrent's
// cumulative upload between samples, not from the instantaneous speed qBittorrent
// reports, so a burst between two reads is neither missed nor overcounted. A torrent
// that disappears between samples simply stops contributing.
type uploadMeter struct {
	mu      sync.Mutex
	last    map[int]map[string]int64
	lastAt  map[int]time.Time
	samples map[int][]uploadSample
}

func newUploadMeter() *uploadMeter {
	return &uploadMeter{
		last:    make(map[int]map[string]int64),
		lastAt:  make(map[int]time.Time),
		samples: make(map[int][]uploadSample),
	}
}

func (m *uploadMeter) observe(instanceID int, at time.Time, torrents []qbt.Torrent) {
	current := make(map[string]int64, len(torrents))
	for _, t := range torrents {
		current[t.Hash] = t.Uploaded
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if prev, ok := m.last[instanceID]; ok {
		var delta int64
		for hash, uploaded := range current {
			if before, seen := prev[hash]; seen && uploaded > before {
				delta += uploaded - before
			}
		}
		kept := m.samples[instanceID][:0]
		for _, s := range m.samples[instanceID] {
			if s.to.After(at.Add(-uploadWindow)) {
				kept = append(kept, s)
			}
		}
		m.samples[instanceID] = append(kept, uploadSample{from: m.lastAt[instanceID], to: at, bytes: delta})
	}
	m.last[instanceID] = current
	m.lastAt[instanceID] = at
}

// rate is the average upload in bytes per second over the samples still inside the
// window. It reports false until the instance has two samples, or when the last
// one is too old to describe the instance now.
func (m *uploadMeter) rate(instanceID int, now time.Time) (int64, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	samples := m.samples[instanceID]
	if len(samples) == 0 || now.Sub(samples[len(samples)-1].to) > uploadWindow {
		return 0, false
	}
	var total int64
	for _, s := range samples {
		total += s.bytes
	}
	span := samples[len(samples)-1].to.Sub(samples[0].from)
	if span <= 0 {
		return 0, false
	}
	return int64(float64(total) / span.Seconds()), true
}

// runSampler has its own goroutine so a slow feed never delays a sample, and runs
// samples one after another so they reach the meter in order.
func (s *Service) runSampler(ctx context.Context) {
	ticker := time.NewTicker(sampleInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.sample(ctx)
		}
	}
}

// sample gives new instances their default floor, reads every instance brush has a
// floor for to meter instances, torrents and sites, then stores and checks
// predictions against what it read.
func (s *Service) sample(ctx context.Context) {
	s.ensureFloors(ctx, s.now())
	configs, err := s.store.InstanceConfigs(ctx)
	if err != nil {
		s.log.Error().Err(err).Msg("read brush instances for sampling")
		return
	}
	start := s.now().Local()
	s.refreshSites(ctx, start)
	s.flushSites(ctx, start)
	day := start.Format(time.DateOnly)
	for id := range configs {
		torrents, err := s.qui.Torrents(ctx, id)
		if err != nil {
			continue
		}
		now := s.now()
		s.meter.observe(id, now, torrents)
		s.values.observe(id, now, torrents)
		s.sites.observe(id, day, torrents)
		s.markFirstUploads(ctx, id, now, torrents)
	}
	now := s.now()
	s.resolvePredictions(ctx, now)
	s.recordPredictions(ctx, now)
}

// firstUploadWindow bounds how long after its add a torrent's first upload is
// recorded, so torrents read for the first time after a restart are not stamped.
const firstUploadWindow = 30 * time.Minute

// markFirstUploads stores when each brush torrent was first seen uploading, within
// one sample of the moment it started.
func (s *Service) markFirstUploads(ctx context.Context, instanceID int, now time.Time, torrents []qbt.Torrent) {
	seen := s.uploading[instanceID]
	next := make(map[string]bool, len(seen))
	for _, t := range torrents {
		if !hasTag(t, Tag) || t.Uploaded <= 0 {
			continue
		}
		next[t.Hash] = true
		if seen[t.Hash] {
			continue
		}
		if err := s.store.MarkFirstUpload(ctx, instanceID, t.Hash, now, now.Add(-firstUploadWindow)); err != nil {
			s.log.Error().Err(err).Msg("record first upload")
		}
	}
	s.uploading[instanceID] = next
}
