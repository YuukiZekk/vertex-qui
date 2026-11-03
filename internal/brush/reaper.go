// Copyright (c) 2026, vertex-qui contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package brush

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	qbt "github.com/autobrr/go-qbittorrent"

	"github.com/autobrr/qui/internal/brush/store"
)

const (
	// reapInterval is how often every instance is evaluated for deletion.
	reapInterval = time.Minute
	// needLookahead is how far ahead the space new torrents will need is predicted.
	needLookahead = 30 * time.Minute
	// arrivalWindow is how far back the rate of new torrents is averaged; a feed
	// younger than minArrivalSpan has no rate yet.
	arrivalWindow  = 24 * time.Hour
	minArrivalSpan = time.Hour
	// yieldWindow is how far back checked predictions of young torrents, those under
	// newTorrentAge, tell what a new torrent uploads. Fewer than minYieldChecks on
	// an instance fall back to every instance.
	yieldWindow    = 7 * 24 * time.Hour
	newTorrentAge  = 30 * time.Minute
	minYieldChecks = 5
	// A torrent is replaced only when its value per GiB is below reapMargin of what a
	// new torrent is expected to bring, it is at least minKeep old, and space has been
	// short for needRounds evaluations in a row, so a torrent is not deleted on a
	// momentary dip.
	reapMargin    = 0.8
	minKeep       = 30 * time.Minute
	needRounds    = 2
	reapRecent    = 100
	deletedMemory = 5 * time.Minute
)

// reannounceWait gives the trackers time to take the last upload before a torrent is
// deleted, as the original Vertex does. A variable so tests need not wait.
var reannounceWait = 2 * time.Second

// Reaper modes of an instance.
const (
	reapOK        = "ok"
	reapWatch     = "watch"
	reapNeedSpace = store.ReapNeedSpace
	reapEmergency = store.ReapEmergency
)

// replacement is what deleting a torrent on an instance would make room for: the
// upload per GiB a new torrent brought there over one horizon, times the chance a
// torrent that passes the feeds' filters arrives within needLookahead.
type replacement struct {
	// Yield is nil without enough checked predictions. YieldFrom says whether it came
	// from the instance or from every instance.
	Yield       *float64 `json:"yield"`
	YieldFrom   string   `json:"yieldFrom"`
	ArrivalRate float64  `json:"arrivalRate"`
	Probability float64  `json:"probability"`
	Value       *float64 `json:"value"`
}

// reapState is one instance's evaluation in a round, kept with each deletion.
type reapState struct {
	InstanceID  int
	Mode        string
	Committed   int64
	FloorBytes  int64
	Need        int64
	Shortfall   int64
	Replacement replacement
}

// reaper is touched by the reaper goroutine, except deleted, which admission also
// uses when it makes room and which mu guards.
type reaper struct {
	mu     sync.Mutex
	streak map[int]int
	// deleted remembers what was just deleted, so a read that still lists it cannot
	// pick it again.
	deleted map[string]time.Time
	// paused holds, per instance, the downloads an emergency paused, to resume once
	// the instance is out of it. Torrents paused by anyone else are left alone.
	paused map[int][]string
}

func newReaper() *reaper {
	return &reaper{streak: make(map[int]int), deleted: make(map[string]time.Time), paused: make(map[int][]string)}
}

func reapKey(instanceID int, hash string) string {
	return fmt.Sprintf("%d|%s", instanceID, hash)
}

// pickReaps decides what one instance should delete. Below the floor everything is
// a candidate, lowest value first and unknown values last, until committed space is
// back at the floor. When space will be short within needLookahead for needRounds
// evaluations in a row, only torrents old enough and earning clearly less than a
// replacement are candidates, until the predicted need fits. Otherwise nothing is
// deleted, however little a torrent uploads, since keeping it costs nothing.
func pickReaps(committed, floor, need int64, streak int, repl *float64, torrents []torrentValue) (string, int64, []torrentValue, int) {
	var mode string
	var shortfall int64
	var eligible []torrentValue
	switch {
	case committed < floor:
		mode, shortfall = reapEmergency, floor-committed
		eligible = slices.Clone(torrents)
	case committed-need < floor:
		if streak < needRounds {
			return reapWatch, 0, nil, 0
		}
		mode, shortfall = reapNeedSpace, floor+need-committed
		if repl == nil {
			return mode, shortfall, nil, 0
		}
		limit := *repl * reapMargin
		for _, tv := range torrents {
			if tv.Density != nil && tv.AgeSeconds >= int64(minKeep/time.Second) && *tv.Density < limit {
				eligible = append(eligible, tv)
			}
		}
	default:
		return reapOK, 0, nil, 0
	}

	slices.SortFunc(eligible, func(a, b torrentValue) int {
		switch {
		case a.Density == nil && b.Density == nil:
			return int(b.AgeSeconds - a.AgeSeconds)
		case a.Density == nil:
			return 1
		case b.Density == nil:
			return -1
		case *a.Density < *b.Density:
			return -1
		case *a.Density > *b.Density:
			return 1
		}
		return 0
	})
	var picks []torrentValue
	var freed int64
	for _, tv := range eligible {
		if freed >= shortfall {
			break
		}
		// Deleting a torrent returns its whole size to committed space: what it has
		// written and what it still had to.
		picks = append(picks, tv)
		freed += tv.SizeBytes
	}
	return mode, shortfall, picks, len(eligible)
}

// demand predicts, per instance, the bytes new torrents will need within
// needLookahead and how many torrents per second may land there. A feed's torrents
// are spread evenly over its instances for the bytes, but each of its instances can
// receive any of them.
func demand(feeds []store.Feed, arrivals []store.Arrival, configs map[int]store.InstanceConfig, now time.Time) (map[int]int64, map[int]float64) {
	byFeed := make(map[int64]store.Arrival, len(arrivals))
	for _, a := range arrivals {
		byFeed[a.FeedID] = a
	}
	need, rate := make(map[int]int64), make(map[int]float64)
	for _, f := range feeds {
		a, ok := byFeed[f.ID]
		if !f.Enabled || !ok || a.FirstSeen == 0 {
			continue
		}
		span := min(arrivalWindow, now.Sub(time.Unix(a.FirstSeen, 0)))
		if span < minArrivalSpan {
			continue
		}
		var ids []int
		targets, all := f.Targets()
		for _, id := range targets {
			if _, ok := configs[id]; ok {
				ids = append(ids, id)
			}
		}
		if all {
			for id := range configs {
				ids = append(ids, id)
			}
		}
		if len(ids) == 0 {
			continue
		}
		bytes := float64(a.Bytes) / span.Seconds() * needLookahead.Seconds() / float64(len(ids))
		for _, id := range ids {
			need[id] += int64(bytes)
			rate[id] += float64(a.Count) / span.Seconds()
		}
	}
	return need, rate
}

func replacementFor(instanceID int, yields map[int]store.Yield, rate float64) replacement {
	r := replacement{ArrivalRate: rate * 3600, Probability: 1 - math.Exp(-rate*needLookahead.Seconds())}
	y, from := yields[instanceID], "instance"
	if y.Count < minYieldChecks || y.Size == 0 {
		var all store.Yield
		for _, v := range yields {
			all.Count, all.Actual, all.Size = all.Count+v.Count, all.Actual+v.Actual, all.Size+v.Size
		}
		y, from = all, "all"
	}
	if y.Count < minYieldChecks || y.Size == 0 {
		return r
	}
	yield := float64(y.Actual) / (float64(y.Size) / float64(1<<30))
	value := yield * r.Probability
	r.Yield, r.YieldFrom, r.Value = &yield, from, &value
	return r
}

// runReaper evaluates every instance each reapInterval and deletes what it picks
// while brushing is on.
func (s *Service) runReaper(ctx context.Context) {
	ticker := time.NewTicker(reapInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.reap(ctx)
		}
	}
}

func (s *Service) reap(ctx context.Context) {
	now := s.now()
	enabled, err := s.store.Enabled(ctx)
	if err != nil {
		s.log.Error().Err(err).Msg("read brush switch for reaping")
		return
	}
	configs, err := s.store.InstanceConfigs(ctx)
	if err != nil {
		s.log.Error().Err(err).Msg("read brush instances for reaping")
		return
	}
	feeds, err := s.store.ListFeeds(ctx)
	if err != nil {
		s.log.Error().Err(err).Msg("read brush feeds for reaping")
		return
	}
	arrivals, err := s.store.Arrivals(ctx, now.Add(-arrivalWindow))
	if err != nil {
		s.log.Error().Err(err).Msg("read brush arrivals")
		return
	}
	yields, err := s.store.NewTorrentYield(ctx, now.Add(-yieldWindow), newTorrentAge)
	if err != nil {
		s.log.Error().Err(err).Msg("read brush new torrent yield")
		return
	}
	need, rate := demand(feeds, arrivals, configs, now)
	byInstance := make(map[int][]torrentValue)
	for _, tv := range s.values.values(now) {
		byInstance[tv.InstanceID] = append(byInstance[tv.InstanceID], tv)
	}
	s.reaper.mu.Lock()
	for key, at := range s.reaper.deleted {
		if now.Sub(at) > deletedMemory {
			delete(s.reaper.deleted, key)
		}
	}
	s.reaper.mu.Unlock()

	ids := make([]int, 0, len(configs))
	for id := range configs {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	for _, id := range ids {
		st := reapState{InstanceID: id, FloorBytes: configs[id].FloorBytes, Need: need[id],
			Replacement: replacementFor(id, yields, rate[id])}
		space, err := s.instanceSpace(ctx, id)
		if err == nil && !s.values.fresh(id, now) {
			err = fmt.Errorf("no recent reading of its torrents")
		}
		if err != nil {
			s.reaper.streak[id] = 0
			s.alert(ctx, fmt.Sprintf("instance:%d", id), id, "读不到下载器", fmt.Sprintf("刷流无法读取这台下载器：%s", err))
			continue
		}
		st.Committed = space.Committed
		if space.Committed-st.Need < st.FloorBytes {
			s.reaper.streak[id]++
		} else {
			s.reaper.streak[id] = 0
		}

		var picks []torrentValue
		st.Mode, st.Shortfall, picks, _ = pickReaps(st.Committed, st.FloorBytes, st.Need, s.reaper.streak[id], st.Replacement.Value, byInstance[id])
		if enabled && st.Mode == reapEmergency {
			s.alert(ctx, fmt.Sprintf("emergency:%d", id), id, "低于安全底线",
				fmt.Sprintf("下载完成后剩余空间 %d GiB，低于安全底线 %d GiB；已暂停刷流下载并开始删种。", st.Committed>>30, st.FloorBytes>>30))
			s.pauseDownloads(ctx, id)
		} else if st.Mode != reapEmergency {
			s.resumeDownloads(ctx, id)
		}
		if enabled && len(picks) > 0 {
			s.deletePicks(ctx, st, picks)
		}
	}

	if err := s.store.PruneReaps(ctx, now); err != nil {
		s.log.Error().Err(err).Msg("prune brush reaps")
	}
}

// deletePicks reannounces the picks so their trackers count the last upload, waits
// for the announce except in an emergency, where space comes first, and deletes them
// one by one. A torrent whose files another torrent shares, like a cross-seed, is
// removed with its files kept. A failed deletion is logged and the rest go on; the
// next round picks again if space is still short.
func (s *Service) deletePicks(ctx context.Context, st reapState, picks []torrentValue) int64 {
	torrents, err := s.qui.Torrents(ctx, st.InstanceID)
	if err != nil {
		s.log.Error().Err(err).Int("instance", st.InstanceID).Msg("read torrents before deleting")
		return 0
	}
	listed := make(map[string]qbt.Torrent, len(torrents))
	for _, t := range torrents {
		listed[t.Hash] = t
	}
	var targets []torrentValue
	var hashes []string
	for _, tv := range picks {
		s.reaper.mu.Lock()
		_, done := s.reaper.deleted[reapKey(st.InstanceID, tv.Hash)]
		s.reaper.mu.Unlock()
		if done {
			continue
		}
		if _, ok := listed[tv.Hash]; ok {
			targets = append(targets, tv)
			hashes = append(hashes, tv.Hash)
		}
	}
	if len(targets) == 0 {
		return 0
	}

	// An emergency and a new torrent waiting for room cannot spare the wait.
	if st.Mode != reapEmergency && st.Mode != store.ReapMakeRoom {
		if err := s.qui.Reannounce(ctx, st.InstanceID, hashes); err != nil {
			s.log.Warn().Err(err).Int("instance", st.InstanceID).Msg("reannounce before deleting")
		} else {
			timer := time.NewTimer(reannounceWait)
			select {
			case <-ctx.Done():
				timer.Stop()
				return 0
			case <-timer.C:
			}
		}
	}

	var freed int64
	for _, tv := range targets {
		t := listed[tv.Hash]
		kept := sharesFiles(t, listed)
		if err := s.qui.DeleteTorrent(ctx, st.InstanceID, tv.Hash, !kept); err != nil {
			s.log.Error().Err(err).Int("instance", st.InstanceID).Str("hash", tv.Hash).Msg("delete brush torrent")
			s.alert(ctx, fmt.Sprintf("delete:%d", st.InstanceID), st.InstanceID, "删种失败", fmt.Sprintf("删除 %s 失败：%s", tv.Name, err))
			continue
		}
		now := s.now()
		s.reaper.mu.Lock()
		s.reaper.deleted[reapKey(st.InstanceID, tv.Hash)] = now
		s.reaper.mu.Unlock()
		delete(listed, tv.Hash)
		freed += tv.SizeBytes
		_, err := s.store.InsertReap(ctx, store.Reap{
			InstanceID: st.InstanceID, InfoHash: tv.Hash, Name: t.Name, SizeBytes: tv.SizeBytes, Reason: st.Mode,
			DeletedAt: now.Unix(), AgeSeconds: tv.AgeSeconds, Predicted: tv.Predicted, Density: tv.Density,
			Replacement: st.Replacement.Value, Committed: st.Committed, Need: st.Need, FloorBytes: st.FloorBytes,
			Uploaded: t.Uploaded, Ratio: t.Ratio, FilesKept: kept,
		})
		if err != nil {
			s.log.Error().Err(err).Msg("record brush deletion")
		}
		s.log.Info().Int("instance", st.InstanceID).Str("hash", tv.Hash).Str("reason", st.Mode).
			Int64("shortfall", st.Shortfall).Bool("filesKept", kept).Msg("deleted brush torrent")
	}
	return freed
}

// handleDeletions returns the latest deletions, newest first.
func (s *Service) handleDeletions(w http.ResponseWriter, r *http.Request) {
	recent, err := s.store.RecentReaps(r.Context(), reapRecent)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if recent == nil {
		recent = []store.Reap{}
	}
	writeJSON(w, http.StatusOK, recent)
}

// pausedStates are qBittorrent states of a torrent that is not running.
var pausedStates = map[qbt.TorrentState]bool{
	qbt.TorrentStatePausedDl: true, qbt.TorrentStatePausedUp: true,
	qbt.TorrentStateStoppedDl: true, qbt.TorrentStateStoppedUp: true,
}

// pauseDownloads stops every brush torrent on the instance that is still
// downloading, the first step of an emergency: pausing stops disk writes at once
// and can be undone, so it buys time while deletions free space.
func (s *Service) pauseDownloads(ctx context.Context, instanceID int) {
	torrents, err := s.qui.Torrents(ctx, instanceID)
	if err != nil {
		return
	}
	already := make(map[string]bool)
	for _, h := range s.reaper.paused[instanceID] {
		already[h] = true
	}
	var hashes []string
	for _, t := range torrents {
		if hasTag(t, Tag) && t.AmountLeft > 0 && !pausedStates[t.State] && !already[t.Hash] {
			hashes = append(hashes, t.Hash)
		}
	}
	if len(hashes) == 0 {
		return
	}
	if err := s.qui.Pause(ctx, instanceID, hashes); err != nil {
		s.log.Error().Err(err).Int("instance", instanceID).Msg("pause brush downloads in an emergency")
		return
	}
	s.reaper.paused[instanceID] = append(s.reaper.paused[instanceID], hashes...)
	s.log.Warn().Int("instance", instanceID).Int("torrents", len(hashes)).Msg("paused brush downloads: below the safety floor")
}

// resumeDownloads starts again what an emergency paused, once the instance is out
// of it. A torrent deleted meanwhile is simply not found.
func (s *Service) resumeDownloads(ctx context.Context, instanceID int) {
	hashes := s.reaper.paused[instanceID]
	if len(hashes) == 0 {
		return
	}
	if err := s.qui.Resume(ctx, instanceID, hashes); err != nil {
		s.log.Error().Err(err).Int("instance", instanceID).Msg("resume brush downloads")
		return
	}
	delete(s.reaper.paused, instanceID)
	s.log.Info().Int("instance", instanceID).Int("torrents", len(hashes)).Msg("resumed brush downloads")
}

// sharesFiles reports whether another listed torrent uses any of t's files: the
// same content path, or one inside the other, like an episode cross-seeded from a
// season pack. Such a torrent is removed with its files kept. Hardlinked or
// reflinked cross-seeds have their own paths and keep their data either way.
func sharesFiles(t qbt.Torrent, listed map[string]qbt.Torrent) bool {
	mine := normalizePath(t.ContentPath)
	if mine == "" {
		return false
	}
	for hash, other := range listed {
		if hash == t.Hash {
			continue
		}
		theirs := normalizePath(other.ContentPath)
		if theirs != "" && (theirs == mine || strings.HasPrefix(theirs, mine+"/") || strings.HasPrefix(mine, theirs+"/")) {
			return true
		}
	}
	return false
}

func normalizePath(p string) string {
	return strings.TrimRight(strings.ReplaceAll(p, "\\", "/"), "/")
}
