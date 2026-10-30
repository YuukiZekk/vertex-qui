// Copyright (c) 2026, vertex-qui contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package store

import (
	"context"
	"errors"
	"fmt"
	"time"
)

var ErrNotFound = errors.New("not found")

type Feed struct {
	ID              int64  `json:"id"`
	SiteID          int64  `json:"siteId"`
	Name            string `json:"name"`
	URL             string `json:"-"`
	IntervalSeconds int    `json:"intervalSeconds"`
	MinSizeBytes    int64  `json:"minSizeBytes"`
	MaxSizeBytes    int64  `json:"maxSizeBytes"`
	FreeOnly        bool   `json:"freeOnly"`
	SkipHR          bool   `json:"skipHr"`
	WindowSeconds   int    `json:"windowSeconds"`
	InstanceID      int    `json:"instanceId"`
	Enabled         bool   `json:"enabled"`
	ETag            string `json:"-"`
	LastModified    string `json:"-"`
	LastFetchAt     int64  `json:"lastFetchAt"`
	LastError       string `json:"lastError"`
}

const feedColumns = `id, site_id, name, url_encrypted, interval_seconds, min_size_bytes, max_size_bytes,
	free_only, skip_hr, window_seconds, instance_id, enabled, etag, last_modified, last_fetch_at, last_error`

func (s *Store) ListFeeds(ctx context.Context) ([]Feed, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT "+feedColumns+" FROM brush_feeds ORDER BY id")
	if err != nil {
		return nil, fmt.Errorf("list brush feeds: %w", err)
	}
	defer rows.Close()

	var feeds []Feed
	for rows.Next() {
		var f Feed
		var sealed string
		var freeOnly, skipHR, enabled int
		if err := rows.Scan(&f.ID, &f.SiteID, &f.Name, &sealed, &f.IntervalSeconds, &f.MinSizeBytes, &f.MaxSizeBytes,
			&freeOnly, &skipHR, &f.WindowSeconds, &f.InstanceID, &enabled, &f.ETag, &f.LastModified, &f.LastFetchAt, &f.LastError); err != nil {
			return nil, fmt.Errorf("scan brush feed: %w", err)
		}
		f.FreeOnly, f.SkipHR, f.Enabled = freeOnly == 1, skipHR == 1, enabled == 1
		if f.URL, err = s.open(sealed, aadFeedURL); err != nil {
			return nil, fmt.Errorf("decrypt url of brush feed %d: %w", f.ID, err)
		}
		feeds = append(feeds, f)
	}
	return feeds, rows.Err()
}

func (s *Store) GetFeed(ctx context.Context, id int64) (Feed, error) {
	feeds, err := s.ListFeeds(ctx)
	if err != nil {
		return Feed{}, err
	}
	for _, f := range feeds {
		if f.ID == id {
			return f, nil
		}
	}
	return Feed{}, fmt.Errorf("brush feed %d: %w", id, ErrNotFound)
}

func (s *Store) CreateFeed(ctx context.Context, f Feed) (int64, error) {
	sealed, err := s.seal(f.URL, aadFeedURL)
	if err != nil {
		return 0, err
	}
	var id int64
	err = s.db.QueryRowContext(ctx, `INSERT INTO brush_feeds (site_id, name, url_encrypted, interval_seconds, min_size_bytes,
		max_size_bytes, free_only, skip_hr, window_seconds, instance_id, enabled, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING id`,
		f.SiteID, f.Name, sealed, f.IntervalSeconds, f.MinSizeBytes, f.MaxSizeBytes, boolInt(f.FreeOnly), boolInt(f.SkipHR),
		f.WindowSeconds, f.InstanceID, boolInt(f.Enabled), time.Now().Unix()).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("create brush feed: %w", err)
	}
	return id, nil
}

// UpdateFeed keeps the stored URL when f.URL is empty: the URL carries the passkey,
// so the API never sends it back to the browser.
func (s *Store) UpdateFeed(ctx context.Context, f Feed) error {
	set := `site_id = ?, name = ?, interval_seconds = ?, min_size_bytes = ?, max_size_bytes = ?, free_only = ?,
		skip_hr = ?, window_seconds = ?, instance_id = ?, enabled = ?`
	args := []any{f.SiteID, f.Name, f.IntervalSeconds, f.MinSizeBytes, f.MaxSizeBytes, boolInt(f.FreeOnly),
		boolInt(f.SkipHR), f.WindowSeconds, f.InstanceID, boolInt(f.Enabled)}
	if f.URL != "" {
		sealed, err := s.seal(f.URL, aadFeedURL)
		if err != nil {
			return err
		}
		// A new URL is a different feed to the server, so its cache validators no longer apply.
		set += ", url_encrypted = ?, etag = '', last_modified = ''"
		args = append(args, sealed)
	}
	args = append(args, f.ID)
	return s.execOne(ctx, "update brush feed", "UPDATE brush_feeds SET "+set+" WHERE id = ?", args...)
}

func (s *Store) DeleteFeed(ctx context.Context, id int64) error {
	return s.execOne(ctx, "delete brush feed", "DELETE FROM brush_feeds WHERE id = ?", id)
}

// RecordFetch stores the outcome of one poll; fetchErr is empty after a success.
func (s *Store) RecordFetch(ctx context.Context, id int64, etag, lastModified, fetchErr string) error {
	return s.execOne(ctx, "record brush feed fetch",
		"UPDATE brush_feeds SET etag = ?, last_modified = ?, last_fetch_at = ?, last_error = ? WHERE id = ?",
		etag, lastModified, time.Now().Unix(), fetchErr, id)
}
