// Copyright (c) 2026, vertex-qui contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package store

import (
	"context"
	"fmt"
)

// InstanceConfig is what brush keeps per qBittorrent instance. UploadCapacity is in
// bytes per second; zero means unknown, and placement then compares space only.
type InstanceConfig struct {
	FloorBytes     int64
	UploadCapacity int64
	// CountryCode is the lower-case ISO 3166 code of where the instance runs, set by
	// hand to show its flag; empty for none.
	CountryCode string
}

// InstanceConfigs returns the config of every instance brush may use. An instance
// without a row has no floor, and the admission check refuses it.
func (s *Store) InstanceConfigs(ctx context.Context) (map[int]InstanceConfig, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT instance_id, floor_bytes, upload_capacity_bytes, country_code FROM brush_instances")
	if err != nil {
		return nil, fmt.Errorf("list brush instances: %w", err)
	}
	defer rows.Close()

	configs := make(map[int]InstanceConfig)
	for rows.Next() {
		var id int
		var c InstanceConfig
		if err := rows.Scan(&id, &c.FloorBytes, &c.UploadCapacity, &c.CountryCode); err != nil {
			return nil, fmt.Errorf("scan brush instance: %w", err)
		}
		configs[id] = c
	}
	return configs, rows.Err()
}

func (s *Store) SetFloor(ctx context.Context, instanceID int, floorBytes int64) error {
	_, err := s.db.ExecContext(ctx,
		"INSERT INTO brush_instances (instance_id, floor_bytes) VALUES (?, ?) ON CONFLICT (instance_id) DO UPDATE SET floor_bytes = excluded.floor_bytes",
		instanceID, floorBytes)
	if err != nil {
		return fmt.Errorf("write brush floor: %w", err)
	}
	return nil
}

// SetUploadCapacity only updates an instance that already has a floor, so setting a
// capacity can never create a row that admits torrents without one.
func (s *Store) SetUploadCapacity(ctx context.Context, instanceID int, bytesPerSecond int64) error {
	return s.execOne(ctx, "write brush upload capacity",
		"UPDATE brush_instances SET upload_capacity_bytes = ? WHERE instance_id = ?", bytesPerSecond, instanceID)
}

// SetCountry sets where an instance that already has a floor runs, like
// SetUploadCapacity.
func (s *Store) SetCountry(ctx context.Context, instanceID int, code string) error {
	return s.execOne(ctx, "set brush instance country", "UPDATE brush_instances SET country_code = ? WHERE instance_id = ?", code, instanceID)
}
