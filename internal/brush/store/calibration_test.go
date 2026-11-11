// Copyright (c) 2026, vertex-qui contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package store

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// FeedCalibration sums checked predictions per feed and age band, only for torrents
// a feed added, only resolved ones, and only since the given time.
func TestFeedCalibration(t *testing.T) {
	for name, open := range dialects() {
		t.Run(name, func(t *testing.T) {
			db := open(t)
			ctx := t.Context()
			s := newTestStore(t, db)

			siteID, err := s.CreateSite(ctx, Site{Name: "pt", BaseURL: "https://pt.example"})
			require.NoError(t, err)
			feedID, err := s.CreateFeed(ctx, Feed{SiteID: siteID, Name: "a", URL: "https://pt.example/a", IntervalSeconds: 60,
				WindowSeconds: 300, InstanceIDs: []int{1}, Enabled: true})
			require.NoError(t, err)
			id, _, err := s.InsertEntry(ctx, Entry{FeedID: feedID, GUID: "g", Title: "t", Status: StatusPending})
			require.NoError(t, err)
			require.NoError(t, s.Claim(ctx, id, "aa", 10, 1))
			require.NoError(t, s.MarkAdded(ctx, id))

			made := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
			predict := func(hash string, instance int, madeAt time.Time, age, predicted, actual int64, outcome string) {
				require.NoError(t, s.InsertPrediction(ctx, Prediction{InstanceID: instance, InfoHash: hash, MadeAt: madeAt.Unix(),
					AgeSeconds: age, SizeBytes: 1 << 30, HorizonSeconds: 1800, Predicted: predicted}))
				due, err := s.DuePredictions(ctx, madeAt.Add(time.Hour))
				require.NoError(t, err)
				require.NoError(t, s.ResolvePrediction(ctx, due[len(due)-1].ID, madeAt.Add(time.Hour), actual, 1800, outcome))
			}
			predict("aa", 1, made, 600, 1000, 50, OutcomeResolved)                   // band 0
			predict("aa", 1, made, 1200, 1000, 70, OutcomeResolved)                  // band 0
			predict("aa", 1, made, 7200, 1000, 600, OutcomeResolved)                 // band 1
			predict("aa", 1, made, 30000, 1000, 900, OutcomeResolved)                // band 2
			predict("aa", 1, made, 600, 1000, 0, OutcomeGone)                        // not resolved
			predict("aa", 1, made.Add(-48*time.Hour), 600, 1000, 0, OutcomeResolved) // too old
			predict("aa", 2, made, 600, 1000, 0, OutcomeResolved)                    // another instance: not this feed's torrent
			predict("bb", 1, made, 600, 1000, 0, OutcomeResolved)                    // not added by a feed

			got, err := s.FeedCalibration(ctx, made.Add(-time.Hour), []int64{3600, 21600})
			require.NoError(t, err)
			require.Equal(t, map[CalibrationKey]Calibration{
				{FeedID: feedID, Band: 0}: {Count: 2, Predicted: 2000, Actual: 120},
				{FeedID: feedID, Band: 1}: {Count: 1, Predicted: 1000, Actual: 600},
				{FeedID: feedID, Band: 2}: {Count: 1, Predicted: 1000, Actual: 900},
			}, got)
		})
	}
}
