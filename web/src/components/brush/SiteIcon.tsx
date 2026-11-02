/*
 * Copyright (c) 2026, vertex-qui contributors.
 * SPDX-License-Identifier: GPL-2.0-or-later
 */

import { TrackerIconImage } from "@/components/ui/tracker-icon"
import { useTrackerIcons } from "@/hooks/useTrackerIcons"
import { pickTrackerIconDomain } from "@/lib/tracker-icons"
import type { BrushSite } from "@/types/brush"

// SiteIcon shows the icon qui keeps for one of the site's tracker hosts, trying the
// site's own address first, and the site's first letter while there is none.
export function SiteIcon({ site }: { site: Pick<BrushSite, "baseUrl" | "trackerHosts"> }) {
  const { data: trackerIcons } = useTrackerIcons()
  return <TrackerIconImage tracker={pickTrackerIconDomain(trackerIcons, [site.baseUrl, ...site.trackerHosts])} trackerIcons={trackerIcons} />
}
