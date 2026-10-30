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
}

export interface BrushSiteInput {
  name: string
  baseUrl: string
  // Empty keeps the stored cookie on update.
  cookie: string
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
