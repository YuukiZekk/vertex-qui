// Copyright (c) 2026, vertex-qui contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

// Package brush is the vertex-qui brushing module: RSS racing, VPS placement and
// value-based deletion. It reaches qui only through quiadapter.
package brush

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog/log"

	"github.com/autobrr/qui/internal/brush/quiadapter"
	"github.com/autobrr/qui/internal/brush/store"
	"github.com/autobrr/qui/internal/dbinterface"
)

type Service struct {
	store *store.Store
	qui   *quiadapter.Adapter
}

func New(ctx context.Context, db dbinterface.Querier, qui *quiadapter.Adapter) (*Service, error) {
	if err := store.Migrate(ctx, db); err != nil {
		return nil, err
	}
	return &Service{store: store.New(db), qui: qui}, nil
}

func (s *Service) Routes(r chi.Router) {
	r.Get("/status", s.handleStatus)
}

type instanceStatus struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	Active    bool   `json:"active"`
	Torrents  int    `json:"torrents"`
	FreeSpace int64  `json:"freeSpace"`
	Error     string `json:"error,omitempty"`
}

type statusResponse struct {
	Enabled   bool             `json:"enabled"`
	Instances []instanceStatus `json:"instances"`
}

func (s *Service) handleStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	enabled, err := s.store.Enabled(ctx)
	if err != nil {
		writeError(w, err)
		return
	}
	instances, err := s.qui.Instances(ctx)
	if err != nil {
		writeError(w, err)
		return
	}

	resp := statusResponse{Enabled: enabled, Instances: make([]instanceStatus, 0, len(instances))}
	for _, inst := range instances {
		resp.Instances = append(resp.Instances, s.instanceStatus(ctx, inst))
	}
	writeJSON(w, http.StatusOK, resp)
}

// instanceStatus reports one instance; a disabled or unreachable instance shows its
// error instead of failing the whole response.
func (s *Service) instanceStatus(ctx context.Context, inst quiadapter.Instance) instanceStatus {
	status := instanceStatus{ID: inst.ID, Name: inst.Name, Active: inst.Active}
	if !inst.Active {
		return status
	}
	torrents, err := s.qui.Torrents(ctx, inst.ID)
	if err != nil {
		status.Error = err.Error()
		return status
	}
	status.Torrents = len(torrents)
	free, err := s.qui.FreeSpace(ctx, inst.ID)
	if err != nil {
		status.Error = err.Error()
		return status
	}
	status.FreeSpace = free
	return status
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Error().Err(err).Msg("brush: encode response")
	}
}

func writeError(w http.ResponseWriter, err error) {
	log.Error().Err(err).Msg("brush: request failed")
	writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
}
