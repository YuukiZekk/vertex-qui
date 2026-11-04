// Copyright (c) 2026, vertex-qui contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package store

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// An instance may be in several groups, a feed places on the instances of its
// groups as they are now, and a feed left with nothing to place on when its group
// is deleted is disabled.
func TestGroups(t *testing.T) {
	for name, open := range dialects() {
		t.Run(name, func(t *testing.T) {
			db := open(t)
			ctx := t.Context()
			s := newTestStore(t, db)

			de, err := s.CreateGroup(ctx, "Germany")
			require.NoError(t, err)
			seed, err := s.CreateGroup(ctx, "Seeding")
			require.NoError(t, err)
			require.NoError(t, s.SetInstanceGroups(ctx, 1, []int64{de, seed}))
			require.NoError(t, s.SetInstanceGroups(ctx, 2, []int64{de}))
			require.NoError(t, s.SetInstanceGroups(ctx, 3, []int64{seed}))

			groups, err := s.ListGroups(ctx)
			require.NoError(t, err)
			require.Equal(t, []Group{{ID: de, Name: "Germany", InstanceIDs: []int{1, 2}}, {ID: seed, Name: "Seeding", InstanceIDs: []int{1, 3}}}, groups)
			byInstance, err := s.InstanceGroups(ctx)
			require.NoError(t, err)
			require.Equal(t, []int64{de, seed}, byInstance[1])

			siteID, err := s.CreateSite(ctx, Site{Name: "pt", BaseURL: "https://pt.example"})
			require.NoError(t, err)
			onlyGroup, err := s.CreateFeed(ctx, Feed{SiteID: siteID, Name: "a", URL: "https://pt.example/a", IntervalSeconds: 60,
				WindowSeconds: 300, InstanceIDs: []int{}, GroupIDs: []int64{de}, Enabled: true})
			require.NoError(t, err)
			mixed, err := s.CreateFeed(ctx, Feed{SiteID: siteID, Name: "b", URL: "https://pt.example/b", IntervalSeconds: 60,
				WindowSeconds: 300, InstanceIDs: []int{5}, GroupIDs: []int64{de}, Enabled: true})
			require.NoError(t, err)

			f, err := s.GetFeed(ctx, mixed)
			require.NoError(t, err)
			ids, all := f.Targets()
			require.False(t, all)
			require.Equal(t, []int{1, 2, 5}, ids)

			// A third instance joins the group: the feed places on it too.
			require.NoError(t, s.SetInstanceGroups(ctx, 4, []int64{de}))
			f, err = s.GetFeed(ctx, onlyGroup)
			require.NoError(t, err)
			ids, _ = f.Targets()
			require.Equal(t, []int{1, 2, 4}, ids)

			// An instance reads at the shortest interval of its groups that set one.
			require.NoError(t, s.SetGroupReadInterval(ctx, seed, 30))
			intervals, err := s.ReadIntervals(ctx)
			require.NoError(t, err)
			require.Equal(t, map[int]int{1: 30, 3: 30}, intervals)
			require.NoError(t, s.SetGroupReadInterval(ctx, de, 5))
			intervals, err = s.ReadIntervals(ctx)
			require.NoError(t, err)
			require.Equal(t, map[int]int{1: 5, 2: 5, 3: 30, 4: 5}, intervals)

			require.NoError(t, s.RenameGroup(ctx, seed, "Seed"))
			require.NoError(t, s.DeleteGroup(ctx, de))
			f, err = s.GetFeed(ctx, onlyGroup)
			require.NoError(t, err)
			require.False(t, f.Enabled, "nothing left to place on")
			f, err = s.GetFeed(ctx, mixed)
			require.NoError(t, err)
			require.True(t, f.Enabled)
			require.Equal(t, []int64{}, f.GroupIDs)

			byInstance, err = s.InstanceGroups(ctx)
			require.NoError(t, err)
			require.Equal(t, []int64{seed}, byInstance[1])
			require.ErrorIs(t, s.DeleteGroup(ctx, de), ErrNotFound)
		})
	}
}
