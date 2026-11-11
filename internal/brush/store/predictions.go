// Copyright (c) 2026, vertex-qui contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package store

import (
	"context"
	"fmt"
	"time"
)

// predictionRetention bounds how long checked predictions are kept to learn from.
const predictionRetention = 14 * 24 * time.Hour

// Outcomes of a prediction once its horizon has passed.
const (
	// OutcomeResolved means the torrent was still there and Actual is what it uploaded.
	OutcomeResolved = "resolved"
	// OutcomeGone means the torrent was removed before the horizon ended.
	OutcomeGone = "gone"
	// OutcomeLost means the instance could not be read until well after the horizon.
	OutcomeLost = "lost"
)

// Prediction is one torrent's predicted upload over the next horizon, with the
// inputs it was made from, and what the torrent really uploaded once checked.
type Prediction struct {
	ID         int64  `json:"id"`
	InstanceID int    `json:"instanceId"`
	InfoHash   string `json:"infoHash"`
	Title      string `json:"title"`
	MadeAt     int64  `json:"madeAt"`
	AgeSeconds int64  `json:"ageSeconds"`
	SizeBytes  int64  `json:"sizeBytes"`
	// Uploaded is the torrent's cumulative upload when the prediction was made.
	Uploaded       int64   `json:"uploaded"`
	Rate           int64   `json:"rate"`
	Leechers       int     `json:"leechers"`
	Decay          float64 `json:"decay"`
	HorizonSeconds int64   `json:"horizonSeconds"`
	Predicted      int64   `json:"predicted"`
	ResolvedAt     int64   `json:"resolvedAt"`
	Actual         int64   `json:"actual"`
	// ActualSeconds is how long Actual was measured over, the horizon plus up to one
	// sample of lateness.
	ActualSeconds int64  `json:"actualSeconds"`
	Outcome       string `json:"outcome"`
}

func (s *Store) InsertPrediction(ctx context.Context, p Prediction) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO brush_predictions (instance_id, info_hash, made_at, age_seconds,
		size_bytes, uploaded, rate, leechers, decay, horizon_seconds, predicted) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.InstanceID, p.InfoHash, p.MadeAt, p.AgeSeconds, p.SizeBytes, p.Uploaded, p.Rate, p.Leechers, p.Decay,
		p.HorizonSeconds, p.Predicted)
	if err != nil {
		return fmt.Errorf("insert brush prediction: %w", err)
	}
	return nil
}

// DuePredictions returns the unchecked predictions whose horizon ended by now.
func (s *Store) DuePredictions(ctx context.Context, now time.Time) ([]Prediction, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, instance_id, info_hash, made_at, uploaded, horizon_seconds
		FROM brush_predictions WHERE resolved_at = 0 AND made_at + horizon_seconds <= ? ORDER BY id`, now.Unix())
	if err != nil {
		return nil, fmt.Errorf("list due brush predictions: %w", err)
	}
	defer rows.Close()

	var out []Prediction
	for rows.Next() {
		var p Prediction
		if err := rows.Scan(&p.ID, &p.InstanceID, &p.InfoHash, &p.MadeAt, &p.Uploaded, &p.HorizonSeconds); err != nil {
			return nil, fmt.Errorf("scan brush prediction: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) ResolvePrediction(ctx context.Context, id int64, at time.Time, actual, actualSeconds int64, outcome string) error {
	return s.execOne(ctx, "resolve brush prediction",
		"UPDATE brush_predictions SET resolved_at = ?, actual = ?, actual_seconds = ?, outcome = ? WHERE id = ?",
		at.Unix(), actual, actualSeconds, outcome, id)
}

// PrunePredictions drops predictions made before the retention period.
func (s *Store) PrunePredictions(ctx context.Context, now time.Time) error {
	if _, err := s.db.ExecContext(ctx, "DELETE FROM brush_predictions WHERE made_at < ?", now.Add(-predictionRetention).Unix()); err != nil {
		return fmt.Errorf("prune brush predictions: %w", err)
	}
	return nil
}

// Accuracy sums the predictions checked since a moment.
type Accuracy struct {
	Count     int64 `json:"count"`
	Predicted int64 `json:"predicted"`
	Actual    int64 `json:"actual"`
	// AbsError is the sum of every prediction's absolute miss, so AbsError / Actual
	// is the error weighted by how much the torrents uploaded.
	AbsError int64 `json:"absError"`
	Gone     int64 `json:"gone"`
}

func (s *Store) PredictionAccuracy(ctx context.Context, since time.Time) (Accuracy, error) {
	var a Accuracy
	err := s.db.QueryRowContext(ctx, `SELECT
			COALESCE(SUM(CASE WHEN outcome = ? THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN outcome = ? THEN predicted ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN outcome = ? THEN actual ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN outcome = ? THEN ABS(predicted - actual) ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN outcome = ? THEN 1 ELSE 0 END), 0)
		FROM brush_predictions WHERE resolved_at >= ?`,
		OutcomeResolved, OutcomeResolved, OutcomeResolved, OutcomeResolved, OutcomeGone, since.Unix()).
		Scan(&a.Count, &a.Predicted, &a.Actual, &a.AbsError, &a.Gone)
	if err != nil {
		return Accuracy{}, fmt.Errorf("read brush prediction accuracy: %w", err)
	}
	return a, nil
}

// RecentResolved returns the latest checked predictions, newest first, with the
// title of the entry that added the torrent when brush added it.
func (s *Store) RecentResolved(ctx context.Context, limit int) ([]Prediction, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT p.id, p.instance_id, p.info_hash, COALESCE(e.title, ''), p.made_at,
			p.age_seconds, p.size_bytes, p.uploaded, p.rate, p.leechers, p.decay, p.horizon_seconds, p.predicted,
			p.resolved_at, p.actual, p.actual_seconds, p.outcome
		FROM brush_predictions p
		LEFT JOIN brush_entries e ON e.info_hash = p.info_hash AND e.instance_id = p.instance_id AND e.status = 'added'
		WHERE p.outcome = ? ORDER BY p.resolved_at DESC, p.id DESC LIMIT ?`, OutcomeResolved, limit)
	if err != nil {
		return nil, fmt.Errorf("list resolved brush predictions: %w", err)
	}
	defer rows.Close()

	var out []Prediction
	for rows.Next() {
		var p Prediction
		if err := rows.Scan(&p.ID, &p.InstanceID, &p.InfoHash, &p.Title, &p.MadeAt, &p.AgeSeconds, &p.SizeBytes,
			&p.Uploaded, &p.Rate, &p.Leechers, &p.Decay, &p.HorizonSeconds, &p.Predicted, &p.ResolvedAt, &p.Actual,
			&p.ActualSeconds, &p.Outcome); err != nil {
			return nil, fmt.Errorf("scan brush prediction: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// CalibrationKey is one feed's torrents in one age band.
type CalibrationKey struct {
	FeedID int64
	Band   int
}

// Calibration sums the checked predictions of one feed's torrents in one age band.
type Calibration struct {
	Count     int64
	Predicted int64
	Actual    int64
}

// FeedCalibration sums, per feed and age band, the predictions checked since a
// moment for torrents a feed added. bands are the ages in seconds where a band ends:
// band 0 is younger than bands[0], band len(bands) at least the last.
// The status is spelled out as the partial index on entries is, so SQLite uses it
// instead of scanning every entry of the instance for each prediction.
func (s *Store) FeedCalibration(ctx context.Context, since time.Time, bands []int64) (map[CalibrationKey]Calibration, error) {
	band := "0"
	args := make([]any, 0, len(bands)+3)
	for _, b := range bands {
		band += " + CASE WHEN p.age_seconds >= ? THEN 1 ELSE 0 END"
		args = append(args, b)
	}
	args = append(args, OutcomeResolved, since.Unix())
	rows, err := s.db.QueryContext(ctx, `SELECT e.feed_id, `+band+` AS band, COUNT(*),
			COALESCE(SUM(p.predicted), 0), COALESCE(SUM(p.actual), 0)
		FROM brush_predictions p
		JOIN brush_entries e ON e.info_hash = p.info_hash AND e.instance_id = p.instance_id
			AND e.status IN ('adding', 'added') AND e.status = 'added'
		WHERE p.outcome = ? AND p.made_at >= ?
		GROUP BY e.feed_id, band`, args...)
	if err != nil {
		return nil, fmt.Errorf("read brush feed calibration: %w", err)
	}
	defer rows.Close()

	out := make(map[CalibrationKey]Calibration)
	for rows.Next() {
		var k CalibrationKey
		var c Calibration
		if err := rows.Scan(&k.FeedID, &k.Band, &c.Count, &c.Predicted, &c.Actual); err != nil {
			return nil, fmt.Errorf("scan brush feed calibration: %w", err)
		}
		out[k] = c
	}
	return out, rows.Err()
}
