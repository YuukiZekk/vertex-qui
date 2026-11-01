// Copyright (c) 2026, vertex-qui contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package store

import (
	"context"
	"fmt"
	"time"
)

// reapRetention bounds how long reap records are kept.
const reapRetention = 14 * 24 * time.Hour

// Reap reasons.
const (
	// ReapNeedSpace means the instance will need the space for new torrents soon and
	// the torrent earns clearly less than a new one would.
	ReapNeedSpace = "need_space"
	// ReapEmergency means committed space is already below the floor.
	ReapEmergency = "emergency"
)

// Reap is one torrent brush picked to delete, or would have in preview mode, and
// what it really uploaded after it was picked. It covers one horizon from FirstAt:
// LastAt and Rounds say how long it stayed picked within it.
type Reap struct {
	ID         int64  `json:"id"`
	InstanceID int    `json:"instanceId"`
	InfoHash   string `json:"infoHash"`
	Name       string `json:"name"`
	SizeBytes  int64  `json:"sizeBytes"`
	Reason     string `json:"reason"`
	// Executed is false while deletion runs in preview mode.
	Executed   bool  `json:"executed"`
	FirstAt    int64 `json:"firstAt"`
	LastAt     int64 `json:"lastAt"`
	Rounds     int   `json:"rounds"`
	AgeSeconds int64 `json:"ageSeconds"`
	// Predicted and Density are nil for a torrent whose value was still unknown, which
	// only an emergency picks.
	Predicted *int64   `json:"predicted"`
	Density   *float64 `json:"density"`
	// Replacement is what a new torrent was expected to upload per GiB, nil when there
	// was no data for it.
	Replacement *float64 `json:"replacement"`
	Committed   int64    `json:"committed"`
	Need        int64    `json:"need"`
	FloorBytes  int64    `json:"floorBytes"`
	// UploadedFirst is the torrent's cumulative upload when it was picked;
	// UploadedLast is the latest reading, at MeasuredAt, at most a horizon later.
	UploadedFirst int64 `json:"uploadedFirst"`
	UploadedLast  int64 `json:"uploadedLast"`
	MeasuredAt    int64 `json:"measuredAt"`
	// Gone means the torrent was removed by something else while it was measured.
	Gone bool `json:"gone"`
}

func (s *Store) InsertReap(ctx context.Context, r Reap) (int64, error) {
	var id int64
	err := s.db.QueryRowContext(ctx, `INSERT INTO brush_reaps (instance_id, info_hash, name, size_bytes, reason, executed,
		first_at, last_at, rounds, age_seconds, predicted, density, replacement, committed, need, floor_bytes,
		uploaded_first, uploaded_last, measured_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING id`,
		r.InstanceID, r.InfoHash, r.Name, r.SizeBytes, r.Reason, boolInt(r.Executed), r.FirstAt, r.LastAt, r.Rounds,
		r.AgeSeconds, r.Predicted, r.Density, r.Replacement, r.Committed, r.Need, r.FloorBytes, r.UploadedFirst,
		r.UploadedLast, r.MeasuredAt).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("insert brush reap: %w", err)
	}
	return id, nil
}

// PickedAgain records that a reap's torrent was picked in another round.
func (s *Store) PickedAgain(ctx context.Context, id int64, at time.Time) error {
	return s.execOne(ctx, "update brush reap", "UPDATE brush_reaps SET last_at = ?, rounds = rounds + 1 WHERE id = ?", at.Unix(), id)
}

// MeasureReap stores the torrent's latest cumulative upload, or that it is gone.
func (s *Store) MeasureReap(ctx context.Context, id int64, at time.Time, uploaded int64, gone bool) error {
	return s.execOne(ctx, "measure brush reap",
		"UPDATE brush_reaps SET uploaded_last = ?, measured_at = ?, gone = ? WHERE id = ?",
		uploaded, at.Unix(), boolInt(gone), id)
}

// RecentReaps returns the newest reaps first.
func (s *Store) RecentReaps(ctx context.Context, limit int) ([]Reap, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, instance_id, info_hash, name, size_bytes, reason, executed, first_at,
			last_at, rounds, age_seconds, predicted, density, replacement, committed, need, floor_bytes, uploaded_first,
			uploaded_last, measured_at, gone
		FROM brush_reaps ORDER BY first_at DESC, id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("list brush reaps: %w", err)
	}
	defer rows.Close()

	var out []Reap
	for rows.Next() {
		var r Reap
		var executed, gone int
		if err := rows.Scan(&r.ID, &r.InstanceID, &r.InfoHash, &r.Name, &r.SizeBytes, &r.Reason, &executed, &r.FirstAt,
			&r.LastAt, &r.Rounds, &r.AgeSeconds, &r.Predicted, &r.Density, &r.Replacement, &r.Committed, &r.Need,
			&r.FloorBytes, &r.UploadedFirst, &r.UploadedLast, &r.MeasuredAt, &gone); err != nil {
			return nil, fmt.Errorf("scan brush reap: %w", err)
		}
		r.Executed, r.Gone = executed != 0, gone != 0
		out = append(out, r)
	}
	return out, rows.Err()
}

// PruneReaps drops reaps picked before the retention period.
func (s *Store) PruneReaps(ctx context.Context, now time.Time) error {
	if _, err := s.db.ExecContext(ctx, "DELETE FROM brush_reaps WHERE first_at < ?", now.Add(-reapRetention).Unix()); err != nil {
		return fmt.Errorf("prune brush reaps: %w", err)
	}
	return nil
}

// Arrival counts the torrents a feed offered since a moment that passed its filters:
// the ones brush added, and the ones it refused only for lack of space.
type Arrival struct {
	FeedID int64
	Count  int64
	Bytes  int64
	// FirstSeen is when the feed recorded its first entry ever, so a feed younger than
	// the window is averaged over the time it has existed.
	FirstSeen int64
}

func (s *Store) Arrivals(ctx context.Context, since time.Time) ([]Arrival, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT f.id,
			COALESCE(SUM(CASE WHEN e.first_seen_at >= ? AND (e.status IN (?, ?, ?) OR e.reason LIKE 'low_space%') THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN e.first_seen_at >= ? AND (e.status IN (?, ?, ?) OR e.reason LIKE 'low_space%') THEN e.size_bytes ELSE 0 END), 0),
			COALESCE(MIN(e.first_seen_at), 0)
		FROM brush_feeds f LEFT JOIN brush_entries e ON e.feed_id = f.id
		GROUP BY f.id ORDER BY f.id`,
		since.Unix(), StatusAdding, StatusAdded, StatusUnknown, since.Unix(), StatusAdding, StatusAdded, StatusUnknown)
	if err != nil {
		return nil, fmt.Errorf("count brush arrivals: %w", err)
	}
	defer rows.Close()

	var out []Arrival
	for rows.Next() {
		var a Arrival
		if err := rows.Scan(&a.FeedID, &a.Count, &a.Bytes, &a.FirstSeen); err != nil {
			return nil, fmt.Errorf("scan brush arrivals: %w", err)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// Yield sums what young torrents really uploaded over one horizon, from checked
// predictions.
type Yield struct {
	Count  int64
	Actual int64
	Size   int64
}

// NewTorrentYield returns, per instance, what torrents younger than maxAge uploaded
// in the horizon after a prediction was made for them, for predictions made since
// a moment.
func (s *Store) NewTorrentYield(ctx context.Context, since time.Time, maxAge time.Duration) (map[int]Yield, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT instance_id, COUNT(*), COALESCE(SUM(actual), 0), COALESCE(SUM(size_bytes), 0)
		FROM brush_predictions WHERE outcome = ? AND made_at >= ? AND age_seconds < ? GROUP BY instance_id`,
		OutcomeResolved, since.Unix(), int64(maxAge/time.Second))
	if err != nil {
		return nil, fmt.Errorf("read brush new torrent yield: %w", err)
	}
	defer rows.Close()

	out := make(map[int]Yield)
	for rows.Next() {
		var id int
		var y Yield
		if err := rows.Scan(&id, &y.Count, &y.Actual, &y.Size); err != nil {
			return nil, fmt.Errorf("scan brush new torrent yield: %w", err)
		}
		out[id] = y
	}
	return out, rows.Err()
}
