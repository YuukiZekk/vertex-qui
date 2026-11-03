/*
 * Copyright (c) 2026, vertex-qui contributors.
 * SPDX-License-Identifier: GPL-2.0-or-later
 */

import { Button } from "@/components/ui/button"
import { formatBytes } from "@/lib/utils"
import type { BrushFlowInstance, BrushFlowPeer } from "@/types/brush"
import { geoGraticule10, geoMercator, geoPath } from "d3-geo"
import { Minus, Plus } from "lucide-react"
import { useMemo, useRef, useState, type PointerEvent, type ReactNode } from "react"
import { useTranslation } from "react-i18next"
import { feature, mesh } from "topojson-client"
import type { GeometryCollection, Topology } from "topojson-specification"
import countries110 from "world-atlas/countries-110m.json"
import { countryPoints } from "./countryPoints"
import { countryName, DOWNLOAD_COLOR, UPLOAD_COLOR, type FlowColorBy } from "./flowStyle"

const WIDTH = 960
const HEIGHT = 540
// Arcs past this many, the slowest ones, are left out so the map stays readable.
const MAX_ARCS = 40
const LABELLED_COUNTRIES = 12
const MAX_ZOOM = 6

const MAP_BG = "#0b1520"
const LAND = "#172636"
const BORDER = "#24394d"
const GRID = "#102030"
const LABEL = "#9fb3c8"

// Mercator from Patagonia to the Arctic islands, as the reference map frames it.
const projection = geoMercator().fitExtent([[12, 12], [WIDTH - 12, HEIGHT - 12]], { type: "MultiPoint", coordinates: [[-180, -56], [180, 78]] })
const path = geoPath(projection)
const topology = countries110 as unknown as Topology<{ countries: GeometryCollection, land: GeometryCollection }>
const landPath = path(feature(topology, topology.objects.land)) ?? ""
const borderPath = path(mesh(topology, topology.objects.countries, (a, b) => a !== b)) ?? ""
const gridPath = path(geoGraticule10()) ?? ""

function place(code: string): [number, number] | null {
  const lonLat = countryPoints[code]
  if (!lonLat) return null
  return projection(lonLat)
}

// arc rises from one point to another by bend times their distance, always
// upward, so arcs between the same places stack instead of crossing.
function arc([x1, y1]: [number, number], [x2, y2]: [number, number], bend: number): string {
  const dx = x2 - x1
  const dy = y2 - y1
  let nx = -dy * bend
  let ny = dx * bend
  if (ny > 0) {
    nx = -nx
    ny = -ny
  }
  return `M${x1},${y1} Q${(x1 + x2) / 2 + nx},${(y1 + y2) / 2 + ny} ${x2},${y2}`
}

interface Line {
  key: string
  d: string
  color: string
  speed: number
  title: string
}

interface CountryDot {
  code: string
  at: [number, number]
  speed: number
  connections: number
  upload: number
  download: number
}

// FlowMap draws each server at its region with glowing arcs to the countries its
// peers are in now. A pulse runs along each arc the way the data goes: out to the
// peer for uploads, in to the server for downloads. Thicker arcs move more.
// colorOf gives each server its colour when arcs are coloured by server.
export function FlowMap({ instances, peers, colorBy, colorOf }: {
  instances: BrushFlowInstance[]
  peers: BrushFlowPeer[]
  colorBy: FlowColorBy
  colorOf: (id: number) => string
}) {
  const { t, i18n } = useTranslation("brush")
  const names = useMemo(() => {
    try {
      return new Intl.DisplayNames([i18n.language], { type: "region" })
    } catch {
      return null
    }
  }, [i18n.language])
  const [view, setView] = useState({ k: 1, x: 0, y: 0 })
  const drag = useRef<{ x: number, y: number, vx: number, vy: number } | null>(null)
  const svgRef = useRef<SVGSVGElement>(null)

  const { clusters, lines, dots } = useMemo(() => {
    // Servers in one country share its point and one label.
    const clusterMap = new Map<string, { code: string, at: [number, number], servers: BrushFlowInstance[] }>()
    for (const inst of instances) {
      const at = place(inst.country)
      if (!at) continue
      const c = clusterMap.get(inst.country) ?? { code: inst.country, at, servers: [] }
      c.servers.push(inst)
      clusterMap.set(inst.country, c)
    }

    const speed = (bytes: number) => `${formatBytes(bytes)}/s`
    const flows = new Map<string, { inst: BrushFlowInstance, country: string, upload: number, download: number }>()
    const dotMap = new Map<string, CountryDot>()
    for (const p of peers) {
      const to = place(p.country)
      if (!to) continue
      const dot = dotMap.get(p.country) ?? { code: p.country, at: to, speed: 0, connections: 0, upload: 0, download: 0 }
      dot.speed += p.uploadSpeed + p.downloadSpeed
      dot.upload += p.uploadSpeed
      dot.download += p.downloadSpeed
      dot.connections++
      dotMap.set(p.country, dot)
      const inst = instances.find((i) => i.id === p.instanceId)
      if (!inst || !clusterMap.has(inst.country) || inst.country === p.country) continue
      const key = `${inst.id}:${p.country}`
      const flow = flows.get(key) ?? { inst, country: p.country, upload: 0, download: 0 }
      flow.upload += p.uploadSpeed
      flow.download += p.downloadSpeed
      flows.set(key, flow)
    }

    const all: Line[] = []
    // Each server bends its arcs a little differently, so arcs from servers in one
    // country fan out instead of lying on top of each other.
    const order = new Map(instances.map((inst, i) => [inst.id, i]))
    for (const f of flows.values()) {
      const from = clusterMap.get(f.inst.country)!.at
      const to = place(f.country)!
      const fan = 0.22 + 0.035 * ((order.get(f.inst.id) ?? 0) % 4)
      const country = countryName(names, f.country)
      if (f.upload > 0) {
        all.push({
          key: `${f.inst.id}:${f.country}:up`, d: arc(from, to, fan), speed: f.upload,
          color: colorBy === "server" ? colorOf(f.inst.id) : UPLOAD_COLOR,
          title: `${f.inst.name} → ${country}  ${speed(f.upload)}`,
        })
      }
      if (f.download > 0) {
        all.push({
          key: `${f.inst.id}:${f.country}:down`, d: arc(to, from, fan + 0.06), speed: f.download,
          color: colorBy === "server" ? colorOf(f.inst.id) : DOWNLOAD_COLOR,
          title: `${country} → ${f.inst.name}  ${speed(f.download)}`,
        })
      }
    }
    all.sort((a, b) => b.speed - a.speed)
    return {
      clusters: [...clusterMap.values()],
      lines: all.slice(0, MAX_ARCS),
      dots: [...dotMap.values()].sort((a, b) => b.speed - a.speed),
    }
  }, [instances, peers, colorBy, colorOf, names])

  const maxSpeed = Math.max(1, ...lines.map((l) => l.speed))
  // Close to linear, so an arc moving twice as much looks clearly thicker.
  const width = (s: number) => 1 + 8 * Math.pow(s / maxSpeed, 0.75)
  const labelled = new Set(dots.slice(0, LABELLED_COUNTRIES).map((d) => d.code))
  const clusterCodes = new Set(clusters.map((c) => c.code))
  const k = view.k
  const clusterText = (c: typeof clusters[number]) => c.servers.length > 1
    ? t("flows.serverCluster", { country: countryName(names, c.code), count: c.servers.length })
    : c.servers[0].name
  const labelledDots = dots.filter((d) => labelled.has(d.code) && !clusterCodes.has(d.code))
  // Servers take their spots first, then the busiest countries.
  const placed = placeLabels([
    ...clusters.map((c) => ({ at: c.at, text: clusterText(c), size: 12 })),
    ...labelledDots.map((d) => ({ at: d.at, text: countryName(names, d.code), size: 11 })),
  ], k)
  const clusterLabel = new Map(clusters.map((c, i) => [c.code, placed[i]]))
  const dotLabel = new Map(labelledDots.map((d, i) => [d.code, placed[clusters.length + i]]))

  const zoom = (factor: number) => setView((v) => {
    const nk = Math.min(MAX_ZOOM, Math.max(1, v.k * factor))
    // Zoom about the middle of the map.
    const cx = WIDTH / 2
    const cy = HEIGHT / 2
    return clamp({ k: nk, x: cx - ((cx - v.x) * nk) / v.k, y: cy - ((cy - v.y) * nk) / v.k })
  })

  const toMap = (e: PointerEvent) => {
    const rect = svgRef.current!.getBoundingClientRect()
    return { x: ((e.clientX - rect.left) / rect.width) * WIDTH, y: ((e.clientY - rect.top) / rect.height) * HEIGHT }
  }

  return (
    <div className="relative overflow-hidden rounded-lg" style={{ background: MAP_BG }}>
      <svg
        ref={svgRef}
        viewBox={`0 0 ${WIDTH} ${HEIGHT}`}
        className="h-auto w-full touch-none select-none"
        style={{ cursor: k > 1 ? "grab" : "default" }}
        role="img"
        aria-label={t("flows.mapLabel")}
        onPointerDown={(e) => {
          if (k === 1) return
          const at = toMap(e)
          drag.current = { x: at.x, y: at.y, vx: view.x, vy: view.y }
          e.currentTarget.setPointerCapture(e.pointerId)
        }}
        onPointerMove={(e) => {
          if (!drag.current) return
          const at = toMap(e)
          const start = drag.current
          setView((v) => clamp({ k: v.k, x: start.vx + at.x - start.x, y: start.vy + at.y - start.y }))
        }}
        onPointerUp={() => { drag.current = null }}
      >
        <defs>
          <filter id="flow-glow" x="-20%" y="-20%" width="140%" height="140%">
            <feGaussianBlur stdDeviation="2.4" result="blur" />
            <feMerge><feMergeNode in="blur" /><feMergeNode in="SourceGraphic" /></feMerge>
          </filter>
        </defs>
        <style>{`
          .flow-pulse { stroke-dasharray: 7 93; animation: flow-run 2.6s linear infinite; }
          .flow-ping { animation: flow-ping 2.4s ease-out infinite; transform-box: fill-box; transform-origin: center; }
          @keyframes flow-run { from { stroke-dashoffset: 100; } to { stroke-dashoffset: 0; } }
          @keyframes flow-ping { from { transform: scale(1); opacity: .7; } to { transform: scale(3.4); opacity: 0; } }
          @media (prefers-reduced-motion: reduce) { .flow-pulse, .flow-ping { animation: none; } }
        `}</style>
        <g transform={`translate(${view.x},${view.y}) scale(${k})`}>
          <path d={gridPath} fill="none" stroke={GRID} strokeWidth={0.6} vectorEffect="non-scaling-stroke" />
          <path d={landPath} fill={LAND} />
          <path d={borderPath} fill="none" stroke={BORDER} strokeWidth={0.6} vectorEffect="non-scaling-stroke" />

          <g filter="url(#flow-glow)">
            {lines.map((l) => (
              <path key={l.key} d={l.d} fill="none" stroke={l.color} strokeOpacity={0.45}
                strokeWidth={width(l.speed)} strokeLinecap="round" vectorEffect="non-scaling-stroke">
                <title>{l.title}</title>
              </path>
            ))}
            {lines.map((l, i) => (
              <path key={`${l.key}:pulse`} d={l.d} pathLength={100} fill="none" stroke={l.color}
                strokeWidth={width(l.speed) + 1.5} strokeLinecap="round" vectorEffect="non-scaling-stroke"
                className="flow-pulse" style={{ animationDelay: `${-(i * 0.37) % 2.6}s` }} pointerEvents="none" />
            ))}
          </g>

          {dots.map((d) => (
            <g key={d.code}>
              <circle cx={d.at[0]} cy={d.at[1]} r={2.6 / k} fill="#e2e8f0" stroke={MAP_BG} strokeWidth={1 / k}>
                <title>{t("flows.countryTitle", {
                  country: countryName(names, d.code), count: d.connections,
                  upload: `${formatBytes(d.upload)}/s`, download: `${formatBytes(d.download)}/s`,
                })}</title>
              </circle>
              {dotLabel.has(d.code) && (
                <text x={dotLabel.get(d.code)!.x} y={dotLabel.get(d.code)!.y} textAnchor={dotLabel.get(d.code)!.anchor} fontSize={11 / k} fill={LABEL}
                  stroke={MAP_BG} strokeWidth={3 / k} paintOrder="stroke" pointerEvents="none">
                  {countryName(names, d.code)}
                </text>
              )}
            </g>
          ))}

          {clusters.map((c) => {
            const color = colorBy === "server" && c.servers.length === 1 ? colorOf(c.servers[0].id) : "#ffffff"
            return (
              <g key={c.code}>
                <circle cx={c.at[0]} cy={c.at[1]} r={4.5 / k} fill={color} className="flow-ping" />
                <circle cx={c.at[0]} cy={c.at[1]} r={4.5 / k} fill={color} stroke={MAP_BG} strokeWidth={1.5 / k}>
                  <title>{c.servers.map((s) => t("flows.serverTitle", {
                    name: s.name, upload: `${formatBytes(s.uploadSpeed)}/s`, download: `${formatBytes(s.downloadSpeed)}/s`,
                  })).join("\n")}</title>
                </circle>
                <text x={clusterLabel.get(c.code)!.x} y={clusterLabel.get(c.code)!.y} textAnchor={clusterLabel.get(c.code)!.anchor}
                  fontSize={12 / k} fontWeight={600} fill="#f1f5f9" stroke={MAP_BG} strokeWidth={3 / k} paintOrder="stroke" pointerEvents="none">
                  {clusterText(c)}
                </text>
              </g>
            )
          })}
        </g>
      </svg>

      <div className="absolute bottom-3 left-3 flex max-w-[70%] flex-wrap items-center gap-x-4 gap-y-1 text-xs" style={{ color: LABEL }}>
        {colorBy === "direction" ? (
          <>
            <LegendItem color={UPLOAD_COLOR} label={t("flows.legendUpload")} />
            <LegendItem color={DOWNLOAD_COLOR} label={t("flows.legendDownload")} />
          </>
        ) : instances.filter((i) => clusterCodes.has(i.country)).map((inst) => (
          <LegendItem key={inst.id} color={colorOf(inst.id)} label={inst.name} />
        ))}
        {lines.length > 0 && (
          <span className="flex items-center gap-1.5">
            {[0.08, 0.35, 1].map((share) => (
              <span key={share} className="w-4 rounded-full bg-slate-300" style={{ height: width(share * maxSpeed) }} />
            ))}
            {t("flows.legendWidth", { speed: `${formatBytes(maxSpeed)}/s` })}
          </span>
        )}
      </div>
      <div className="absolute bottom-3 right-3 flex gap-1">
        <MapButton label={t("flows.zoomIn")} onClick={() => zoom(1.5)}><Plus className="h-3.5 w-3.5" /></MapButton>
        <MapButton label={t("flows.zoomOut")} onClick={() => zoom(1 / 1.5)}><Minus className="h-3.5 w-3.5" /></MapButton>
        <MapButton label={t("flows.resetView")} onClick={() => setView({ k: 1, x: 0, y: 0 })}>{t("flows.resetView")}</MapButton>
      </div>
    </div>
  )
}

interface Placed {
  x: number
  y: number
  anchor: "start" | "end"
}

// Offsets from the dot, in screen pixels, tried in turn: right or left of it,
// above or below, then further out.
const LABEL_SPOTS: Array<[number, number, "start" | "end"]> = [
  [8, -6, "start"], [8, 14, "start"], [-8, -6, "end"], [-8, 14, "end"],
  [8, -22, "start"], [-8, -22, "end"], [8, 30, "start"], [-8, 30, "end"],
]

// placeLabels puts each label in the first spot that overlaps no label placed
// before it, so labels earlier in the list win the best spots.
function placeLabels(items: Array<{ at: [number, number], text: string, size: number }>, k: number): Placed[] {
  const boxes: Array<[number, number, number, number]> = []
  return items.map(({ at, text, size }) => {
    const font = size / k
    // CJK characters are about as wide as the font is tall, latin ones a bit over half.
    const w = [...text].reduce((sum, ch) => sum + (ch.charCodeAt(0) > 0x2e80 ? 1 : 0.6), 0) * font
    const h = font * 1.2
    let pick: Placed | null = null
    for (const [dx, dy, anchor] of LABEL_SPOTS) {
      const x = at[0] + dx / k
      const y = at[1] + dy / k
      const left = anchor === "start" ? x : x - w
      const box: [number, number, number, number] = [left, y - font, left + w, y - font + h]
      if (!boxes.some((b) => box[0] < b[2] && b[0] < box[2] && box[1] < b[3] && b[1] < box[3])) {
        boxes.push(box)
        pick = { x, y, anchor }
        break
      }
    }
    return pick ?? { x: at[0] + LABEL_SPOTS[0][0] / k, y: at[1] + LABEL_SPOTS[0][1] / k, anchor: "start" }
  })
}

// clamp keeps the zoomed map covering the whole frame.
function clamp(v: { k: number, x: number, y: number }) {
  return {
    k: v.k,
    x: Math.min(0, Math.max(WIDTH - WIDTH * v.k, v.x)),
    y: Math.min(0, Math.max(HEIGHT - HEIGHT * v.k, v.y)),
  }
}

function LegendItem({ color, label }: { color: string, label: string }) {
  return (
    <span className="flex items-center gap-1.5">
      <span className="h-0.5 w-4 rounded" style={{ background: color, boxShadow: `0 0 6px ${color}` }} />
      {label}
    </span>
  )
}

function MapButton({ label, onClick, children }: { label: string, onClick: () => void, children: ReactNode }) {
  return (
    <Button size="sm" variant="outline" aria-label={label} onClick={onClick}
      className="h-7 min-w-7 border-white/15 bg-white/5 px-2 text-xs text-slate-200 hover:bg-white/10 hover:text-white">
      {children}
    </Button>
  )
}
