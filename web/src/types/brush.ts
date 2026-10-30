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
  // Brush puts each torrent on the least busy of these. Empty, only on feeds saved
  // before instances could be picked one by one, means every instance with a floor.
  instanceIds: number[]
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

export interface BrushTransferInstance {
  id: number
  name: string
  uploaded: number
  downloaded: number
  // Current speeds in bytes per second; null when the instance cannot be read now.
  uploadSpeed: number | null
  downloadSpeed: number | null
}

export interface BrushTorrentValue {
  instanceId: number
  hash: string
  name: string
  sizeBytes: number
  ageSeconds: number
  uploaded: number
  leechers: number
  // Average upload over the last 10 minutes in bytes per second, the predicted
  // upload over the horizon, and that per GiB; null until 10 minutes have been read.
  rate: number | null
  predicted: number | null
  decay: number | null
  density: number | null
}

export interface BrushPredictionCheck {
  id: number
  instanceId: number
  infoHash: string
  title: string
  madeAt: number
  ageSeconds: number
  sizeBytes: number
  rate: number
  predicted: number
  resolvedAt: number
  actual: number
}

export interface BrushValues {
  horizonSeconds: number
  torrents: BrushTorrentValue[]
  // Sums over the predictions checked in the last 24 hours.
  accuracy: { count: number, predicted: number, actual: number, absError: number, gone: number }
  recent: BrushPredictionCheck[]
}

export interface BrushTransferToday {
  day: string
  instances: BrushTransferInstance[]
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
