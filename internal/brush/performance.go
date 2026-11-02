// Copyright (c) 2026, vertex-qui contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package brush

import (
	"errors"
	"fmt"
	"math"
	"net/http"
	"slices"
	"strconv"
	"time"

	"github.com/autobrr/qui/internal/brush/store"
)

const (
	// slowEntry is the entry delay past which a bucket counts as slow.
	slowEntry = 20 * time.Second
	// firstUploadWithin is how soon after the add a torrent counts as starting fast;
	// a torrent is only judged once firstUploadJudged has passed since its add.
	firstUploadWithin = 30 * time.Second
	firstUploadJudged = time.Minute
)

// perfStats is how fast brush got torrents in: discovery is publish to first seen,
// processing is first seen to qBittorrent's confirmation, and entry is both. Delays
// are in seconds and nil without samples.
type perfStats struct {
	Added          int      `json:"added"`
	Failed         int      `json:"failed"`
	Pending        int      `json:"pending"`
	DiscoveryP50   *float64 `json:"discoveryP50"`
	DiscoveryP95   *float64 `json:"discoveryP95"`
	ProcessingP50  *float64 `json:"processingP50"`
	ProcessingP95  *float64 `json:"processingP95"`
	EntryP95       *float64 `json:"entryP95"`
	FastStart      int      `json:"fastStart"`
	FastStartJudge int      `json:"fastStartJudged"`
}

type perfBucket struct {
	Start    int64    `json:"start"`
	Added    int      `json:"added"`
	EntryP95 *float64 `json:"entryP95"`
}

type perfGroup struct {
	Key     string       `json:"key"`
	ID      int64        `json:"id"`
	Name    string       `json:"name"`
	Stats   perfStats    `json:"stats"`
	Buckets []perfBucket `json:"buckets"`
	Slow    int          `json:"slow"`
}

type perfResponse struct {
	Since         int64       `json:"since"`
	BucketSeconds int64       `json:"bucketSeconds"`
	SlowSeconds   int64       `json:"slowSeconds"`
	Overall       perfStats   `json:"overall"`
	Groups        []perfGroup `json:"groups"`
}

// bucketFor picks a bucket size that gives a few dozen bars for the window.
func bucketFor(window time.Duration) time.Duration {
	switch {
	case window <= time.Hour:
		return 5 * time.Minute
	case window <= 6*time.Hour:
		return 15 * time.Minute
	case window <= 24*time.Hour:
		return 30 * time.Minute
	default:
		return 4 * time.Hour
	}
}

// percentile is the nearest-rank percentile of the values, nil when there are none.
func percentile(values []float64, p float64) *float64 {
	if len(values) == 0 {
		return nil
	}
	sorted := slices.Clone(values)
	slices.Sort(sorted)
	idx := int(math.Ceil(p/100*float64(len(sorted)))) - 1
	v := sorted[max(0, min(idx, len(sorted)-1))]
	return &v
}

type perfSample struct {
	entry      store.PerfEntry
	discovery  *float64
	processing *float64
}

func sampleOf(e store.PerfEntry, countDiscovery bool) perfSample {
	s := perfSample{entry: e}
	// A pubDate is only trusted when it is not after the first sight; revival feeds,
	// whose pubDate is the original upload, have no discovery delay to speak of.
	if countDiscovery && e.PublishedAt > 0 && e.PublishedAt <= e.FirstSeenAt {
		d := float64(e.FirstSeenAt - e.PublishedAt)
		s.discovery = &d
	}
	if e.Status == store.StatusAdded && e.AddedAt >= e.FirstSeenAt {
		p := float64(e.AddedAt - e.FirstSeenAt)
		s.processing = &p
	}
	return s
}

func (s perfSample) entryDelay() *float64 {
	if s.processing == nil {
		return nil
	}
	total := *s.processing
	if s.discovery != nil {
		total += *s.discovery
	}
	return &total
}

func statsOf(samples []perfSample, now time.Time) perfStats {
	var st perfStats
	var disc, proc, entry []float64
	for _, s := range samples {
		switch s.entry.Status {
		case store.StatusAdded:
			st.Added++
		case store.StatusFailed:
			st.Failed++
		default:
			st.Pending++
		}
		if s.discovery != nil {
			disc = append(disc, *s.discovery)
		}
		if s.processing != nil {
			proc = append(proc, *s.processing)
			entry = append(entry, *s.entryDelay())
		}
		e := s.entry
		if e.Status == store.StatusAdded && e.AddedAt > 0 && now.Unix()-e.AddedAt >= int64(firstUploadJudged/time.Second) {
			st.FastStartJudge++
			if e.FirstUploadAt > 0 && e.FirstUploadAt-e.AddedAt <= int64(firstUploadWithin/time.Second) {
				st.FastStart++
			}
		}
	}
	st.DiscoveryP50, st.DiscoveryP95 = percentile(disc, 50), percentile(disc, 95)
	st.ProcessingP50, st.ProcessingP95 = percentile(proc, 50), percentile(proc, 95)
	st.EntryP95 = percentile(entry, 95)
	return st
}

// handlePerformance summarises how fast brush got torrents in over the last hours,
// per site or per instance, with the entry delay P95 per time bucket. It reports
// what happened; nothing in placement or deletion reads it.
func (s *Service) handlePerformance(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	hours, err := strconv.Atoi(r.URL.Query().Get("hours"))
	if err != nil || hours <= 0 {
		hours = 24
	}
	hours = min(hours, 24*7)
	byInstance := r.URL.Query().Get("by") == "instance"
	now := s.now()
	window := time.Duration(hours) * time.Hour
	bucket := bucketFor(window)
	since := now.Add(-window).Truncate(bucket)

	entries, err := s.store.PerfEntries(ctx, since)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	feeds, err := s.store.ListFeeds(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	revival := make(map[int64]bool)
	for _, f := range feeds {
		revival[f.ID] = f.AgeFromFirstSeen
	}
	names := make(map[int64]string)
	if byInstance {
		if instances, err := s.qui.Instances(ctx); err == nil {
			for _, inst := range instances {
				names[int64(inst.ID)] = inst.Name
			}
		}
	} else if sites, err := s.store.ListSites(ctx); err == nil {
		for _, st := range sites {
			names[st.ID] = st.Name
		}
	}

	all := make([]perfSample, 0, len(entries))
	grouped := make(map[int64][]perfSample)
	for _, e := range entries {
		sample := sampleOf(e, !revival[e.FeedID])
		all = append(all, sample)
		key := e.SiteID
		if byInstance {
			key = int64(e.InstanceID)
		}
		grouped[key] = append(grouped[key], sample)
	}

	nBuckets := int(now.Sub(since)/bucket) + 1
	resp := perfResponse{
		Since: since.Unix(), BucketSeconds: int64(bucket / time.Second), SlowSeconds: int64(slowEntry / time.Second),
		Overall: statsOf(all, now), Groups: []perfGroup{},
	}
	for id, samples := range grouped {
		g := perfGroup{ID: id, Name: names[id], Stats: statsOf(samples, now)}
		if byInstance {
			g.Key = "instance"
		} else {
			g.Key = "site"
		}
		if g.Name == "" {
			g.Name = fmt.Sprintf("#%d", id)
		}
		perBucket := make([][]float64, nBuckets)
		for _, sm := range samples {
			d := sm.entryDelay()
			if d == nil {
				continue
			}
			i := int((sm.entry.FirstSeenAt - since.Unix()) / int64(bucket/time.Second))
			if i >= 0 && i < nBuckets {
				perBucket[i] = append(perBucket[i], *d)
			}
		}
		g.Buckets = make([]perfBucket, nBuckets)
		for i, values := range perBucket {
			b := perfBucket{Start: since.Add(time.Duration(i) * bucket).Unix(), Added: len(values), EntryP95: percentile(values, 95)}
			if b.EntryP95 != nil && *b.EntryP95 > slowEntry.Seconds() {
				g.Slow++
			}
			g.Buckets[i] = b
		}
		resp.Groups = append(resp.Groups, g)
	}
	slices.SortFunc(resp.Groups, func(a, b perfGroup) int { return b.Stats.Added - a.Stats.Added })
	writeJSON(w, http.StatusOK, resp)
}

// perfDetailLimit bounds a drill-down list; a busy week can hold thousands.
const perfDetailLimit = 500

// handlePerformanceEntries lists the entries behind one group of the overview,
// for its whole window or one bucket, each with its stage times for a timeline.
func (s *Service) handlePerformanceEntries(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	id, err := strconv.ParseInt(q.Get("id"), 10, 64)
	from, err2 := strconv.ParseInt(q.Get("from"), 10, 64)
	to, err3 := strconv.ParseInt(q.Get("to"), 10, 64)
	if err != nil || err2 != nil || err3 != nil || to <= from {
		writeError(w, http.StatusBadRequest, errors.New("id, from and to are required, with from before to"))
		return
	}
	var siteID int64
	var instanceID int
	if q.Get("by") == "instance" {
		instanceID = int(id)
	} else {
		siteID = id
	}
	details, err := s.store.PerfDetails(r.Context(), time.Unix(from, 0), time.Unix(to, 0), siteID, instanceID, perfDetailLimit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if details == nil {
		details = []store.PerfDetail{}
	}
	writeJSON(w, http.StatusOK, details)
}
