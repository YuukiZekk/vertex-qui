// Copyright (c) 2026, vertex-qui contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package site

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"

	"golang.org/x/net/html"
)

const (
	maxPageBytes    = 5 << 20
	maxTorrentBytes = 10 << 20
	// requestGap spaces requests to one site so a burst of new torrents does not
	// trip the tracker's rate limit.
	requestGap = 300 * time.Millisecond
)

var ErrNotLoggedIn = errors.New("site page has no user link; the cookie is probably expired")

type Inspection struct {
	Free bool
	HR   bool
	// HRUnknown is set when the site has no H&R rule, so HR false proves nothing.
	HRUnknown bool
	// Rule names the site rule that read the page.
	Rule string
}

// NexusPHP talks to one site with the user's cookie: a NexusPHP site, or M-Team,
// where the cookie field holds the API key.
type NexusPHP struct {
	client *http.Client
	cookie string

	mu   sync.Mutex
	last time.Time
	// apiLast spaces M-Team API calls, which it limits to about one every 3 seconds.
	apiLast time.Time
}

func NewNexusPHP(cookie string) *NexusPHP {
	return &NexusPHP{client: &http.Client{Timeout: 15 * time.Second}, cookie: cookie}
}

// FetchFeed polls the RSS URL with the validators from the last poll. notModified
// is true on 304, and then body is nil.
func (n *NexusPHP) FetchFeed(ctx context.Context, feedURL, etag, lastModified string) (body []byte, newETag, newLastModified string, notModified bool, err error) {
	req, err := n.request(ctx, feedURL)
	if err != nil {
		return nil, "", "", false, err
	}
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}
	if lastModified != "" {
		req.Header.Set("If-Modified-Since", lastModified)
	}
	resp, err := n.do(req)
	if err != nil {
		return nil, "", "", false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotModified {
		return nil, etag, lastModified, true, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, "", "", false, fmt.Errorf("rss returned HTTP %d", resp.StatusCode)
	}
	body, err = readLimited(resp.Body, maxPageBytes)
	if err != nil {
		return nil, "", "", false, err
	}
	return body, resp.Header.Get("ETag"), resp.Header.Get("Last-Modified"), false, nil
}

// Inspect reads the details page and reports the promotion and H&R markers, using
// the rule for the page's host.
func (n *NexusPHP) Inspect(ctx context.Context, detailsURL string) (Inspection, error) {
	u, err := url.Parse(detailsURL)
	if err != nil {
		return Inspection{}, errors.New("invalid details URL")
	}
	if isMTeam(u.Host) {
		return n.inspectMTeam(ctx, detailsURL)
	}
	if isColapt(u.Host) {
		return n.inspectColapt(ctx, detailsURL)
	}
	req, err := n.request(ctx, detailsURL)
	if err != nil {
		return Inspection{}, err
	}
	resp, err := n.do(req)
	if err != nil {
		return Inspection{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Inspection{}, fmt.Errorf("details page returned HTTP %d", resp.StatusCode)
	}
	body, err := readLimited(resp.Body, maxPageBytes)
	if err != nil {
		return Inspection{}, err
	}
	return InspectPage(u.Host, body)
}

// InspectPage finds the promotion and H&R markers in a details page of the host.
func InspectPage(host string, page []byte) (Inspection, error) {
	r := ruleFor(host)
	login := r.login
	if login == "" {
		login = "userdetails"
	}
	if !bytes.Contains(page, []byte(login)) {
		return Inspection{}, ErrNotLoggedIn
	}
	doc, err := html.Parse(bytes.NewReader(page))
	if err != nil {
		return Inspection{}, fmt.Errorf("parse details page: %w", err)
	}
	free, err := r.free(doc)
	if err != nil {
		return Inspection{}, err
	}
	insp := Inspection{Free: free, Rule: r.name, HRUnknown: r.hr == nil}
	if r.hr != nil {
		insp.HR = r.hr(doc)
	}
	return insp, nil
}

// ReadsHR reports whether brush knows how the host marks H&R torrents. Like Vertex,
// it reads H&R only on the sites Vertex does, so skipping H&R elsewhere refuses
// every torrent rather than letting an unrecognised H&R one through.
func ReadsHR(host string) bool {
	return !isMTeam(host) && !isColapt(host) && ruleFor(host).hr != nil
}

func (n *NexusPHP) Download(ctx context.Context, rawURL string) ([]byte, error) {
	req, err := n.request(ctx, rawURL)
	if err != nil {
		return nil, err
	}
	resp, err := n.do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("torrent download returned HTTP %d", resp.StatusCode)
	}
	return readLimited(resp.Body, maxTorrentBytes)
}

func (n *NexusPHP) request(ctx context.Context, rawURL string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("User-Agent", "vertex-qui")
	// M-Team's RSS and download links are signed; its "cookie" is an API key.
	if n.cookie != "" && !isMTeam(req.URL.Host) {
		req.Header.Set("Cookie", n.cookie)
	}
	return req, nil
}

func (n *NexusPHP) do(req *http.Request) (*http.Response, error) {
	if err := n.wait(req.Context()); err != nil {
		return nil, err
	}
	resp, err := n.client.Do(req)
	if err != nil {
		// *url.Error prints the full URL, and feed and download URLs carry the passkey.
		var urlErr *url.Error
		if errors.As(err, &urlErr) {
			return nil, fmt.Errorf("request to %s failed: %w", req.URL.Host, urlErr.Err)
		}
		return nil, fmt.Errorf("request to %s failed: %w", req.URL.Host, err)
	}
	return resp, nil
}

func (n *NexusPHP) wait(ctx context.Context) error {
	n.mu.Lock()
	next := n.last.Add(requestGap)
	now := time.Now()
	if next.Before(now) {
		next = now
	}
	n.last = next
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

func readLimited(r io.Reader, limit int64) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	if int64(len(body)) > limit {
		return nil, fmt.Errorf("response larger than %d bytes", limit)
	}
	return body, nil
}

func findByID(n *html.Node, id string) *html.Node {
	if n.Type == html.ElementNode && attr(n, "id") == id {
		return n
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if found := findByID(c, id); found != nil {
			return found
		}
	}
	return nil
}

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}
