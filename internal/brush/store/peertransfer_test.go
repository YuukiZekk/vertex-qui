// Copyright (c) 2026, vertex-qui contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package store

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPeerTransferAddsUpAndPrunes(t *testing.T) {
	for name, open := range dialects() {
		t.Run(name, func(t *testing.T) {
			db := open(t)
			ctx := t.Context()
			s := newTestStore(t, db)

			require.NoError(t, s.AddPeerTransfer(ctx, "2026-09-01", []PeerTransfer{{SiteID: 1, InstanceID: 2, CountryCode: "de", Uploaded: 5}}))
			require.NoError(t, s.AddPeerTransfer(ctx, "2026-11-02", []PeerTransfer{
				{SiteID: 1, InstanceID: 2, CountryCode: "de", Uploaded: 10, Downloaded: 1},
				{SiteID: 1, InstanceID: 2, CountryCode: "", Uploaded: 3},
			}))
			require.NoError(t, s.AddPeerTransfer(ctx, "2026-11-03", []PeerTransfer{{SiteID: 1, InstanceID: 2, CountryCode: "de", Uploaded: 7}}))
			require.NoError(t, s.AddPeerTransfer(ctx, "2026-11-03", []PeerTransfer{{SiteID: 1, InstanceID: 2, CountryCode: "de", Uploaded: 1, Downloaded: 2}}))

			all, err := s.PeerTransfersSince(ctx, "2026-01-01")
			require.NoError(t, err)
			require.Equal(t, []PeerTransfer{
				{Day: "2026-11-02", SiteID: 1, InstanceID: 2, CountryCode: "", Uploaded: 3},
				{Day: "2026-11-02", SiteID: 1, InstanceID: 2, CountryCode: "de", Uploaded: 10, Downloaded: 1},
				{Day: "2026-11-03", SiteID: 1, InstanceID: 2, CountryCode: "de", Uploaded: 8, Downloaded: 2},
			}, all, "September is past the retention period")
		})
	}
}
