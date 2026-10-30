// Copyright (c) 2026, vertex-qui contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package site

import (
	"bytes"
	"fmt"

	"github.com/autobrr/go-torrent/metainfo"
)

type TorrentFile struct {
	InfoHash string
	Size     int64
}

func ParseTorrent(data []byte) (TorrentFile, error) {
	mi, err := metainfo.Load(bytes.NewReader(data))
	if err != nil {
		return TorrentFile{}, fmt.Errorf("parse torrent: %w", err)
	}
	info, err := mi.UnmarshalInfo()
	if err != nil {
		return TorrentFile{}, fmt.Errorf("parse torrent info: %w", err)
	}
	return TorrentFile{InfoHash: mi.HashInfoBytes().HexString(), Size: info.TotalLength()}, nil
}
