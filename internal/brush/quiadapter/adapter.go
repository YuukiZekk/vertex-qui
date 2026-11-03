// Copyright (c) 2026, vertex-qui contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

// Package quiadapter is the only brush package that imports qui internals, so an
// upstream API change is fixed here and nowhere else in internal/brush.
package quiadapter

import (
	"context"
	"strconv"

	qbt "github.com/autobrr/go-qbittorrent"

	"github.com/autobrr/qui/internal/fsops"
	"github.com/autobrr/qui/internal/models"
	"github.com/autobrr/qui/internal/qbittorrent"
	"github.com/autobrr/qui/internal/services/notifications"
)

type Instance struct {
	ID     int
	Name   string
	Active bool
}

type Adapter struct {
	instances *models.InstanceStore
	sync      *qbittorrent.SyncManager
	pool      *fsops.Pool
}

func New(instances *models.InstanceStore, sync *qbittorrent.SyncManager) *Adapter {
	return &Adapter{instances: instances, sync: sync}
}

func (a *Adapter) Instances(ctx context.Context) ([]Instance, error) {
	list, err := a.instances.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Instance, 0, len(list))
	for _, inst := range list {
		out = append(out, Instance{ID: inst.ID, Name: inst.Name, Active: inst.IsActive})
	}
	return out, nil
}

// Torrents reads qui's cached torrent list; the cache syncs with qBittorrent when it is
// more than 2 seconds old, so polling here does not add qBittorrent requests beyond that.
func (a *Adapter) Torrents(ctx context.Context, instanceID int) ([]qbt.Torrent, error) {
	return a.sync.GetAllTorrents(ctx, instanceID)
}

// Transfer is an instance's all-time counters and current speeds, in bytes and
// bytes per second.
type Transfer struct {
	AlltimeDL, AlltimeUL int64
	DLSpeed, ULSpeed     int64
}

// Transfer returns qBittorrent's all-time counters and current speeds from the
// server state qui cached at its last sync. ok is false before the first sync.
func (a *Adapter) Transfer(ctx context.Context, instanceID int) (Transfer, bool, error) {
	client, err := a.sync.GetClient(ctx, instanceID)
	if err != nil {
		return Transfer{}, false, err
	}
	state := client.GetCachedServerState()
	if state == nil {
		return Transfer{}, false, nil
	}
	return Transfer{
		AlltimeDL: state.AlltimeDl,
		AlltimeUL: state.AlltimeUl,
		DLSpeed:   state.DlInfoSpeed,
		ULSpeed:   state.UpInfoSpeed,
	}, true, nil
}

func (a *Adapter) FreeSpace(ctx context.Context, instanceID int) (int64, error) {
	return a.sync.GetFreeSpace(ctx, instanceID)
}

func (a *Adapter) HasTorrent(ctx context.Context, instanceID int, infoHash string) (bool, error) {
	_, found, err := a.sync.HasTorrentByAnyHash(ctx, instanceID, []string{infoHash})
	return found, err
}

// AddTorrent adds a started torrent with the given tag. Both pause flags are sent
// because qBittorrent renamed "paused" to "stopped" in WebAPI 2.11 and ignores the
// one it does not know, so the instance's "add stopped" default never applies.
// category is the qBittorrent category, empty for none. upLimit is the torrent's
// upload limit and dlLimit its download limit, in bytes per second; zero leaves
// them unlimited.
func (a *Adapter) AddTorrent(ctx context.Context, instanceID int, file []byte, tag, category string, upLimit, dlLimit int64) error {
	options := map[string]string{
		"tags":    tag,
		"paused":  "false",
		"stopped": "false",
	}
	if category != "" {
		options["category"] = category
	}
	if upLimit > 0 {
		options["upLimit"] = strconv.FormatInt(upLimit, 10)
	}
	if dlLimit > 0 {
		options["dlLimit"] = strconv.FormatInt(dlLimit, 10)
	}
	_, err := a.sync.AddTorrent(ctx, instanceID, file, options)
	return err
}

// Reannounce asks the torrents' trackers to take their latest upload now.
func (a *Adapter) Reannounce(ctx context.Context, instanceID int, hashes []string) error {
	return a.sync.BulkAction(ctx, instanceID, hashes, "reannounce")
}

// DeleteTorrent removes a torrent, with its files or keeping them.
func (a *Adapter) DeleteTorrent(ctx context.Context, instanceID int, hash string, withFiles bool) error {
	action := "delete"
	if withFiles {
		action = "deleteWithFiles"
	}
	return a.sync.BulkAction(ctx, instanceID, []string{hash}, action)
}

// Pause stops the torrents, so a download stops writing to the disk at once.
func (a *Adapter) Pause(ctx context.Context, instanceID int, hashes []string) error {
	return a.sync.BulkAction(ctx, instanceID, hashes, "pause")
}

// Resume starts torrents Pause stopped.
func (a *Adapter) Resume(ctx context.Context, instanceID int, hashes []string) error {
	return a.sync.BulkAction(ctx, instanceID, hashes, "resume")
}

// Notifier sends brush alerts through qui's notification targets, as the
// brush_alert event.
type Notifier struct {
	service *notifications.Service
}

func NewNotifier(service *notifications.Service) *Notifier {
	return &Notifier{service: service}
}

// Alert sends one alert; instanceID is zero when it is not about an instance.
func (n *Notifier) Alert(ctx context.Context, instanceID int, title, message string) {
	if n == nil || n.service == nil {
		return
	}
	n.service.Notify(ctx, notifications.Event{Type: notifications.EventBrushAlert, Title: title, Message: message, InstanceID: instanceID})
}

// WithFilesystem lets the adapter read disk sizes through qui's filesystem
// backends, for instances that have filesystem access.
func (a *Adapter) WithFilesystem(pool *fsops.Pool) *Adapter {
	a.pool = pool
	return a
}

// DiskTotal is the size of the disk holding path on the instance, false when the
// instance has no filesystem access in qui or the disk cannot be read.
func (a *Adapter) DiskTotal(ctx context.Context, instanceID int, path string) (int64, bool) {
	if a.pool == nil || path == "" {
		return 0, false
	}
	backend, _, err := a.pool.Require(ctx, instanceID, models.CapabilityRead)
	if err != nil {
		return 0, false
	}
	result, err := backend.Statfs(ctx, path)
	if err != nil || result == nil || result.BytesTotal <= 0 {
		return 0, false
	}
	return result.BytesTotal, true
}

// Peers lists a torrent's connected peers, keyed by address. qBittorrent names
// each peer's country when its "resolve peer countries" option is on.
func (a *Adapter) Peers(ctx context.Context, instanceID int, hash string) (map[string]qbt.TorrentPeer, error) {
	resp, err := a.sync.GetTorrentPeers(ctx, instanceID, hash)
	if err != nil || resp == nil {
		return nil, err
	}
	return resp.Peers, nil
}
