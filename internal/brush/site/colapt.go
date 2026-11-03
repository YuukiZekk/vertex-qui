// Copyright (c) 2026, vertex-qui contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package site

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

// colapt is not NexusPHP: its pages are a single-page app over a JSON API at
// /api/v1, and only its RSS speaks NexusPHP. The API takes the logged-in session
// cookie, like the browser app does. It has no H&R marker on a torrent.

// colaptTorrentPath takes the id at the end of /torrents/N or of an RSS download
// path /feeds/downloads/<passkey>/N.
var colaptTorrentPath = regexp.MustCompile(`/(\d+)/?$`)

func isColapt(host string) bool {
	return strings.Contains(strings.ToLower(host), "colapt")
}

// colaptTorrentAPI turns an item's details link, details.php?id=N as ParseFeed
// makes it or /torrents/N as the app shows it, into the API address of the torrent.
func colaptTorrentAPI(detailsURL string) (string, error) {
	u, err := url.Parse(detailsURL)
	if err != nil || u.Host == "" {
		return "", errors.New("invalid details URL")
	}
	id := u.Query().Get("id")
	if m := colaptTorrentPath.FindStringSubmatch(u.Path); id == "" && m != nil {
		id = m[1]
	}
	if id == "" {
		return "", errors.New("colapt details URL has no torrent id")
	}
	return (&url.URL{Scheme: u.Scheme, Host: u.Host, Path: "/api/v1/torrents/" + id}).String(), nil
}

type colaptRatio struct {
	Num int64 `json:"num"`
	Den int64 `json:"den"`
}

type colaptTorrent struct {
	Promotion *struct {
		Download colaptRatio `json:"download"`
		Upload   colaptRatio `json:"upload"`
	} `json:"promotion"`
}

// inspectColapt reads a torrent's promotion from the API; a download ratio of zero
// is free.
func (n *NexusPHP) inspectColapt(ctx context.Context, detailsURL string) (Inspection, error) {
	api, err := colaptTorrentAPI(detailsURL)
	if err != nil {
		return Inspection{}, err
	}
	body, err := n.colaptGet(ctx, api)
	if err != nil {
		return Inspection{}, err
	}
	var t colaptTorrent
	if err := json.Unmarshal(body, &t); err != nil {
		return Inspection{}, errors.New("colapt API returned an unreadable torrent")
	}
	free := t.Promotion != nil && t.Promotion.Download.Num == 0
	return Inspection{Free: free, HRUnknown: true, Rule: "colapt"}, nil
}

// colaptGet requests one API address with the session cookie. 401 and 403 mean the
// cookie is not logged in.
func (n *NexusPHP) colaptGet(ctx context.Context, rawURL string) ([]byte, error) {
	req, err := n.request(ctx, rawURL)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := n.do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return nil, fmt.Errorf("colapt API refused the cookie with HTTP %d; copy the Cookie header of a request to /api/v1 in a logged-in browser: %w", resp.StatusCode, ErrNotLoggedIn)
	case resp.StatusCode != http.StatusOK:
		return nil, fmt.Errorf("colapt API returned HTTP %d", resp.StatusCode)
	}
	return readLimited(resp.Body, maxPageBytes)
}
