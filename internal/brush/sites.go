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

type siteTransferView struct {
	SiteID     int64  `json:"siteId"`
	Name       string `json:"name"`
	Uploaded   int64  `json:"uploaded"`
	Downloaded int64  `json:"downloaded"`
}

type siteTransferResponse struct {
	// First and Last are the local days summed, both included.
	First string `json:"first"`
	Last  string `json:"last"`
	// Seconds is how long the range has run so far, for average speeds.
	Seconds int64              `json:"seconds"`
	Sites   []siteTransferView `json:"sites"`
}

// siteTransferRange turns a range name into the local days it sums and how many
// seconds of them have passed: today, yesterday, or the last 7 days with today.
func siteTransferRange(name string, now time.Time) (first, last string, seconds int64, ok bool) {
	midnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	sinceMidnight := int64(now.Sub(midnight) / time.Second)
	switch name {
	case "", "today":
		day := midnight.Format(time.DateOnly)
		return day, day, sinceMidnight, true
	case "yesterday":
		start := midnight.AddDate(0, 0, -1)
		day := start.Format(time.DateOnly)
		return day, day, int64(midnight.Sub(start) / time.Second), true
	case "week":
		start := midnight.AddDate(0, 0, -6)
		return start.Format(time.DateOnly), midnight.Format(time.DateOnly), int64(now.Sub(start) / time.Second), true
	}
	return "", "", 0, false
}

// handleSiteTransfer returns what every site uploaded and downloaded over a range
// of local days, most uploaded first.
func (s *Service) handleSiteTransfer(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	first, last, seconds, ok := siteTransferRange(r.URL.Query().Get("range"), s.now().Local())
	if !ok {
		writeError(w, http.StatusBadRequest, errors.New("range must be today, yesterday or week"))
		return
	}
	sites, err := s.store.ListSites(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	totals, err := s.store.SiteTransfersBetween(ctx, first, last)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	resp := siteTransferResponse{First: first, Last: last, Seconds: seconds, Sites: make([]siteTransferView, 0, len(sites))}
	for _, st := range sites {
		t := totals[st.ID]
		resp.Sites = append(resp.Sites, siteTransferView{SiteID: st.ID, Name: st.Name, Uploaded: t.Uploaded, Downloaded: t.Downloaded})
	}
	slices.SortStableFunc(resp.Sites, func(a, b siteTransferView) int {
		switch {
		case a.Uploaded > b.Uploaded:
			return -1
		case a.Uploaded < b.Uploaded:
			return 1
		}
		return strings.Compare(a.Name, b.Name)
	})
	writeJSON(w, http.StatusOK, resp)
}
