// Copyright (c) 2026, vertex-qui contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package quiadapter

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/autobrr/qui/internal/models"
	"github.com/autobrr/qui/internal/qbittorrent"
	"github.com/autobrr/qui/internal/testutil/testdb"
)

// The brush tests use a fake qBittorrent, so this runs a delete through qui's
// real sync manager: qui refuses deletes that do not say who started them, and
// that refusal comes before any qBittorrent is reached.
func TestDeleteTorrentGetsPastQuisDeleteChecks(t *testing.T) {
	db := testdb.NewMigratedSQLite(t, t.Name())
	instances, err := models.NewInstanceStore(db, make([]byte, 32))
	require.NoError(t, err)
	pool, err := qbittorrent.NewClientPool(instances, models.NewInstanceErrorStore(db), time.Second)
	require.NoError(t, err)
	require.NoError(t, pool.Close())
	a := New(instances, qbittorrent.NewSyncManager(pool, nil))

	for _, withFiles := range []bool{false, true} {
		err := a.DeleteTorrent(t.Context(), 1, "abc", withFiles)
		require.ErrorIs(t, err, qbittorrent.ErrPoolClosed, "withFiles=%v: the delete stopped before reaching qBittorrent", withFiles)
	}
}
