/*
 * Copyright (c) 2026, vertex-qui contributors.
 * SPDX-License-Identifier: GPL-2.0-or-later
 */

import { CountryFlag } from "@/components/brush/CountryFlag"
import { FlowGlobe } from "@/components/brush/FlowGlobe"
import { FlowMap } from "@/components/brush/FlowMap"
import { countryName, downloadText, serverColor, uploadText, type FlowColorBy } from "@/components/brush/flowStyle"
import { EmptyState, SectionCard } from "@/components/brush/section"
import { FlowSummary } from "@/components/brush/FlowSummary"
import { readOpen, sectionHeaderRowClass, sectionTableClass, writeOpen } from "@/components/brush/sectionStyles"
import { SiteIcon } from "@/components/brush/SiteIcon"
import { useBrushGroups } from "@/components/brush/useBrushGroups"
import { Button } from "@/components/ui/button"
import { Card, CardContent } from "@/components/ui/card"
import { Checkbox } from "@/components/ui/checkbox"
import { Label } from "@/components/ui/label"
import { Switch } from "@/components/ui/switch"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { api } from "@/lib/api"
import { cn, formatBytes } from "@/lib/utils"
import type { BrushFlowInstance, BrushFlowPeer } from "@/types/brush"
import { useQuery } from "@tanstack/react-query"
import { useCallback, useEffect, useMemo, useRef, useState } from "react"
import { useTranslation } from "react-i18next"

// The server reads peers every 15 seconds; polling at that pace shows each read.
const POLL_MS = 15000
const ROWS_SHOWN = 100
const HIDDEN_KEY = "brush.flows.hidden"
const COLOR_KEY = "brush.flows.colorBy"
const VIEW_KEY = "brush.flows.view"
const TABLE_OPEN_KEY = "brush.flows.tableOpen"

type FlowView = "globe" | "flat"

// maskIP keeps the network part of an address: 1.2.*.* or 2001:db8:*.
function maskIP(ip: string): string {
  if (ip.includes(":")) return `${ip.split(":").slice(0, 2).join(":")}:*`
  const parts = ip.split(".")
  return parts.length === 4 ? `${parts[0]}.${parts[1]}.*.*` : ip
}

const speed = (bytes: number) => `${formatBytes(bytes)}/s`

// The servers left out and the arc colouring are remembered per browser.
function readStored<T>(key: string, fallback: T): T {
  try {
    const raw = localStorage.getItem(key)
    return raw ? JSON.parse(raw) as T : fallback
  } catch {
    return fallback
  }
}

function writeStored(key: string, value: unknown) {
  try {
    localStorage.setItem(key, JSON.stringify(value))
  } catch {
    // Storage can be unavailable; the choice still holds for this visit.
  }
}

function Metric({ label, value, className }: { label: string, value: string, className?: string }) {
  return (
    <Card className="gap-0 py-0">
      <CardContent className="space-y-1 p-4">
        <p className="text-xs text-muted-foreground">{label}</p>
        <p className={cn("text-2xl font-semibold tabular-nums", className)}>{value}</p>
      </CardContent>
    </Card>
  )
}

// ServerGroup is one row of the server picker: a box for the whole group, then
// one per server.
function ServerGroup({ label, servers, hidden, onChange, colorOf, colorBy }: {
  label: string
  servers: BrushFlowInstance[]
  hidden: Set<number>
  onChange: (ids: number[], show: boolean) => void
  colorOf: (id: number) => string
  colorBy: FlowColorBy
}) {
  if (servers.length === 0) return null
  const shown = servers.filter((s) => !hidden.has(s.id)).length
  const all = servers.map((s) => s.id)
  return (
    <div className="flex flex-wrap items-center gap-x-4 gap-y-2">
      <label className="flex w-32 shrink-0 cursor-pointer items-center gap-2 text-sm font-medium">
        <Checkbox
          checked={shown === servers.length ? true : shown === 0 ? false : "indeterminate"}
          onCheckedChange={(v) => onChange(all, v === true)}
        />
        {label}
      </label>
      {servers.map((s) => (
        <label key={s.id} className="flex cursor-pointer items-center gap-2 text-sm">
          <Checkbox checked={!hidden.has(s.id)} onCheckedChange={(v) => onChange([s.id], v === true)} />
          {colorBy === "server" && <span className="h-2 w-2 rounded-full" style={{ background: colorOf(s.id) }} />}
          <CountryFlag code={s.country} />
          <span className={cn(hidden.has(s.id) && "text-muted-foreground")}>{s.name}</span>
        </label>
      ))}
    </div>
  )
}

export function Flows() {
  const { t, i18n } = useTranslation("brush")
  const { data } = useQuery({ queryKey: ["brush", "flows"], queryFn: () => api.getBrushFlows(), refetchInterval: POLL_MS })
  const { data: sites } = useQuery({ queryKey: ["brush", "sites"], queryFn: () => api.getBrushSites() })
  const { data: groups } = useBrushGroups()
  const [hidden, setHidden] = useState(() => new Set(readStored<number[]>(HIDDEN_KEY, [])))
  const [colorBy, setColorBy] = useState<FlowColorBy>(() => readStored<FlowColorBy>(COLOR_KEY, "direction"))
  const [view, setView] = useState<FlowView>(() => readStored<FlowView>(VIEW_KEY, "globe"))
  const [showIPs, setShowIPs] = useState(false)
  const [tableOpen, setTableOpen] = useState(() => readOpen(TABLE_OPEN_KEY))
  const names = useMemo(() => {
    try {
      return new Intl.DisplayNames([i18n.language], { type: "region" })
    } catch {
      return null
    }
  }, [i18n.language])

  const toggle = (ids: number[], show: boolean) => setHidden((prev) => {
    const next = new Set(prev)
    for (const id of ids) {
      if (show) next.delete(id)
      else next.add(id)
    }
    writeStored(HIDDEN_KEY, [...next])
    return next
  })
  const pickView = (v: FlowView) => {
    setView(v)
    writeStored(VIEW_KEY, v)
  }
  const pickColor = (by: FlowColorBy) => {
    setColorBy(by)
    writeStored(COLOR_KEY, by)
  }

  // This visit's traffic adds each instance's speed over the time between reads.
  const lastRead = useRef<number>(0)
  const [session, setSession] = useState<Record<number, { up: number, down: number }>>({})
  useEffect(() => {
    if (!data?.sampledAt || data.sampledAt <= lastRead.current) return
    const seconds = lastRead.current ? data.sampledAt - lastRead.current : 0
    lastRead.current = data.sampledAt
    if (!seconds) return
    setSession((prev) => {
      const next = { ...prev }
      for (const inst of data.instances) {
        const was = next[inst.id] ?? { up: 0, down: 0 }
        next[inst.id] = { up: was.up + inst.uploadSpeed * seconds, down: was.down + inst.downloadSpeed * seconds }
      }
      return next
    })
  }, [data])

  const allInstances = useMemo(() => data?.instances ?? [], [data])
  // Each server keeps its colour whichever servers are shown.
  const colorOf = useCallback((id: number) => serverColor(Math.max(0, allInstances.findIndex((i) => i.id === id))), [allInstances])
  const instances = useMemo(() => allInstances.filter((i) => !hidden.has(i.id)), [allInstances, hidden])
  const peers = useMemo(() => {
    const ids = new Set(instances.map((i) => i.id))
    return (data?.peers ?? []).filter((p) => ids.has(p.instanceId))
  }, [data, instances])

  const totals = instances.reduce((sum, i) => ({
    up: sum.up + i.uploadSpeed, down: sum.down + i.downloadSpeed, connections: sum.connections + i.connections,
    session: sum.session + (session[i.id]?.up ?? 0) + (session[i.id]?.down ?? 0),
  }), { up: 0, down: 0, connections: 0, session: 0 })

  // Today's totals of the servers shown, which fill the pools on the map.
  const today = useMemo(() => {
    const ids = new Set(instances.map((i) => i.id))
    return (data?.today ?? []).filter((d) => ids.has(d.instanceId))
  }, [data, instances])
  const countries = useMemo(() => {
    const byCode = new Map<string, { code: string, uploaded: number, downloaded: number }>()
    for (const d of today) {
      const row = byCode.get(d.country) ?? { code: d.country, uploaded: 0, downloaded: 0 }
      row.uploaded += d.uploaded
      row.downloaded += d.downloaded
      byCode.set(d.country, row)
    }
    return [...byCode.values()].sort((a, b) => (b.uploaded + b.downloaded) - (a.uploaded + a.downloaded)).slice(0, 10)
  }, [today])

  const instanceOf = (id: number) => allInstances.find((i) => i.id === id)
  const siteOf = (p: BrushFlowPeer) => sites?.find((s) => s.id === p.siteId)
  const noCountry = instances.filter((i) => !i.country)
  const loading = !data || data.sampledAt === 0

  return (
    <div className="container mx-auto space-y-6 overflow-x-hidden px-6 py-6">
      <div className="space-y-2">
        <h1 className="text-2xl font-semibold">{t("flows.title")}</h1>
        <p className="text-sm text-muted-foreground">{t("flows.description")}</p>
      </div>

      {allInstances.length > 0 && (
        <Card className="gap-0 py-0">
          <CardContent className="space-y-3 p-4">
            {(groups ?? []).map((g) => (
              <ServerGroup key={g.id} label={g.name} servers={allInstances.filter((i) => i.groups.includes(g.id))}
                hidden={hidden} onChange={toggle} colorOf={colorOf} colorBy={colorBy} />
            ))}
            <ServerGroup label={groups?.length ? t("flows.ungrouped") : t("flows.allServers")} servers={allInstances.filter((i) => i.groups.length === 0)}
              hidden={hidden} onChange={toggle} colorOf={colorOf} colorBy={colorBy} />
            {!groups?.length && <p className="text-xs text-muted-foreground">{t("flows.groupsHint")}</p>}
          </CardContent>
        </Card>
      )}

      <div className="grid grid-cols-2 gap-4 lg:grid-cols-4">
        <Metric label={t("flows.connections")} value={String(totals.connections)} />
        <Metric label={t("flows.upload")} value={speed(totals.up)} className={uploadText} />
        <Metric label={t("flows.download")} value={speed(totals.down)} className={downloadText} />
        <Metric label={t("flows.session")} value={formatBytes(totals.session)} />
      </div>

      <SectionCard
        title={t("flows.mapTitle")}
        info={t("flows.mapInfo")}
        description={data?.sampledAt ? t("flows.sampledAt", { time: new Date(data.sampledAt * 1000).toLocaleTimeString(i18n.language) }) : undefined}
        actions={
          <div className="flex flex-wrap gap-2">
            <div className="flex rounded-md border p-0.5">
              {(["globe", "flat"] as const).map((v) => (
                <Button key={v} size="sm" variant={view === v ? "secondary" : "ghost"} className="h-7 px-3 text-xs" onClick={() => pickView(v)}>
                  {v === "globe" ? t("flows.viewGlobe") : t("flows.viewFlat")}
                </Button>
              ))}
            </div>
            <div className="flex rounded-md border p-0.5">
              {(["direction", "server"] as const).map((by) => (
                <Button key={by} size="sm" variant={colorBy === by ? "secondary" : "ghost"} className="h-7 px-3 text-xs" onClick={() => pickColor(by)}>
                  {by === "direction" ? t("flows.colorByDirection") : t("flows.colorByServer")}
                </Button>
              ))}
            </div>
          </div>
        }
      >
        {loading ? <EmptyState>{t("flows.waiting")}</EmptyState> : (
          <div className="grid gap-0 lg:grid-cols-[minmax(0,1fr)_280px]">
            <div className="min-w-0 p-4">
              {view === "globe" ? (
                <FlowGlobe instances={instances} peers={peers} today={today} colorBy={colorBy} colorOf={colorOf}
                  fallback={<FlowMap instances={instances} peers={peers} today={today} colorBy={colorBy} colorOf={colorOf} />} />
              ) : (
                <FlowMap instances={instances} peers={peers} today={today} colorBy={colorBy} colorOf={colorOf} />
              )}
              {noCountry.length > 0 && (
                <p className="mt-2 text-xs text-muted-foreground">{t("flows.noCountry", { names: noCountry.map((i) => i.name).join("、") })}</p>
              )}
            </div>
            <div className="border-t p-4 lg:border-l lg:border-t-0">
              <p className="mb-3 text-sm font-medium">{t("flows.topCountries")}</p>
              {countries.length === 0 ? <p className="text-sm text-muted-foreground">{t("flows.idle")}</p> : (
                <ul className="space-y-2 text-sm">
                  {countries.map((c) => (
                    <li key={c.code || "unknown"} className="flex items-center gap-2">
                      <CountryFlag code={c.code} iconBox />
                      <span className="min-w-0 flex-1 truncate">{c.code ? countryName(names, c.code) : t("flows.unknownCountry")}</span>
                      <span className={cn("tabular-nums text-xs", uploadText)}>{formatBytes(c.uploaded)}</span>
                      <span className={cn("tabular-nums text-xs", downloadText)}>{formatBytes(c.downloaded)}</span>
                    </li>
                  ))}
                </ul>
              )}
              <div className="mt-5 border-t pt-4">
                <FlowSummary today={today} instances={allInstances} sites={sites ?? []} />
              </div>
            </div>
          </div>
        )}
      </SectionCard>

      <SectionCard
        open={tableOpen}
        onOpenChange={(open) => {
          setTableOpen(open)
          writeOpen(TABLE_OPEN_KEY, open)
        }}
        title={t("flows.tableTitle")}
        description={t("flows.tableDescription", { count: peers.length })}
        actions={
          <div className="flex items-center gap-2">
            <Switch id="flows-show-ips" checked={showIPs} onCheckedChange={setShowIPs} />
            <Label htmlFor="flows-show-ips" className="text-sm font-normal">{t("flows.showIPs")}</Label>
          </div>
        }
      >
        {peers.length === 0 ? <EmptyState>{loading ? t("flows.waiting") : t("flows.idle")}</EmptyState> : (
          <div className="overflow-x-auto">
            <Table className={sectionTableClass}>
              <TableHeader>
                <TableRow className={sectionHeaderRowClass}>
                  <TableHead>{t("flows.server")}</TableHead>
                  <TableHead>{t("flows.torrent")}</TableHead>
                  <TableHead>{t("flows.peer")}</TableHead>
                  <TableHead>{t("flows.client")}</TableHead>
                  <TableHead className="text-right">{t("flows.uploadShort")}</TableHead>
                  <TableHead className="text-right">{t("flows.downloadShort")}</TableHead>
                  <TableHead className="text-right">{t("flows.progress")}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {peers.slice(0, ROWS_SHOWN).map((p) => {
                  const inst = instanceOf(p.instanceId)
                  const site = siteOf(p)
                  return (
                    <TableRow key={`${p.instanceId}:${p.hash}:${p.ip}:${p.port}`}>
                      <TableCell>
                        <div className="flex items-center gap-2 whitespace-nowrap">
                          <CountryFlag code={inst?.country} iconBox />
                          {inst?.name}
                        </div>
                      </TableCell>
                      <TableCell className="max-w-[320px]">
                        <div className="flex items-center gap-2">
                          {site && <SiteIcon site={site} />}
                          <span className="truncate" title={p.torrent}>{p.torrent}</span>
                        </div>
                      </TableCell>
                      <TableCell>
                        <div className="flex items-center gap-2 whitespace-nowrap font-mono text-xs">
                          <CountryFlag code={p.country} iconBox />
                          {showIPs ? `${p.ip}:${p.port}` : maskIP(p.ip)}
                        </div>
                      </TableCell>
                      <TableCell className="max-w-[160px] truncate text-muted-foreground" title={p.client}>{p.client}</TableCell>
                      <TableCell className={cn("text-right tabular-nums", p.uploadSpeed > 0 && uploadText)}>
                        {p.uploadSpeed ? speed(p.uploadSpeed) : "-"}
                      </TableCell>
                      <TableCell className={cn("text-right tabular-nums", p.downloadSpeed > 0 && downloadText)}>
                        {p.downloadSpeed ? speed(p.downloadSpeed) : "-"}
                      </TableCell>
                      <TableCell className="text-right tabular-nums">{Math.round(p.progress * 100)}%</TableCell>
                    </TableRow>
                  )
                })}
              </TableBody>
            </Table>
            {peers.length > ROWS_SHOWN && (
              <p className="px-6 py-3 text-xs text-muted-foreground">{t("flows.moreRows", { count: peers.length - ROWS_SHOWN })}</p>
            )}
          </div>
        )}
      </SectionCard>
    </div>
  )
}
