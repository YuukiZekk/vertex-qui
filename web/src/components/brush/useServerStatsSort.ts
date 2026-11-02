/*
 * Copyright (c) 2026, vertex-qui contributors.
 * SPDX-License-Identifier: GPL-2.0-or-later
 */

import type { ServerState } from "@/types"
import { useState } from "react"
import type { TodayTransfer } from "./useTodayTransfer"

export type ServerSortKey =
  | "name" | "downloaded" | "downloadedSession" | "uploaded" | "uploadedSession" | "ratio" | "peers"
  | "todayUploaded" | "todayDownloaded" | "uploadSpeed" | "averageUpload"

export type ServerSort = { key: ServerSortKey, desc: boolean }

const SORT_KEY = "vertex.serverStats.sort"

function readSort(): ServerSort | null {
  try {
    const raw = localStorage.getItem(SORT_KEY)
    return raw ? JSON.parse(raw) as ServerSort : null
  } catch {
    return null
  }
}

// useServerStatsSort keeps the server statistics' sort column per browser, like the
// tracker breakdown keeps its own. With none chosen the rows keep qui's order.
export function useServerStatsSort(): [ServerSort | null, (key: ServerSortKey) => void] {
  const [sort, setSort] = useState<ServerSort | null>(readSort)
  const toggle = (key: ServerSortKey) => setSort((prev) => {
    const next = prev?.key === key ? { key, desc: !prev.desc } : { key, desc: key !== "name" }
    try {
      localStorage.setItem(SORT_KEY, JSON.stringify(next))
    } catch {
      // Storage can be unavailable; the order still applies for this visit.
    }
    return next
  })
  return [sort, toggle]
}

type Row = { instance: { id: number, name: string }, serverState: ServerState | null }

function valueOf(row: Row, key: ServerSortKey, today: TodayTransfer): number | null {
  const s = row.serverState
  const t = today?.rows.get(row.instance.id)
  switch (key) {
    case "downloaded": return s?.alltime_dl ?? 0
    case "downloadedSession": return s?.dl_info_data ?? 0
    case "uploaded": return s?.alltime_ul ?? 0
    case "uploadedSession": return s?.up_info_data ?? 0
    case "ratio": return s?.alltime_dl ? (s.alltime_ul ?? 0) / s.alltime_dl : 0
    case "peers": return s?.total_peer_connections ?? 0
    case "todayUploaded": return t?.uploaded ?? null
    case "todayDownloaded": return t?.downloaded ?? null
    case "uploadSpeed": return t?.uploadSpeed ?? null
    case "averageUpload": return t?.averageUpload ?? null
    default: return null
  }
}

// sortServerRows orders the server statistics rows; an instance without a value
// for the column goes last either way.
export function sortServerRows<T extends Row>(rows: T[], sort: ServerSort | null, today: TodayTransfer): T[] {
  if (!sort) return rows
  return [...rows].sort((a, b) => {
    if (sort.key === "name") {
      const order = a.instance.name.localeCompare(b.instance.name)
      return sort.desc ? -order : order
    }
    const x = valueOf(a, sort.key, today)
    const y = valueOf(b, sort.key, today)
    if (x === null || y === null) return x === y ? 0 : x === null ? 1 : -1
    return sort.desc ? y - x : x - y
  })
}
