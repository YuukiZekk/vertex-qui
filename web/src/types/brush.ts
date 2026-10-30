/*
 * Copyright (c) 2026, vertex-qui contributors.
 * SPDX-License-Identifier: GPL-2.0-or-later
 */

export interface BrushInstanceStatus {
  id: number
  name: string
  active: boolean
  torrents: number
  freeSpace: number
  left: number
  reserved: number
  committed: number
  floorBytes: number | null
  // Bytes per second; 0 when not set.
  uploadCapacity: number
  // Average upload over the last minute in bytes per second, null until measured.
  uploadRate: number | null
  error?: string
}

export interface BrushStatus {
  enabled: boolean
  instances: BrushInstanceStatus[]
}

export interface BrushSite {
  id: number
  name: string
  baseUrl: string
  hasCookie: boolean
  // Per-torrent upload limit in bytes per second, 0 for none.
  torrentUploadLimit: number
}

export interface BrushSiteInput {
  name: string
  baseUrl: string
  // Empty keeps the stored cookie on update.
  cookie: string
  torrentUploadLimit: number
}

export interface BrushFeed {
  id: number
  siteId: number
  name: string
  intervalSeconds: number
  minSizeBytes: number
  maxSizeBytes: number
  freeOnly: boolean
  skipHr: boolean
  windowSeconds: number
  // 0 lets brush pick the least busy instance for each torrent.
  instanceId: number
  enabled: boolean
  lastFetchAt: number
  lastError: string
}

export interface BrushFeedInput extends Omit<BrushFeed, "id" | "lastFetchAt" | "lastError"> {
  // Empty keeps the stored URL on update; the URL carries the passkey and is never returned.
  url: string
}

export interface BrushEntry {
  id: number
  feedId: number
  feedName: string
  title: string
  sizeBytes: number
  link: string
  publishedAt: number
  firstSeenAt: number
  status: string
  reason: string
  infoHash: string
  instanceId: number
  decidedAt: number
  placement?: BrushPlacement
}

// How each candidate instance scored for one torrent; chosen is 0 when none could take it.
export interface BrushPlacement {
  chosen: number
  candidates: BrushPlacementCandidate[]
}

export interface BrushPlacementCandidate {
  instanceId: number
  name: string
  reason?: string
  detail?: string
  committed: number
  floor: number
  upload: number
  uploadKnown: boolean
  fresh: number
  capacity: number
  spaceCost: number
  uploadCost: number
  cost: number
}

export interface BrushTransferToday {
  day: string
  // Seconds of the day that have passed, on the server's clock.
  elapsed: number
  instances: { id: number, name: string, uploaded: number, downloaded: number }[]
}

export interface BrushTestItem {
  title: string
  sizeBytes: number
  publishedAt: number
  ageSeconds: number
  inWindow: boolean
  sizeOk: boolean
  free: boolean | null
  hr: boolean | null
  error?: string
}
