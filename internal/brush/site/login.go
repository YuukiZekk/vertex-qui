// Copyright (c) 2026, vertex-qui contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package site

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// mteamProfileAPI is a variable so tests can point it at a local server.
var mteamProfileAPI = "https://api.m-team.cc/api/member/profile"

// CheckLogin reports whether the site accepts the stored cookie, or for M-Team the
// API key: a NexusPHP home page must link to the user's details.
func (n *NexusPHP) CheckLogin(ctx context.Context, baseURL string) error {
	u, err := url.Parse(baseURL)
	if err != nil || u.Host == "" {
		return errors.New("invalid site address")
	}
	if isMTeam(u.Host) {
		return n.checkMTeam(ctx)
	}
	if isColapt(u.Host) {
		_, err := n.colaptGet(ctx, (&url.URL{Scheme: u.Scheme, Host: u.Host, Path: "/api/v1/me"}).String())
		return err
	}
	req, err := n.request(ctx, strings.TrimRight(baseURL, "/")+"/index.php")
	if err != nil {
		return err
	}
	resp, err := n.do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("home page returned HTTP %d", resp.StatusCode)
	}
	body, err := readLimited(resp.Body, maxPageBytes)
	if err != nil {
		return err
	}
	if !bytes.Contains(body, []byte("userdetails")) {
		return ErrNotLoggedIn
	}
	return nil
}

func (n *NexusPHP) checkMTeam(ctx context.Context) error {
	if err := n.waitAPI(ctx); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, mteamProfileAPI, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("User-Agent", "vertex-qui")
	req.Header.Set("x-api-key", n.cookie)
	resp, err := n.client.Do(req)
	if err != nil {
		return fmt.Errorf("request to M-Team API failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("M-Team API returned HTTP %d", resp.StatusCode)
	}
	body, err := readLimited(resp.Body, maxPageBytes)
	if err != nil {
		return err
	}
	var profile struct {
		Code    json.RawMessage `json:"code"`
		Message string          `json:"message"`
	}
	if err := json.Unmarshal(body, &profile); err != nil {
		return errors.New("M-Team API returned an unreadable response")
	}
	if strings.Trim(string(profile.Code), `"`) != "0" {
		return fmt.Errorf("M-Team API refused the API key: %s", profile.Message)
	}
	return nil
}
