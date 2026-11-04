// Copyright (c) 2026, vertex-qui contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package brush

import (
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/autobrr/qui/internal/brush/store"
)

const maxGroupName = 40

func (s *Service) handleListGroups(w http.ResponseWriter, r *http.Request) {
	groups, err := s.store.ListGroups(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, groups)
}

// handleSaveGroup creates or renames a group, and sets its read interval when the
// body has one. Names are unique regardless of case.
func (s *Service) handleSaveGroup(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name                string `json:"name"`
		ReadIntervalSeconds *int   `json:"readIntervalSeconds"`
	}
	if !decode(w, r, &body) {
		return
	}
	name := strings.TrimSpace(body.Name)
	if name == "" || len([]rune(name)) > maxGroupName {
		writeError(w, http.StatusBadRequest, errors.New("name must be 1 to 40 characters"))
		return
	}
	if v := body.ReadIntervalSeconds; v != nil && *v != 0 && (*v < minReadSeconds || *v > maxReadSeconds) {
		writeError(w, http.StatusBadRequest, fmt.Errorf("readIntervalSeconds must be 0 or %d to %d", minReadSeconds, maxReadSeconds))
		return
	}
	groups, err := s.store.ListGroups(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	s.saveAndRespond(w, r, func(id int64, create bool) (int64, error) {
		for _, g := range groups {
			if g.ID != id && strings.EqualFold(g.Name, name) {
				return 0, errGroupNameTaken
			}
		}
		if create {
			created, err := s.store.CreateGroup(r.Context(), name)
			if err != nil {
				return 0, err
			}
			id = created
		} else if err := s.store.RenameGroup(r.Context(), id, name); err != nil {
			return 0, err
		}
		if body.ReadIntervalSeconds != nil {
			return id, s.store.SetGroupReadInterval(r.Context(), id, *body.ReadIntervalSeconds)
		}
		return id, nil
	})
}

var errGroupNameTaken = errors.New("a group with this name already exists")

// handleDeleteGroup removes a group. Feeds that targeted only this group are disabled.
func (s *Service) handleDeleteGroup(w http.ResponseWriter, r *http.Request) {
	s.deleteAndRespond(w, r, s.store.DeleteGroup)
}

// handleInstanceGroups sets the groups an instance is in.
func (s *Service) handleInstanceGroups(w http.ResponseWriter, r *http.Request) {
	instanceID, err := strconv.Atoi(chi.URLParam(r, "instanceID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, errors.New("invalid instance id"))
		return
	}
	var body struct {
		GroupIDs []int64 `json:"groupIds"`
	}
	if !decode(w, r, &body) {
		return
	}
	groups, err := s.store.ListGroups(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	ids := slices.Clone(body.GroupIDs)
	slices.Sort(ids)
	ids = slices.Compact(ids)
	for _, id := range ids {
		if !slices.ContainsFunc(groups, func(g store.Group) bool { return g.ID == id }) {
			writeError(w, http.StatusBadRequest, errors.New("unknown group"))
			return
		}
	}
	if err := s.store.SetInstanceGroups(r.Context(), instanceID, ids); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
