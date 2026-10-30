// Copyright (c) 2026, vertex-qui contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package site

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestParseFeed(t *testing.T) {
	body := []byte(`<?xml version="1.0" encoding="utf-8"?>
<rss version="2.0"><channel>
<item>
  <title>Sample Show S01 2160p WEB-DL-GRP</title>
  <link>https://pt.example/details.php?id=11</link>
  <guid isPermaLink="false">abc11</guid>
  <pubDate>Fri, 30 Oct 2026 20:00:00 +0800</pubDate>
  <enclosure url="https://pt.example/download.php?id=11&amp;passkey=secret" length="21474836480" type="application/x-bittorrent"/>
</item>
<item>
  <title>No Guid Item</title>
  <link>https://pt.example/details.php?id=12</link>
  <pubDate>not a date</pubDate>
  <enclosure url="https://pt.example/download.php?id=12&amp;passkey=secret" length="0"/>
</item>
<item>
  <title>No Download</title>
  <link>https://pt.example/details.php?id=13</link>
</item>
</channel></rss>`)

	items, err := ParseFeed(body)
	require.NoError(t, err)
	require.Len(t, items, 2)

	require.Equal(t, "abc11", items[0].GUID)
	require.Equal(t, int64(20<<30), items[0].Size)
	require.Equal(t, time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC), items[0].Published.UTC())
	require.Equal(t, "https://pt.example/download.php?id=11&passkey=secret", items[0].DownloadURL)

	require.Equal(t, "https://pt.example/details.php?id=12", items[1].GUID, "guid falls back to the link, never to the passkey URL")
	require.Zero(t, items[1].Size)
	require.True(t, items[1].Published.IsZero())
}

func TestInspectPage(t *testing.T) {
	const user = `<a href="userdetails.php?id=1">me</a>`
	tests := []struct {
		name    string
		page    string
		want    Inspection
		wantErr error
	}{
		{
			name: "free in title",
			page: user + `<h1 id="top">Name <b>[<font class="free">Free</font>]</b></h1>`,
			want: Inspection{Free: true},
		},
		{
			name: "2x free icon in title",
			page: user + `<h1 id="top">Name <img class="pro_free2up" src="pic/trans.gif"></h1>`,
			want: Inspection{Free: true},
		},
		{
			name: "free class outside title is ignored",
			page: user + `<h1 id="top">Name</h1><div><span class="free">site-wide notice</span></div>`,
			want: Inspection{},
		},
		{
			name: "hit and run",
			page: user + `<h1 id="top">Name</h1><img class="hitandrun" src="pic/hr.png">`,
			want: Inspection{HR: true},
		},
		{
			name:    "logged out",
			page:    `<form action="takelogin.php"></form>`,
			wantErr: ErrNotLoggedIn,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := InspectPage([]byte(tt.page))
			if tt.wantErr != nil {
				require.True(t, errors.Is(err, tt.wantErr), "got %v", err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}

	_, err := InspectPage([]byte(user + `<h1>no id</h1>`))
	require.Error(t, err, "a page without #top cannot be read, so it must not count as not-free")
}
