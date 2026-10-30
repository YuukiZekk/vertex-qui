// Copyright (c) 2026, vertex-qui contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package brush

import (
	"bytes"
	"context"
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	qbt "github.com/autobrr/go-qbittorrent"
	"github.com/autobrr/go-torrent/bencode"
	"github.com/autobrr/go-torrent/metainfo"
	"github.com/stretchr/testify/require"

	"github.com/autobrr/qui/internal/brush/quiadapter"
	"github.com/autobrr/qui/internal/brush/store"
	"github.com/autobrr/qui/internal/models"
	"github.com/autobrr/qui/internal/testutil/testdb"
)

const gib = int64(1) << 30

// fakeQB serves one active instance per entry in free, named vps-<id>.
type fakeQB struct {
	mu    sync.Mutex
	free  map[int]int64
	added map[int]int
}

func (f *fakeQB) Instances(context.Context) ([]quiadapter.Instance, error) {
	out := make([]quiadapter.Instance, 0, len(f.free))
	for id := range f.free {
		out = append(out, quiadapter.Instance{ID: id, Name: fmt.Sprintf("vps-%d", id), Active: true})
	}
	return out, nil
}

// Torrents never lists what brush added, so admission has to rely on its reservations.
func (f *fakeQB) Torrents(context.Context, int) ([]qbt.Torrent, error) { return nil, nil }

func (f *fakeQB) FreeSpace(_ context.Context, id int) (int64, error) { return f.free[id], nil }

func (f *fakeQB) HasTorrent(context.Context, int, string) (bool, error) { return false, nil }

func (f *fakeQB) AddTorrent(_ context.Context, id int, _ []byte, tag string) error {
	if tag != Tag {
		return fmt.Errorf("unexpected tag %q", tag)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.added == nil {
		f.added = make(map[int]int)
	}
	f.added[id]++
	return nil
}

func (f *fakeQB) addCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, c := range f.added {
		n += c
	}
	return n
}

func (f *fakeQB) addedTo() map[int]int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return maps.Clone(f.added)
}

func torrentBytes(t *testing.T, name string, size int64) []byte {
	t.Helper()
	info := metainfo.Info{Name: name, PieceLength: 1 << 24, Length: size, Pieces: make([]byte, 20)}
	infoBytes, err := bencode.Marshal(info)
	require.NoError(t, err)
	var buf bytes.Buffer
	require.NoError(t, (&metainfo.MetaInfo{InfoBytes: infoBytes}).Write(&buf))
	return buf.Bytes()
}

type fakeTorrent struct {
	id        int
	published time.Time
	size      int64
	free      bool
}

// fakeSite serves a NexusPHP-shaped RSS feed, details pages and torrent files.
type fakeSite struct {
	mu       sync.Mutex
	torrents []fakeTorrent
	files    map[int][]byte
}

func (s *fakeSite) set(t *testing.T, torrents ...fakeTorrent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.torrents = torrents
	for _, ft := range torrents {
		s.files[ft.id] = torrentBytes(t, fmt.Sprintf("t%d", ft.id), ft.size)
	}
}

func (s *fakeSite) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	host := "http://" + r.Host
	switch r.URL.Path {
	case "/rss.php":
		var items strings.Builder
		for _, ft := range s.torrents {
			fmt.Fprintf(&items, `<item><title>t%d</title><link>%s/details.php?id=%d</link><guid>g%d</guid><pubDate>%s</pubDate>`+
				`<enclosure url="%s/download.php?id=%d&amp;passkey=k" length="%d"/></item>`,
				ft.id, host, ft.id, ft.id, ft.published.Format(time.RFC1123Z), host, ft.id, ft.size)
		}
		fmt.Fprintf(w, `<rss version="2.0"><channel>%s</channel></rss>`, items.String())
	case "/details.php":
		for _, ft := range s.torrents {
			if r.URL.Query().Get("id") == fmt.Sprint(ft.id) {
				promo := ""
				if ft.free {
					promo = `<font class="free">Free</font>`
				}
				fmt.Fprintf(w, `<a href="userdetails.php?id=1">me</a><h1 id="top">t%d %s</h1>`, ft.id, promo)
				return
			}
		}
		http.NotFound(w, r)
	case "/download.php":
		var id int
		fmt.Sscan(r.URL.Query().Get("id"), &id)
		_, _ = w.Write(s.files[id])
	default:
		http.NotFound(w, r)
	}
}

// TestPipelineRacesOnlyFreshFreeTorrentsWithinCommittedSpace covers the M1 guarantees:
// the first poll is a baseline, stale and non-free torrents are refused, and two
// torrents that arrive together are not both admitted against the same free space.
func TestPipelineRacesOnlyFreshFreeTorrentsWithinCommittedSpace(t *testing.T) {
	ctx := t.Context()
	db := testdb.NewMigratedSQLite(t, t.Name())
	cipher, err := models.NewCredentialCipher(make([]byte, 32))
	require.NoError(t, err)
	qb := &fakeQB{free: map[int]int64{1: 100 * gib}}
	svc, err := New(ctx, db, cipher, qb)
	require.NoError(t, err)

	fs := &fakeSite{files: map[int][]byte{}}
	srv := httptest.NewServer(fs)
	t.Cleanup(srv.Close)

	siteID, err := svc.store.CreateSite(ctx, store.Site{Name: "pt", BaseURL: srv.URL, Cookie: "c=1"})
	require.NoError(t, err)
	feedID, err := svc.store.CreateFeed(ctx, store.Feed{SiteID: siteID, Name: "all", URL: srv.URL + "/rss.php",
		IntervalSeconds: 15, MinSizeBytes: 5 * gib, MaxSizeBytes: 50 * gib, FreeOnly: true, WindowSeconds: 300,
		InstanceID: 1, Enabled: true})
	require.NoError(t, err)
	require.NoError(t, svc.store.SetFloor(ctx, 1, 50*gib))

	now := time.Now()
	fs.set(t, fakeTorrent{id: 1, published: now.Add(-time.Minute), size: 10 * gib, free: true})
	feed, err := svc.store.GetFeed(ctx, feedID)
	require.NoError(t, err)
	svc.poll(ctx, feed)
	require.Zero(t, qb.addCount(), "the first poll only records a baseline")

	fs.set(t,
		fakeTorrent{id: 1, published: now.Add(-time.Minute), size: 10 * gib, free: true},
		fakeTorrent{id: 2, published: now.Add(-30 * time.Second), size: 40 * gib, free: true},
		fakeTorrent{id: 3, published: now.Add(-20 * time.Second), size: 40 * gib, free: true},
		fakeTorrent{id: 4, published: now.Add(-10 * time.Second), size: 20 * gib, free: false},
		fakeTorrent{id: 5, published: now.Add(-10 * time.Minute), size: 20 * gib, free: true},
		fakeTorrent{id: 6, published: now.Add(-5 * time.Second), size: 60 * gib, free: true},
	)
	svc.poll(ctx, feed)

	var entries []store.Entry
	require.Eventually(t, func() bool {
		entries, err = svc.store.RecentEntries(ctx, 20)
		require.NoError(t, err)
		for _, e := range entries {
			if e.Status == store.StatusPending || e.Status == store.StatusAdding {
				return false
			}
		}
		return len(entries) == 5
	}, 10*time.Second, 50*time.Millisecond)

	byTitle := map[string]store.Entry{}
	for _, e := range entries {
		byTitle[e.Title] = e
	}
	require.Equal(t, store.StatusRejected, byTitle["t4"].Status)
	require.Equal(t, reasonNotFree, byTitle["t4"].Reason)
	require.Equal(t, store.StatusRejected, byTitle["t5"].Status)
	require.True(t, strings.HasPrefix(byTitle["t5"].Reason, reasonWindow))
	require.Equal(t, reasonSize, byTitle["t6"].Reason)

	// 100 GiB free, 50 GiB floor: one 40 GiB torrent fits, the second would leave 20 GiB.
	statuses := []string{byTitle["t2"].Status, byTitle["t3"].Status}
	require.ElementsMatch(t, []string{store.StatusAdded, store.StatusRejected}, statuses)
	for _, title := range []string{"t2", "t3"} {
		if byTitle[title].Status == store.StatusRejected {
			require.True(t, strings.HasPrefix(byTitle[title].Reason, reasonLowSpace), byTitle[title].Reason)
		}
	}
	require.Equal(t, 1, qb.addCount())
}

func TestCommittedSpaceCountsReservationsUntilListed(t *testing.T) {
	torrents := []qbt.Torrent{{Hash: "listed", AmountLeft: 30 * gib}, {Hash: "done", AmountLeft: 0}}
	reservations := []store.Reservation{{InfoHash: "listed", SizeBytes: 40 * gib}, {InfoHash: "pending", SizeBytes: 10 * gib}}

	got := committedSpace(200*gib, torrents, reservations)
	require.Equal(t, spaceReport{Torrents: 2, Free: 200 * gib, Left: 30 * gib, Reserved: 10 * gib, Committed: 160 * gib}, got)
}

func TestRaceAge(t *testing.T) {
	now := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	firstSeen := now.Add(-20 * time.Second)
	require.Equal(t, 3*time.Minute, raceAge(now.Add(-3*time.Minute), firstSeen, now))
	require.Equal(t, 20*time.Second, raceAge(time.Time{}, firstSeen, now), "no pubDate falls back to first seen")
	require.Equal(t, 20*time.Second, raceAge(now.Add(time.Hour), firstSeen, now), "a pubDate far in the future is not trusted")
	require.Equal(t, time.Duration(0), raceAge(now.Add(time.Minute), firstSeen, now), "small clock skew counts as brand new")
}
