// Copyright (c) 2026, vertex-qui contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package brush

import (
	"testing"
	"time"

	qbt "github.com/autobrr/go-qbittorrent"
	"github.com/stretchr/testify/require"

	"github.com/autobrr/qui/internal/models"
	"github.com/autobrr/qui/internal/testutil/testdb"
)

// An instance without a floor gets 10% of its estimated disk, rounded up to a GiB;
// a floor the user set is kept, and an instance whose free space is not known yet
// is tried again a minute later.
func TestEnsureFloors(t *testing.T) {
	ctx := t.Context()
	db := testdb.NewMigratedSQLite(t, t.Name())
	cipher, err := models.NewCredentialCipher(make([]byte, 32))
	require.NoError(t, err)
	qb := &fakeQB{
		free: map[int]int64{1: 895 * gib, 2: 0, 3: 50 * gib},
		listed: map[int][]qbt.Torrent{1: {
			{Hash: "a", Completed: 100 * gib},
			{Hash: "b", Completed: 5 * gib, AmountLeft: 20 * gib},
		}},
	}
	svc, err := New(ctx, db, cipher, qb)
	require.NoError(t, err)
	require.NoError(t, svc.store.SetFloor(ctx, 3, 7*gib))

	now := time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)
	svc.ensureFloors(ctx, now)
	configs, err := svc.store.InstanceConfigs(ctx)
	require.NoError(t, err)
	require.Equal(t, 100*gib, configs[1].FloorBytes, "10% of 895 + 105 GiB")
	require.Equal(t, 7*gib, configs[3].FloorBytes, "the user's floor stays")
	require.NotContains(t, configs, 2, "free space not known yet")

	qb.free[2] = 12 * gib
	svc.ensureFloors(ctx, now.Add(30*time.Second))
	configs, err = svc.store.InstanceConfigs(ctx)
	require.NoError(t, err)
	require.NotContains(t, configs, 2, "checked at most once a minute")

	svc.ensureFloors(ctx, now.Add(floorCheckEvery))
	configs, err = svc.store.InstanceConfigs(ctx)
	require.NoError(t, err)
	require.Equal(t, 2*gib, configs[2].FloorBytes, "1.2 GiB rounds up")
}
