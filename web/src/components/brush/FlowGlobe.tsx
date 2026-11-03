/*
 * Copyright (c) 2026, vertex-qui contributors.
 * SPDX-License-Identifier: GPL-2.0-or-later
 */

import { formatBytes } from "@/lib/utils"
import type { BrushFlowInstance, BrushFlowPeer } from "@/types/brush"
import type { GlobeInstance } from "globe.gl"
import { useEffect, useMemo, useRef, useState, type ReactNode } from "react"
import { useTranslation } from "react-i18next"
import { feature } from "topojson-client"
import type { GeometryCollection, Topology } from "topojson-specification"
import countries110 from "world-atlas/countries-110m.json"
import { buildFlows, type FlowArc } from "./flowModel"
import { countryOfNumeric } from "./countryPoints"
import { countryName, DOWNLOAD_COLOR, heatColor, heatGradient, heatShare, UPLOAD_COLOR, type FlowColorBy } from "./flowStyle"

const LABELLED_COUNTRIES = 12
const GLOBE_COLOR = "#081626"
const DOTS_COLOR = "#35587a"

const topology = countries110 as unknown as Topology<{ countries: GeometryCollection }>
const countryShapes = feature(topology, topology.objects.countries).features

interface CountryShape {
  shape: (typeof countryShapes)[number]
  code: string
  share: number
  label: string
}

function escapeHTML(text: string) {
  return text.replace(/[&<>"']/g, (ch) => `&#${ch.charCodeAt(0)};`)
}

// withAlpha turns #rrggbb into rgba with the given opacity.
function withAlpha(hex: string, alpha: number) {
  const n = parseInt(hex.slice(1), 16)
  return `rgba(${n >> 16},${(n >> 8) & 255},${n & 255},${alpha})`
}

interface GlobeArc {
  startLat: number
  startLng: number
  endLat: number
  endLng: number
  colors: [string, string]
  stroke: number
  altitudeScale: number
  dashTime: number
  label: string
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

// FlowGlobe draws the flows on a slowly turning globe: dotted continents, a pulse
// running along each arc the way the data goes, from dim at the start to bright at
// the end, and a ring spreading from each server. It loads three.js only when it
// is shown, and shows fallback when WebGL is missing.
export function FlowGlobe({ instances, peers, colorBy, colorOf, fallback }: {
  instances: BrushFlowInstance[]
  peers: BrushFlowPeer[]
  colorBy: FlowColorBy
  colorOf: (id: number) => string
  fallback: ReactNode
}) {
  const { t, i18n } = useTranslation("brush")
  const host = useRef<HTMLDivElement>(null)
  const globe = useRef<GlobeInstance | null>(null)
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
    Promise.all([import("globe.gl"), import("three")]).then(([{ default: Globe }, { MeshPhongMaterial }]) => {
      if (disposed || !el) return
      const still = window.matchMedia("(prefers-reduced-motion: reduce)").matches
      const g = new Globe(el, { rendererConfig: { antialias: true, alpha: true } })
        .backgroundColor("rgba(0,0,0,0)")
        .globeMaterial(new MeshPhongMaterial({ color: GLOBE_COLOR, emissive: "#050d18", shininess: 6 }))
        .showAtmosphere(true)
        .atmosphereColor("#4f8ff7")
        .atmosphereAltitude(0.2)
        .showGraticules(true)
        .hexPolygonsData(countryShapes)
        .hexPolygonResolution(3)
        .hexPolygonMargin(0.62)
        .hexPolygonUseDots(true)
        .hexPolygonColor(() => DOTS_COLOR)
        .arcStartLat("startLat")
        .arcStartLng("startLng")
        .arcEndLat("endLat")
        .arcEndLng("endLng")
        .arcColor("colors")
        .arcStroke("stroke")
        .arcAltitudeAutoScale("altitudeScale")
        .arcDashLength(0.45)
        .arcDashGap(0.12)
        .arcDashInitialGap(() => Math.random())
        .arcDashAnimateTime(still ? 0 : "dashTime")
        .arcLabel("label")
        .arcsTransitionDuration(0)
        .pointLat("lat")
        .pointLng("lng")
        .pointColor("color")
        .pointRadius("radius")
        .pointAltitude(0.005)
        .pointLabel("label")
        .pointsTransitionDuration(0)
        .ringLat("lat")
        .ringLng("lng")
        .ringColor((d: object) => (t: number) => withAlpha((d as { color: string }).color, 1 - t))
        .ringMaxRadius(3.2)
        .ringPropagationSpeed(1.6)
        .ringRepeatPeriod(still ? 0 : 1600)
        // world-atlas country shapes are Polygons and MultiPolygons, which is what the globe takes.
        .polygonGeoJsonGeometry((d: object) => (d as CountryShape).shape.geometry as unknown as { type: string, coordinates: number[] })
        .polygonCapColor((d: object) => heatColor((d as CountryShape).share, 0.55))
        .polygonSideColor((d: object) => heatColor((d as CountryShape).share, 0.25))
        .polygonStrokeColor((d: object) => heatColor((d as CountryShape).share, 0.9))
        // Busier countries rise a little off the globe.
        .polygonAltitude((d: object) => 0.006 + 0.045 * (d as CountryShape).share)
        .polygonLabel("label")
        .polygonsTransitionDuration(still ? 0 : 600)
        .htmlLat("lat")
        .htmlLng("lng")
        .htmlAltitude(0.01)
        .htmlTransitionDuration(0)
        .htmlElement((d: object) => {
          const label = d as GlobeLabel
          const div = document.createElement("div")
          div.textContent = label.text
          div.style.cssText = `pointer-events:none;white-space:nowrap;transform:translate(8px,calc(-50% + ${label.shift}px));font-size:${label.server ? 12 : 11}px;`
            + `font-weight:${label.server ? 600 : 400};color:${label.server ? "#f1f5f9" : "#9fb3c8"};text-shadow:0 0 3px #000,0 0 6px #000;`
          return div
        })
      const controls = g.controls()
      controls.autoRotate = !still
      controls.autoRotateSpeed = 0.35
      controls.enableZoom = true
      controls.minDistance = 160
      controls.maxDistance = 520
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

  const model = useMemo(() => buildFlows(instances, peers), [instances, peers])
  const maxSpeed = Math.max(1, ...model.arcs.map((a) => a.speed))

  useEffect(() => {
    const g = globe.current
    if (!ready || !g) return
    const speed = (bytes: number) => `${formatBytes(bytes)}/s`
    const colorOfArc = (a: FlowArc) => colorBy === "server" ? colorOf(a.server.id) : a.direction === "up" ? UPLOAD_COLOR : DOWNLOAD_COLOR
    const arcs: GlobeArc[] = model.arcs.map((a) => {
      const up = a.direction === "up"
      const [from, to] = up ? [a.serverAt, a.peerAt] : [a.peerAt, a.serverAt]
      const color = colorOfArc(a)
      const country = countryName(names, a.country)
      const share = a.speed / maxSpeed
      return {
        startLat: from[1], startLng: from[0], endLat: to[1], endLng: to[0],
        colors: [withAlpha(color, 0.3), color],
        // Thin lines; a faster arc is wider and its pulse runs quicker.
        stroke: 0.12 + 0.6 * Math.pow(share, 0.75),
        altitudeScale: 0.32 + 0.05 * (a.order % 4) + (up ? 0 : 0.06),
        dashTime: 3200 - 1600 * share,
        label: escapeHTML(up ? `${a.server.name} → ${country}  ${speed(a.speed)}` : `${country} → ${a.server.name}  ${speed(a.speed)}`),
      }
    })
    const serverColor = (servers: BrushFlowInstance[]) => colorBy === "server" && servers.length === 1 ? colorOf(servers[0].id) : "#ffffff"
    const clusterLabel = (servers: BrushFlowInstance[], code: string) => servers.length > 1
      ? t("flows.serverCluster", { country: countryName(names, code), count: servers.length })
      : servers[0].name
    const serverCodes = new Set(model.clusters.map((c) => c.code))
    const points = [
      ...model.countries.map((c) => ({
        lat: c.at[1], lng: c.at[0], color: "#e2e8f0", radius: 0.22,
        label: escapeHTML(t("flows.countryTitle", { country: countryName(names, c.code), count: c.connections, upload: speed(c.upload), download: speed(c.download) })),
      })),
      ...model.clusters.map((c) => ({
        lat: c.at[1], lng: c.at[0], color: serverColor(c.servers), radius: 0.42,
        label: c.servers.map((s) => escapeHTML(t("flows.serverTitle", { name: s.name, upload: speed(s.uploadSpeed), download: speed(s.downloadSpeed) }))).join("<br>"),
      })),
    ]
    const labels = stackLabels([
      ...model.clusters.map((c) => ({ lat: c.at[1], lng: c.at[0], text: clusterLabel(c.servers, c.code), server: true })),
      ...model.countries.filter((c) => !serverCodes.has(c.code)).slice(0, LABELLED_COUNTRIES)
        .map((c) => ({ lat: c.at[1], lng: c.at[0], text: countryName(names, c.code), server: false })),
    ])
    const maxCountry = Math.max(1, ...model.countries.map((c) => c.speed))
    const byCode = new Map(model.countries.map((c) => [c.code, c]))
    const shaded: CountryShape[] = countryShapes.flatMap((shape) => {
      const code = countryOfNumeric[String(shape.id)]
      const c = code ? byCode.get(code) : undefined
      if (!c || c.speed <= 0) return []
      return [{
        shape, code, share: heatShare(c.speed, maxCountry),
        label: escapeHTML(t("flows.countryTitle", { country: countryName(names, code), count: c.connections, upload: speed(c.upload), download: speed(c.download) })),
      }]
    })
    g.polygonsData(shaded)
      .arcsData(arcs)
      .pointsData(points)
      .ringsData(model.clusters.map((c) => ({ lat: c.at[1], lng: c.at[0], color: serverColor(c.servers) })))
      .htmlElementsData(labels)
    // Face the servers the first time there are any.
    if (!aimed.current && model.clusters.length > 0) {
      aimed.current = true
      const lng = model.clusters.reduce((sum, c) => sum + c.at[0], 0) / model.clusters.length
      g.pointOfView({ lat: 38, lng: lng + 30, altitude: 2.1 }, 800)
    }
  }, [ready, model, maxSpeed, colorBy, colorOf, names, t])

  if (failed) return <>{fallback}</>
  return (
    <div className="relative overflow-hidden rounded-lg bg-[radial-gradient(ellipse_at_center,#0f2236_0%,#060c14_70%)]">
      <div ref={host} className="h-[clamp(320px,56vw,600px)] w-full cursor-grab active:cursor-grabbing" role="img" aria-label={t("flows.mapLabel")} />
      <div className="pointer-events-none absolute bottom-3 left-3 flex max-w-[80%] flex-wrap items-center gap-x-4 gap-y-1 text-xs text-[#9fb3c8]">
        {colorBy === "direction" ? (
          <>
            <Legend color={UPLOAD_COLOR} label={t("flows.legendUpload")} />
            <Legend color={DOWNLOAD_COLOR} label={t("flows.legendDownload")} />
          </>
        ) : model.clusters.flatMap((c) => c.servers).map((s) => <Legend key={s.id} color={colorOf(s.id)} label={s.name} />)}
        {model.arcs.length > 0 && <span>{t("flows.legendWidth", { speed: `${formatBytes(maxSpeed)}/s` })}</span>}
        {model.countries.length > 0 && (
          <span className="flex items-center gap-1.5">
            {t("flows.heatLow")}
            <span className="h-1.5 w-16 rounded-full" style={{ background: heatGradient }} />
            {t("flows.heatHigh")}
          </span>
        )}
      </div>
      <p className="pointer-events-none absolute right-3 top-3 text-xs text-[#9fb3c8]/70">{t("flows.globeHint")}</p>
    </div>
  )
}

function Legend({ color, label }: { color: string, label: string }) {
  return (
    <span className="flex items-center gap-1.5">
      <span className="h-0.5 w-4 rounded" style={{ background: color, boxShadow: `0 0 6px ${color}` }} />
      {label}
    </span>
  )
}
