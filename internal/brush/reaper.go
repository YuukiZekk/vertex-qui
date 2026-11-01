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
	"sync"
	"time"

	"github.com/autobrr/qui/internal/brush/model"
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
	reapMargin = 0.8
	minKeep    = 30 * time.Minute
	needRounds = 2
	reapRecent = 50
)

// Reaper modes of an instance.
const (
	reapOK          = "ok"
	reapWatch       = "watch"
	reapNeedSpace   = store.ReapNeedSpace
	reapEmergency   = store.ReapEmergency
	reapUnavailable = "unavailable"
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

type reapPick struct {
	Hash       string   `json:"hash"`
	Name       string   `json:"name"`
	SizeBytes  int64    `json:"sizeBytes"`
	AgeSeconds int64    `json:"ageSeconds"`
	Predicted  *int64   `json:"predicted"`
	Density    *float64 `json:"density"`
}

// reapState is the latest evaluation of one instance.
type reapState struct {
	InstanceID  int         `json:"instanceId"`
	Name        string      `json:"name"`
	At          int64       `json:"at"`
	Mode        string      `json:"mode"`
	Committed   int64       `json:"committed"`
	FloorBytes  int64       `json:"floorBytes"`
	Need        int64       `json:"need"`
	Shortfall   int64       `json:"shortfall"`
	Streak      int         `json:"streak"`
	Replacement replacement `json:"replacement"`
	Torrents    int         `json:"torrents"`
	Unknown     int         `json:"unknown"`
	Eligible    int         `json:"eligible"`
	Picks       []reapPick  `json:"picks"`
	Error       string      `json:"error,omitempty"`
}

type openReap struct {
	id         int64
	instanceID int
	hash       string
	firstAt    time.Time
}

type reaper struct {
	mu     sync.Mutex
	states map[int]reapState
	// streak and open are only touched by the reaper goroutine.
	streak map[int]int
	open   map[string]*openReap
}

func newReaper() *reaper {
	return &reaper{states: make(map[int]reapState), streak: make(map[int]int), open: make(map[string]*openReap)}
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
		for _, id := range f.InstanceIDs {
			if _, ok := configs[id]; ok {
				ids = append(ids, id)
			}
		}
		if len(f.InstanceIDs) == 0 {
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

// runReaper evaluates every instance each reapInterval. It only runs in preview
// mode: what it would delete is recorded and measured, nothing is deleted.
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
	s.measureReaps(ctx, now)

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
	names := make(map[int]string)
	if instances, err := s.qui.Instances(ctx); err == nil {
		for _, inst := range instances {
			names[inst.ID] = inst.Name
		}
	}
	need, rate := demand(feeds, arrivals, configs, now)
	byInstance := make(map[int][]torrentValue)
	for _, tv := range s.values.values(now) {
		byInstance[tv.InstanceID] = append(byInstance[tv.InstanceID], tv)
	}

	ids := make([]int, 0, len(configs))
	for id := range configs {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	states := make(map[int]reapState, len(ids))
	for _, id := range ids {
		st := reapState{InstanceID: id, Name: names[id], At: now.Unix(), FloorBytes: configs[id].FloorBytes,
			Need: need[id], Replacement: replacementFor(id, yields, rate[id]), Picks: []reapPick{}}
		if st.Name == "" {
			st.Name = fmt.Sprintf("#%d", id)
		}
		space, err := s.instanceSpace(ctx, id)
		if err == nil && !s.values.fresh(id, now) {
			err = fmt.Errorf("no recent reading of its torrents")
		}
		if err != nil {
			s.reaper.streak[id] = 0
			st.Mode, st.Error = reapUnavailable, err.Error()
			states[id] = st
			continue
		}
		st.Committed = space.Committed
		torrents := byInstance[id]
		st.Torrents = len(torrents)
		for _, tv := range torrents {
			if tv.Density == nil {
				st.Unknown++
			}
		}
		if space.Committed-st.Need < st.FloorBytes {
			s.reaper.streak[id]++
		} else {
			s.reaper.streak[id] = 0
		}
		st.Streak = s.reaper.streak[id]

		var picks []torrentValue
		st.Mode, st.Shortfall, picks, st.Eligible = pickReaps(st.Committed, st.FloorBytes, st.Need, st.Streak, st.Replacement.Value, torrents)
		for _, tv := range picks {
			st.Picks = append(st.Picks, reapPick{Hash: tv.Hash, Name: tv.Name, SizeBytes: tv.SizeBytes,
				AgeSeconds: tv.AgeSeconds, Predicted: tv.Predicted, Density: tv.Density})
			s.recordReap(ctx, now, st, tv)
		}
		states[id] = st
	}

	s.reaper.mu.Lock()
	s.reaper.states = states
	s.reaper.mu.Unlock()
	if err := s.store.PruneReaps(ctx, now); err != nil {
		s.log.Error().Err(err).Msg("prune brush reaps")
	}
}

// recordReap stores a pick, or counts another round of a pick still being measured.
func (s *Service) recordReap(ctx context.Context, now time.Time, st reapState, tv torrentValue) {
	key := reapKey(st.InstanceID, tv.Hash)
	if open, ok := s.reaper.open[key]; ok {
		if err := s.store.PickedAgain(ctx, open.id, now); err != nil {
			s.log.Error().Err(err).Msg("update brush reap")
		}
		return
	}
	id, err := s.store.InsertReap(ctx, store.Reap{
		InstanceID: st.InstanceID, InfoHash: tv.Hash, Name: tv.Name, SizeBytes: tv.SizeBytes, Reason: st.Mode,
		FirstAt: now.Unix(), LastAt: now.Unix(), Rounds: 1, AgeSeconds: tv.AgeSeconds, Predicted: tv.Predicted,
		Density: tv.Density, Replacement: st.Replacement.Value, Committed: st.Committed, Need: st.Need,
		FloorBytes: st.FloorBytes, UploadedFirst: tv.Uploaded, UploadedLast: tv.Uploaded, MeasuredAt: now.Unix(),
	})
	if err != nil {
		s.log.Error().Err(err).Msg("store brush reap")
		return
	}
	s.log.Info().Int("instance", st.InstanceID).Str("hash", tv.Hash).Str("reason", st.Mode).
		Int64("shortfall", st.Shortfall).Msg("preview: would delete torrent")
	s.reaper.open[key] = &openReap{id: id, instanceID: st.InstanceID, hash: tv.Hash, firstAt: now}
}

// measureReaps reads what every pick still inside its horizon has uploaded since it
// was picked, so the preview shows what deleting it would have cost.
func (s *Service) measureReaps(ctx context.Context, now time.Time) {
	for key, open := range s.reaper.open {
		t, present, fresh := s.values.lookup(open.instanceID, open.hash, now)
		ended := now.Sub(open.firstAt) >= model.Horizon
		if fresh {
			if err := s.store.MeasureReap(ctx, open.id, now, t.Uploaded, !present); err != nil {
				s.log.Error().Err(err).Msg("measure brush reap")
				continue
			}
		}
		if (fresh && !present) || (fresh && ended) || now.Sub(open.firstAt) > model.Horizon+lostAfter {
			delete(s.reaper.open, key)
		}
	}
}

type reaperResponse struct {
	Preview          bool         `json:"preview"`
	HorizonSeconds   int64        `json:"horizonSeconds"`
	LookaheadSeconds int64        `json:"lookaheadSeconds"`
	MarginPercent    int          `json:"marginPercent"`
	MinKeepSeconds   int64        `json:"minKeepSeconds"`
	NeedRounds       int          `json:"needRounds"`
	Instances        []reapState  `json:"instances"`
	Recent           []store.Reap `json:"recent"`
}

func (s *Service) handleReaper(w http.ResponseWriter, r *http.Request) {
	recent, err := s.store.RecentReaps(r.Context(), reapRecent)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	resp := reaperResponse{
		Preview: true, HorizonSeconds: int64(model.Horizon / time.Second),
		LookaheadSeconds: int64(needLookahead / time.Second), MarginPercent: int(math.Round((1 - reapMargin) * 100)),
		MinKeepSeconds: int64(minKeep / time.Second), NeedRounds: needRounds,
		Instances: []reapState{}, Recent: recent,
	}
	s.reaper.mu.Lock()
	for _, st := range s.reaper.states {
		resp.Instances = append(resp.Instances, st)
	}
	s.reaper.mu.Unlock()
	slices.SortFunc(resp.Instances, func(a, b reapState) int { return a.InstanceID - b.InstanceID })
	if resp.Recent == nil {
		resp.Recent = []store.Reap{}
	}
	writeJSON(w, http.StatusOK, resp)
}
