// Copyright (c) 2026, vertex-qui contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package brush

import (
	"context"
	"errors"
	"sort"
	"time"

	"github.com/autobrr/qui/internal/brush/site"
	"github.com/autobrr/qui/internal/brush/store"
)

// Tag marks every torrent brush adds, so deletion can stay inside brush's own torrents.
const Tag = "vertex-qui"

// clockSkew is how far a pubDate may sit in the future before it counts as wrong.
const clockSkew = 5 * time.Minute

// Rejection reasons stored on an entry. The UI translates the code; the detail after
// ": " is shown as is.
const (
	reasonWindow     = "window"
	reasonSize       = "size"
	reasonNotFree    = "not_free"
	reasonHR         = "hr"
	reasonInspect    = "inspect_failed"
	reasonDownload   = "download_failed"
	reasonTorrent    = "invalid_torrent"
	reasonNoFloor    = "no_floor"
	reasonLowSpace   = "low_space"
	reasonInstance   = "instance_unavailable"
	reasonDuplicate  = "duplicate"
	reasonSpaceCheck = "space_check_failed"
	reasonAdd        = "add_failed"
	reasonSite       = "site_unavailable"
)

func (s *Service) run(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.tick(ctx)
		}
	}
}

// tick starts a poll for every enabled feed whose interval has passed. While the
// master switch is off, nothing polls and every feed starts over with a baseline.
func (s *Service) tick(ctx context.Context) {
	enabled, err := s.store.Enabled(ctx)
	if err != nil {
		s.log.Error().Err(err).Msg("read master switch")
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !enabled {
		clear(s.baselined)
		return
	}

	feeds, err := s.store.ListFeeds(ctx)
	if err != nil {
		s.log.Error().Err(err).Msg("list feeds")
		return
	}
	now := s.now()
	for _, f := range feeds {
		if !f.Enabled {
			delete(s.baselined, f.ID)
			continue
		}
		if s.polling[f.ID] || now.Sub(time.Unix(f.LastFetchAt, 0)) < time.Duration(f.IntervalSeconds)*time.Second {
			continue
		}
		s.polling[f.ID] = true
		go func(f store.Feed) {
			defer func() {
				s.mu.Lock()
				delete(s.polling, f.ID)
				s.mu.Unlock()
			}()
			s.poll(ctx, f)
		}(f)
	}
}

// poll fetches one feed and starts the pipeline for each item it has not seen. The
// first poll after start or after the feed was switched on only records what the
// feed lists, so a restart never adds the backlog.
func (s *Service) poll(ctx context.Context, f store.Feed) {
	record := func(etag, lastModified, fetchErr string) {
		if err := s.store.RecordFetch(ctx, f.ID, etag, lastModified, fetchErr); err != nil {
			s.log.Error().Err(err).Int64("feed", f.ID).Msg("record fetch")
		}
	}

	client, err := s.siteClient(ctx, f.SiteID)
	if err != nil {
		record(f.ETag, f.LastModified, err.Error())
		return
	}
	body, etag, lastModified, notModified, err := client.FetchFeed(ctx, f.URL, f.ETag, f.LastModified)
	if err != nil {
		record(f.ETag, f.LastModified, err.Error())
		return
	}
	if notModified {
		s.markBaselined(f.ID)
		record(etag, lastModified, "")
		return
	}
	items, err := site.ParseFeed(body)
	if err != nil {
		record(f.ETag, f.LastModified, err.Error())
		return
	}

	// Newest first: when several items arrive in one poll, the youngest one has the
	// most race time left and the most to gain.
	sort.SliceStable(items, func(i, j int) bool { return items[i].Published.After(items[j].Published) })

	baseline := !s.isBaselined(f.ID)
	now := s.now()
	for _, item := range items {
		status := store.StatusPending
		if baseline {
			status = store.StatusBaseline
		}
		id, inserted, err := s.store.InsertEntry(ctx, store.Entry{
			FeedID: f.ID, GUID: item.GUID, Title: item.Title, SizeBytes: item.Size, Link: item.Link,
			PublishedAt: unixOrZero(item.Published), FirstSeenAt: now.Unix(), Status: status,
		})
		if err != nil {
			s.log.Error().Err(err).Int64("feed", f.ID).Msg("insert entry")
			continue
		}
		if inserted && !baseline {
			go s.process(ctx, f, client, item, id, now)
		}
	}
	s.markBaselined(f.ID)
	record(etag, lastModified, "")
}

// raceAge is how long ago the torrent was published. A missing pubDate, or one too
// far in the future to trust, falls back to when brush first saw the item.
func raceAge(published, firstSeen, now time.Time) time.Duration {
	if !published.IsZero() && published.Before(now.Add(clockSkew)) {
		return max(now.Sub(published), 0)
	}
	return max(now.Sub(firstSeen), 0)
}

// process runs one new item through the pipeline. Every site request runs under a
// deadline at the end of the race window, so a slow site cannot add a stale torrent.
func (s *Service) process(ctx context.Context, f store.Feed, client *site.NexusPHP, item site.Item, entryID int64, firstSeen time.Time) {
	reject := func(code, detail string) {
		reason := code
		if detail != "" {
			reason += ": " + detail
		}
		if err := s.store.Reject(ctx, entryID, reason); err != nil {
			s.log.Error().Err(err).Int64("entry", entryID).Msg("reject entry")
		}
	}

	window := time.Duration(f.WindowSeconds) * time.Second
	age := raceAge(item.Published, firstSeen, s.now())
	if age > window {
		reject(reasonWindow, age.Truncate(time.Second).String())
		return
	}
	if item.Size > 0 && !sizeAllowed(f, item.Size) {
		reject(reasonSize, "")
		return
	}

	raceCtx, cancel := context.WithTimeout(ctx, window-age)
	defer cancel()
	stepFailed := func(code string, err error) {
		if errors.Is(err, context.DeadlineExceeded) {
			reject(reasonWindow, "timed out")
			return
		}
		reject(code, err.Error())
	}

	if f.FreeOnly || f.SkipHR {
		insp, err := client.Inspect(raceCtx, item.Link)
		if err != nil {
			stepFailed(reasonInspect, err)
			return
		}
		if f.FreeOnly && !insp.Free {
			reject(reasonNotFree, "")
			return
		}
		if f.SkipHR && insp.HR {
			reject(reasonHR, "")
			return
		}
	}

	data, err := client.Download(raceCtx, item.DownloadURL)
	if err != nil {
		stepFailed(reasonDownload, err)
		return
	}
	tf, err := site.ParseTorrent(data)
	if err != nil {
		reject(reasonTorrent, err.Error())
		return
	}
	if !sizeAllowed(f, tf.Size) {
		reject(reasonSize, "")
		return
	}

	// The site's per-torrent limit goes on the torrent itself, so a fast instance never
	// uploads it faster than the site allows. Without it the torrent is not added.
	st, err := s.store.GetSite(ctx, f.SiteID)
	if err != nil {
		reject(reasonSite, err.Error())
		return
	}
	instanceID, code, detail := s.admit(raceCtx, f, entryID, tf)
	if code != "" {
		reject(code, detail)
		return
	}
	if err := s.qui.AddTorrent(raceCtx, instanceID, data, Tag, st.TorrentUploadLimit); err != nil {
		if failErr := s.store.Fail(ctx, entryID, reasonAdd+": "+err.Error()); failErr != nil {
			s.log.Error().Err(failErr).Int64("entry", entryID).Msg("fail entry")
		}
		return
	}
	if err := s.store.MarkAdded(ctx, entryID); err != nil {
		s.log.Error().Err(err).Int64("entry", entryID).Msg("mark entry added")
	}
	s.log.Info().Int64("feed", f.ID).Int("instance", instanceID).Str("hash", tf.InfoHash).Msg("added torrent")
}

func sizeAllowed(f store.Feed, size int64) bool {
	return size >= f.MinSizeBytes && (f.MaxSizeBytes == 0 || size <= f.MaxSizeBytes)
}

func (s *Service) isBaselined(feedID int64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.baselined[feedID]
}

func (s *Service) markBaselined(feedID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.baselined[feedID] = true
}

// siteClient keeps one client per site, so the request spacing holds across feeds of
// the same site. A changed cookie gets a new client.
func (s *Service) siteClient(ctx context.Context, siteID int64) (*site.NexusPHP, error) {
	st, err := s.store.GetSite(ctx, siteID)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if c, ok := s.clients[siteID]; ok && c.cookie == st.Cookie {
		return c.client, nil
	}
	c := siteClientEntry{cookie: st.Cookie, client: site.NewNexusPHP(st.Cookie)}
	s.clients[siteID] = c
	return c.client, nil
}

func unixOrZero(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.Unix()
}
