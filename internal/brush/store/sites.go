// Copyright (c) 2026, vertex-qui contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package store

import (
	"context"
	"fmt"
	"time"
)

type Site struct {
	ID      int64  `json:"id"`
	Name    string `json:"name"`
	BaseURL string `json:"baseUrl"`
	Cookie  string `json:"-"`
}

func (s *Store) ListSites(ctx context.Context) ([]Site, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id, name, base_url, cookie_encrypted FROM brush_sites ORDER BY id")
	if err != nil {
		return nil, fmt.Errorf("list brush sites: %w", err)
	}
	defer rows.Close()

	var sites []Site
	for rows.Next() {
		var site Site
		var sealed string
		if err := rows.Scan(&site.ID, &site.Name, &site.BaseURL, &sealed); err != nil {
			return nil, fmt.Errorf("scan brush site: %w", err)
		}
		if site.Cookie, err = s.open(sealed, aadSiteCookie); err != nil {
			return nil, fmt.Errorf("decrypt cookie of brush site %d: %w", site.ID, err)
		}
		sites = append(sites, site)
	}
	return sites, rows.Err()
}

func (s *Store) GetSite(ctx context.Context, id int64) (Site, error) {
	sites, err := s.ListSites(ctx)
	if err != nil {
		return Site{}, err
	}
	for _, site := range sites {
		if site.ID == id {
			return site, nil
		}
	}
	return Site{}, fmt.Errorf("brush site %d: %w", id, ErrNotFound)
}

func (s *Store) CreateSite(ctx context.Context, site Site) (int64, error) {
	sealed, err := s.seal(site.Cookie, aadSiteCookie)
	if err != nil {
		return 0, err
	}
	var id int64
	err = s.db.QueryRowContext(ctx,
		"INSERT INTO brush_sites (name, base_url, cookie_encrypted, created_at) VALUES (?, ?, ?, ?) RETURNING id",
		site.Name, site.BaseURL, sealed, time.Now().Unix()).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("create brush site: %w", err)
	}
	return id, nil
}

// UpdateSite keeps the stored cookie when site.Cookie is empty, because the API
// never sends the cookie back to the browser.
func (s *Store) UpdateSite(ctx context.Context, site Site) error {
	query := "UPDATE brush_sites SET name = ?, base_url = ? WHERE id = ?"
	args := []any{site.Name, site.BaseURL, site.ID}
	if site.Cookie != "" {
		sealed, err := s.seal(site.Cookie, aadSiteCookie)
		if err != nil {
			return err
		}
		query = "UPDATE brush_sites SET name = ?, base_url = ?, cookie_encrypted = ? WHERE id = ?"
		args = []any{site.Name, site.BaseURL, sealed, site.ID}
	}
	return s.execOne(ctx, "update brush site", query, args...)
}

func (s *Store) DeleteSite(ctx context.Context, id int64) error {
	return s.execOne(ctx, "delete brush site", "DELETE FROM brush_sites WHERE id = ?", id)
}

func (s *Store) seal(plaintext string, aad []byte) (string, error) {
	if plaintext == "" {
		return "", nil
	}
	sealed, err := s.cipher.Encrypt(plaintext, aad)
	if err != nil {
		return "", fmt.Errorf("encrypt brush secret: %w", err)
	}
	return sealed, nil
}

func (s *Store) open(sealed string, aad []byte) (string, error) {
	if sealed == "" {
		return "", nil
	}
	return s.cipher.Decrypt(sealed, aad)
}

func (s *Store) execOne(ctx context.Context, what, query string, args ...any) error {
	res, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("%s: %w", what, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("%s: %w", what, err)
	}
	if n == 0 {
		return fmt.Errorf("%s: %w", what, ErrNotFound)
	}
	return nil
}
