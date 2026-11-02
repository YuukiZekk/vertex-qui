// Copyright (c) 2026, vertex-qui contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package brush

import (
	"errors"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/autobrr/qui/internal/brush/site"
	"github.com/autobrr/qui/internal/brush/store"
)

// siteView is a site with what its torrents do today across every instance brush
// reads. Totals are written once a minute; speeds, counts and sizes are live.
type siteView struct {
	store.Site
	HasCookie bool `json:"hasCookie"`
	// Rule names how brush reads the site's torrents, from its address.
	Rule            string `json:"rule"`
	Feeds           int    `json:"feeds"`
	TodayUploaded   int64  `json:"todayUploaded"`
	TodayDownloaded int64  `json:"todayDownloaded"`
	UploadSpeed     int64  `json:"uploadSpeed"`
	DownloadSpeed   int64  `json:"downloadSpeed"`
	Torrents        int    `json:"torrents"`
	SizeBytes       int64  `json:"sizeBytes"`
	// TrackerHosts are the tracker hosts its torrents announce to, most used first,
	// so the page can show the icon qui keeps for a tracker.
	TrackerHosts []string `json:"trackerHosts"`
}

func (s *Service) handleListSites(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sites, err := s.store.ListSites(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	feeds, err := s.store.FeedCounts(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	today, err := s.store.SiteTransfersOn(ctx, s.now().Local().Format(time.DateOnly))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	owners, err := s.store.BrushTorrentSites(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	configs, err := s.store.InstanceConfigs(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	views := make(map[int64]*siteView, len(sites))
	out := make([]*siteView, 0, len(sites))
	for _, st := range sites {
		v := &siteView{Site: st, HasCookie: st.Cookie != "", Rule: site.RuleName(hostOf(st.BaseURL)), Feeds: feeds[st.ID],
			TodayUploaded: today[st.ID].Uploaded, TodayDownloaded: today[st.ID].Downloaded}
		views[st.ID] = v
		out = append(out, v)
	}
	matcher := newSiteMatcher(sites)
	hosts := make(map[int64]map[string]int)
	for id := range configs {
		torrents, err := s.qui.Torrents(ctx, id)
		if err != nil {
			continue
		}
		for _, t := range torrents {
			v := views[siteOf(owners[id], matcher, nil, t)]
			if v == nil {
				continue
			}
			v.Torrents++
			v.SizeBytes += t.Size
			v.UploadSpeed += t.UpSpeed
			v.DownloadSpeed += t.DlSpeed
			if host := hostOf(t.Tracker); host != "" {
				if hosts[v.ID] == nil {
					hosts[v.ID] = make(map[string]int)
				}
				hosts[v.ID][host]++
			}
		}
	}
	for _, v := range out {
		v.TrackerHosts = mostUsed(hosts[v.ID])
	}
	writeJSON(w, http.StatusOK, out)
}

// mostUsed lists the keys by count, highest first, ties by name.
func mostUsed(counts map[string]int) []string {
	keys := make([]string, 0, len(counts))
	for k := range counts {
		keys = append(keys, k)
	}
	slices.SortFunc(keys, func(a, b string) int {
		if counts[a] != counts[b] {
			return counts[b] - counts[a]
		}
		return strings.Compare(a, b)
	})
	return keys
}

// handleTestSite checks that the site accepts the stored cookie, or M-Team the
// API key, without touching any torrent.
func (s *Service) handleTestSite(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, errors.New("invalid site id"))
		return
	}
	st, err := s.store.GetSite(r.Context(), id)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	client, err := s.siteClient(r.Context(), id)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	result := struct {
		OK    bool   `json:"ok"`
		Rule  string `json:"rule"`
		Error string `json:"error,omitempty"`
	}{OK: true, Rule: site.RuleName(hostOf(st.BaseURL))}
	if err := client.CheckLogin(r.Context(), st.BaseURL); err != nil {
		result.OK, result.Error = false, err.Error()
	}
	writeJSON(w, http.StatusOK, result)
}
