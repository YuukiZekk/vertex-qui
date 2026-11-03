// Copyright (c) 2026, vertex-qui contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package brush

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/autobrr/qui/internal/brush/site"
	"github.com/autobrr/qui/internal/brush/store"
)

const (
	historyLimit  = 200
	testItemLimit = 10
	minInterval   = 10
)

func (s *Service) Routes(r chi.Router) {
	r.Get("/status", s.handleStatus)
	r.Put("/settings", s.handleSettings)
	r.Put("/instances/{instanceID}/floor", s.handleFloor)
	r.Put("/instances/{instanceID}/capacity", s.handleCapacity)
	r.Put("/instances/{instanceID}/country", s.handleCountry)
	r.Put("/instances/{instanceID}/groups", s.handleInstanceGroups)

	r.Get("/groups", s.handleListGroups)
	r.Post("/groups", s.handleSaveGroup)
	r.Put("/groups/{id}", s.handleSaveGroup)
	r.Delete("/groups/{id}", s.handleDeleteGroup)

	r.Get("/sites", s.handleListSites)
	r.Post("/sites", s.handleSaveSite)
	r.Put("/sites/{id}", s.handleSaveSite)
	r.Delete("/sites/{id}", s.handleDeleteSite)
	r.Post("/sites/{id}/test", s.handleTestSite)

	r.Get("/feeds", s.handleListFeeds)
	r.Post("/feeds", s.handleSaveFeed)
	r.Put("/feeds/{id}", s.handleSaveFeed)
	r.Delete("/feeds/{id}", s.handleDeleteFeed)
	r.Post("/feeds/{id}/test", s.handleTestFeed)

	r.Get("/entries", s.handleEntries)
	r.Get("/transfer/today", s.handleTransferToday)
	r.Get("/values", s.handleValues)
	r.Get("/deletions", s.handleDeletions)
	r.Get("/performance", s.handlePerformance)
	r.Get("/performance/entries", s.handlePerformanceEntries)
	r.Get("/flows", s.handleFlows)
}

type instanceStatus struct {
	ID         int    `json:"id"`
	Name       string `json:"name"`
	Active     bool   `json:"active"`
	Torrents   int    `json:"torrents"`
	FreeSpace  int64  `json:"freeSpace"`
	Left       int64  `json:"left"`
	Reserved   int64  `json:"reserved"`
	Committed  int64  `json:"committed"`
	FloorBytes *int64 `json:"floorBytes"`
	// UploadCapacity is in bytes per second, zero when not set. UploadRate is the
	// meter's average, null until it has samples.
	UploadCapacity int64 `json:"uploadCapacity"`
	// CountryCode is where the instance runs, for its flag; empty when not set.
	CountryCode string `json:"countryCode"`
	// GroupIDs are the groups the instance is in.
	GroupIDs   []int64 `json:"groupIds"`
	UploadRate *int64  `json:"uploadRate"`
	Error      string  `json:"error,omitempty"`
}

type statusResponse struct {
	Enabled   bool             `json:"enabled"`
	Instances []instanceStatus `json:"instances"`
	// Today counts since midnight in the server's time zone.
	Today store.TodayCounts `json:"today"`
}

func (s *Service) handleStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	enabled, err := s.store.Enabled(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	configs, err := s.store.InstanceConfigs(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	instances, err := s.qui.Instances(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	groups, err := s.store.InstanceGroups(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	resp := statusResponse{Enabled: enabled, Instances: make([]instanceStatus, 0, len(instances))}
	local := s.now().Local()
	if resp.Today, err = s.store.CountsSince(ctx, time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, local.Location())); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	for _, inst := range instances {
		st := instanceStatus{ID: inst.ID, Name: inst.Name, Active: inst.Active, GroupIDs: groups[inst.ID]}
		if st.GroupIDs == nil {
			st.GroupIDs = []int64{}
		}
		if cfg, ok := configs[inst.ID]; ok {
			st.FloorBytes = &cfg.FloorBytes
			st.UploadCapacity = cfg.UploadCapacity
			st.CountryCode = cfg.CountryCode
		}
		if rate, ok := s.meter.rate(inst.ID, s.now()); ok {
			st.UploadRate = &rate
		}
		if inst.Active {
			// A disabled or unreachable instance shows its error instead of failing the response.
			if space, err := s.instanceSpace(ctx, inst.ID); err != nil {
				st.Error = err.Error()
			} else {
				st.Torrents, st.FreeSpace, st.Left, st.Reserved, st.Committed = space.Torrents, space.Free, space.Left, space.Reserved, space.Committed
			}
		}
		resp.Instances = append(resp.Instances, st)
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Service) handleSettings(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Enabled bool `json:"enabled"`
	}
	if !decode(w, r, &body) {
		return
	}
	if err := s.store.SetEnabled(r.Context(), body.Enabled); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Service) handleFloor(w http.ResponseWriter, r *http.Request) {
	instanceID, err := strconv.Atoi(chi.URLParam(r, "instanceID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, errors.New("invalid instance id"))
		return
	}
	var body struct {
		FloorBytes int64 `json:"floorBytes"`
	}
	if !decode(w, r, &body) {
		return
	}
	if body.FloorBytes <= 0 {
		writeError(w, http.StatusBadRequest, errors.New("floorBytes must be positive"))
		return
	}
	if err := s.store.SetFloor(r.Context(), instanceID, body.FloorBytes); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Service) handleCapacity(w http.ResponseWriter, r *http.Request) {
	instanceID, err := strconv.Atoi(chi.URLParam(r, "instanceID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, errors.New("invalid instance id"))
		return
	}
	var body struct {
		UploadCapacity int64 `json:"uploadCapacity"`
	}
	if !decode(w, r, &body) {
		return
	}
	if body.UploadCapacity < 0 {
		writeError(w, http.StatusBadRequest, errors.New("uploadCapacity must not be negative"))
		return
	}
	if err := s.store.SetUploadCapacity(r.Context(), instanceID, body.UploadCapacity); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusBadRequest, errors.New("set the safety floor first"))
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Service) handleSaveSite(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name                 string `json:"name"`
		BaseURL              string `json:"baseUrl"`
		Cookie               string `json:"cookie"`
		TorrentUploadLimit   int64  `json:"torrentUploadLimit"`
		TorrentDownloadLimit int64  `json:"torrentDownloadLimit"`
	}
	if !decode(w, r, &body) {
		return
	}
	st := store.Site{Name: strings.TrimSpace(body.Name), BaseURL: strings.TrimRight(strings.TrimSpace(body.BaseURL), "/"),
		Cookie: strings.TrimSpace(body.Cookie), TorrentUploadLimit: body.TorrentUploadLimit, TorrentDownloadLimit: body.TorrentDownloadLimit}
	if st.Name == "" || !isHTTPURL(st.BaseURL) {
		writeError(w, http.StatusBadRequest, errors.New("name and an http(s) baseUrl are required"))
		return
	}
	if st.TorrentUploadLimit < 0 || st.TorrentDownloadLimit < 0 {
		writeError(w, http.StatusBadRequest, errors.New("torrentUploadLimit and torrentDownloadLimit must not be negative"))
		return
	}
	s.saveAndRespond(w, r, func(id int64, create bool) (int64, error) {
		if create {
			return s.store.CreateSite(r.Context(), st)
		}
		st.ID = id
		return id, s.store.UpdateSite(r.Context(), st)
	})
}

// handleDeleteSite refuses a site that still has feeds, which would be left without
// a cookie to fetch with.
func (s *Service) handleDeleteSite(w http.ResponseWriter, r *http.Request) {
	s.deleteAndRespond(w, r, func(ctx context.Context, id int64) error {
		feeds, err := s.store.FeedCounts(ctx)
		if err != nil {
			return err
		}
		if feeds[id] > 0 {
			return errSiteHasFeeds
		}
		return s.store.DeleteSite(ctx, id)
	})
}

var errSiteHasFeeds = errors.New("delete the site's feeds first")

type feedBody struct {
	SiteID           int64   `json:"siteId"`
	Name             string  `json:"name"`
	URL              string  `json:"url"`
	IntervalSeconds  int     `json:"intervalSeconds"`
	MinSizeBytes     int64   `json:"minSizeBytes"`
	MaxSizeBytes     int64   `json:"maxSizeBytes"`
	FreeOnly         bool    `json:"freeOnly"`
	SkipHR           bool    `json:"skipHr"`
	WindowSeconds    int     `json:"windowSeconds"`
	AgeFromFirstSeen bool    `json:"ageFromFirstSeen"`
	Category         string  `json:"category"`
	InstanceIDs      []int   `json:"instanceIds"`
	GroupIDs         []int64 `json:"groupIds"`
	Enabled          bool    `json:"enabled"`
}

func (s *Service) handleListFeeds(w http.ResponseWriter, r *http.Request) {
	feeds, err := s.store.ListFeeds(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if feeds == nil {
		feeds = []store.Feed{}
	}
	writeJSON(w, http.StatusOK, feeds)
}

func (s *Service) handleSaveFeed(w http.ResponseWriter, r *http.Request) {
	var body feedBody
	if !decode(w, r, &body) {
		return
	}
	f := store.Feed{
		SiteID: body.SiteID, Name: strings.TrimSpace(body.Name), URL: strings.TrimSpace(body.URL),
		IntervalSeconds: body.IntervalSeconds, MinSizeBytes: body.MinSizeBytes, MaxSizeBytes: body.MaxSizeBytes,
		FreeOnly: body.FreeOnly, SkipHR: body.SkipHR, WindowSeconds: body.WindowSeconds, AgeFromFirstSeen: body.AgeFromFirstSeen, Category: strings.TrimSpace(body.Category),
		InstanceIDs: normalizeIDs(body.InstanceIDs), GroupIDs: normalizeIDs(body.GroupIDs), Enabled: body.Enabled,
	}
	create := chi.URLParam(r, "id") == ""
	if msg := validateFeed(f, create); msg != "" {
		writeError(w, http.StatusBadRequest, errors.New(msg))
		return
	}
	s.saveAndRespond(w, r, func(id int64, create bool) (int64, error) {
		if create {
			return s.store.CreateFeed(r.Context(), f)
		}
		f.ID = id
		return id, s.store.UpdateFeed(r.Context(), f)
	})
}

// normalizeIDs sorts the ids and drops duplicates.
func normalizeIDs[T int | int64](ids []T) []T {
	if ids == nil {
		return []T{}
	}
	out := slices.Clone(ids)
	slices.Sort(out)
	return slices.Compact(out)
}

func validateFeed(f store.Feed, create bool) string {
	switch {
	case f.Name == "" || f.SiteID == 0:
		return "name and siteId are required"
	case len(f.InstanceIDs) == 0 && len(f.GroupIDs) == 0:
		return "pick at least one instance or group"
	case len(f.InstanceIDs) > 0 && f.InstanceIDs[0] <= 0, len(f.GroupIDs) > 0 && f.GroupIDs[0] <= 0:
		return "instance and group ids must be positive"
	case create && !isHTTPURL(f.URL), !create && f.URL != "" && !isHTTPURL(f.URL):
		return "url must be an http(s) URL"
	case f.IntervalSeconds < minInterval:
		return "intervalSeconds must be at least 10"
	case f.WindowSeconds <= 0:
		return "windowSeconds must be positive"
	case f.MinSizeBytes < 0 || f.MaxSizeBytes < 0 || (f.MaxSizeBytes > 0 && f.MaxSizeBytes < f.MinSizeBytes):
		return "size range is invalid"
	}
	return ""
}

func (s *Service) handleDeleteFeed(w http.ResponseWriter, r *http.Request) {
	s.deleteAndRespond(w, r, s.store.DeleteFeed)
}

type testItem struct {
	Title       string `json:"title"`
	SizeBytes   int64  `json:"sizeBytes"`
	PublishedAt int64  `json:"publishedAt"`
	AgeSeconds  int64  `json:"ageSeconds"`
	InWindow    bool   `json:"inWindow"`
	SizeOK      bool   `json:"sizeOk"`
	Free        *bool  `json:"free"`
	HR          *bool  `json:"hr"`
	Rule        string `json:"rule,omitempty"`
	Error       string `json:"error,omitempty"`
}

// handleTestFeed fetches the feed now and inspects its newest items without adding
// anything, so the user can compare brush's free and H&R reading with the site.
func (s *Service) handleTestFeed(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, errors.New("invalid feed id"))
		return
	}
	f, err := s.store.GetFeed(ctx, id)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	client, err := s.siteClient(ctx, f.SiteID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	body, _, _, _, err := client.FetchFeed(ctx, f.URL, "", "")
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	items, err := site.ParseFeed(f.URL, body)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].Published.After(items[j].Published) })
	if len(items) > testItemLimit {
		items = items[:testItemLimit]
	}

	now := s.now()
	window := time.Duration(f.WindowSeconds) * time.Second
	out := make([]testItem, 0, len(items))
	for _, item := range items {
		age := feedAge(f, item.Published, now, now)
		ti := testItem{
			Title: item.Title, SizeBytes: item.Size, PublishedAt: unixOrZero(item.Published),
			AgeSeconds: int64(age / time.Second), InWindow: age <= window, SizeOK: item.Size == 0 || sizeAllowed(f, item.Size),
		}
		if insp, err := client.Inspect(ctx, item.Link); err != nil {
			ti.Error = err.Error()
		} else {
			ti.Free, ti.HR, ti.Rule = &insp.Free, &insp.HR, insp.Rule
		}
		out = append(out, ti)
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Service) handleEntries(w http.ResponseWriter, r *http.Request) {
	entries, err := s.store.RecentEntries(r.Context(), historyLimit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if entries == nil {
		entries = []store.Entry{}
	}
	writeJSON(w, http.StatusOK, entries)
}

func (s *Service) saveAndRespond(w http.ResponseWriter, r *http.Request, save func(id int64, create bool) (int64, error)) {
	param := chi.URLParam(r, "id")
	var id int64
	if param != "" {
		var err error
		if id, err = strconv.ParseInt(param, 10, 64); err != nil {
			writeError(w, http.StatusBadRequest, errors.New("invalid id"))
			return
		}
	}
	saved, err := save(id, param == "")
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]int64{"id": saved})
}

func (s *Service) deleteAndRespond(w http.ResponseWriter, r *http.Request, del func(context.Context, int64) error) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, errors.New("invalid id"))
		return
	}
	if err := del(r.Context(), id); err != nil {
		writeStoreError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func isHTTPURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(v); err != nil {
		writeError(w, http.StatusBadRequest, errors.New("invalid JSON body"))
		return false
	}
	return true
}

func writeStoreError(w http.ResponseWriter, err error) {
	if errors.Is(err, errSiteHasFeeds) || errors.Is(err, errGroupNameTaken) {
		writeError(w, http.StatusConflict, err)
		return
	}
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, err)
		return
	}
	writeError(w, http.StatusInternalServerError, err)
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, code int, err error) {
	writeJSON(w, code, map[string]string{"error": err.Error()})
}

var countryCode = regexp.MustCompile(`^[a-z]{2}$`)

func (s *Service) handleCountry(w http.ResponseWriter, r *http.Request) {
	instanceID, err := strconv.Atoi(chi.URLParam(r, "instanceID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, errors.New("invalid instance id"))
		return
	}
	var body struct {
		CountryCode string `json:"countryCode"`
	}
	if !decode(w, r, &body) {
		return
	}
	code := strings.ToLower(strings.TrimSpace(body.CountryCode))
	if code != "" && !countryCode.MatchString(code) {
		writeError(w, http.StatusBadRequest, errors.New("countryCode must be a two-letter ISO code"))
		return
	}
	if err := s.store.SetCountry(r.Context(), instanceID, code); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusBadRequest, errors.New("set the safety floor first"))
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
