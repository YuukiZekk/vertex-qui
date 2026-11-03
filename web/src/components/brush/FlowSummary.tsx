/*
 * Copyright (c) 2026, vertex-qui contributors.
 * SPDX-License-Identifier: GPL-2.0-or-later
 */

import { formatBytes } from "@/lib/utils"
import type { BrushFlowCountry, BrushFlowInstance, BrushSite } from "@/types/brush"
import { useMemo } from "react"
import { useTranslation } from "react-i18next"
import { regionOf, type WorldRegion } from "./countryPoints"
import { countryName, heatColor } from "./flowStyle"

type Region = WorldRegion | "unknown"

// A region gets the analysis' attention once it carries this share of the traffic.
const CONCENTRATED = 0.6

interface Share {
  key: string
  bytes: number
  share: number
}

// shares sums bytes by key and orders the keys by their share of the whole.
function shares(rows: Array<{ key: string, bytes: number }>): Share[] {
  const sums = new Map<string, number>()
  for (const r of rows) sums.set(r.key, (sums.get(r.key) ?? 0) + r.bytes)
  const total = [...sums.values()].reduce((a, b) => a + b, 0)
  return [...sums.entries()]
    .map(([key, bytes]) => ({ key, bytes, share: total > 0 ? bytes / total : 0 }))
    .sort((a, b) => b.bytes - a.bytes)
}

const pct = (share: number) => `${Math.round(share * 100)}%`

// FlowSummary reads today's traffic of the servers shown: how it splits over the
// continents, how much the top countries take, and where each site and each
// server finds its peers.
export function FlowSummary({ today, instances, sites }: { today: BrushFlowCountry[], instances: BrushFlowInstance[], sites: BrushSite[] }) {
  const { t, i18n } = useTranslation("brush")
  const names = useMemo(() => {
    try {
      return new Intl.DisplayNames([i18n.language], { type: "region" })
    } catch {
      return null
    }
  }, [i18n.language])

  const summary = useMemo(() => {
    const rows = today.map((d) => ({ ...d, bytes: d.uploaded + d.downloaded, region: (regionOf[d.country] ?? "unknown") as Region }))
    const total = rows.reduce((sum, r) => sum + r.bytes, 0)
    const uploaded = rows.reduce((sum, r) => sum + r.uploaded, 0)
    const regions = shares(rows.map((r) => ({ key: r.region, bytes: r.bytes })))
    const countries = shares(rows.map((r) => ({ key: r.country, bytes: r.bytes })))
    // Where each site, then each server, finds most of its traffic.
    const bestOf = (keyOf: (r: (typeof rows)[number]) => string) => {
      const groups = new Map<string, typeof rows>()
      for (const r of rows) groups.set(keyOf(r), [...(groups.get(keyOf(r)) ?? []), r])
      return [...groups.entries()].map(([key, list]) => {
        const bytes = list.reduce((sum, r) => sum + r.bytes, 0)
        return {
          key, bytes,
          region: shares(list.map((r) => ({ key: r.region, bytes: r.bytes })))[0],
          country: shares(list.map((r) => ({ key: r.country, bytes: r.bytes })))[0],
        }
      }).sort((a, b) => b.bytes - a.bytes)
    }
    return {
      total, uploaded, regions, countries,
      bySite: bestOf((r) => String(r.siteId)),
      byServer: bestOf((r) => String(r.instanceId)),
    }
  }, [today])

  if (summary.total === 0) return null
  const regionName = (key: string) => t(`flows.regions.${key as Region}`)
  const place = (key: string) => key ? countryName(names, key) : t("flows.unknownCountry")
  const siteName = (key: string) => sites.find((s) => String(s.id) === key)?.name ?? t("flows.noSite")
  const serverName = (key: string) => instances.find((i) => String(i.id) === key)?.name ?? `#${key}`
  const top = summary.regions[0]
  const top3 = summary.countries.slice(0, 3)
  const top3Share = top3.reduce((sum, c) => sum + c.share, 0)

  return (
    <div className="space-y-4 text-sm">
      <p className="font-medium">{t("flows.summaryTitle")}</p>
      <p className="text-muted-foreground">
        {t("flows.summaryTotal", { total: formatBytes(summary.total), uploaded: formatBytes(summary.uploaded), share: pct(summary.uploaded / summary.total) })}
      </p>

      <div className="space-y-1.5">
        {summary.regions.map((r) => (
          <div key={r.key} className="flex items-center gap-2 text-xs">
            <span className="w-12 shrink-0 text-muted-foreground">{regionName(r.key)}</span>
            <span className="h-1.5 flex-1 overflow-hidden rounded-full bg-muted">
              <span className="block h-full rounded-full" style={{ width: pct(r.share), background: heatColor(Math.sqrt(r.share)) }} />
            </span>
            <span className="w-9 shrink-0 text-right tabular-nums">{pct(r.share)}</span>
          </div>
        ))}
      </div>

      <p>
        {top.share >= CONCENTRATED
          ? t("flows.summaryConcentrated", { region: regionName(top.key), share: pct(top.share) })
          : t("flows.summarySpread", { region: regionName(top.key), share: pct(top.share) })}
        {" "}
        {t("flows.summaryTopCountries", { countries: top3.map((c) => place(c.key)).join("、"), share: pct(top3Share) })}
      </p>

      <div className="space-y-1">
        <p className="text-xs font-medium text-muted-foreground">{t("flows.summaryBySite")}</p>
        <ul className="space-y-1 text-xs">
          {summary.bySite.map((s) => (
            <li key={s.key} className="flex gap-1">
              <span className="min-w-0 flex-1 truncate">{siteName(s.key)}</span>
              <span className="shrink-0 text-muted-foreground">
                {t("flows.summaryBest", { region: regionName(s.region.key), share: pct(s.region.share), country: place(s.country.key), countryShare: pct(s.country.share) })}
              </span>
            </li>
          ))}
        </ul>
      </div>

      <div className="space-y-1">
        <p className="text-xs font-medium text-muted-foreground">{t("flows.summaryByServer")}</p>
        <ul className="space-y-1 text-xs">
          {summary.byServer.map((s) => (
            <li key={s.key} className="flex gap-1">
              <span className="min-w-0 flex-1 truncate">{serverName(s.key)}</span>
              <span className="shrink-0 text-muted-foreground">
                {t("flows.summaryBest", { region: regionName(s.region.key), share: pct(s.region.share), country: place(s.country.key), countryShare: pct(s.country.share) })}
              </span>
            </li>
          ))}
        </ul>
      </div>
    </div>
  )
}
