// Copyright (c) 2026, vertex-qui contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package store

import (
	"context"
	"fmt"
	"time"
)

// reapRetention bounds how long the deletion log is kept.
const reapRetention = 30 * 24 * time.Hour

// Reap reasons.
const (
	// ReapNeedSpace means the instance will need the space for new torrents soon and
	// the torrent earned clearly less than a new one would.
	ReapNeedSpace = "need_space"
	// ReapEmergency means committed space was already below the floor.
	ReapEmergency = "emergency"
	// ReapMakeRoom means a new torrent needed the space and no instance had it.
	ReapMakeRoom = "make_room"
	// ReapEarly means the torrent's feed checks young torrents and this one had
	// uploaded too little of its size by the feed's check age.
	ReapEarly = "early"
)

// Reap is one torrent brush deleted, with the numbers it decided on. Rows written
// while deletion ran in preview mode have executed = 0 and are not listed.
type Reap struct {
	ID         int64  `json:"id"`
	InstanceID int    `json:"instanceId"`
	InfoHash   string `json:"infoHash"`
	Name       string `json:"name"`
	SizeBytes  int64  `json:"sizeBytes"`
	Reason     string `json:"reason"`
	DeletedAt  int64  `json:"deletedAt"`
	AgeSeconds int64  `json:"ageSeconds"`
	// Predicted and Density are nil for a torrent whose value was still unknown, which
	// only an emergency deletes.
	Predicted *int64   `json:"predicted"`
	Density   *float64 `json:"density"`
	// Replacement is what a new torrent was expected to upload per GiB, nil when there
	// was no data for it.
	Replacement *float64 `json:"replacement"`
	Committed   int64    `json:"committed"`
	Need        int64    `json:"need"`
	FloorBytes  int64    `json:"floorBytes"`
	// Uploaded and Ratio are the torrent's totals when it was deleted.
	Uploaded int64   `json:"uploaded"`
	Ratio    float64 `json:"ratio"`
	// FilesKept means another torrent shared its files, so only the torrent was removed.
	FilesKept bool `json:"filesKept"`
}

func (s *Store) InsertReap(ctx context.Context, r Reap) (int64, error) {
	var id int64
	err := s.db.QueryRowContext(ctx, `INSERT INTO brush_reaps (instance_id, info_hash, name, size_bytes, reason, executed,
		first_at, last_at, rounds, age_seconds, predicted, density, replacement, committed, need, floor_bytes,
		uploaded_first, uploaded_last, measured_at, ratio, files_kept)
		VALUES (?, ?, ?, ?, ?, 1, ?, ?, 1, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING id`,
		r.InstanceID, r.InfoHash, r.Name, r.SizeBytes, r.Reason, r.DeletedAt, r.DeletedAt, r.AgeSeconds, r.Predicted,
		r.Density, r.Replacement, r.Committed, r.Need, r.FloorBytes, r.Uploaded, r.Uploaded, r.DeletedAt, r.Ratio,
		boolInt(r.FilesKept)).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("insert brush reap: %w", err)
	}
	return id, nil
}

// RecentReaps returns the newest deletions first. A non-empty search keeps the
// deletions whose name or info hash contains it, ignoring case.
func (s *Store) RecentReaps(ctx context.Context, search string, limit int) ([]Reap, error) {
	p := likePattern(search)
	rows, err := s.db.QueryContext(ctx, `SELECT id, instance_id, info_hash, name, size_bytes, reason, first_at, age_seconds,
			predicted, density, replacement, committed, need, floor_bytes, uploaded_first, ratio, files_kept
		FROM brush_reaps WHERE executed = 1
			AND (CAST(? AS TEXT) = '' OR LOWER(name) LIKE ? ESCAPE '!' OR LOWER(info_hash) LIKE ? ESCAPE '!')
		ORDER BY first_at DESC, id DESC LIMIT ?`, p, p, p, limit)
	if err != nil {
		return nil, fmt.Errorf("list brush reaps: %w", err)
	}
	defer rows.Close()

	var out []Reap
	for rows.Next() {
		var r Reap
		var kept int
		if err := rows.Scan(&r.ID, &r.InstanceID, &r.InfoHash, &r.Name, &r.SizeBytes, &r.Reason, &r.DeletedAt,
			&r.AgeSeconds, &r.Predicted, &r.Density, &r.Replacement, &r.Committed, &r.Need, &r.FloorBytes,
			&r.Uploaded, &r.Ratio, &kept); err != nil {
			return nil, fmt.Errorf("scan brush reap: %w", err)
		}
		r.FilesKept = kept != 0
		out = append(out, r)
	}
	return out, rows.Err()
}

// PruneReaps drops deletions older than the retention period.
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
