// Copyright (c) 2026, vertex-qui contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/autobrr/qui/internal/dbinterface"
	"github.com/autobrr/qui/internal/models"
)

const keyEnabled = "enabled"

// AAD values bind each sealed column to its purpose, so a cookie cannot be
// copied into the feed URL column and still decrypt.
var (
	aadSiteCookie = []byte("brush_sites.cookie")
	aadFeedURL    = []byte("brush_feeds.url")
)

type Store struct {
	db     dbinterface.Querier
	cipher *models.CredentialCipher
}

func New(db dbinterface.Querier, cipher *models.CredentialCipher) *Store {
	return &Store{db: db, cipher: cipher}
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

func (s *Store) SetEnabled(ctx context.Context, enabled bool) error {
	value := "false"
	if enabled {
		value = "true"
	}
	_, err := s.db.ExecContext(ctx,
		"INSERT INTO brush_settings (key, value) VALUES (?, ?) ON CONFLICT (key) DO UPDATE SET value = excluded.value",
		keyEnabled, value)
	if err != nil {
		return fmt.Errorf("write brush enabled setting: %w", err)
	}
	return nil
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
