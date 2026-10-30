// Copyright (c) 2026, vertex-qui contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// transferRetentionDays bounds how many days of totals are kept.
const transferRetentionDays = 62

// DailyTransfer is how much one instance uploaded and downloaded on one local day.
type DailyTransfer struct {
	InstanceID   int    `json:"instanceId"`
	Day          string `json:"day"`
	Downloaded   int64  `json:"downloaded"`
	Uploaded     int64  `json:"uploaded"`
	LastSampleAt int64  `json:"lastSampleAt"`
}

// RecordTransfer turns qBittorrent's all-time counters into per-day totals: the
// change since the instance's previous sample is added to the day of this sample.
// The first sample of an instance only sets the starting point, so a day an
// instance joins counts from that moment. Only the sampler calls it, one instance
// at a time.
func (s *Store) RecordTransfer(ctx context.Context, instanceID int, at time.Time, alltimeDL, alltimeUL int64) error {
	day := at.Format(time.DateOnly)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin brush transfer sample: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var lastDay string
	var lastDL, lastUL int64
	err = tx.QueryRowContext(ctx, `SELECT day, last_alltime_dl, last_alltime_ul FROM brush_daily_transfer
		WHERE instance_id = ? ORDER BY day DESC LIMIT 1`, instanceID).Scan(&lastDay, &lastDL, &lastUL)
	var dl, ul int64
	switch {
	case errors.Is(err, sql.ErrNoRows):
	case err != nil:
		return fmt.Errorf("read brush transfer sample: %w", err)
	default:
		dl, ul = counterDelta(alltimeDL, lastDL), counterDelta(alltimeUL, lastUL)
	}

	if lastDay == day {
		_, err = tx.ExecContext(ctx, `UPDATE brush_daily_transfer SET downloaded = downloaded + ?, uploaded = uploaded + ?,
			last_alltime_dl = ?, last_alltime_ul = ?, last_sample_at = ? WHERE instance_id = ? AND day = ?`,
			dl, ul, alltimeDL, alltimeUL, at.Unix(), instanceID, day)
	} else {
		_, err = tx.ExecContext(ctx, `INSERT INTO brush_daily_transfer (instance_id, day, downloaded, uploaded,
			last_alltime_dl, last_alltime_ul, last_sample_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			instanceID, day, dl, ul, alltimeDL, alltimeUL, at.Unix())
	}
	if err != nil {
		return fmt.Errorf("write brush transfer sample: %w", err)
	}

	cutoff := at.AddDate(0, 0, -transferRetentionDays).Format(time.DateOnly)
	if _, err := tx.ExecContext(ctx, "DELETE FROM brush_daily_transfer WHERE instance_id = ? AND day < ?", instanceID, cutoff); err != nil {
		return fmt.Errorf("prune brush transfer samples: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit brush transfer sample: %w", err)
	}
	return nil
}

// TransfersOn returns every instance's totals for one day.
func (s *Store) TransfersOn(ctx context.Context, day string) ([]DailyTransfer, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT instance_id, day, downloaded, uploaded, last_sample_at
		FROM brush_daily_transfer WHERE day = ? ORDER BY instance_id`, day)
	if err != nil {
		return nil, fmt.Errorf("list brush transfers: %w", err)
	}
	defer rows.Close()

	var out []DailyTransfer
	for rows.Next() {
		var t DailyTransfer
		if err := rows.Scan(&t.InstanceID, &t.Day, &t.Downloaded, &t.Uploaded, &t.LastSampleAt); err != nil {
			return nil, fmt.Errorf("scan brush transfer: %w", err)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// counterDelta is the growth of an all-time counter. A counter that went down is
// taken as a new starting point and adds nothing: qBittorrent saves its all-time
// statistics only now and then, so after a crash it restarts slightly behind, and
// counting its whole value as growth would add its lifetime total to one day.
func counterDelta(current, previous int64) int64 {
	if current < previous {
		return 0
	}
	return current - previous
}
