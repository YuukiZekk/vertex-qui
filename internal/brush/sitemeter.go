// Copyright (c) 2026, vertex-qui contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package brush

import (
	"context"
	"net/url"
	"slices"
	"strings"
	"time"

	qbt "github.com/autobrr/go-qbittorrent"

	"github.com/autobrr/qui/internal/brush/store"
)

const (
	// siteRefreshEvery is how often the sampler rereads the sites and which site
	// each brush torrent came from.
	siteRefreshEvery = time.Minute
	// siteFlushEvery is how often the per-site totals are written.
	siteFlushEvery = time.Minute
)

// secondLevels are second-level labels under a country code, like edu.cn or co.uk,
// where the site's own name is one label further in.
var secondLevels = []string{"com", "net", "org", "edu", "gov", "ac", "co"}

// baseDomain is the domain a site and its trackers share: pt.sjtu.edu.cn and
// tracker.sjtu.edu.cn give sjtu.edu.cn, hdsky.me and tracker.hdsky.me give hdsky.me.
func baseDomain(host string) string {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	if h, _, ok := strings.Cut(host, ":"); ok {
		host = h
	}
	labels := strings.Split(host, ".")
	n := len(labels)
	if n <= 2 {
		return host
	}
	if n >= 3 && len(labels[n-1]) == 2 && slices.Contains(secondLevels, labels[n-2]) {
		return strings.Join(labels[n-3:], ".")
	}
	return strings.Join(labels[n-2:], ".")
}

func hostOf(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return ""
	}
	return u.Hostname()
}

type siteDomain struct {
	id    int64
	base  string
	mteam bool
}

// siteMatcher tells which site a tracker belongs to by their shared base domain.
// M-Team announces on other domains than its web address, so any m-team tracker
// is M-Team.
type siteMatcher []siteDomain

func newSiteMatcher(sites []store.Site) siteMatcher {
	m := make(siteMatcher, 0, len(sites))
	for _, s := range sites {
		host := hostOf(s.BaseURL)
		if host == "" {
			continue
		}
		m = append(m, siteDomain{id: s.ID, base: baseDomain(host), mteam: strings.Contains(host, "m-team")})
	}
	return m
}

func (m siteMatcher) match(trackerURL string) int64 {
	host := hostOf(trackerURL)
	if host == "" {
		return 0
	}
	base := baseDomain(host)
	for _, s := range m {
		if s.base == base || (s.mteam && strings.Contains(host, "m-team")) {
			return s.id
		}
	}
	return 0
}

type torrentCounters struct {
	up, down int64
}

// siteMeter adds up what the torrents of every site upload and download, from the
// change in each torrent's counters between samples. A torrent brush added belongs
// to its feed's site; any other is matched by its tracker, and keeps the last site
// it matched while qBittorrent reports no working tracker. Only the sampler
// goroutine uses it.
type siteMeter struct {
	last    map[int]map[string]torrentCounters
	known   map[int]map[string]int64
	pending map[int64]*store.SiteTransfer
	day     string

	matcher     siteMatcher
	owners      map[int]map[string]int64
	refreshedAt time.Time
	flushedAt   time.Time
}

func newSiteMeter() *siteMeter {
	return &siteMeter{
		last:    make(map[int]map[string]torrentCounters),
		known:   make(map[int]map[string]int64),
		pending: make(map[int64]*store.SiteTransfer),
	}
}

// siteOf is the site of one torrent, 0 for none: the feed site of a torrent brush
// added, else the site its tracker matches, else the last site it matched.
func siteOf(owners map[string]int64, matcher siteMatcher, known map[string]int64, t qbt.Torrent) int64 {
	if id := owners[t.Hash]; id != 0 {
		return id
	}
	if id := matcher.match(t.Tracker); id != 0 {
		return id
	}
	return known[t.Hash]
}

// observe records one read of an instance. The first read of a torrent only sets
// its starting point, and a counter that went down adds nothing.
func (m *siteMeter) observe(instanceID int, day string, torrents []qbt.Torrent) {
	// flushSites runs before the first sample of a new day and writes what is still
	// pending to the day before.
	m.day = day
	prev := m.last[instanceID]
	next := make(map[string]torrentCounters, len(torrents))
	known := make(map[string]int64, len(torrents))
	for _, t := range torrents {
		next[t.Hash] = torrentCounters{up: t.Uploaded, down: t.Downloaded}
		site := siteOf(m.owners[instanceID], m.matcher, m.known[instanceID], t)
		if site == 0 {
			continue
		}
		known[t.Hash] = site
		before, seen := prev[t.Hash]
		if !seen {
			continue
		}
		up, down := max(0, t.Uploaded-before.up), max(0, t.Downloaded-before.down)
		if up == 0 && down == 0 {
			continue
		}
		p := m.pending[site]
		if p == nil {
			p = &store.SiteTransfer{SiteID: site}
			m.pending[site] = p
		}
		p.Uploaded += up
		p.Downloaded += down
	}
	m.last[instanceID] = next
	m.known[instanceID] = known
}

// refreshSites rereads the sites and the brush torrents' sites, at most once per
// siteRefreshEvery.
func (s *Service) refreshSites(ctx context.Context, now time.Time) {
	m := s.sites
	if now.Sub(m.refreshedAt) < siteRefreshEvery {
		return
	}
	sites, err := s.store.ListSites(ctx)
	if err != nil {
		s.log.Error().Err(err).Msg("read brush sites for site totals")
		return
	}
	owners, err := s.store.BrushTorrentSites(ctx)
	if err != nil {
		s.log.Error().Err(err).Msg("read brush torrent sites")
		return
	}
	m.matcher, m.owners, m.refreshedAt = newSiteMatcher(sites), owners, now
}

// flushSites writes the pending per-site totals at most once per siteFlushEvery,
// or at once when the day changed, so a day's traffic is never added to the next.
func (s *Service) flushSites(ctx context.Context, now time.Time) {
	m := s.sites
	day := now.Format(time.DateOnly)
	if len(m.pending) == 0 || (m.day == day && now.Sub(m.flushedAt) < siteFlushEvery) {
		return
	}
	add := make([]store.SiteTransfer, 0, len(m.pending))
	for _, p := range m.pending {
		add = append(add, *p)
	}
	if err := s.store.AddSiteTransfer(ctx, m.day, add); err != nil {
		s.log.Error().Err(err).Msg("store brush site totals")
		return
	}
	m.pending, m.flushedAt = make(map[int64]*store.SiteTransfer), now
}
