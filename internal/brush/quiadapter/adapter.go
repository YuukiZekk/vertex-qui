// Copyright (c) 2026, vertex-qui contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

// Package quiadapter is the only brush package that imports qui internals, so an
// upstream API change is fixed here and nowhere else in internal/brush.
package quiadapter

import (
	"context"
	"strconv"

	qbt "github.com/autobrr/go-qbittorrent"

	"github.com/autobrr/qui/internal/models"
	"github.com/autobrr/qui/internal/qbittorrent"
)

type Instance struct {
	ID     int
	Name   string
	Active bool
}

type Adapter struct {
	instances *models.InstanceStore
	sync      *qbittorrent.SyncManager
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
