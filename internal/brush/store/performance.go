// Copyright (c) 2026, vertex-qui contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package store

import (
	"context"
	"fmt"
	"time"
)

// RecordStages stores when an entry's details page was read and its torrent file
// fetched, zero for a step it did not take. Only performance reads them.
func (s *Store) RecordStages(ctx context.Context, id int64, inspectedAt, fetchedAt time.Time) error {
	unix := func(t time.Time) int64 {
		if t.IsZero() {
			return 0
		}
		return t.Unix()
	}
	return s.execOne(ctx, "record brush entry stages",
		"UPDATE brush_entries SET inspected_at = ?, fetched_at = ? WHERE id = ?", unix(inspectedAt), unix(fetchedAt), id)
}

// MarkFirstUpload stores when a torrent brush added was first seen uploading. It
// only touches a torrent added since addedSince, so a restart does not stamp old
// torrents with the time of their first sample, and a later call changes nothing.
func (s *Store) MarkFirstUpload(ctx context.Context, instanceID int, hash string, at, addedSince time.Time) error {
	_, err := s.db.ExecContext(ctx, `UPDATE brush_entries SET first_upload_at = ?
		WHERE instance_id = ? AND info_hash = ? AND status = ? AND first_upload_at = 0 AND added_at >= ?`,
		at.Unix(), instanceID, hash, StatusAdded, addedSince.Unix())
	if err != nil {
		return fmt.Errorf("mark brush first upload: %w", err)
	}
	return nil
}

// PerfEntry is one entry that got past the filters, with its stage times in unix
// seconds, zero when unknown.
type PerfEntry struct {
	FeedID        int64
	SiteID        int64
	InstanceID    int
	Status        string
	PublishedAt   int64
	FirstSeenAt   int64
	InspectedAt   int64
	FetchedAt     int64
	AddedAt       int64
	FirstUploadAt int64
}

// PerfEntries lists the entries first seen since a moment that brush tried to add:
// added, failed, or still unconfirmed.
func (s *Store) PerfEntries(ctx context.Context, since time.Time) ([]PerfEntry, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT e.feed_id, f.site_id, e.instance_id, e.status, e.published_at,
			e.first_seen_at, e.inspected_at, e.fetched_at, e.added_at, e.first_upload_at
		FROM brush_entries e JOIN brush_feeds f ON f.id = e.feed_id
		WHERE e.first_seen_at >= ? AND e.status IN (?, ?, ?, ?)`,
		since.Unix(), StatusAdding, StatusAdded, StatusFailed, StatusUnknown)
	if err != nil {
		return nil, fmt.Errorf("list brush performance entries: %w", err)
	}
	defer rows.Close()
	var out []PerfEntry
	for rows.Next() {
		var e PerfEntry
		if err := rows.Scan(&e.FeedID, &e.SiteID, &e.InstanceID, &e.Status, &e.PublishedAt, &e.FirstSeenAt,
			&e.InspectedAt, &e.FetchedAt, &e.AddedAt, &e.FirstUploadAt); err != nil {
			return nil, fmt.Errorf("scan brush performance entry: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
