// Copyright (c) 2026, vertex-qui contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

// Package model predicts how much a torrent will upload. Placement and deletion
// both judge torrents by this one number.
package model

import (
	"math"
	"time"
)

const (
	// Horizon is how far ahead a torrent's upload is predicted.
	Horizon = 30 * time.Minute
	// RateWindow is how far back a torrent's recent upload speed reaches. The speed
	// comes from the change in its cumulative upload over the window, not from the
	// instantaneous speeds qBittorrent reports.
	RateWindow = 10 * time.Minute
)

// decayPoint says that a torrent this old is expected to average Factor times its
// last RateWindow speed over the next Horizon.
type decayPoint struct {
	Age    time.Duration
	Factor float64
}

// defaultDecay is the built-in curve used until brush has learned its own from the
// predictions it checks. Race torrents lose their peers fastest in the first hour,
// then settle into a slow tail.
var defaultDecay = []decayPoint{
	{0, 0.6},
	{time.Hour, 0.7},
	{3 * time.Hour, 0.8},
	{12 * time.Hour, 0.9},
	{48 * time.Hour, 0.95},
}

// DefaultDecay interpolates the built-in curve linearly between its points and
// holds the last factor beyond them.
func DefaultDecay(age time.Duration) float64 {
	if age <= defaultDecay[0].Age {
		return defaultDecay[0].Factor
	}
	for i := 1; i < len(defaultDecay); i++ {
		lo, hi := defaultDecay[i-1], defaultDecay[i]
		if age <= hi.Age {
			t := float64(age-lo.Age) / float64(hi.Age-lo.Age)
			return lo.Factor + t*(hi.Factor-lo.Factor)
		}
	}
	return defaultDecay[len(defaultDecay)-1].Factor
}

// LeecherBoost raises the prediction of a torrent that still has peers to upload
// to: up to half again as much, approached as the leechers grow, so a swarm that
// is still downloading counts for more but never without bound.
func LeecherBoost(leechers int) float64 {
	if leechers <= 0 {
		return 1
	}
	n := float64(leechers)
	return 1 + 0.5*n/(n+10)
}

// Input is what a prediction is made from.
type Input struct {
	Age time.Duration
	// Rate is the average upload in bytes per second over the last RateWindow.
	Rate     float64
	Leechers int
}

// Prediction is the expected upload over Horizon and the factors that made it.
type Prediction struct {
	Decay float64
	Boost float64
	Bytes int64
}

// Predict expects the recent speed to continue for Horizon, slowed by the decay
// for the torrent's age and raised by its leechers.
func Predict(in Input) Prediction {
	p := Prediction{Decay: DefaultDecay(in.Age), Boost: LeecherBoost(in.Leechers)}
	p.Bytes = int64(math.Round(math.Max(in.Rate, 0) * p.Decay * p.Boost * Horizon.Seconds()))
	return p
}
