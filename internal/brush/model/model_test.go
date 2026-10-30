// Copyright (c) 2026, vertex-qui contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package model

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDefaultDecay(t *testing.T) {
	require.InDelta(t, 0.6, DefaultDecay(0), 1e-9)
	require.InDelta(t, 0.65, DefaultDecay(30*time.Minute), 1e-9, "halfway between 0 and 1 hour")
	require.InDelta(t, 0.7, DefaultDecay(time.Hour), 1e-9)
	require.InDelta(t, 0.95, DefaultDecay(30*24*time.Hour), 1e-9, "held past the last point")
}

func TestLeecherBoost(t *testing.T) {
	require.InDelta(t, 1.0, LeecherBoost(0), 1e-9)
	require.InDelta(t, 1.25, LeecherBoost(10), 1e-9)
	require.Less(t, LeecherBoost(1000), 1.5)
	require.Greater(t, LeecherBoost(20), LeecherBoost(10))
}

func TestPredict(t *testing.T) {
	// 10 MiB/s, one hour old, no leechers: 0.7 of 10 MiB/s for 1800 s.
	p := Predict(Input{Age: time.Hour, Rate: 10 << 20})
	require.Equal(t, int64(0.7*float64(10<<20)*1800), p.Bytes)

	require.Zero(t, Predict(Input{Age: time.Hour, Rate: -5}).Bytes, "a counter that went down predicts nothing")
}
