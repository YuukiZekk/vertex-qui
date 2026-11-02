// Copyright (c) 2026, vertex-qui contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package brush

import (
	"context"
	"sync"
	"time"
)

// alertEvery bounds how often one problem is sent again while it lasts.
const alertEvery = 30 * time.Minute

// notifier sends an alert to the user; quiadapter.Notifier sends it through qui's
// notification targets.
type notifier interface {
	Alert(ctx context.Context, instanceID int, title, message string)
}

type alerter struct {
	mu   sync.Mutex
	sent map[string]time.Time
	to   notifier
}

// SetNotifier gives brush somewhere to send alerts; without one they are only logged.
func (s *Service) SetNotifier(n notifier) {
	s.alerts.mu.Lock()
	defer s.alerts.mu.Unlock()
	s.alerts.to = n
}

// alert sends a problem at most once per alertEvery for the same key, so a feed
// failing every minute raises one notification, not sixty an hour.
func (s *Service) alert(ctx context.Context, key string, instanceID int, title, message string) {
	now := s.now()
	s.alerts.mu.Lock()
	if last, ok := s.alerts.sent[key]; ok && now.Sub(last) < alertEvery {
		s.alerts.mu.Unlock()
		return
	}
	s.alerts.sent[key] = now
	to := s.alerts.to
	s.alerts.mu.Unlock()
	s.log.Warn().Str("alert", key).Int("instance", instanceID).Str("title", title).Msg(message)
	if to != nil {
		to.Alert(ctx, instanceID, title, message)
	}
}
