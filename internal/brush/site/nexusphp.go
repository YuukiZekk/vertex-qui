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
	"strings"
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

// freeClasses are the NexusPHP promotion classes that mean the download is not counted.
var freeClasses = []string{"free", "twoupfree", "pro_free", "pro_free2up"}

type Inspection struct {
	Free bool
	HR   bool
}

// NexusPHP talks to one NexusPHP site with the user's cookie.
type NexusPHP struct {
	client *http.Client
	cookie string

	mu   sync.Mutex
	last time.Time
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

// Inspect reads the details page and reports the promotion and H&R markers.
func (n *NexusPHP) Inspect(ctx context.Context, detailsURL string) (Inspection, error) {
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
	return InspectPage(body)
}

// InspectPage finds the markers in a details page. Promotion classes are read only
// inside the title (#top), because the rest of the page can carry the same classes
// for other torrents or for site-wide notices.
func InspectPage(page []byte) (Inspection, error) {
	if !bytes.Contains(page, []byte("userdetails.php")) {
		return Inspection{}, ErrNotLoggedIn
	}
	doc, err := html.Parse(bytes.NewReader(page))
	if err != nil {
		return Inspection{}, fmt.Errorf("parse details page: %w", err)
	}
	top := findByID(doc, "top")
	if top == nil {
		return Inspection{}, errors.New("details page has no #top title")
	}
	return Inspection{
		Free: hasClass(top, freeClasses...),
		HR:   hasClass(doc, "hitandrun"),
	}, nil
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
	if n.cookie != "" {
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

func hasClass(n *html.Node, classes ...string) bool {
	if n.Type == html.ElementNode {
		for _, have := range strings.Fields(attr(n, "class")) {
			for _, want := range classes {
				if have == want {
					return true
				}
			}
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if hasClass(c, classes...) {
			return true
		}
	}
	return false
}

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}
