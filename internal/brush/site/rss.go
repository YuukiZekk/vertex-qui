// Copyright (c) 2026, vertex-qui contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package site

import (
	"encoding/xml"
	"fmt"
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

// ParseFeed reads a NexusPHP RSS 2.0 feed. Items without a download URL are dropped,
// because brush cannot add them.
func ParseFeed(body []byte) ([]Item, error) {
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

func parsePubDate(s string) time.Time {
	s = strings.TrimSpace(s)
	for _, layout := range pubDateLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}
