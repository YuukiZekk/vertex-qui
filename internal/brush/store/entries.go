// Copyright (c) 2026, vertex-qui contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// Entry statuses. "adding" is held while the add request is in flight; the unique
// index on (info_hash, instance_id) over adding/added stops a second feed that
// carries the same torrent from adding it to the same instance.
const (
	StatusBaseline = "baseline"
	StatusPending  = "pending"
	StatusRejected = "rejected"
	StatusAdding   = "adding"
	StatusAdded    = "added"
	StatusFailed   = "failed"
	// StatusUnknown marks an add that was in flight when qui stopped. It is never
	// retried, because qBittorrent may already have the torrent.
	StatusUnknown = "unknown"
)

type Entry struct {
	ID          int64  `json:"id"`
	FeedID      int64  `json:"feedId"`
	FeedName    string `json:"feedName"`
	GUID        string `json:"-"`
	Title       string `json:"title"`
	SizeBytes   int64  `json:"sizeBytes"`
	Link        string `json:"link"`
	PublishedAt int64  `json:"publishedAt"`
	FirstSeenAt int64  `json:"firstSeenAt"`
	Status      string `json:"status"`
	Reason      string `json:"reason"`
	InfoHash    string `json:"infoHash"`
	InstanceID  int    `json:"instanceId"`
	DecidedAt   int64  `json:"decidedAt"`
	// Placement is the JSON record of how each candidate instance was scored, empty
	// when the entry never reached placement.
	Placement json.RawMessage `json:"placement,omitempty"`
}

// InsertEntry records a newly seen RSS item. It returns false when the feed has
// already seen this guid.
func (s *Store) InsertEntry(ctx context.Context, e Entry) (int64, bool, error) {
	var id int64
	err := s.db.QueryRowContext(ctx, `INSERT INTO brush_entries (feed_id, guid, title, size_bytes, link, published_at,
		first_seen_at, status, reason, decided_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (feed_id, guid) DO NOTHING RETURNING id`,
		e.FeedID, e.GUID, e.Title, e.SizeBytes, e.Link, e.PublishedAt, e.FirstSeenAt, e.Status, e.Reason, e.DecidedAt).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("insert brush entry: %w", err)
	}
	return id, true, nil
}

func (s *Store) Reject(ctx context.Context, id int64, reason string) error {
	return s.setStatus(ctx, id, StatusRejected, reason)
}

func (s *Store) Fail(ctx context.Context, id int64, reason string) error {
	return s.setStatus(ctx, id, StatusFailed, reason)
}

func (s *Store) MarkAdded(ctx context.Context, id int64) error {
	return s.setStatus(ctx, id, StatusAdded, "")
}

func (s *Store) setStatus(ctx context.Context, id int64, status, reason string) error {
	return s.execOne(ctx, "update brush entry",
		"UPDATE brush_entries SET status = ?, reason = ?, decided_at = ? WHERE id = ?",
		status, reason, time.Now().Unix(), id)
}

// HashTaken reports whether this torrent is already being added to, or was added
// to, the instance by any feed.
func (s *Store) HashTaken(ctx context.Context, hash string, instanceID int) (bool, error) {
	var n int
	err := s.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM brush_entries WHERE info_hash = ? AND instance_id = ? AND status IN (?, ?)",
		hash, instanceID, StatusAdding, StatusAdded).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("check brush hash: %w", err)
	}
	return n > 0, nil
}

// Claim moves an entry to "adding" for one instance before the add request is sent.
func (s *Store) Claim(ctx context.Context, id int64, hash string, size int64, instanceID int) error {
	return s.execOne(ctx, "claim brush entry",
		"UPDATE brush_entries SET status = ?, info_hash = ?, size_bytes = ?, instance_id = ?, decided_at = ? WHERE id = ?",
		StatusAdding, hash, size, instanceID, time.Now().Unix(), id)
}

// SetPlacement stores the placement record, for an admitted and a refused entry alike.
func (s *Store) SetPlacement(ctx context.Context, id int64, placement []byte) error {
	return s.execOne(ctx, "record brush placement", "UPDATE brush_entries SET placement = ? WHERE id = ?", string(placement), id)
}

type Reservation struct {
	InfoHash  string
	SizeBytes int64
}

// Reservations lists torrents brush sent to the instance since the given time. The
// admission check counts the ones qBittorrent does not show yet.
func (s *Store) Reservations(ctx context.Context, instanceID int, since time.Time) ([]Reservation, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT info_hash, size_bytes FROM brush_entries WHERE instance_id = ? AND status IN (?, ?) AND decided_at >= ?",
		instanceID, StatusAdding, StatusAdded, since.Unix())
	if err != nil {
		return nil, fmt.Errorf("list brush reservations: %w", err)
	}
	defer rows.Close()

	var out []Reservation
	for rows.Next() {
		var r Reservation
		if err := rows.Scan(&r.InfoHash, &r.SizeBytes); err != nil {
			return nil, fmt.Errorf("scan brush reservation: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// MarkInterruptedAdds turns entries left in "adding" by a previous run into "unknown".
func (s *Store) MarkInterruptedAdds(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, "UPDATE brush_entries SET status = ?, reason = ? WHERE status = ?",
		StatusUnknown, "interrupted", StatusAdding)
	if err != nil {
		return fmt.Errorf("mark interrupted brush adds: %w", err)
	}
	return nil
}

// RecentEntries returns the newest decisions for the history view, skipping the
// baseline rows a first poll writes.
func (s *Store) RecentEntries(ctx context.Context, limit int) ([]Entry, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT e.id, e.feed_id, f.name, e.title, e.size_bytes, e.link, e.published_at,
		e.first_seen_at, e.status, e.reason, e.info_hash, e.instance_id, e.decided_at, e.placement
		FROM brush_entries e JOIN brush_feeds f ON f.id = e.feed_id
		WHERE e.status <> ? ORDER BY e.id DESC LIMIT ?`, StatusBaseline, limit)
	if err != nil {
		return nil, fmt.Errorf("list brush entries: %w", err)
	}
	defer rows.Close()

	var out []Entry
	for rows.Next() {
		var e Entry
		var placement string
		if err := rows.Scan(&e.ID, &e.FeedID, &e.FeedName, &e.Title, &e.SizeBytes, &e.Link, &e.PublishedAt,
			&e.FirstSeenAt, &e.Status, &e.Reason, &e.InfoHash, &e.InstanceID, &e.DecidedAt, &placement); err != nil {
			return nil, fmt.Errorf("scan brush entry: %w", err)
		}
		if placement != "" {
			e.Placement = json.RawMessage(placement)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
