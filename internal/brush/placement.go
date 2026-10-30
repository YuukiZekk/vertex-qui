// Copyright (c) 2026, vertex-qui contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package brush

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/autobrr/qui/internal/brush/quiadapter"
	"github.com/autobrr/qui/internal/brush/site"
	"github.com/autobrr/qui/internal/brush/store"
)

const (
	// freshPeriod is how long a torrent brush just sent counts as upload that the
	// average does not show yet: a new race torrent peaks before the meter sees it.
	freshPeriod = 2 * time.Minute
	// freshShare is the share of an instance's upload capacity each fresh torrent is
	// expected to take, so a burst of torrents spreads over instances instead of
	// all landing on the one that looked idle a moment ago.
	freshShare = 0.25
)

// candidate is how one instance scored for one torrent. It is stored with the entry
// so the history can show why an instance was picked or passed over.
type candidate struct {
	InstanceID  int     `json:"instanceId"`
	Name        string  `json:"name"`
	Reason      string  `json:"reason,omitempty"`
	Detail      string  `json:"detail,omitempty"`
	Committed   int64   `json:"committed"`
	Floor       int64   `json:"floor"`
	Upload      int64   `json:"upload"`
	UploadKnown bool    `json:"uploadKnown"`
	Fresh       int     `json:"fresh"`
	FreshUpload int64   `json:"freshUpload"`
	Capacity    int64   `json:"capacity"`
	SpaceCost   float64 `json:"spaceCost"`
	UploadCost  float64 `json:"uploadCost"`
	Cost        float64 `json:"cost"`
}

type placement struct {
	Chosen     int         `json:"chosen"`
	Candidates []candidate `json:"candidates"`
}

// admit picks the instance for a torrent and claims the entry on it under one lock,
// so torrents that arrive together are each scored against the reservations of the
// ones before them. A feed with a fixed instance is scored the same way and is
// refused rather than sent elsewhere when that instance cannot take the torrent.
//
// The cost of an instance is the share of its spare space above the floor the
// torrent would take, plus how busy its upload is: the measured average plus the
// expected pull of torrents brush sent in the last freshPeriod (capped by their
// sites' per-torrent limits), over its capacity.
// The lowest cost wins. Freeing space by deleting torrents is not part of the cost
// until brush can delete.
func (s *Service) admit(ctx context.Context, f store.Feed, entryID int64, tf site.TorrentFile) (int, string, string) {
	s.admitMu.Lock()
	defer s.admitMu.Unlock()

	configs, err := s.store.InstanceConfigs(ctx)
	if err != nil {
		return 0, reasonSpaceCheck, err.Error()
	}
	instances, err := s.qui.Instances(ctx)
	if err != nil {
		return 0, reasonInstance, err.Error()
	}
	byID := make(map[int]quiadapter.Instance, len(instances))
	for _, inst := range instances {
		byID[inst.ID] = inst
	}

	ids := []int{f.InstanceID}
	if f.InstanceID == 0 {
		ids = ids[:0]
		for id := range configs {
			ids = append(ids, id)
		}
		sort.Ints(ids)
		if len(ids) == 0 {
			return 0, reasonNoFloor, ""
		}
	}

	p := placement{Candidates: make([]candidate, 0, len(ids))}
	best := -1
	for _, id := range ids {
		cfg, configured := configs[id]
		inst, known := byID[id]
		c := s.evaluate(ctx, id, cfg, configured, inst, known, tf)
		p.Candidates = append(p.Candidates, c)
		if c.Reason == reasonDuplicate {
			// The torrent is already on one of the instances; adding it to another
			// would only split its peers.
			s.recordPlacement(ctx, entryID, p)
			return 0, reasonDuplicate, c.Name
		}
		if c.Reason == "" && (best < 0 || c.Cost < p.Candidates[best].Cost) {
			best = len(p.Candidates) - 1
		}
	}

	if best < 0 {
		s.recordPlacement(ctx, entryID, p)
		code, detail := refusal(p.Candidates)
		return 0, code, detail
	}
	chosen := p.Candidates[best]
	p.Chosen = chosen.InstanceID
	s.recordPlacement(ctx, entryID, p)
	if err := s.store.Claim(ctx, entryID, tf.InfoHash, tf.Size, chosen.InstanceID); err != nil {
		return 0, reasonSpaceCheck, err.Error()
	}
	return chosen.InstanceID, "", ""
}

func (s *Service) evaluate(ctx context.Context, id int, cfg store.InstanceConfig, configured bool,
	inst quiadapter.Instance, known bool, tf site.TorrentFile) candidate {
	c := candidate{InstanceID: id, Name: inst.Name, Floor: cfg.FloorBytes, Capacity: cfg.UploadCapacity}
	if c.Name == "" {
		c.Name = fmt.Sprintf("#%d", id)
	}
	refuse := func(code, detail string) candidate {
		c.Reason, c.Detail = code, detail
		return c
	}

	if !configured {
		return refuse(reasonNoFloor, "")
	}
	if !known || !inst.Active {
		return refuse(reasonInstance, "")
	}
	if taken, err := s.store.HashTaken(ctx, tf.InfoHash, id); err != nil {
		return refuse(reasonSpaceCheck, err.Error())
	} else if taken {
		return refuse(reasonDuplicate, "")
	}
	if has, err := s.qui.HasTorrent(ctx, id, tf.InfoHash); err != nil {
		return refuse(reasonInstance, err.Error())
	} else if has {
		return refuse(reasonDuplicate, "")
	}

	space, err := s.instanceSpace(ctx, id)
	if err != nil {
		return refuse(reasonSpaceCheck, err.Error())
	}
	c.Committed = space.Committed
	if after := space.Committed - tf.Size; after < cfg.FloorBytes {
		s.log.Warn().Int("instance", id).Int64("committedAfter", after).Int64("floor", cfg.FloorBytes).
			Msg("refused torrent: committed space would fall below the safety floor")
		return refuse(reasonLowSpace, fmt.Sprintf("%d < %d", after, cfg.FloorBytes))
	}
	c.SpaceCost = float64(tf.Size) / float64(space.Committed-cfg.FloorBytes)

	now := s.now()
	fresh, err := s.store.Reservations(ctx, id, now.Add(-freshPeriod))
	if err != nil {
		return refuse(reasonSpaceCheck, err.Error())
	}
	c.Fresh = len(fresh)
	c.Upload, c.UploadKnown = s.meter.rate(id, now)
	if c.Capacity > 0 {
		c.FreshUpload = freshPull(fresh, c.Capacity)
		c.UploadCost = float64(c.Upload+c.FreshUpload) / float64(c.Capacity)
	}
	c.Cost = c.SpaceCost + c.UploadCost
	return c
}

// freshPull is the upload the fresh torrents are expected to take: freshShare of the
// capacity each, but never more than their site lets a single torrent upload.
func freshPull(fresh []store.Reservation, capacity int64) int64 {
	share := int64(freshShare * float64(capacity))
	var total int64
	for _, r := range fresh {
		if r.UploadLimit > 0 {
			total += min(share, r.UploadLimit)
		} else {
			total += share
		}
	}
	return total
}

// refusal turns the candidates' reasons into the entry's reason. A single candidate
// keeps its own; with several, low space wins because it is the one the user can
// act on, and the detail lists every instance.
func refusal(cands []candidate) (string, string) {
	if len(cands) == 1 {
		return cands[0].Reason, cands[0].Detail
	}
	code := cands[0].Reason
	parts := make([]string, 0, len(cands))
	for _, c := range cands {
		if c.Reason == reasonLowSpace {
			code = reasonLowSpace
		}
		parts = append(parts, c.Name+" "+c.Reason)
	}
	return code, strings.Join(parts, ", ")
}

func (s *Service) recordPlacement(ctx context.Context, entryID int64, p placement) {
	data, err := json.Marshal(p)
	if err == nil {
		err = s.store.SetPlacement(ctx, entryID, data)
	}
	if err != nil {
		s.log.Error().Err(err).Int64("entry", entryID).Msg("record placement")
	}
}
