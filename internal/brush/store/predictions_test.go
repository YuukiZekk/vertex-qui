// Copyright (c) 2026, vertex-qui contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package store

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestPredictionsDueResolveAndAccuracy(t *testing.T) {
	for name, open := range dialects() {
		t.Run(name, func(t *testing.T) {
			db := open(t)
			ctx := t.Context()
			s := newTestStore(t, db)

			made := time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)
			for _, hash := range []string{"a", "b"} {
				require.NoError(t, s.InsertPrediction(ctx, Prediction{InstanceID: 1, InfoHash: hash, MadeAt: made.Unix(),
					AgeSeconds: 600, SizeBytes: 1 << 30, Uploaded: 100, Rate: 10, Leechers: 2, Decay: 0.65,
					HorizonSeconds: 1800, Predicted: 1000}))
			}

			due, err := s.DuePredictions(ctx, made.Add(29*time.Minute))
			require.NoError(t, err)
			require.Empty(t, due)
			at := made.Add(30*time.Minute + 5*time.Second)
			due, err = s.DuePredictions(ctx, at)
			require.NoError(t, err)
			require.Len(t, due, 2)
			require.Equal(t, int64(100), due[0].Uploaded)

			require.NoError(t, s.ResolvePrediction(ctx, due[0].ID, at, 1500, 1805, OutcomeResolved))
			require.NoError(t, s.ResolvePrediction(ctx, due[1].ID, at, 0, 1805, OutcomeGone))
			due, err = s.DuePredictions(ctx, at)
			require.NoError(t, err)
			require.Empty(t, due)

			acc, err := s.PredictionAccuracy(ctx, made)
			require.NoError(t, err)
			require.Equal(t, Accuracy{Count: 1, Predicted: 1000, Actual: 1500, AbsError: 500, Gone: 1}, acc)

			recent, err := s.RecentResolved(ctx, 10)
			require.NoError(t, err)
			require.Len(t, recent, 1)
			require.Equal(t, "a", recent[0].InfoHash)
			require.InDelta(t, 0.65, recent[0].Decay, 1e-9)

			require.NoError(t, s.PrunePredictions(ctx, made.Add(predictionRetention+time.Second)))
			acc, err = s.PredictionAccuracy(ctx, made)
			require.NoError(t, err)
			require.Zero(t, acc.Count+acc.Gone)
		})
	}
}
