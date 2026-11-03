// Copyright (c) 2026, vertex-qui contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package store

import (
	"context"
	"fmt"
	"time"
)

// PeerTransfer is what one instance uploaded to and downloaded from the peers of
// one country for the torrents of one site, on one local day. SiteID is zero for
// torrents of no known site and CountryCode empty where qBittorrent named no
// country. No peer address is kept.
type PeerTransfer struct {
	Day         string `json:"day"`
	SiteID      int64  `json:"siteId"`
	InstanceID  int    `json:"instanceId"`
	CountryCode string `json:"countryCode"`
	Uploaded    int64  `json:"uploaded"`
	Downloaded  int64  `json:"downloaded"`
}

// AddPeerTransfer adds to the day's totals and drops days past the retention period.
func (s *Store) AddPeerTransfer(ctx context.Context, day string, add []PeerTransfer) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin brush peer transfer: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	for _, t := range add {
		if _, err := tx.ExecContext(ctx, `INSERT INTO brush_peer_daily (day, site_id, instance_id, country_code, uploaded, downloaded)
			VALUES (?, ?, ?, ?, ?, ?)
			ON CONFLICT (day, site_id, instance_id, country_code) DO UPDATE SET uploaded = brush_peer_daily.uploaded + excluded.uploaded,
			downloaded = brush_peer_daily.downloaded + excluded.downloaded`,
			day, t.SiteID, t.InstanceID, t.CountryCode, t.Uploaded, t.Downloaded); err != nil {
			return fmt.Errorf("add brush peer transfer: %w", err)
		}
	}
	at, err := time.Parse(time.DateOnly, day)
	if err == nil {
		cutoff := at.AddDate(0, 0, -transferRetentionDays).Format(time.DateOnly)
		if _, err := tx.ExecContext(ctx, "DELETE FROM brush_peer_daily WHERE day < ?", cutoff); err != nil {
			return fmt.Errorf("prune brush peer transfer: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit brush peer transfer: %w", err)
	}
	return nil
}

// PeerTransfersSince returns every day's totals from day on.
func (s *Store) PeerTransfersSince(ctx context.Context, day string) ([]PeerTransfer, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT day, site_id, instance_id, country_code, uploaded, downloaded
		FROM brush_peer_daily WHERE day >= ? ORDER BY day, site_id, instance_id, country_code`, day)
	if err != nil {
		return nil, fmt.Errorf("list brush peer transfers: %w", err)
	}
	defer rows.Close()
	var out []PeerTransfer
	for rows.Next() {
		var t PeerTransfer
		if err := rows.Scan(&t.Day, &t.SiteID, &t.InstanceID, &t.CountryCode, &t.Uploaded, &t.Downloaded); err != nil {
			return nil, fmt.Errorf("scan brush peer transfer: %w", err)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}
