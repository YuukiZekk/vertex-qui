/*
 * Copyright (c) 2026, vertex-qui contributors.
 * SPDX-License-Identifier: GPL-2.0-or-later
 */

import { formatBytes } from "@/lib/utils"
import type { BrushFlowCountry, BrushFlowInstance, BrushFlowPeer } from "@/types/brush"
import type { GlobeInstance } from "globe.gl"
import { useEffect, useMemo, useRef, useState, type ReactNode } from "react"
import { useTranslation } from "react-i18next"
import { feature } from "topojson-client"
import type { GeometryCollection, Topology } from "topojson-specification"
import { countryOfNumeric } from "./countryPoints"
import { buildFlows, type FlowArc } from "./flowModel"
import { countryName, DOWNLOAD_COLOR, heatColor, heatGradient, heatShare, UPLOAD_COLOR, type FlowColorBy } from "./flowStyle"

const LABELLED_COUNTRIES = 12
const OCEAN = "#061423"
const LAND = "#10253a"
const BORDER = "rgba(110,160,210,0.28)"
// The busiest country rises this far off the globe, in globe radii.
const MAX_RISE = 0.07

function escapeHTML(text: string) {
  return text.replace(/[&<>"']/g, (ch) => `&#${ch.charCodeAt(0)};`)
}

// withAlpha turns #rrggbb into rgba with the given opacity.
function withAlpha(hex: string, alpha: number) {
  const n = parseInt(hex.slice(1), 16)
  return `rgba(${n >> 16},${(n >> 8) & 255},${n & 255},${alpha})`
}

// A country outline on the globe. The objects are made once and keep their
// identity, so a new reading only recolours and raises the countries whose share
// changed instead of rebuilding every detailed shape; share and label are
// rewritten in place.
interface CountryShape {
  geometry: { type: string, coordinates: number[] }
  code: string
  share: number
  label: string
}

interface GlobeArc {
  startLat: number
  startLng: number
  endLat: number
  endLng: number
  colors: [string, string]
  stroke: number
  altitudeScale: number
  dashLength: number
  dashGap: number
  initialGap: number
  dashTime: number
  label: string
}

// A ring spreading from a server or a pool; tint gives its colour at an opacity.
interface GlobeRing {
  lat: number
  lng: number
  tint: (alpha: number) => string
  maxRadius: number
  speed: number
  period: number
}

interface GlobeLabel {
  lat: number
  lng: number
  text: string
  server: boolean
  // shift moves a label down, in pixels, below labels placed near it before.
  shift: number
}

// Labels closer than this, in degrees, would overlap on the globe.
const NEAR_DEGREES = 9

// stackLabels shifts each label below the ones placed near it before, and drops a
// country label that lands on a server label.
function stackLabels(items: Array<Omit<GlobeLabel, "shift">>): GlobeLabel[] {
  const placed: GlobeLabel[] = []
  for (const item of items) {
    const near = placed.filter((p) => Math.hypot(p.lat - item.lat, p.lng - item.lng) < NEAR_DEGREES)
    if (!item.server && near.some((p) => p.server)) continue
    placed.push({ ...item, shift: near.length * 15 })
  }
  return placed
}

// FlowGlobe draws the flows on a slowly turning globe as pipes and pools: each arc
// is a pipe as wide as what moves through it now, with a pulse running the way the
// data goes, and each peer country is a pool that fills with what it moved today,
// a glowing column with ripples, its country shaded to match. It loads three.js
// and the detailed borders only when it is shown, and shows fallback without WebGL.
export function FlowGlobe({ instances, peers, today, colorBy, colorOf, fallback }: {
  instances: BrushFlowInstance[]
  peers: BrushFlowPeer[]
  today: BrushFlowCountry[]
  colorBy: FlowColorBy
  colorOf: (id: number) => string
  fallback: ReactNode
}) {
  const { t, i18n } = useTranslation("brush")
  const host = useRef<HTMLDivElement>(null)
  const globe = useRef<GlobeInstance | null>(null)
  const shapes = useRef<CountryShape[]>([])
  const aimed = useRef(false)
  const [ready, setReady] = useState(false)
  const [failed, setFailed] = useState(false)
  const names = useMemo(() => {
    try {
      return new Intl.DisplayNames([i18n.language], { type: "region" })
    } catch {
      return null
    }
  }, [i18n.language])

  useEffect(() => {
    let disposed = false
    let observer: ResizeObserver | undefined
    const el = host.current
    Promise.all([import("globe.gl"), import("three"), import("world-atlas/countries-50m.json")]).then(([{ default: Globe }, { MeshPhongMaterial }, atlas]) => {
      if (disposed || !el) return
      const topology = (atlas.default ?? atlas) as unknown as Topology<{ countries: GeometryCollection }>
      shapes.current = feature(topology, topology.objects.countries).features.map((f) => ({
        geometry: f.geometry as unknown as CountryShape["geometry"],
        code: countryOfNumeric[String(f.id)] ?? "",
        share: 0,
        label: "",
      }))
      const still = window.matchMedia("(prefers-reduced-motion: reduce)").matches
      const g = new Globe(el, { rendererConfig: { antialias: true, alpha: true } })
        .backgroundColor("rgba(0,0,0,0)")
        .globeMaterial(new MeshPhongMaterial({ color: OCEAN, emissive: "#030a14", shininess: 4 }))
        .showAtmosphere(true)
        .atmosphereColor("#4f8ff7")
        .atmosphereAltitude(0.18)
        .showGraticules(true)
        .polygonsData(shapes.current)
        .polygonGeoJsonGeometry("geometry")
        .polygonAltitude((d: object) => rise(d as CountryShape))
        .polygonSideColor((d: object) => sideColor(d as CountryShape))
        .polygonStrokeColor(() => BORDER)
        .polygonCapColor((d: object) => capColor(d as CountryShape))
        .polygonLabel((d: object) => (d as CountryShape).label)
        .polygonsTransitionDuration(still ? 0 : 900)
        .arcStartLat("startLat")
        .arcStartLng("startLng")
        .arcEndLat("endLat")
        .arcEndLng("endLng")
        .arcColor("colors")
        .arcStroke("stroke")
        .arcAltitudeAutoScale("altitudeScale")
        .arcDashLength("dashLength")
        .arcDashGap("dashGap")
        .arcDashInitialGap("initialGap")
        .arcDashAnimateTime(still ? 0 : "dashTime")
        .arcLabel("label")
        .arcsTransitionDuration(0)
        .pointLat("lat")
        .pointLng("lng")
        .pointColor("color")
        .pointRadius("radius")
        .pointAltitude("altitude")
        .pointLabel("label")
        .pointsMerge(false)
        .pointsTransitionDuration(still ? 0 : 600)
        .ringLat("lat")
        .ringLng("lng")
        .ringColor((d: object) => (f: number) => (d as GlobeRing).tint(0.9 * (1 - f)))
        .ringMaxRadius("maxRadius")
        .ringPropagationSpeed("speed")
        .ringRepeatPeriod(still ? 0 : "period")
        .htmlLat("lat")
        .htmlLng("lng")
        .htmlAltitude(0.012)
        .htmlTransitionDuration(0)
        .htmlElement((d: object) => {
          const label = d as GlobeLabel
          const div = document.createElement("div")
          div.textContent = label.text
          div.style.cssText = `pointer-events:none;white-space:nowrap;transform:translate(8px,calc(-50% + ${label.shift}px));`
            + `font-size:${label.server ? 12 : 11}px;font-weight:${label.server ? 600 : 400};`
            + `color:${label.server ? "#f1f5f9" : "#c4d4e6"};text-shadow:0 0 3px #000,0 0 6px #000;`
          return div
        })
      const controls = g.controls()
      controls.autoRotate = !still
      controls.autoRotateSpeed = 0.3
      controls.minDistance = 140
      controls.maxDistance = 520
      g.renderer().setPixelRatio(Math.min(2, window.devicePixelRatio || 1))
      g.pointOfView({ lat: 25, lng: 60, altitude: 2.3 })
      const resize = () => g.width(el.clientWidth).height(el.clientHeight)
      resize()
      observer = new ResizeObserver(resize)
      observer.observe(el)
      globe.current = g
      setReady(true)
    }).catch(() => {
      if (!disposed) setFailed(true)
    })
    return () => {
      disposed = true
      observer?.disconnect()
      globe.current?._destructor()
      globe.current = null
      el?.replaceChildren()
    }
  }, [])

  const model = useMemo(() => buildFlows(instances, peers, today), [instances, peers, today])
  const maxSpeed = Math.max(1, ...model.arcs.map((a) => a.speed))
  const maxTotal = Math.max(1, ...model.countries.map((c) => c.total))

  useEffect(() => {
    const g = globe.current
    if (!ready || !g) return
    const speed = (bytes: number) => `${formatBytes(bytes)}/s`
    const countryTitle = (c: (typeof model.countries)[number]) => escapeHTML(t("flows.countryToday", {
      country: countryName(names, c.code), uploaded: formatBytes(c.uploaded), downloaded: formatBytes(c.downloaded),
      count: c.connections, upload: speed(c.upload), download: speed(c.download),
    }))

    // Pools: each country rises and is shaded by what it moved today.
    const byCode = new Map(model.countries.map((c) => [c.code, c]))
    for (const s of shapes.current) {
      const c = byCode.get(s.code)
      s.share = c && c.total > 0 ? heatShare(c.total, maxTotal) : 0
      s.label = c ? countryTitle(c) : ""
    }
    // New accessors make the globe recolour and raise the same shapes.
    g.polygonCapColor((d: object) => capColor(d as CountryShape))
      .polygonSideColor((d: object) => sideColor(d as CountryShape))
      .polygonAltitude((d: object) => rise(d as CountryShape))

    // Pipes: a faint tube as wide as what moves through it now, and a stream of
    // particles inside it that runs the way the data goes. A busier pipe carries
    // more particles, closer together and faster.
    const colorOfArc = (a: FlowArc) => colorBy === "server" ? colorOf(a.server.id) : a.direction === "up" ? UPLOAD_COLOR : DOWNLOAD_COLOR
    const arcs: GlobeArc[] = model.arcs.flatMap((a) => {
      const up = a.direction === "up"
      const [from, to] = up ? [a.serverAt, a.peerAt] : [a.peerAt, a.serverAt]
      const color = colorOfArc(a)
      const country = countryName(names, a.country)
      const share = a.speed / maxSpeed
      const width = 0.15 + 0.95 * Math.pow(share, 0.7)
      const base = {
        startLat: from[1], startLng: from[0], endLat: to[1], endLng: to[0],
        altitudeScale: 0.32 + 0.05 * (a.order % 4) + (up ? 0 : 0.06),
        label: escapeHTML(up ? `${a.server.name} → ${country}  ${speed(a.speed)}` : `${country} → ${a.server.name}  ${speed(a.speed)}`),
      }
      return [
        { ...base, colors: [withAlpha(color, 0.12), withAlpha(color, 0.35)], stroke: width, dashLength: 1, dashGap: 0, initialGap: 0, dashTime: 0 },
        {
          ...base, colors: [withAlpha(color, 0.6), "#ffffff"], stroke: width * 0.9 + 0.1,
          dashLength: 0.012 + 0.01 * share, dashGap: 0.11 - 0.07 * share, initialGap: Math.random(), dashTime: 4200 - 2400 * share,
        },
      ]
    })

    const serverColor = (servers: BrushFlowInstance[]) => colorBy === "server" && servers.length === 1 ? colorOf(servers[0].id) : "#ffffff"
    const pools = model.countries.filter((c) => c.total > 0)
    const points = [
      ...model.clusters.map((c) => ({
        lat: c.at[1], lng: c.at[0], color: serverColor(c.servers), radius: 0.42, altitude: 0.006,
        label: c.servers.map((s) => escapeHTML(t("flows.serverTitle", { name: s.name, upload: speed(s.uploadSpeed), download: speed(s.downloadSpeed) }))).join("<br>"),
      })),
    ]
    const rings: GlobeRing[] = [
      ...pools.map((c) => {
        const share = heatShare(c.total, maxTotal)
        return { lat: c.at[1], lng: c.at[0], tint: (alpha: number) => heatColor(share, alpha * 0.7), maxRadius: 1 + 3 * share, speed: 0.7, period: 3000 }
      }),
      ...model.clusters.map((c) => {
        const color = serverColor(c.servers)
        return { lat: c.at[1], lng: c.at[0], tint: (alpha: number) => withAlpha(color, alpha), maxRadius: 3, speed: 1.6, period: 1600 }
      }),
    ]

    const clusterLabel = (servers: BrushFlowInstance[], code: string) => servers.length > 1
      ? t("flows.serverCluster", { country: countryName(names, code), count: servers.length })
      : servers[0].name
    const serverCodes = new Set(model.clusters.map((c) => c.code))
    const labels = stackLabels([
      ...model.clusters.map((c) => ({ lat: c.at[1], lng: c.at[0], text: clusterLabel(c.servers, c.code), server: true })),
      ...model.countries.filter((c) => !serverCodes.has(c.code)).slice(0, LABELLED_COUNTRIES)
        .map((c) => ({ lat: c.at[1], lng: c.at[0], text: countryName(names, c.code), server: false })),
    ])

    g.arcsData(arcs).pointsData(points).ringsData(rings).htmlElementsData(labels)
    // Face the servers the first time there are any.
    if (!aimed.current && model.clusters.length > 0) {
      aimed.current = true
      const lng = model.clusters.reduce((sum, c) => sum + c.at[0], 0) / model.clusters.length
      g.pointOfView({ lat: 38, lng: lng + 30, altitude: 2.1 }, 800)
    }
  }, [ready, model, maxSpeed, maxTotal, colorBy, colorOf, names, t])

  if (failed) return <>{fallback}</>
  return (
    <div className="relative overflow-hidden rounded-lg bg-[radial-gradient(ellipse_at_center,#0f2236_0%,#060c14_70%)]">
      <div ref={host} className="h-[clamp(320px,56vw,600px)] w-full cursor-grab active:cursor-grabbing" role="img" aria-label={t("flows.mapLabel")} />
      <div className="pointer-events-none absolute bottom-3 left-3 flex max-w-[85%] flex-wrap items-center gap-x-4 gap-y-1 text-xs text-[#9fb3c8]">
        {colorBy === "direction" ? (
          <>
            <Legend color={UPLOAD_COLOR} label={t("flows.legendUpload")} />
            <Legend color={DOWNLOAD_COLOR} label={t("flows.legendDownload")} />
          </>
        ) : model.clusters.flatMap((c) => c.servers).map((s) => <Legend key={s.id} color={colorOf(s.id)} label={s.name} />)}
        {model.arcs.length > 0 && <span>{t("flows.legendPipe", { speed: `${formatBytes(maxSpeed)}/s` })}</span>}
        {model.countries.some((c) => c.total > 0) && (
          <span className="flex items-center gap-1.5">
            {t("flows.legendPool")}
            <span className="h-1.5 w-16 rounded-full" style={{ background: heatGradient }} />
            {t("flows.heatHigh", { total: formatBytes(maxTotal) })}
          </span>
        )}
      </div>
      <p className="pointer-events-none absolute right-3 top-3 text-xs text-[#9fb3c8]/70">{t("flows.globeHint")}</p>
    </div>
  )
}

function capColor(d: CountryShape) {
  return d.share > 0 ? heatColor(d.share, 0.85) : LAND
}

function sideColor(d: CountryShape) {
  return d.share > 0 ? heatColor(d.share * 0.8, 0.55) : "rgba(0,0,0,0)"
}

function rise(d: CountryShape) {
  return 0.004 + MAX_RISE * d.share
}

function Legend({ color, label }: { color: string, label: string }) {
  return (
    <span className="flex items-center gap-1.5">
      <span className="h-0.5 w-4 rounded" style={{ background: color, boxShadow: `0 0 6px ${color}` }} />
      {label}
    </span>
  )
}
