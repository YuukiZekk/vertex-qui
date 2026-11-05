// Copyright (c) 2026, vertex-qui contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package brush

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"

	"github.com/autobrr/qui/internal/brush/store"
	"github.com/autobrr/qui/internal/models"
	"github.com/autobrr/qui/internal/testutil/testdb"
)

// The site transfer API sums today, yesterday or the last 7 days with today, and
// lists every site, most uploaded first, even one that moved nothing.
func TestSiteTransferRanges(t *testing.T) {
	ctx := t.Context()
	db := testdb.NewMigratedSQLite(t, t.Name())
	cipher, err := models.NewCredentialCipher(make([]byte, 32))
	require.NoError(t, err)
	svc, err := New(ctx, db, cipher, &fakeQB{})
	require.NoError(t, err)
	now := time.Date(2026, 11, 10, 6, 0, 0, 0, time.Local)
	svc.now = func() time.Time { return now }
	sky, err := svc.store.CreateSite(ctx, store.Site{Name: "sky", BaseURL: "https://hdsky.me"})
	require.NoError(t, err)
	zm, err := svc.store.CreateSite(ctx, store.Site{Name: "zm", BaseURL: "https://zmpt.cc"})
	require.NoError(t, err)
	_, err = svc.store.CreateSite(ctx, store.Site{Name: "idle", BaseURL: "https://idle.example"})
	require.NoError(t, err)
	for day, add := range map[string][]store.SiteTransfer{
		"2026-11-10": {{SiteID: sky, Uploaded: 10, Downloaded: 1}},
		"2026-11-09": {{SiteID: sky, Uploaded: 20, Downloaded: 2}, {SiteID: zm, Uploaded: 50, Downloaded: 5}},
		"2026-11-04": {{SiteID: zm, Uploaded: 100, Downloaded: 10}},
		"2026-11-03": {{SiteID: sky, Uploaded: 1000, Downloaded: 100}},
	} {
		require.NoError(t, svc.store.AddSiteTransfer(ctx, day, add))
	}

	r := chi.NewRouter()
	svc.Routes(r)
	get := func(q string) (int, siteTransferResponse) {
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/sites/transfer"+q, nil))
		var resp siteTransferResponse
		if rec.Code == http.StatusOK {
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		}
		return rec.Code, resp
	}
	ups := func(resp siteTransferResponse) map[string]int64 {
		out := map[string]int64{}
		for _, s := range resp.Sites {
			out[s.Name] = s.Uploaded
		}
		return out
	}

	code, today := get("?range=today")
	require.Equal(t, http.StatusOK, code)
	require.Equal(t, "2026-11-10", today.First)
	require.Equal(t, int64(6*3600), today.Seconds)
	require.Equal(t, map[string]int64{"sky": 10, "zm": 0, "idle": 0}, ups(today))
	require.Equal(t, "sky", today.Sites[0].Name)

	_, yday := get("?range=yesterday")
	require.Equal(t, "2026-11-09", yday.Last)
	require.Equal(t, int64(86400), yday.Seconds)
	require.Equal(t, "zm", yday.Sites[0].Name)
	require.Equal(t, int64(5), yday.Sites[0].Downloaded)

	_, week := get("?range=week")
	require.Equal(t, "2026-11-04", week.First)
	require.Equal(t, "2026-11-10", week.Last)
	require.Equal(t, int64(6*86400+6*3600), week.Seconds)
	require.Equal(t, map[string]int64{"sky": 30, "zm": 150, "idle": 0}, ups(week), "the 8th day back is left out")

	code, _ = get("?range=month")
	require.Equal(t, http.StatusBadRequest, code)
}
