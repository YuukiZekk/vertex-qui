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

func TestPercentile(t *testing.T) {
	require.Nil(t, percentile(nil, 95))
	values := []float64{9, 1, 2, 3, 4, 5, 6, 7, 8, 10}
	require.InDelta(t, 5, *percentile(values, 50), 0)
	require.InDelta(t, 10, *percentile(values, 95), 0)
}

// The overview splits each added torrent's delay into discovery and processing,
// counts failures and unconfirmed adds, judges the fast start only after a minute,
// and leaves the discovery of a revival feed out.
func TestPerformanceOverview(t *testing.T) {
	ctx := t.Context()
	db := testdb.NewMigratedSQLite(t, t.Name())
	cipher, err := models.NewCredentialCipher(make([]byte, 32))
	require.NoError(t, err)
	qb := &fakeQB{free: map[int]int64{1: 100 * gib}, listed: map[int][]qbt.Torrent{}}
	svc, err := New(ctx, db, cipher, qb)
	require.NoError(t, err)
	now := time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return now }

	siteID, err := svc.store.CreateSite(ctx, store.Site{Name: "zm", BaseURL: "https://zmpt.cc"})
	require.NoError(t, err)
	feed, err := svc.store.CreateFeed(ctx, store.Feed{SiteID: siteID, Name: "zm", URL: "https://zmpt.cc/rss",
		IntervalSeconds: 60, WindowSeconds: 300, InstanceIDs: []int{1}})
	require.NoError(t, err)
	revival, err := svc.store.CreateFeed(ctx, store.Feed{SiteID: siteID, Name: "revival", URL: "https://zmpt.cc/rss2",
		IntervalSeconds: 60, WindowSeconds: 300, InstanceIDs: []int{1}, AgeFromFirstSeen: true})
	require.NoError(t, err)

	insert := func(feedID int64, guid, status string, published, seen, added, firstUpload time.Time) {
		_, _, err := svc.store.InsertEntry(ctx, store.Entry{FeedID: feedID, GUID: guid, Title: guid, Status: status,
			PublishedAt: published.Unix(), FirstSeenAt: seen.Unix()})
		require.NoError(t, err)
		var addedAt, upAt int64
		if !added.IsZero() {
			addedAt = added.Unix()
		}
		if !firstUpload.IsZero() {
			upAt = firstUpload.Unix()
		}
		_, err = db.ExecContext(ctx, "UPDATE brush_entries SET instance_id = 1, info_hash = ?, added_at = ?, first_upload_at = ? WHERE guid = ?",
			guid, addedAt, upAt, guid)
		require.NoError(t, err)
	}
	seen := now.Add(-time.Hour)
	// 30 s to discover, 4 s to add, uploading 10 s later.
	insert(feed, "a", store.StatusAdded, seen.Add(-30*time.Second), seen, seen.Add(4*time.Second), seen.Add(14*time.Second))
	// 10 s to discover, 2 s to add, never uploading.
	insert(feed, "b", store.StatusAdded, seen.Add(-10*time.Second), seen, seen.Add(2*time.Second), time.Time{})
	// Old pubDate on a revival feed: no discovery delay.
	insert(revival, "c", store.StatusAdded, seen.Add(-15*24*time.Hour), seen, seen.Add(3*time.Second), seen.Add(5*time.Second))
	insert(feed, "d", store.StatusFailed, seen, seen, time.Time{}, time.Time{})
	insert(feed, "e", store.StatusUnknown, seen, seen, time.Time{}, time.Time{})
	// Added 30 s ago: too soon to judge its start.
	insert(feed, "f", store.StatusAdded, now.Add(-40*time.Second), now.Add(-35*time.Second), now.Add(-30*time.Second), time.Time{})

	r := chi.NewRouter()
	svc.Routes(r)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/performance?hours=6", nil))
	require.Equal(t, http.StatusOK, rec.Code)
	var resp perfResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	o := resp.Overall
	require.Equal(t, 4, o.Added)
	require.Equal(t, 1, o.Failed)
	require.Equal(t, 1, o.Pending)
	require.InDelta(t, 30, *o.DiscoveryP95, 0, "a, b and f, not the revival")
	require.InDelta(t, 5, *o.ProcessingP95, 0)
	require.InDelta(t, 34, *o.EntryP95, 0)
	require.Equal(t, 3, o.FastStartJudge)
	require.Equal(t, 2, o.FastStart)
	require.Equal(t, int64(15*60), resp.BucketSeconds)
	require.Len(t, resp.Groups, 1)
	require.Equal(t, "zm", resp.Groups[0].Name)
	require.Equal(t, 1, resp.Groups[0].Slow, "the bucket with a 34 s entry")

	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/performance?hours=6&by=instance", nil))
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Groups, 1)
	require.Equal(t, "vps-1", resp.Groups[0].Name)

	// The drill-down lists one site's entries in a period, newest first, with their stage times.
	rec = httptest.NewRecorder()
	url := "/performance/entries?by=site&id=" + strconv.FormatInt(siteID, 10) +
		"&from=" + strconv.FormatInt(seen.Unix(), 10) + "&to=" + strconv.FormatInt(seen.Add(time.Minute).Unix(), 10)
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, url, nil))
	require.Equal(t, http.StatusOK, rec.Code)
	var details []store.PerfDetail
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &details))
	require.Len(t, details, 5, "a to e; f was seen later")
	byTitle := map[string]store.PerfDetail{}
	for _, d := range details {
		byTitle[d.Title] = d
	}
	require.Equal(t, seen.Add(4*time.Second).Unix(), byTitle["a"].AddedAt)
	require.Equal(t, seen.Add(14*time.Second).Unix(), byTitle["a"].FirstUploadAt)
	require.Equal(t, "zm", byTitle["a"].FeedName)

	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/performance/entries?by=site&id=1", nil))
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

// The sampler stamps a brush torrent's first upload once, and only for torrents
// added recently.
func TestMarkFirstUploads(t *testing.T) {
	ctx := t.Context()
	db := testdb.NewMigratedSQLite(t, t.Name())
	cipher, err := models.NewCredentialCipher(make([]byte, 32))
	require.NoError(t, err)
	svc, err := New(ctx, db, cipher, &fakeQB{})
	require.NoError(t, err)
	siteID, err := svc.store.CreateSite(ctx, store.Site{Name: "zm", BaseURL: "https://zmpt.cc"})
	require.NoError(t, err)
	feed, err := svc.store.CreateFeed(ctx, store.Feed{SiteID: siteID, Name: "zm", URL: "https://zmpt.cc/rss",
		IntervalSeconds: 60, WindowSeconds: 300, InstanceIDs: []int{1}})
	require.NoError(t, err)
	now := time.Now()
	for _, h := range []string{"new", "old"} {
		id, _, err := svc.store.InsertEntry(ctx, store.Entry{FeedID: feed, GUID: h, Title: h, Status: store.StatusPending})
		require.NoError(t, err)
		require.NoError(t, svc.store.Claim(ctx, id, h, 1, 1))
		require.NoError(t, svc.store.MarkAdded(ctx, id))
	}
	_, err = db.ExecContext(ctx, "UPDATE brush_entries SET added_at = ? WHERE info_hash = 'old'", now.Add(-time.Hour).Unix())
	require.NoError(t, err)

	torrents := []qbt.Torrent{
		{Hash: "new", Tags: Tag, Uploaded: 10},
		{Hash: "old", Tags: Tag, Uploaded: 10},
	}
	svc.markFirstUploads(ctx, 1, now, torrents)
	svc.markFirstUploads(ctx, 1, now.Add(time.Minute), torrents)
	var newAt, oldAt int64
	require.NoError(t, db.QueryRowContext(ctx, "SELECT first_upload_at FROM brush_entries WHERE info_hash = 'new'").Scan(&newAt))
	require.NoError(t, db.QueryRowContext(ctx, "SELECT first_upload_at FROM brush_entries WHERE info_hash = 'old'").Scan(&oldAt))
	require.Equal(t, now.Unix(), newAt, "stamped once, at the first sight")
	require.Zero(t, oldAt, "added before the window")
}
