// Copyright (c) 2026, vertex-qui contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/autobrr/qui/internal/dbinterface"
)

const keyEnabled = "enabled"

type Store struct {
	db dbinterface.Querier
}

func New(db dbinterface.Querier) *Store {
	return &Store{db: db}
}

// Enabled reports the brush master switch. A fresh install has no row, so brushing
// stays off until the user turns it on.
func (s *Store) Enabled(ctx context.Context) (bool, error) {
	var value string
	err := s.db.QueryRowContext(ctx, "SELECT value FROM brush_settings WHERE key = ?", keyEnabled).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read brush enabled setting: %w", err)
	}
	return value == "true", nil
}
