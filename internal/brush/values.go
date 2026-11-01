// Copyright (c) 2026, vertex-qui contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package brush

import (
	"context"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	qbt "github.com/autobrr/go-qbittorrent"

	"github.com/autobrr/qui/internal/brush/model"
	"github.com/autobrr/qui/internal/brush/store"
)

const (
	// predictEvery is how often each brush torrent's prediction is stored to be
	// checked later. One every 10 minutes is enough to learn from and keeps the table
	// small.
	predictEvery = 10 * time.Minute
	// lostAfter is how long past its horizon a prediction waits for an instance that
	// cannot be read before it is given up.
	lostAfter = 30 * time.Minute
	// snapshotFresh is how old an instance's last read may be and still say whether
	// a torrent is there.
	snapshotFresh = time.Minute
)

type uploadPoint struct {
	at       time.Time
	uploaded int64
}

type trackedTorrent struct {
	torrent qbt.Torrent
	points  []uploadPoint
}

// valueTracker keeps, for every brush torrent, enough cumulative upload readings to
// span model.RateWindow, and the instance's latest read of the torrent. Readings
// live in memory, so after a restart a torrent's value is unknown until a full
// window has been read again.
type valueTracker struct {
	mu       sync.Mutex
	torrents map[int]map[string]*trackedTorrent
	readAt   map[int]time.Time
}

func newValueTracker() *valueTracker {
	return &valueTracker{torrents: make(map[int]map[string]*trackedTorrent), readAt: make(map[int]time.Time)}
}

func hasTag(t qbt.Torrent, tag string) bool {
	for _, x := range strings.Split(t.Tags, ",") {
		if strings.TrimSpace(x) == tag {
			return true
		}
	}
	return false
}

// observe records one read of an instance. Torrents brush did not add are ignored,
// and a torrent missing from the read is forgotten.
func (v *valueTracker) observe(instanceID int, at time.Time, torrents []qbt.Torrent) {
	v.mu.Lock()
	defer v.mu.Unlock()
	prev := v.torrents[instanceID]
	next := make(map[string]*trackedTorrent)
	for _, t := range torrents {
		if !hasTag(t, Tag) {
			continue
		}
		tt := prev[t.Hash]
		if tt == nil {
			tt = &trackedTorrent{}
		}
		tt.torrent = t
		tt.points = append(tt.points, uploadPoint{at: at, uploaded: t.Uploaded})
		// Keep the newest reading at least a window old as the start of the window.
		for len(tt.points) > 1 && !tt.points[1].at.After(at.Add(-model.RateWindow)) {
			tt.points = tt.points[1:]
		}
		next[t.Hash] = tt
	}
	v.torrents[instanceID] = next
	v.readAt[instanceID] = at
}

// rate is the torrent's average upload over the last model.RateWindow, false until
// its readings span the window.
func (tt *trackedTorrent) rate() (float64, bool) {
	first, last := tt.points[0], tt.points[len(tt.points)-1]
	span := last.at.Sub(first.at)
	// One sample of slack: readings are sampleInterval apart, so the span is never
	// exactly the window.
	if span < model.RateWindow-sampleInterval/2 {
		return 0, false
	}
	return float64(last.uploaded-first.uploaded) / span.Seconds(), true
}

type torrentValue struct {
	InstanceID int    `json:"instanceId"`
	Hash       string `json:"hash"`
	Name       string `json:"name"`
	SizeBytes  int64  `json:"sizeBytes"`
	AgeSeconds int64  `json:"ageSeconds"`
	Uploaded   int64  `json:"uploaded"`
	Leechers   int    `json:"leechers"`
	// Rate, Predicted and Density are nil until the torrent has a full window of readings.
	Rate      *int64   `json:"rate"`
	Predicted *int64   `json:"predicted"`
	Decay     *float64 `json:"decay"`
	// Density is the predicted upload per GiB the torrent takes once complete.
	Density *float64 `json:"density"`
}

// values predicts every tracked torrent now, leaving out instances whose last read
// is too old to describe them.
func (v *valueTracker) values(now time.Time) []torrentValue {
	v.mu.Lock()
	defer v.mu.Unlock()
	var out []torrentValue
	for instanceID, torrents := range v.torrents {
		if now.Sub(v.readAt[instanceID]) > snapshotFresh {
			continue
		}
		for _, tt := range torrents {
			t := tt.torrent
			tv := torrentValue{
				InstanceID: instanceID, Hash: t.Hash, Name: t.Name, SizeBytes: t.Size,
				AgeSeconds: max(0, now.Unix()-t.AddedOn), Uploaded: t.Uploaded, Leechers: int(t.NumIncomplete),
			}
			if rate, ok := tt.rate(); ok {
				p := model.Predict(model.Input{Age: time.Duration(tv.AgeSeconds) * time.Second, Rate: rate, Leechers: tv.Leechers})
				r := int64(rate)
				tv.Rate, tv.Predicted, tv.Decay = &r, &p.Bytes, &p.Decay
				if t.Size > 0 {
					d := float64(p.Bytes) / (float64(t.Size) / float64(1<<30))
					tv.Density = &d
				}
			}
			out = append(out, tv)
		}
	}
	slices.SortFunc(out, func(a, b torrentValue) int {
		if a.InstanceID != b.InstanceID {
			return a.InstanceID - b.InstanceID
		}
		return strings.Compare(a.Hash, b.Hash)
	})
	return out
}

// fresh reports whether the instance was read recently enough to describe it now.
func (v *valueTracker) fresh(instanceID int, now time.Time) bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	at, read := v.readAt[instanceID]
	return read && now.Sub(at) <= snapshotFresh
}

// lookup reports whether the instance's last read is fresh, and the torrent in it.
func (v *valueTracker) lookup(instanceID int, hash string, now time.Time) (qbt.Torrent, bool, bool) {
	v.mu.Lock()
	defer v.mu.Unlock()
	at, read := v.readAt[instanceID]
	if !read || now.Sub(at) > snapshotFresh {
		return qbt.Torrent{}, false, false
	}
	tt, ok := v.torrents[instanceID][hash]
	if !ok {
		return qbt.Torrent{}, false, true
	}
	return tt.torrent, true, true
}

// recordPredictions stores the current prediction of every torrent that has one,
// at most once per predictEvery. Until some torrent has a prediction it tries on
// every sample.
func (s *Service) recordPredictions(ctx context.Context, now time.Time) {
	if now.Sub(s.lastPrediction) < predictEvery {
		return
	}
	for _, tv := range s.values.values(now) {
		if tv.Predicted == nil {
			continue
		}
		s.lastPrediction = now
		err := s.store.InsertPrediction(ctx, store.Prediction{
			InstanceID: tv.InstanceID, InfoHash: tv.Hash, MadeAt: now.Unix(), AgeSeconds: tv.AgeSeconds,
			SizeBytes: tv.SizeBytes, Uploaded: tv.Uploaded, Rate: *tv.Rate, Leechers: tv.Leechers, Decay: *tv.Decay,
			HorizonSeconds: int64(model.Horizon / time.Second), Predicted: *tv.Predicted,
		})
		if err != nil {
			s.log.Error().Err(err).Msg("store brush prediction")
			return
		}
	}
	if err := s.store.PrunePredictions(ctx, now); err != nil {
		s.log.Error().Err(err).Msg("prune brush predictions")
	}
}

// resolvePredictions checks every prediction whose horizon has ended against what
// the torrent really uploaded since.
func (s *Service) resolvePredictions(ctx context.Context, now time.Time) {
	due, err := s.store.DuePredictions(ctx, now)
	if err != nil {
		s.log.Error().Err(err).Msg("list due brush predictions")
		return
	}
	for _, p := range due {
		t, present, fresh := s.values.lookup(p.InstanceID, p.InfoHash, now)
		var actual int64
		outcome := ""
		switch {
		case fresh && present:
			// A counter that went down, as after a qBittorrent crash, counts as nothing.
			actual, outcome = max(0, t.Uploaded-p.Uploaded), store.OutcomeResolved
		case fresh:
			outcome = store.OutcomeGone
		case now.Unix() > p.MadeAt+p.HorizonSeconds+int64(lostAfter/time.Second):
			outcome = store.OutcomeLost
		default:
			continue
		}
		if err := s.store.ResolvePrediction(ctx, p.ID, now, actual, now.Unix()-p.MadeAt, outcome); err != nil {
			s.log.Error().Err(err).Int64("prediction", p.ID).Msg("resolve brush prediction")
			return
		}
	}
}

type valuesResponse struct {
	HorizonSeconds int64              `json:"horizonSeconds"`
	Torrents       []torrentValue     `json:"torrents"`
	Accuracy       store.Accuracy     `json:"accuracy"`
	Recent         []store.Prediction `json:"recent"`
}

// handleValues returns every brush torrent's current prediction, how accurate the
// predictions checked in the last day were, and the latest checks.
func (s *Service) handleValues(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	now := s.now()
	accuracy, err := s.store.PredictionAccuracy(ctx, now.Add(-24*time.Hour))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	recent, err := s.store.RecentResolved(ctx, 20)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	resp := valuesResponse{
		HorizonSeconds: int64(model.Horizon / time.Second),
		Torrents:       s.values.values(now),
		Accuracy:       accuracy,
		Recent:         recent,
	}
	if resp.Torrents == nil {
		resp.Torrents = []torrentValue{}
	}
	if resp.Recent == nil {
		resp.Recent = []store.Prediction{}
	}
	writeJSON(w, http.StatusOK, resp)
}
