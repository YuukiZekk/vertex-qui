// Copyright (c) 2026, vertex-qui contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package store

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
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
	// AgeFromFirstSeen counts a torrent's age from when the feed first listed it, not
	// from its pubDate. It is for feeds of old torrents that come back, like a
	// revival section, whose pubDate is the original upload.
	AgeFromFirstSeen bool `json:"ageFromFirstSeen"`
	// Category is the qBittorrent category the feed's torrents are added with, empty
	// for none. qBittorrent creates it when it does not exist yet.
	Category string `json:"category"`
	// EarlyCheckSeconds, when not zero, deletes a torrent of this feed that has
	// uploaded less than EarlyMinRatio of its size once it is this old, so a torrent
	// nobody wants stops taking download and space early. Zero turns it off.
	EarlyCheckSeconds int     `json:"earlyCheckSeconds"`
	EarlyMinRatio     float64 `json:"earlyMinRatio"`
	// InstanceIDs are the instances placement may choose from, in ascending order.
	// Empty means every instance with a floor, which only feeds saved before
	// instances could be picked one by one still have.
	InstanceIDs []int `json:"instanceIds"`
	// GroupIDs are groups whose instances placement may also choose from.
	GroupIDs []int64 `json:"groupIds"`
	// members are the instances of GroupIDs when the feed was read.
	members      []int
	Enabled      bool   `json:"enabled"`
	ETag         string `json:"-"`
	LastModified string `json:"-"`
	LastFetchAt  int64  `json:"lastFetchAt"`
	LastError    string `json:"lastError"`
}

const feedColumns = `id, site_id, name, url_encrypted, interval_seconds, min_size_bytes, max_size_bytes,
	free_only, skip_hr, window_seconds, instance_id, instance_ids, enabled, etag, last_modified, last_fetch_at, last_error,
	age_from_first_seen, category, group_ids, early_check_seconds, early_min_ratio`

// formatInstanceIDs stores a feed's instances as a comma-separated list.
func formatInstanceIDs(ids []int) string {
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = strconv.Itoa(id)
	}
	return strings.Join(parts, ",")
}

// parseInstanceIDs reads instance_ids, falling back to the single instance_id
// column feeds used before; 0 there meant every instance.
func parseInstanceIDs(list string, legacy int) ([]int, error) {
	if list == "" {
		if legacy > 0 {
			return []int{legacy}, nil
		}
		return []int{}, nil
	}
	parts := strings.Split(list, ",")
	ids := make([]int, 0, len(parts))
	for _, p := range parts {
		id, err := strconv.Atoi(p)
		if err != nil {
			return nil, fmt.Errorf("instance ids %q: %w", list, err)
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func (s *Store) ListFeeds(ctx context.Context) ([]Feed, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT "+feedColumns+" FROM brush_feeds ORDER BY id")
	if err != nil {
		return nil, fmt.Errorf("list brush feeds: %w", err)
	}
	defer rows.Close()

	var feeds []Feed
	for rows.Next() {
		var f Feed
		var sealed, instanceIDs, groupIDs string
		var freeOnly, skipHR, enabled, legacyInstance, ageFromFirstSeen int
		if err := rows.Scan(&f.ID, &f.SiteID, &f.Name, &sealed, &f.IntervalSeconds, &f.MinSizeBytes, &f.MaxSizeBytes,
			&freeOnly, &skipHR, &f.WindowSeconds, &legacyInstance, &instanceIDs, &enabled, &f.ETag, &f.LastModified,
			&f.LastFetchAt, &f.LastError, &ageFromFirstSeen, &f.Category, &groupIDs,
			&f.EarlyCheckSeconds, &f.EarlyMinRatio); err != nil {
			return nil, fmt.Errorf("scan brush feed: %w", err)
		}
		f.FreeOnly, f.SkipHR, f.Enabled = freeOnly == 1, skipHR == 1, enabled == 1
		f.AgeFromFirstSeen = ageFromFirstSeen == 1
		if f.InstanceIDs, err = parseInstanceIDs(instanceIDs, legacyInstance); err != nil {
			return nil, fmt.Errorf("brush feed %d: %w", f.ID, err)
		}
		if f.GroupIDs, err = parseGroupIDs(groupIDs); err != nil {
			return nil, fmt.Errorf("brush feed %d: %w", f.ID, err)
		}
		if f.URL, err = s.open(sealed, aadFeedURL); err != nil {
			return nil, fmt.Errorf("decrypt url of brush feed %d: %w", f.ID, err)
		}
		feeds = append(feeds, f)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	members, err := s.GroupMembers(ctx)
	if err != nil {
		return nil, err
	}
	for i := range feeds {
		for _, g := range feeds[i].GroupIDs {
			feeds[i].members = append(feeds[i].members, members[g]...)
		}
	}
	return feeds, nil
}

// Targets are the instances placement may choose from: the feed's own and those
// of its groups, in ascending order. all is true only for a feed saved before
// instances could be picked, which names neither, and may use every instance.
func (f Feed) Targets() (ids []int, all bool) {
	if len(f.InstanceIDs) == 0 && len(f.GroupIDs) == 0 {
		return nil, true
	}
	ids = append(slices.Clone(f.InstanceIDs), f.members...)
	slices.Sort(ids)
	return slices.Compact(ids), false
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
		max_size_bytes, free_only, skip_hr, window_seconds, instance_id, instance_ids, enabled, age_from_first_seen, category, group_ids,
		early_check_seconds, early_min_ratio, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 0, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING id`,
		f.SiteID, f.Name, sealed, f.IntervalSeconds, f.MinSizeBytes, f.MaxSizeBytes, boolInt(f.FreeOnly), boolInt(f.SkipHR),
		f.WindowSeconds, formatInstanceIDs(f.InstanceIDs), boolInt(f.Enabled), boolInt(f.AgeFromFirstSeen), f.Category, formatGroupIDs(f.GroupIDs),
		f.EarlyCheckSeconds, f.EarlyMinRatio, time.Now().Unix()).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("create brush feed: %w", err)
	}
	return id, nil
}

// UpdateFeed keeps the stored URL when f.URL is empty: the URL carries the passkey,
// so the API never sends it back to the browser.
func (s *Store) UpdateFeed(ctx context.Context, f Feed) error {
	set := `site_id = ?, name = ?, interval_seconds = ?, min_size_bytes = ?, max_size_bytes = ?, free_only = ?,
		skip_hr = ?, window_seconds = ?, instance_id = 0, instance_ids = ?, enabled = ?, age_from_first_seen = ?, category = ?, group_ids = ?,
		early_check_seconds = ?, early_min_ratio = ?`
	args := []any{f.SiteID, f.Name, f.IntervalSeconds, f.MinSizeBytes, f.MaxSizeBytes, boolInt(f.FreeOnly),
		boolInt(f.SkipHR), f.WindowSeconds, formatInstanceIDs(f.InstanceIDs), boolInt(f.Enabled), boolInt(f.AgeFromFirstSeen), f.Category,
		formatGroupIDs(f.GroupIDs), f.EarlyCheckSeconds, f.EarlyMinRatio}
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
