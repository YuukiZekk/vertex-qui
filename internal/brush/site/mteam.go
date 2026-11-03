// Copyright (c) 2026, vertex-qui contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package site

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

const mteamAttempts = 5

// mteamAPI and mteamGap, M-Team's API rate limit of about one call every 3 seconds,
// are variables so tests can point them at a local server.
var (
	mteamAPI = "https://api.m-team.cc/api/torrent/detail"
	mteamGap = 3 * time.Second
)

var mteamID = regexp.MustCompile(`/(\d+)`)

func isMTeam(host string) bool {
	return strings.Contains(strings.ToLower(host), "m-team")
}

type mteamDetail struct {
	Code    json.RawMessage `json:"code"`
	Message string          `json:"message"`
	Data    *struct {
		Status struct {
			Discount string `json:"discount"`
		} `json:"status"`
	} `json:"data"`
}

// inspectMTeam asks M-Team's API for the torrent's discount, with the API key the
// site stores in place of a cookie. Code 4 means rate limited, and a gateway now and
// then cuts the JSON short; both are retried. M-Team has no H&R marker.
func (n *NexusPHP) inspectMTeam(ctx context.Context, detailsURL string) (Inspection, error) {
	u, err := url.Parse(detailsURL)
	if err != nil {
		return Inspection{}, errors.New("invalid details URL")
	}
	m := mteamID.FindStringSubmatch(u.Path)
	if m == nil {
		return Inspection{}, errors.New("M-Team details URL has no torrent id")
	}
	var last string
	for attempt := 1; attempt <= mteamAttempts; attempt++ {
		if err := n.waitAPI(ctx); err != nil {
			return Inspection{}, err
		}
		body, err := n.mteamRequest(ctx, m[1])
		if err != nil {
			return Inspection{}, err
		}
		var detail mteamDetail
		if err := json.Unmarshal(body, &detail); err != nil {
			last = "incomplete response"
			continue
		}
		if strings.Trim(string(detail.Code), `"`) == "4" {
			last = "rate limited"
			continue
		}
		if detail.Data == nil {
			return Inspection{}, fmt.Errorf("M-Team API refused the request, check the API key: %s", detail.Message)
		}
		return Inspection{Free: strings.Contains(detail.Data.Status.Discount, "FREE"), HRUnknown: true, Rule: "m-team"}, nil
	}
	return Inspection{}, fmt.Errorf("M-Team API failed %d times: %s", mteamAttempts, last)
}

func (n *NexusPHP) mteamRequest(ctx context.Context, id string) ([]byte, error) {
	var form bytes.Buffer
	w := multipart.NewWriter(&form)
	if err := w.WriteField("id", id); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, mteamAPI, &form)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("User-Agent", "vertex-qui")
	req.Header.Set("x-api-key", n.cookie)
	resp, err := n.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request to M-Team API failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("M-Team API returned HTTP %d", resp.StatusCode)
	}
	return readLimited(resp.Body, maxPageBytes)
}

func (n *NexusPHP) waitAPI(ctx context.Context) error {
	n.mu.Lock()
	next := n.apiLast.Add(mteamGap)
	now := time.Now()
	if next.Before(now) {
		next = now
	}
	n.apiLast = next
	n.mu.Unlock()

	timer := time.NewTimer(time.Until(next))
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
