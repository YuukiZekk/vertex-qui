// Copyright (c) 2026, vertex-qui contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package brush

import (
	"context"
	"net/http"
	"time"
)

// transferInterval is how often every instance's all-time counters are sampled
// into the day's totals. A day boundary splits at most one interval wrongly.
const transferInterval = time.Minute

// runTransferSampler records the daily upload and download of every active
// instance, whether brush uses it or not, for the dashboard. Days follow the
// server's local time zone (TZ).
func (s *Service) runTransferSampler(ctx context.Context) {
	s.sampleTransfers(ctx)
	ticker := time.NewTicker(transferInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.sampleTransfers(ctx)
		}
	}
}

func (s *Service) sampleTransfers(ctx context.Context) {
	instances, err := s.qui.Instances(ctx)
	if err != nil {
		s.log.Error().Err(err).Msg("list instances for transfer sampling")
		return
	}
	for _, inst := range instances {
		if !inst.Active {
			continue
		}
		// Reading the torrents makes qui sync when its cache is stale, which also
		// refreshes the server state the counters come from.
		if _, err := s.qui.Torrents(ctx, inst.ID); err != nil {
			continue
		}
		dl, ul, ok, err := s.qui.AlltimeTransfer(ctx, inst.ID)
		if err != nil || !ok {
			continue
		}
		if err := s.store.RecordTransfer(ctx, inst.ID, s.now().Local(), dl, ul); err != nil {
			s.log.Error().Err(err).Int("instance", inst.ID).Msg("record daily transfer")
		}
	}
}

type transferInstance struct {
	ID         int    `json:"id"`
	Name       string `json:"name"`
	Uploaded   int64  `json:"uploaded"`
	Downloaded int64  `json:"downloaded"`
}

type transferResponse struct {
	Day string `json:"day"`
	// Elapsed is how many seconds of the day have passed, for the day's average speed.
	Elapsed   int64              `json:"elapsed"`
	Instances []transferInstance `json:"instances"`
}

// handleTransferToday returns today's totals for every instance that has them.
func (s *Service) handleTransferToday(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	now := s.now().Local()
	day := now.Format(time.DateOnly)
	totals, err := s.store.TransfersOn(ctx, day)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	instances, err := s.qui.Instances(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	names := make(map[int]string, len(instances))
	for _, inst := range instances {
		names[inst.ID] = inst.Name
	}

	midnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	resp := transferResponse{Day: day, Elapsed: int64(now.Sub(midnight) / time.Second), Instances: []transferInstance{}}
	for _, t := range totals {
		name, ok := names[t.InstanceID]
		if !ok {
			continue
		}
		resp.Instances = append(resp.Instances, transferInstance{ID: t.InstanceID, Name: name, Uploaded: t.Uploaded, Downloaded: t.Downloaded})
	}
	writeJSON(w, http.StatusOK, resp)
}
