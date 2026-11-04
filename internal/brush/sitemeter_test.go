// Copyright (c) 2026, vertex-qui contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package brush

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	qbt "github.com/autobrr/go-qbittorrent"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"

	"github.com/autobrr/qui/internal/brush/store"
	"github.com/autobrr/qui/internal/models"
	"github.com/autobrr/qui/internal/testutil/testdb"
)

func TestSiteMatcher(t *testing.T) {
	m := newSiteMatcher([]store.Site{
		{ID: 1, BaseURL: "https://hdsky.me"},
		{ID: 2, BaseURL: "https://pt.sjtu.edu.cn/"},
		{ID: 3, BaseURL: "https://kp.m-team.cc"},
		{ID: 4, BaseURL: "https://www.uhddolby.com"},
	})
	for tracker, want := range map[string]int64{
		"https://tracker.hdsky.me/announce.php?passkey=x": 1,
		"https://hdsky.me:443/announce":                   1,
		"https://tracker.sjtu.edu.cn/announce":            2,
		"https://pku.edu.cn/announce":                     0,
		"https://tracker.m-team.io/announce":              3,
		"https://tracker.uhddolby.com/announce":           4,
		"https://hddolby.com/announce":                    0,
		"":                                                0,
	} {
		require.Equal(t, want, m.match(tracker), tracker)
	}
}

// The sampler adds each torrent's growth to its site: a brush torrent to its
// feed's site whatever its tracker, any other by its tracker, keeping the last site
// while no tracker works. A first sight and a counter that went down add nothing,
// and a new day's sample writes what was pending to the day before.
func TestSiteTotals(t *testing.T) {
	ctx := t.Context()
	db := testdb.NewMigratedSQLite(t, t.Name())
	cipher, err := models.NewCredentialCipher(make([]byte, 32))
	require.NoError(t, err)
	qb := &fakeQB{free: map[int]int64{1: 100 * gib}, listed: map[int][]qbt.Torrent{}}
	svc, err := New(ctx, db, cipher, qb)
	require.NoError(t, err)
	require.NoError(t, svc.store.SetFloor(ctx, 1, 10*gib))
	sky, err := svc.store.CreateSite(ctx, store.Site{Name: "sky", BaseURL: "https://hdsky.me"})
	require.NoError(t, err)
	zm, err := svc.store.CreateSite(ctx, store.Site{Name: "zm", BaseURL: "https://zmpt.cc"})
	require.NoError(t, err)
	feed, err := svc.store.CreateFeed(ctx, store.Feed{SiteID: zm, Name: "zm", URL: "https://zmpt.cc/rss", IntervalSeconds: 60,
		WindowSeconds: 300, InstanceIDs: []int{1}})
	require.NoError(t, err)
	entry, _, err := svc.store.InsertEntry(ctx, store.Entry{FeedID: feed, GUID: "g", Title: "b", Status: store.StatusPending})
	require.NoError(t, err)
	require.NoError(t, svc.store.Claim(ctx, entry, "brush", 1, 1))
	require.NoError(t, svc.store.MarkAdded(ctx, entry))

	// Days follow the server's local time zone.
	loc := time.Local
	at := time.Date(2026, 11, 2, 23, 58, 0, 0, loc)
	svc.now = func() time.Time { return at }
	read := func(manual, brush qbt.Torrent) {
		qb.mu.Lock()
		qb.listed[1] = []qbt.Torrent{manual, brush}
		qb.mu.Unlock()
		svc.sample(ctx)
	}
	manual := qbt.Torrent{Hash: "manual", Tracker: "https://tracker.hdsky.me/announce", Uploaded: 1000, Downloaded: 50}
	// The brush torrent announces to a tracker that matches no site.
	brush := qbt.Torrent{Hash: "brush", Tracker: "https://other.example/announce", Uploaded: 500}
	read(manual, brush)

	at = at.Add(10 * time.Second)
	manual.Uploaded, manual.Downloaded, manual.Tracker = 1600, 80, ""
	brush.Uploaded = 900
	read(manual, brush)

	at = at.Add(10 * time.Second)
	manual.Uploaded = 100 // qBittorrent restarted behind
	read(manual, brush)

	// Past midnight: the pending totals go to the day before.
	at = time.Date(2026, 11, 3, 0, 0, 30, 0, loc)
	svc.now = func() time.Time { return at }
	manual.Uploaded = 400
	read(manual, brush)

	day1, err := svc.store.SiteTransfersOn(ctx, "2026-11-02")
	require.NoError(t, err)
	require.Equal(t, store.SiteTransfer{SiteID: sky, Uploaded: 600, Downloaded: 30}, day1[sky])
	require.Equal(t, store.SiteTransfer{SiteID: zm, Uploaded: 400}, day1[zm])

	// The next flush writes the new day's growth to the new day.
	at = at.Add(siteFlushEvery)
	read(manual, brush)
	day2, err := svc.store.SiteTransfersOn(ctx, "2026-11-03")
	require.NoError(t, err)
	require.Equal(t, int64(300), day2[sky].Uploaded)

	// The sites API shows today's totals and what the torrents do now.
	manual.UpSpeed, manual.Size, manual.Tracker = 7, 40, "https://tracker.hdsky.me/announce"
	qb.listed[1] = []qbt.Torrent{manual, brush}
	r := chi.NewRouter()
	svc.Routes(r)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/sites", nil))
	require.Equal(t, http.StatusOK, rec.Code)
	var views []siteView
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &views))
	require.Len(t, views, 2)
	require.Equal(t, "sky", views[0].Name)
	require.Equal(t, int64(300), views[0].TodayUploaded)
	require.Equal(t, int64(7), views[0].UploadSpeed)
	require.Equal(t, int64(40), views[0].SizeBytes)
	require.Equal(t, 1, views[0].Torrents)
	require.Equal(t, []string{"tracker.hdsky.me"}, views[0].TrackerHosts)
	require.Equal(t, 1, views[1].Torrents, "the brush torrent belongs to its feed's site")
	require.Equal(t, 1, views[1].Feeds)
	require.Equal(t, "nexusphp", views[1].Rule)

	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/sites/"+strconv.FormatInt(zm, 10), nil))
	require.Equal(t, http.StatusConflict, rec.Code, "a site with feeds stays")
}

// Each instance is read at the shortest interval of its groups, others at the
// default, and a seeding group can be read less often.
func TestSamplerReadsEachInstanceAtItsInterval(t *testing.T) {
	ctx := t.Context()
	db := testdb.NewMigratedSQLite(t, t.Name())
	cipher, err := models.NewCredentialCipher(make([]byte, 32))
	require.NoError(t, err)
	qb := &fakeQB{listed: map[int][]qbt.Torrent{}}
	svc, err := New(ctx, db, cipher, qb)
	require.NoError(t, err)
	for _, id := range []int{1, 2, 3} {
		require.NoError(t, svc.store.SetFloor(ctx, id, 10*gib))
	}
	fast, err := svc.store.CreateGroup(ctx, "brush")
	require.NoError(t, err)
	slow, err := svc.store.CreateGroup(ctx, "seeding")
	require.NoError(t, err)
	require.NoError(t, svc.store.SetGroupReadInterval(ctx, fast, 5))
	require.NoError(t, svc.store.SetGroupReadInterval(ctx, slow, 30))
	require.NoError(t, svc.store.SetInstanceGroups(ctx, 1, []int64{fast, slow}))
	require.NoError(t, svc.store.SetInstanceGroups(ctx, 2, []int64{slow}))

	at := time.Date(2026, 11, 5, 12, 0, 0, 0, time.Local)
	svc.now = func() time.Time { return at }
	for range 60 {
		svc.sample(ctx)
		at = at.Add(samplerBeat)
	}
	qb.mu.Lock()
	defer qb.mu.Unlock()
	require.Equal(t, map[int]int{1: 12, 2: 2, 3: 6}, qb.reads, "5 s in both groups, 30 s seeding, 10 s by default, over a minute")
}
