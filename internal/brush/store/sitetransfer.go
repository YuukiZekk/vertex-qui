// Copyright (c) 2026, vertex-qui contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package store

import (
	"context"
	"fmt"
	"time"
)

// SiteTransfer is what the torrents of one site uploaded and downloaded on one
// local day, across every instance.
type SiteTransfer struct {
	SiteID     int64 `json:"siteId"`
	Uploaded   int64 `json:"uploaded"`
	Downloaded int64 `json:"downloaded"`
}

// AddSiteTransfer adds to each site's totals for the day and drops days past the
// retention period.
func (s *Store) AddSiteTransfer(ctx context.Context, day string, add []SiteTransfer) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin brush site transfer: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	for _, t := range add {
		if _, err := tx.ExecContext(ctx, `INSERT INTO brush_site_daily (site_id, day, uploaded, downloaded) VALUES (?, ?, ?, ?)
			ON CONFLICT (site_id, day) DO UPDATE SET uploaded = brush_site_daily.uploaded + excluded.uploaded,
			downloaded = brush_site_daily.downloaded + excluded.downloaded`,
			t.SiteID, day, t.Uploaded, t.Downloaded); err != nil {
			return fmt.Errorf("add brush site transfer: %w", err)
		}
	}
	at, err := time.Parse(time.DateOnly, day)
	if err == nil {
		cutoff := at.AddDate(0, 0, -transferRetentionDays).Format(time.DateOnly)
		if _, err := tx.ExecContext(ctx, "DELETE FROM brush_site_daily WHERE day < ?", cutoff); err != nil {
			return fmt.Errorf("prune brush site transfer: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit brush site transfer: %w", err)
	}
	return nil
}

// SiteTransfersOn returns every site's totals for one day.
func (s *Store) SiteTransfersOn(ctx context.Context, day string) (map[int64]SiteTransfer, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT site_id, uploaded, downloaded FROM brush_site_daily WHERE day = ?", day)
	if err != nil {
		return nil, fmt.Errorf("list brush site transfers: %w", err)
	}
	defer rows.Close()
	out := make(map[int64]SiteTransfer)
	for rows.Next() {
		var t SiteTransfer
		if err := rows.Scan(&t.SiteID, &t.Uploaded, &t.Downloaded); err != nil {
			return nil, fmt.Errorf("scan brush site transfer: %w", err)
		}
		out[t.SiteID] = t
	}
	return out, rows.Err()
}

// BrushTorrentSites maps each torrent brush added, by instance and hash, to the
// site of the feed that added it.
func (s *Store) BrushTorrentSites(ctx context.Context) (map[int]map[string]int64, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT e.instance_id, e.info_hash, f.site_id
		FROM brush_entries e JOIN brush_feeds f ON f.id = e.feed_id
		WHERE e.status IN (?, ?, ?) AND e.info_hash <> ''`, StatusAdding, StatusAdded, StatusUnknown)
	if err != nil {
		return nil, fmt.Errorf("list brush torrent sites: %w", err)
	}
	defer rows.Close()
	out := make(map[int]map[string]int64)
	for rows.Next() {
		var instanceID int
		var hash string
		var siteID int64
		if err := rows.Scan(&instanceID, &hash, &siteID); err != nil {
			return nil, fmt.Errorf("scan brush torrent site: %w", err)
		}
		if out[instanceID] == nil {
			out[instanceID] = make(map[string]int64)
		}
		out[instanceID][hash] = siteID
	}
	return out, rows.Err()
}

// FeedCounts returns how many feeds each site has.
func (s *Store) FeedCounts(ctx context.Context) (map[int64]int, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT site_id, COUNT(*) FROM brush_feeds GROUP BY site_id")
	if err != nil {
		return nil, fmt.Errorf("count brush feeds: %w", err)
	}
	defer rows.Close()
	out := make(map[int64]int)
	for rows.Next() {
		var id int64
		var n int
		if err := rows.Scan(&id, &n); err != nil {
			return nil, fmt.Errorf("scan brush feed count: %w", err)
		}
		out[id] = n
	}
	return out, rows.Err()
}
