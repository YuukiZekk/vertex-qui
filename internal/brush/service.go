// Copyright (c) 2026, vertex-qui contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

// Package brush is the vertex-qui brushing module: RSS racing, VPS placement and
// value-based deletion. It reaches qui only through quiadapter.
package brush

import (
	"context"
	"sync"
	"time"

	qbt "github.com/autobrr/go-qbittorrent"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/autobrr/qui/internal/brush/quiadapter"
	"github.com/autobrr/qui/internal/brush/site"
	"github.com/autobrr/qui/internal/brush/store"
	"github.com/autobrr/qui/internal/dbinterface"
	"github.com/autobrr/qui/internal/models"
)

// qbittorrent is what brush needs from qui; quiadapter.Adapter implements it.
type qbittorrent interface {
	Instances(ctx context.Context) ([]quiadapter.Instance, error)
	Torrents(ctx context.Context, instanceID int) ([]qbt.Torrent, error)
	FreeSpace(ctx context.Context, instanceID int) (int64, error)
	HasTorrent(ctx context.Context, instanceID int, infoHash string) (bool, error)
	AddTorrent(ctx context.Context, instanceID int, file []byte, tag string) error
}

type siteClientEntry struct {
	cookie string
	client *site.NexusPHP
}

type Service struct {
	store *store.Store
	qui   qbittorrent
	log   zerolog.Logger
	now   func() time.Time

	// admitMu serialises admission across all feeds and instances.
	admitMu sync.Mutex

	mu        sync.Mutex
	baselined map[int64]bool
	polling   map[int64]bool
	clients   map[int64]siteClientEntry
}

func New(ctx context.Context, db dbinterface.Querier, cipher *models.CredentialCipher, qui qbittorrent) (*Service, error) {
	if err := store.Migrate(ctx, db); err != nil {
		return nil, err
	}
	return &Service{
		store:     store.New(db, cipher),
		qui:       qui,
		log:       log.With().Str("module", "brush").Logger(),
		now:       time.Now,
		baselined: make(map[int64]bool),
		polling:   make(map[int64]bool),
		clients:   make(map[int64]siteClientEntry),
	}, nil
}

// Start resolves adds a previous run left in flight and starts polling feeds.
func (s *Service) Start(ctx context.Context) error {
	if err := s.store.MarkInterruptedAdds(ctx); err != nil {
		return err
	}
	go s.run(ctx)
	return nil
}
