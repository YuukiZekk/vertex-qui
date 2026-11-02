// Copyright (c) 2026, vertex-qui contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package brush

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type fakeNotifier struct{ titles []string }

func (f *fakeNotifier) Alert(_ context.Context, _ int, title, _ string) {
	f.titles = append(f.titles, title)
}

// One problem is sent once per alertEvery while it lasts; another is sent at once.
func TestAlertThrottles(t *testing.T) {
	now := time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)
	svc := &Service{alerts: &alerter{sent: make(map[string]time.Time)}, now: func() time.Time { return now }}
	n := &fakeNotifier{}
	svc.SetNotifier(n)
	ctx := t.Context()

	svc.alert(ctx, "feed:1", 0, "a", "")
	svc.alert(ctx, "feed:1", 0, "a", "")
	svc.alert(ctx, "feed:2", 0, "b", "")
	now = now.Add(alertEvery)
	svc.alert(ctx, "feed:1", 0, "a", "")
	require.Equal(t, []string{"a", "b", "a"}, n.titles)
}
