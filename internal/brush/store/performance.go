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
	FeedID        int64  `json:"feedId"`
	SiteID        int64  `json:"siteId"`
	InstanceID    int    `json:"instanceId"`
	Status        string `json:"status"`
	PublishedAt   int64  `json:"publishedAt"`
	FirstSeenAt   int64  `json:"firstSeenAt"`
	InspectedAt   int64  `json:"inspectedAt"`
	FetchedAt     int64  `json:"fetchedAt"`
	AddedAt       int64  `json:"addedAt"`
	FirstUploadAt int64  `json:"firstUploadAt"`
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

// PerfDetail is one entry of the performance drill-down, with what the overview
// does not need: its name, size, feed and outcome.
type PerfDetail struct {
	PerfEntry
	ID        int64  `json:"id"`
	Title     string `json:"title"`
	SizeBytes int64  `json:"sizeBytes"`
	FeedName  string `json:"feedName"`
	Reason    string `json:"reason"`
}

// PerfDetails lists the entries first seen in [from, to) that brush tried to add,
// for one site (siteID > 0) or one instance, newest first.
func (s *Store) PerfDetails(ctx context.Context, from, to time.Time, siteID int64, instanceID int, limit int) ([]PerfDetail, error) {
	filter, arg := "f.site_id = ?", any(siteID)
	if siteID == 0 {
		filter, arg = "e.instance_id = ?", any(instanceID)
	}
	rows, err := s.db.QueryContext(ctx, `SELECT e.id, e.title, e.size_bytes, f.name, e.reason, e.feed_id, f.site_id, e.instance_id,
			e.status, e.published_at, e.first_seen_at, e.inspected_at, e.fetched_at, e.added_at, e.first_upload_at
		FROM brush_entries e JOIN brush_feeds f ON f.id = e.feed_id
		WHERE e.first_seen_at >= ? AND e.first_seen_at < ? AND e.status IN (?, ?, ?, ?) AND `+filter+`
		ORDER BY e.first_seen_at DESC, e.id DESC LIMIT ?`,
		from.Unix(), to.Unix(), StatusAdding, StatusAdded, StatusFailed, StatusUnknown, arg, limit)
	if err != nil {
		return nil, fmt.Errorf("list brush performance details: %w", err)
	}
	defer rows.Close()
	var out []PerfDetail
	for rows.Next() {
		var d PerfDetail
		if err := rows.Scan(&d.ID, &d.Title, &d.SizeBytes, &d.FeedName, &d.Reason, &d.FeedID, &d.SiteID, &d.InstanceID,
			&d.Status, &d.PublishedAt, &d.FirstSeenAt, &d.InspectedAt, &d.FetchedAt, &d.AddedAt, &d.FirstUploadAt); err != nil {
			return nil, fmt.Errorf("scan brush performance detail: %w", err)
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// TodayCounts is what brush did since a moment, for the brush page's status line:
// torrents added and deleted, and the ones that passed the filters but were missed
// for lack of space or because the race window closed.
type TodayCounts struct {
	Added        int `json:"added"`
	Deleted      int `json:"deleted"`
	MissedSpace  int `json:"missedSpace"`
	MissedWindow int `json:"missedWindow"`
}

func (s *Store) CountsSince(ctx context.Context, since time.Time) (TodayCounts, error) {
	var c TodayCounts
	err := s.db.QueryRowContext(ctx, `SELECT
			COALESCE(SUM(CASE WHEN status = ? THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN status = ? AND reason LIKE 'low_space%' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN status = ? AND reason LIKE 'window%' THEN 1 ELSE 0 END), 0)
		FROM brush_entries WHERE decided_at >= ? AND status <> ?`,
		StatusAdded, StatusRejected, StatusRejected, since.Unix(), StatusBaseline).Scan(&c.Added, &c.MissedSpace, &c.MissedWindow)
	if err != nil {
		return TodayCounts{}, fmt.Errorf("count brush entries: %w", err)
	}
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM brush_reaps WHERE executed = 1 AND first_at >= ?", since.Unix()).Scan(&c.Deleted); err != nil {
		return TodayCounts{}, fmt.Errorf("count brush deletions: %w", err)
	}
	return c, nil
}
