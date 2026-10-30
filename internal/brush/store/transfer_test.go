// Copyright (c) 2026, vertex-qui contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package store

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/autobrr/qui/internal/models"
	"github.com/autobrr/qui/internal/testutil/testdb"
)

func TestRecordTransferSplitsDaysAndIgnoresCounterDrops(t *testing.T) {
	ctx := t.Context()
	db := testdb.NewMigratedSQLite(t, t.Name())
	require.NoError(t, Migrate(ctx, db))
	cipher, err := models.NewCredentialCipher(make([]byte, 32))
	require.NoError(t, err)
	s := New(db, cipher)

	loc := time.FixedZone("CST", 8*3600)
	at := func(day, hour int) time.Time { return time.Date(2026, 10, day, hour, 0, 0, 0, loc) }
	day := func(d int) DailyTransfer {
		t.Helper()
		rows, err := s.TransfersOn(ctx, at(d, 0).Format(time.DateOnly))
		require.NoError(t, err)
		require.Len(t, rows, 1)
		return rows[0]
	}

	// The first sample is only the starting point.
	require.NoError(t, s.RecordTransfer(ctx, 1, at(30, 22), 1000, 5000))
	require.Zero(t, day(30).Uploaded)

	require.NoError(t, s.RecordTransfer(ctx, 1, at(30, 23), 1100, 5600))
	require.Equal(t, int64(600), day(30).Uploaded)
	require.Equal(t, int64(100), day(30).Downloaded)

	// After midnight the growth goes to the new day.
	require.NoError(t, s.RecordTransfer(ctx, 1, at(31, 1), 1100, 6000))
	require.Equal(t, int64(600), day(30).Uploaded)
	require.Equal(t, int64(400), day(31).Uploaded)

	// qBittorrent restarted behind its last saved counters: nothing is added, and
	// growth after that counts from the new value.
	require.NoError(t, s.RecordTransfer(ctx, 1, at(31, 2), 1100, 5900))
	require.Equal(t, int64(400), day(31).Uploaded)
	require.NoError(t, s.RecordTransfer(ctx, 1, at(31, 3), 1100, 6100))
	require.Equal(t, int64(600), day(31).Uploaded)
}
