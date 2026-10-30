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
// upLimit is the torrent's upload limit in bytes per second; zero leaves it unlimited.
func (a *Adapter) AddTorrent(ctx context.Context, instanceID int, file []byte, tag string, upLimit int64) error {
	options := map[string]string{
		"tags":    tag,
		"paused":  "false",
		"stopped": "false",
	}
	if upLimit > 0 {
		options["upLimit"] = strconv.FormatInt(upLimit, 10)
	}
	_, err := a.sync.AddTorrent(ctx, instanceID, file, options)
	return err
}
