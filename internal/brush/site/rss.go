// Copyright (c) 2026, vertex-qui contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package site

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type Item struct {
	GUID        string
	Title       string
	Link        string
	DownloadURL string
	// Size is the enclosure length; 0 when the feed does not give one.
	Size int64
	// Published is zero when the feed has no usable pubDate.
	Published time.Time
}

type rssDoc struct {
	Items []struct {
		Title     string `xml:"title"`
		Link      string `xml:"link"`
		GUID      string `xml:"guid"`
		PubDate   string `xml:"pubDate"`
		Enclosure struct {
			URL    string `xml:"url,attr"`
			Length string `xml:"length,attr"`
		} `xml:"enclosure"`
	} `xml:"channel>item"`
}

var pubDateLayouts = []string{time.RFC1123Z, time.RFC1123, time.RFC822Z, time.RFC822, "Mon, 2 Jan 2006 15:04:05 -0700"}

var (
	rssElement = regexp.MustCompile(`(?s)<rss.*</rss>`)
	// titleSize is PuTao's size in the title, like [12.34 GB], in powers of 1000.
	titleSize  = regexp.MustCompile(`\[(\d+(?:\.\d+)?) ([KMGT])B\]`)
	unitFactor = map[string]float64{"K": 1e3, "M": 1e6, "G": 1e9, "T": 1e12}
)

// ParseFeed reads a NexusPHP RSS 2.0 feed from the feed's URL. Items without a
// download URL are dropped, because brush cannot add them. Site quirks follow the
// original Vertex's rss.js: a feed served inside the browser's XML viewer page, and
// PuTao, whose link is the download URL and whose size is in the title.
func ParseFeed(feedURL string, body []byte) ([]Item, error) {
	if bytes.Contains(body, []byte("xml-viewer-style")) {
		if rss := rssElement.Find(body); rss != nil {
			body = rss
		}
	}
	host := ""
	if u, err := url.Parse(feedURL); err == nil {
		host = strings.ToLower(u.Host)
	}
	if host == "pt.sjtu.edu.cn" {
		return parsePuTao(body)
	}

	var doc rssDoc
	if err := xml.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("parse rss: %w", err)
	}

	items := make([]Item, 0, len(doc.Items))
	for _, raw := range doc.Items {
		item := Item{
			Title:       strings.TrimSpace(raw.Title),
			Link:        strings.TrimSpace(raw.Link),
			DownloadURL: strings.TrimSpace(raw.Enclosure.URL),
			GUID:        strings.TrimSpace(raw.GUID),
		}
		if item.DownloadURL == "" {
			continue
		}
		if item.GUID == "" {
			item.GUID = item.Link
		}
		if item.GUID == "" {
			// The download URL carries the passkey, so it is never stored as the guid.
			continue
		}
		if n, err := strconv.ParseInt(strings.TrimSpace(raw.Enclosure.Length), 10, 64); err == nil && n > 0 {
			item.Size = n
		}
		item.Published = parsePubDate(raw.PubDate)
		items = append(items, item)
	}
	return items, nil
}

func parsePuTao(body []byte) ([]Item, error) {
	var doc rssDoc
	if err := xml.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("parse rss: %w", err)
	}
	items := make([]Item, 0, len(doc.Items))
	for _, raw := range doc.Items {
		download := strings.TrimSpace(raw.Link)
		if download == "" {
			continue
		}
		details, _, _ := strings.Cut(download, "&passkey=")
		item := Item{
			Title:       strings.TrimSpace(raw.Title),
			Link:        strings.Replace(details, "download", "details", 1),
			DownloadURL: download,
			GUID:        strings.TrimSpace(raw.GUID),
			Published:   parsePubDate(raw.PubDate),
		}
		if item.GUID == "" {
			item.GUID = item.Link
		}
		if m := titleSize.FindStringSubmatch(item.Title); m != nil {
			if v, err := strconv.ParseFloat(m[1], 64); err == nil {
				item.Size = int64(v * unitFactor[m[2]])
			}
		}
		items = append(items, item)
	}
	return items, nil
}

func parsePubDate(s string) time.Time {
	s = strings.TrimSpace(s)
	for _, layout := range pubDateLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}
