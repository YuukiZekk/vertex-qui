// Copyright (c) 2026, vertex-qui contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package store

import (
	"context"
	"fmt"
)

// Floors returns each instance's safety floor in bytes. An instance without a row
// has no floor, and the admission check refuses it.
func (s *Store) Floors(ctx context.Context) (map[int]int64, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT instance_id, floor_bytes FROM brush_instances")
	if err != nil {
		return nil, fmt.Errorf("list brush floors: %w", err)
	}
	defer rows.Close()

	floors := make(map[int]int64)
	for rows.Next() {
		var id int
		var floor int64
		if err := rows.Scan(&id, &floor); err != nil {
			return nil, fmt.Errorf("scan brush floor: %w", err)
		}
		floors[id] = floor
	}
	return floors, rows.Err()
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
