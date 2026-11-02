/*
 * Copyright (c) 2026, vertex-qui contributors.
 * SPDX-License-Identifier: GPL-2.0-or-later
 */

import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { api } from "@/lib/api"
import { cn } from "@/lib/utils"
import type { BrushPerfGroup, BrushPerfStats } from "@/types/brush"
import { useQuery } from "@tanstack/react-query"
import { Server } from "lucide-react"
import { useState } from "react"
import { useTranslation } from "react-i18next"
import { InstanceFlag } from "./CountryFlag"
import { EmptyState, SectionCard, StateBadge } from "./section"
import { readOpen, writeOpen } from "./sectionStyles"
import { SiteIcon } from "./SiteIcon"

const BY_KEY = "brush.performance.by"
const HOURS_KEY = "brush.performance.hours"
// Bars share one scale so groups compare at a glance.
const SCALE_SECONDS = 60

function readPref(key: string, fallback: string): string {
  try {
    return localStorage.getItem(key) ?? fallback
  } catch {
    return fallback
  }
}

function writePref(key: string, value: string) {
  try {
    localStorage.setItem(key, value)
  } catch {
    // Storage can be unavailable; the choice still applies for this visit.
  }
}

function formatSeconds(v: number | null): string {
  if (v === null) return "-"
  if (v < 10) return `${v.toFixed(1)} s`
  if (v < 60) return `${Math.round(v)} s`
  const m = Math.floor(v / 60)
  return `${m}m ${String(Math.round(v % 60)).padStart(2, "0")}s`
}

function fastStartRate(s: BrushPerfStats): string {
  return s.fastStartJudged > 0 ? `${((s.fastStart / s.fastStartJudged) * 100).toFixed(1)}%` : "-"
}

function formatClock(unix: number, withDay: boolean): string {
  const d = new Date(unix * 1000)
  return withDay
    ? d.toLocaleString([], { month: "2-digit", day: "2-digit", hour: "2-digit", minute: "2-digit" })
    : d.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })
}

// BrushPerformanceCard shows how fast brush gets torrents in, per site or per
// downloader: discovery (publish to first sight), processing (first sight to
// qBittorrent's confirmation) and how many start uploading within 30 seconds. It
// reports what happened; placement and deletion do not read it.
export function BrushPerformanceCard() {
  const { t } = useTranslation("brush")
  const [open, setOpen] = useState(() => readOpen("brush.performance.open"))
  const [by, setBy] = useState(() => readPref(BY_KEY, "site"))
  const [hours, setHours] = useState(() => readPref(HOURS_KEY, "24"))
  const { data } = useQuery({
    queryKey: ["brush", "performance", by, hours],
    queryFn: () => api.getBrushPerformance(Number(hours), by === "instance" ? "instance" : "site"),
    refetchInterval: 30000,
    enabled: open,
  })
  const { data: sites } = useQuery({ queryKey: ["brush", "sites"], queryFn: () => api.getBrushSites(), enabled: open })
  const o = data?.overall

  const controls = (
    <div className="flex flex-wrap items-center gap-2">
      <Tabs value={by} onValueChange={(v) => { setBy(v); writePref(BY_KEY, v) }}>
        <TabsList className="h-8">
          <TabsTrigger value="site" className="text-xs">{t("performance.bySite")}</TabsTrigger>
          <TabsTrigger value="instance" className="text-xs">{t("performance.byInstance")}</TabsTrigger>
        </TabsList>
      </Tabs>
      <Select value={hours} onValueChange={(v) => { setHours(v); writePref(HOURS_KEY, v) }}>
        <SelectTrigger className="h-8 w-28 text-xs"><SelectValue /></SelectTrigger>
        <SelectContent>
          {["1", "6", "24", "168"].map((h) => <SelectItem key={h} value={h}>{t(`performance.window.${h}`)}</SelectItem>)}
        </SelectContent>
      </Select>
    </div>
  )

  return (
    <SectionCard
      title={t("performance.title")}
      description={t("performance.description")}
      info={t("performance.info", { slow: data?.slowSeconds ?? 20 })}
      actions={controls}
      open={open}
      onOpenChange={(next) => { setOpen(next); writeOpen("brush.performance.open", next) }}
    >
      {o && (
        <div className="grid grid-cols-2 gap-px border-b bg-border md:grid-cols-4">
          <Stat label={t("performance.entryP95")} value={formatSeconds(o.entryP95)} />
          <Stat
            label={t("performance.splitP95")}
            value={`${formatSeconds(o.discoveryP95)} / ${formatSeconds(o.processingP95)}`}
            hint={t("performance.splitHint")}
          />
          <Stat label={t("performance.fastStart")} value={fastStartRate(o)} />
          <Stat
            label={t("performance.outcomes")}
            value={`${o.added} / ${o.pending} / ${o.failed}`}
            hint={t("performance.outcomesHint")}
            tone={o.failed > 0 ? "text-destructive" : undefined}
          />
        </div>
      )}
      {data && data.groups.length === 0 && <EmptyState>{t("performance.empty")}</EmptyState>}
      {data && data.groups.length > 0 && (
        <div className="grid gap-4 p-6 md:grid-cols-2">
          {data.groups.map((g) => (
            <GroupCard
              key={`${g.key}-${g.id}`}
              group={g}
              bucketSeconds={data.bucketSeconds}
              slowSeconds={data.slowSeconds}
              withDay={Number(hours) > 24}
              icon={g.key === "site"
                ? (() => {
                  const site = sites?.find((s) => s.id === g.id)
                  return site ? <SiteIcon site={site} /> : null
                })()
                : <><Server className="h-4 w-4 text-muted-foreground" /><InstanceFlag instanceId={g.id} /></>}
            />
          ))}
        </div>
      )}
    </SectionCard>
  )
}

function Stat({ label, value, hint, tone }: { label: string, value: string, hint?: string, tone?: string }) {
  return (
    <div className="bg-card px-6 py-4">
      <p className="text-xs text-muted-foreground">{label}</p>
      <p className={cn("mt-1 text-2xl font-semibold tabular-nums", tone)}>{value}</p>
      {hint && <p className="mt-0.5 text-xs text-muted-foreground">{hint}</p>}
    </div>
  )
}

function GroupCard({ group, bucketSeconds, slowSeconds, withDay, icon }: {
  group: BrushPerfGroup
  bucketSeconds: number
  slowSeconds: number
  withDay: boolean
  icon: React.ReactNode
}) {
  const { t } = useTranslation("brush")
  const s = group.stats
  const first = group.buckets[0]
  const last = group.buckets[group.buckets.length - 1]

  return (
    <div className="space-y-4 rounded-lg border p-4">
      <div className="flex items-center justify-between gap-2">
        <div className="flex min-w-0 items-center gap-2">
          {icon}
          <span className="truncate font-medium">{group.name}</span>
        </div>
        {group.slow > 0
          ? <StateBadge tone="warn">{t("performance.slowBuckets", { count: group.slow, slow: slowSeconds })}</StateBadge>
          : <StateBadge tone="ok">{t("performance.allFast", { slow: slowSeconds })}</StateBadge>}
      </div>

      <div className="grid grid-cols-2 gap-4">
        <div>
          <p className="text-xs text-muted-foreground">{t("performance.entryP95")}</p>
          <p className="text-xl font-semibold tabular-nums">{formatSeconds(s.entryP95)}</p>
          <p className="text-xs text-muted-foreground tabular-nums">
            {t("performance.splitValue", { discovery: formatSeconds(s.discoveryP95), processing: formatSeconds(s.processingP95) })}
          </p>
        </div>
        <div>
          <p className="text-xs text-muted-foreground">{t("performance.fastStart")}</p>
          <p className="text-xl font-semibold tabular-nums">{fastStartRate(s)}</p>
          <p className="text-xs text-muted-foreground tabular-nums">
            {t("performance.fastStartValue", { fast: s.fastStart, judged: s.fastStartJudged })}
          </p>
        </div>
      </div>

      <div className="space-y-1">
        <div className="flex items-center justify-between text-xs text-muted-foreground">
          <span>{t("performance.barsLabel", { minutes: bucketSeconds / 60 })}</span>
          <span>0–{SCALE_SECONDS} s</span>
        </div>
        <div className="flex h-16 items-end gap-[2px]" role="img" aria-label={t("performance.barsLabel", { minutes: bucketSeconds / 60 })}>
          {group.buckets.map((b) => {
            const p95 = b.entryP95
            const slow = p95 !== null && p95 > slowSeconds
            const height = p95 === null ? 0 : Math.max(8, Math.min(p95, SCALE_SECONDS) / SCALE_SECONDS * 100)
            return (
              <div
                key={b.start}
                className="flex h-full flex-1 items-end"
                title={t("performance.barTitle", {
                  time: formatClock(b.start, withDay),
                  count: b.added,
                  p95: formatSeconds(p95),
                })}
              >
                {p95 === null ? (
                  <div className="h-[3px] w-full rounded-sm bg-muted" />
                ) : (
                  <div
                    className={cn("w-full rounded-sm transition-[height] duration-300",
                      slow ? "bg-amber-500" : "bg-emerald-500/70",
                      b.added < 3 && "opacity-50")}
                    style={{ height: `${height}%` }}
                  />
                )}
              </div>
            )
          })}
        </div>
        {first && last && (
          <div className="flex justify-between text-xs text-muted-foreground tabular-nums">
            <span>{formatClock(first.start, withDay)}</span>
            <span>{formatClock(last.start, withDay)}</span>
          </div>
        )}
      </div>

      <p className="border-t pt-3 text-xs text-muted-foreground">
        {t("performance.footer", { added: s.added, pending: s.pending, failed: s.failed })}
      </p>
    </div>
  )
}
