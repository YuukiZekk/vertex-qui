/*
 * Copyright (c) 2026, vertex-qui contributors.
 * SPDX-License-Identifier: GPL-2.0-or-later
 */

import { formatBytes } from "@/lib/utils"
import type { BrushFlowInstance, BrushFlowPeer } from "@/types/brush"
import { geoNaturalEarth1, geoPath } from "d3-geo"
import { useMemo } from "react"
import { useTranslation } from "react-i18next"
import { feature } from "topojson-client"
import type { GeometryCollection, Topology } from "topojson-specification"
import land110 from "world-atlas/land-110m.json"
import { countryPoints } from "./countryPoints"
import { countryName, DOWNLOAD_COLOR, UPLOAD_COLOR } from "./flowStyle"

const WIDTH = 960
const HEIGHT = 470

const SERVER_COLOR = "#f59e0b"

const projection = geoNaturalEarth1().fitExtent([[8, 8], [WIDTH - 8, HEIGHT - 8]], { type: "Sphere" })
const path = geoPath(projection)
const topology = land110 as unknown as Topology<{ land: GeometryCollection }>
const landPath = path(feature(topology, topology.objects.land)) ?? ""
const spherePath = path({ type: "Sphere" }) ?? ""

function place(code: string): [number, number] | null {
  const lonLat = countryPoints[code]
  if (!lonLat) return null
  return projection(lonLat)
}

// arc bends a line between two points upward, by a quarter of its length.
function arc([x1, y1]: [number, number], [x2, y2]: [number, number]): string {
  const mx = (x1 + x2) / 2
  const my = (y1 + y2) / 2
  const dx = x2 - x1
  const dy = y2 - y1
  const bend = 0.25
  let nx = -dy * bend
  let ny = dx * bend
  if (ny > 0) {
    nx = -nx
    ny = -ny
  }
  return `M${x1},${y1} Q${mx + nx},${my + ny} ${x2},${y2}`
}

interface Flow {
  key: string
  from: [number, number]
  to: [number, number]
  country: string
  instance: string
  upload: number
  download: number
}

interface CountryDot {
  code: string
  at: [number, number]
  upload: number
  download: number
  connections: number
}

// FlowMap draws each server at its country and an arc to every country its peers
// are in now: green arcs carry uploads out, blue ones bring downloads in, and
// thicker arcs move more.
export function FlowMap({ instances, peers }: { instances: BrushFlowInstance[], peers: BrushFlowPeer[] }) {
  const { t, i18n } = useTranslation("brush")
  const names = useMemo(() => {
    try {
      return new Intl.DisplayNames([i18n.language], { type: "region" })
    } catch {
      return null
    }
  }, [i18n.language])

  const { servers, flows, dots } = useMemo(() => {
    // Servers in one country sit in a small circle around its point.
    const byCountry = new Map<string, BrushFlowInstance[]>()
    for (const inst of instances) {
      if (!place(inst.country)) continue
      byCountry.set(inst.country, [...(byCountry.get(inst.country) ?? []), inst])
    }
    const serverAt = new Map<number, [number, number]>()
    for (const [code, list] of byCountry) {
      const [x, y] = place(code)!
      list.forEach((inst, i) => {
        const angle = (2 * Math.PI * i) / list.length
        const r = list.length > 1 ? 7 : 0
        serverAt.set(inst.id, [x + r * Math.cos(angle), y + r * Math.sin(angle)])
      })
    }

    const flowMap = new Map<string, Flow>()
    const dotMap = new Map<string, CountryDot>()
    for (const p of peers) {
      const to = place(p.country)
      if (!to) continue
      const dot = dotMap.get(p.country) ?? { code: p.country, at: to, upload: 0, download: 0, connections: 0 }
      dot.upload += p.uploadSpeed
      dot.download += p.downloadSpeed
      dot.connections++
      dotMap.set(p.country, dot)
      const from = serverAt.get(p.instanceId)
      if (!from) continue
      const key = `${p.instanceId}:${p.country}`
      const flow = flowMap.get(key) ?? {
        key, from, to, country: p.country,
        instance: instances.find((i) => i.id === p.instanceId)?.name ?? "",
        upload: 0, download: 0,
      }
      flow.upload += p.uploadSpeed
      flow.download += p.downloadSpeed
      flowMap.set(key, flow)
    }
    return {
      servers: instances.flatMap((inst) => {
        const at = serverAt.get(inst.id)
        return at ? [{ inst, at }] : []
      }),
      flows: [...flowMap.values()],
      dots: [...dotMap.values()],
    }
  }, [instances, peers])

  const maxFlow = Math.max(1, ...flows.map((f) => Math.max(f.upload, f.download)))
  const maxDot = Math.max(1, ...dots.map((d) => d.upload + d.download))
  const width = (speed: number) => 0.8 + 3.2 * Math.sqrt(speed / maxFlow)
  const speed = (bytes: number) => `${formatBytes(bytes)}/s`

  return (
    <svg viewBox={`0 0 ${WIDTH} ${HEIGHT}`} className="h-auto w-full" role="img" aria-label={t("flows.mapLabel")}>
      <style>{`
        .flow-line { stroke-dasharray: 5 7; animation: flow-dash 1.4s linear infinite; }
        .flow-ping { animation: flow-ping 2.4s ease-out infinite; transform-box: fill-box; transform-origin: center; }
        @keyframes flow-dash { to { stroke-dashoffset: -24; } }
        @keyframes flow-ping { from { transform: scale(1); opacity: .6; } to { transform: scale(3.2); opacity: 0; } }
        @media (prefers-reduced-motion: reduce) { .flow-line, .flow-ping { animation: none; } }
      `}</style>
      <path d={spherePath} fill="var(--muted)" fillOpacity={0.25} />
      <path d={landPath} fill="var(--muted-foreground)" fillOpacity={0.18} stroke="var(--border)" strokeWidth={0.5} />

      {flows.map((f) => (
        <g key={f.key}>
          {f.download > 0 && (
            <path d={arc(f.to, f.from)} fill="none" stroke={DOWNLOAD_COLOR} strokeOpacity={0.75}
              strokeWidth={width(f.download)} strokeLinecap="round" className="flow-line">
              <title>{`${countryName(names, f.country)} → ${f.instance}: ${speed(f.download)}`}</title>
            </path>
          )}
          {f.upload > 0 && (
            <path d={arc(f.from, f.to)} fill="none" stroke={UPLOAD_COLOR} strokeOpacity={0.8}
              strokeWidth={width(f.upload)} strokeLinecap="round" className="flow-line">
              <title>{`${f.instance} → ${countryName(names, f.country)}: ${speed(f.upload)}`}</title>
            </path>
          )}
        </g>
      ))}

      {dots.map((d) => (
        <circle key={d.code} cx={d.at[0]} cy={d.at[1]} r={2.5 + 4 * Math.sqrt((d.upload + d.download) / maxDot)}
          fill={d.upload >= d.download ? UPLOAD_COLOR : DOWNLOAD_COLOR} fillOpacity={0.85} stroke="var(--background)" strokeWidth={1}>
          <title>{t("flows.countryTitle", {
            country: countryName(names, d.code), count: d.connections, upload: speed(d.upload), download: speed(d.download),
          })}</title>
        </circle>
      ))}

      {servers.map(({ inst, at }) => (
        <g key={inst.id}>
          <circle cx={at[0]} cy={at[1]} r={5} fill={SERVER_COLOR} className="flow-ping" />
          <circle cx={at[0]} cy={at[1]} r={5} fill={SERVER_COLOR} stroke="var(--background)" strokeWidth={1.5}>
            <title>{t("flows.serverTitle", { name: inst.name, upload: speed(inst.uploadSpeed), download: speed(inst.downloadSpeed) })}</title>
          </circle>
        </g>
      ))}
    </svg>
  )
}

