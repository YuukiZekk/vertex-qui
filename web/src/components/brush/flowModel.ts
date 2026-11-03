/*
 * Copyright (c) 2026, vertex-qui contributors.
 * SPDX-License-Identifier: GPL-2.0-or-later
 */

import type { BrushFlowCountry, BrushFlowInstance, BrushFlowPeer } from "@/types/brush"
import { countryPoints } from "./countryPoints"

// Arcs past this many, the slowest ones, are left out so the map stays readable.
export const MAX_ARCS = 40

export type LonLat = [number, number]

// A cluster is the servers of one country, drawn at one point with one label.
export interface FlowCluster {
  code: string
  at: LonLat
  servers: BrushFlowInstance[]
}

export interface FlowArc {
  key: string
  server: BrushFlowInstance
  // order is the server's place in the list, to fan out arcs from one country.
  order: number
  country: string
  serverAt: LonLat
  peerAt: LonLat
  direction: "up" | "down"
  speed: number
}

// A peer country: what moves now, and what it moved today, which fills its pool.
export interface FlowCountry {
  code: string
  at: LonLat
  upload: number
  download: number
  connections: number
  speed: number
  uploaded: number
  downloaded: number
  total: number
}

// buildFlows groups the connections into one arc per server, peer country and
// direction, fastest first, and sums each peer country now and today, over the
// given servers. Peers in the server's own country, and servers or peers with no
// known country, get no arc.
export function buildFlows(instances: BrushFlowInstance[], peers: BrushFlowPeer[], today: BrushFlowCountry[]) {
  const clusters = new Map<string, FlowCluster>()
  for (const inst of instances) {
    const at = countryPoints[inst.country]
    if (!at) continue
    const c = clusters.get(inst.country) ?? { code: inst.country, at, servers: [] }
    c.servers.push(inst)
    clusters.set(inst.country, c)
  }

  const order = new Map(instances.map((inst, i) => [inst.id, i]))
  const byServer = new Map(instances.map((inst) => [inst.id, inst]))
  const arcs = new Map<string, FlowArc>()
  const countries = new Map<string, FlowCountry>()
  for (const p of peers) {
    const peerAt = countryPoints[p.country]
    if (!peerAt) continue
    const c = countries.get(p.country) ?? emptyCountry(p.country, peerAt)
    c.upload += p.uploadSpeed
    c.download += p.downloadSpeed
    c.speed += p.uploadSpeed + p.downloadSpeed
    c.connections++
    countries.set(p.country, c)

    const server = byServer.get(p.instanceId)
    const serverAt = server && countryPoints[server.country]
    if (!server || !serverAt || server.country === p.country) continue
    for (const [direction, speed] of [["up", p.uploadSpeed], ["down", p.downloadSpeed]] as const) {
      if (speed <= 0) continue
      const key = `${server.id}:${p.country}:${direction}`
      const arc = arcs.get(key) ?? { key, server, order: order.get(server.id) ?? 0, country: p.country, serverAt, peerAt, direction, speed: 0 }
      arc.speed += speed
      arcs.set(key, arc)
    }
  }

  for (const d of today) {
    const at = countryPoints[d.country]
    if (!at || !byServer.has(d.instanceId)) continue
    const c = countries.get(d.country) ?? emptyCountry(d.country, at)
    c.uploaded += d.uploaded
    c.downloaded += d.downloaded
    c.total += d.uploaded + d.downloaded
    countries.set(d.country, c)
  }

  return {
    clusters: [...clusters.values()],
    arcs: [...arcs.values()].sort((a, b) => b.speed - a.speed).slice(0, MAX_ARCS),
    // Busiest today first, then busiest now.
    countries: [...countries.values()].sort((a, b) => b.total - a.total || b.speed - a.speed),
  }
}

function emptyCountry(code: string, at: LonLat): FlowCountry {
  return { code, at, upload: 0, download: 0, connections: 0, speed: 0, uploaded: 0, downloaded: 0, total: 0 }
}
