// Copyright (c) 2026, vertex-qui contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package site

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Each site rule follows the original Vertex's scrape.js for that site.
func TestSiteRules(t *testing.T) {
	const user = `<a href="userdetails.php?id=1">me</a>`
	tests := []struct {
		name, host, page string
		free, hr         bool
		rule             string
	}{
		{"generic site uses the title", "zmpt.cc", `<h1 id="top">x <b><font class="free">免费</font></b></h1>`, true, false, "nexusphp"},
		{"listed NexusPHP site uses the title", "hdsky.me", `<h1 id="top">x <font class="halfdown">50%</font></h1>`, false, false, "nexusphp"},
		{"hit and run is an image", "ourbits.club", `<h1 id="top">x</h1><span class="hitandrun">legend</span>`, false, false, "nexusphp"},
		{"opencd free icon", "open.cd", `<div class="title">x <img class="pro_free" src="t.gif"></div>`, true, false, "opencd"},
		{"opencd first icon is not free", "www.open.cd", `<div class="title">x <img class="pro_50pctdown"><img class="pro_free"></div>`, false, false, "opencd"},
		{"ttg free and hr", "totheglory.im", `<img src="/pic/ico_free.gif" class="topic"><img src="/pic/hit_run.gif" alt="Hit & Run">`, true, true, "totheglory"},
		{"ttg not free", "totheglory.im", `<img src="/pic/ico_half.gif" class="topic">`, false, false, "totheglory"},
		{"u2 free icon", "u2.dmhy.org", `<table><tr><td valign="top"><img class="pro_free2up"></td></tr></table>`, true, false, "u2"},
		{"u2 custom zero download", "u2.dmhy.org", `<table><tr><td valign="top"><img class="arrowdown"> <b>0.00X</b></td></tr></table>`, true, false, "u2"},
		{"u2 custom half download", "u2.dmhy.org", `<table><tr><td valign="top"><img class="arrowdown"><b>0.50X</b></td></tr></table>`, false, false, "u2"},
		{"hares bold font", "club.hares.top", `<b><font class="twoupfree">2xFree</font></b>`, true, false, "bold-font"},
		{"putao bold font", "pt.sjtu.edu.cn", `<b><font class="halfdown">50%</font></b><b><font class="free">x</font></b>`, false, false, "bold-font"},
		{"byr share", "byr.pt", `<div id="share"><span class="free">免费</span></div>`, true, false, "byr"},
		{"hhanclub tag", "hhanclub.net", `<span class="promotion-tag promotion-tag-free">Free</span>`, true, false, "hhanclub"},
		{"hudbt minor list", "hudbt.hust.edu.cn", `<div class="minor-list"><img class="twoupfree"></div>`, true, false, "hudbt"},
		{"hdcity font", "hdcity.city", `<font class="free">Free</font>`, true, false, "hdcity"},
		{"chdbits text hr", "ptchdbits.co", `<h1 id="top">x <font class="free">Free</font></h1><table><tr><td><b>H&amp;R:&nbsp;</b>5day</td></tr></table>`, true, true, "chdbits"},
		// The basic info row as the source writes it: a raw "&", line breaks, and the row left open.
		{"chdbits revival hr in the info row", "ptchdbits.co", "<h1 id=\"top\">x <font class=\"free\">Free</font></h1><table><tr><td class=\"rowhead\">基本信息</td>\n" +
			"<td class=\"rowfollow\"><b>大小：</b>23.36 GB&nbsp;&nbsp;&nbsp;<b>制作组:&nbsp;</b>压制组&nbsp;&nbsp;&nbsp;<b>H&R:&nbsp;</b>3day\n</table>", true, true, "chdbits"},
		{"chdbits without hr", "ptchdbits.co", `<h1 id="top">x</h1><table><tr><td><b>制作组:&nbsp;</b>压制组</td></tr></table>`, false, false, "chdbits"},
		{"chdbits h&r of another length is not hr", "ptchdbits.co", `<h1 id="top">x</h1><table><tr><td><b>H&amp;R:&nbsp;</b>0day</td></tr></table>`, false, false, "chdbits"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := InspectPage(tt.host, []byte(user+tt.page))
			require.NoError(t, err)
			require.Equal(t, Inspection{Free: tt.free, HR: tt.hr, Rule: tt.rule}, got)
		})
	}
}

func TestParseFeedQuirks(t *testing.T) {
	t.Run("feed inside the XML viewer page", func(t *testing.T) {
		body := []byte(`<html><head><style id="xml-viewer-style">x</style></head><body><rss version="2.0"><channel>
<item><title>A</title><link>https://pt.example/details.php?id=1</link><guid>g1</guid>
<enclosure url="https://pt.example/download.php?id=1&amp;passkey=k" length="10"/></item>
</channel></rss></body></html>`)
		items, err := ParseFeed("https://pt.example/torrentrss.php", body)
		require.NoError(t, err)
		require.Len(t, items, 1)
		require.Equal(t, int64(10), items[0].Size)
	})

	t.Run("linktype=dl points the link at the details page", func(t *testing.T) {
		body := []byte(`<?xml version="1.0"?><rss version="2.0"><channel>
<item><title>A</title><link>https://www.pt.example/download.php?id=77&amp;passkey=secret</link><guid>g77</guid>
<enclosure url="https://www.pt.example/download.php?id=77&amp;passkey=secret" length="5"/></item>
<item><title>B</title><link>https://www.pt.example/download.php?id=78&amp;passkey=secret</link><guid>g78</guid></item>
</channel></rss>`)
		items, err := ParseFeed("https://www.pt.example/torrentrss.php?rows=50&linktype=dl", body)
		require.NoError(t, err)
		require.Len(t, items, 2)
		require.Equal(t, "https://www.pt.example/details.php?id=77", items[0].Link)
		require.Equal(t, "https://www.pt.example/download.php?id=77&passkey=secret", items[0].DownloadURL)
		require.Equal(t, "https://www.pt.example/download.php?id=78&passkey=secret", items[1].DownloadURL, "the link is the download when there is no enclosure")
	})

	t.Run("putao link and title size", func(t *testing.T) {
		body := []byte(`<?xml version="1.0"?><rss version="2.0"><channel>
<item><title>Some.Movie.2160p [12.50 GB]</title>
<link>https://pt.sjtu.edu.cn/download.php?id=42&amp;passkey=secret&amp;https=1</link>
<guid>pt42</guid><pubDate>Fri, 30 Oct 2026 20:00:00 +0800</pubDate></item>
</channel></rss>`)
		items, err := ParseFeed("https://pt.sjtu.edu.cn/torrentrss.php?rows=10", body)
		require.NoError(t, err)
		require.Len(t, items, 1)
		require.Equal(t, "https://pt.sjtu.edu.cn/details.php?id=42", items[0].Link)
		require.Equal(t, "https://pt.sjtu.edu.cn/download.php?id=42&passkey=secret&https=1", items[0].DownloadURL)
		require.Equal(t, int64(12_500_000_000), items[0].Size)
		require.Equal(t, "pt42", items[0].GUID)
	})
}

// M-Team is read through its API with the API key, retrying on rate limits and on
// a cut-off response, and never gets the key sent as a cookie.
func TestInspectMTeam(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "key-123", r.Header.Get("x-api-key"))
		require.Empty(t, r.Header.Get("Cookie"))
		require.NoError(t, r.ParseMultipartForm(1<<20))
		switch calls.Add(1) {
		case 1:
			_, _ = io.WriteString(w, `{"code":"4","message":"rate"}`)
		case 2:
			_, _ = io.WriteString(w, `{"code":"0","data":{"sta`)
		default:
			require.Equal(t, "987", r.FormValue("id"))
			_, _ = io.WriteString(w, `{"code":"0","message":"SUCCESS","data":{"status":{"discount":"_2X_FREE"}}}`)
		}
	}))
	defer srv.Close()
	oldAPI, oldGap := mteamAPI, mteamGap
	mteamAPI, mteamGap = srv.URL, time.Millisecond
	defer func() { mteamAPI, mteamGap = oldAPI, oldGap }()

	n := NewNexusPHP("key-123")
	got, err := n.Inspect(context.Background(), "https://kp.m-team.cc/detail/987")
	require.NoError(t, err)
	require.Equal(t, Inspection{Free: true, Rule: "m-team"}, got)
	require.Equal(t, int32(3), calls.Load())

	srv.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"code":"1","message":"key invalid","data":null}`)
	})
	_, err = n.Inspect(context.Background(), "https://kp.m-team.cc/detail/987")
	require.Error(t, err)
	require.True(t, strings.Contains(err.Error(), "API key"), err.Error())

	req, err := n.request(context.Background(), "https://rss.m-team.cc/api/rss/dl/abc")
	require.NoError(t, err)
	require.Empty(t, req.Header.Get("Cookie"), "the API key is not a cookie")
	req, err = n.request(context.Background(), "https://pt.example/download.php?id=1")
	require.NoError(t, err)
	require.Equal(t, "key-123", req.Header.Get("Cookie"))
}

func TestCheckLogin(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/index.php", r.URL.Path)
		if r.Header.Get("Cookie") == "good" {
			_, _ = io.WriteString(w, `<a href="userdetails.php?id=1">me</a>`)
			return
		}
		_, _ = io.WriteString(w, `<form action="takelogin.php"></form>`)
	}))
	defer srv.Close()
	require.NoError(t, NewNexusPHP("good").CheckLogin(context.Background(), srv.URL+"/"))
	require.ErrorIs(t, NewNexusPHP("bad").CheckLogin(context.Background(), srv.URL), ErrNotLoggedIn)

	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") == "key" {
			_, _ = io.WriteString(w, `{"code":"0","message":"SUCCESS","data":{}}`)
			return
		}
		_, _ = io.WriteString(w, `{"code":"1","message":"bad key"}`)
	}))
	defer api.Close()
	oldAPI, oldGap := mteamProfileAPI, mteamGap
	mteamProfileAPI, mteamGap = api.URL, time.Millisecond
	defer func() { mteamProfileAPI, mteamGap = oldAPI, oldGap }()
	require.NoError(t, NewNexusPHP("key").CheckLogin(context.Background(), "https://kp.m-team.cc"))
	require.ErrorContains(t, NewNexusPHP("nope").CheckLogin(context.Background(), "https://kp.m-team.cc"), "bad key")

	require.Equal(t, "m-team", RuleName("kp.m-team.cc"))
	require.Equal(t, "chdbits", RuleName("ptchdbits.co"))
	require.Equal(t, "nexusphp", RuleName("zmpt.cc"))
}

// colapt is read through its JSON API with the session cookie.
func TestColapt(t *testing.T) {
	api, err := colaptTorrentAPI("https://www.colapt.com/details.php?id=77")
	require.NoError(t, err)
	require.Equal(t, "https://www.colapt.com/api/v1/torrents/77", api)
	api, err = colaptTorrentAPI("https://www.colapt.com/torrents/78")
	require.NoError(t, err)
	require.Equal(t, "https://www.colapt.com/api/v1/torrents/78", api)
	api, err = colaptTorrentAPI("https://www.colapt.com/feeds/downloads/abc123/79")
	require.NoError(t, err)
	require.Equal(t, "https://www.colapt.com/api/v1/torrents/79", api)
	require.Equal(t, "colapt", RuleName("www.colapt.com"))

	items, err := ParseFeed("https://www.colapt.com/torrentrss.php?rows=50&linktype=dl", []byte(`<?xml version="1.0"?><rss version="2.0"><channel>
<item><title>A</title><link>https://www.colapt.com/feeds/downloads/abc123/80</link><guid>https://www.colapt.com/torrents/80</guid></item>
</channel></rss>`))
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, "https://www.colapt.com/torrents/80", items[0].Link, "the passkey path is not kept as the details link")
	require.Equal(t, "https://www.colapt.com/feeds/downloads/abc123/80", items[0].DownloadURL)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Cookie") != "session=1" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		require.Equal(t, "application/json", r.Header.Get("Accept"))
		switch r.URL.Path {
		case "/api/v1/torrents/1":
			_, _ = io.WriteString(w, `{"id":1,"promotion":{"upload":{"num":2,"den":1},"download":{"num":0,"den":1},"ends_at":null}}`)
		case "/api/v1/torrents/2":
			_, _ = io.WriteString(w, `{"id":2,"promotion":{"upload":{"num":1,"den":1},"download":{"num":1,"den":2},"ends_at":null}}`)
		case "/api/v1/torrents/3":
			_, _ = io.WriteString(w, `{"id":3,"promotion":null}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	n := NewNexusPHP("session=1")
	for id, free := range map[string]bool{"1": true, "2": false, "3": false} {
		got, err := n.inspectColapt(context.Background(), srv.URL+"/details.php?id="+id)
		require.NoError(t, err)
		require.Equal(t, Inspection{Free: free, Rule: "colapt"}, got, id)
	}
	_, err = NewNexusPHP("bad").inspectColapt(context.Background(), srv.URL+"/details.php?id=1")
	require.ErrorIs(t, err, ErrNotLoggedIn)
}
