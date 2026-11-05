/*
 * Copyright (c) 2026, vertex-qui contributors.
 * SPDX-License-Identifier: GPL-2.0-or-later
 */

import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { api } from "@/lib/api"
import { formatBytes } from "@/lib/utils"
import type { BrushSite, BrushSiteTransferRange } from "@/types/brush"
import { useQuery } from "@tanstack/react-query"
import { useState } from "react"
import { useTranslation } from "react-i18next"
import { SiteIcon } from "./SiteIcon"
import { bytesPerSecondToMBs } from "./units"

const RANGE_KEY = "brush.performance.trafficRange"
const RANGES: BrushSiteTransferRange[] = ["today", "yesterday", "week"]

function readRange(): BrushSiteTransferRange {
  try {
    const v = localStorage.getItem(RANGE_KEY)
    return RANGES.find((r) => r === v) ?? "today"
  } catch {
    return "today"
  }
}

function writeRange(v: BrushSiteTransferRange) {
  try {
    localStorage.setItem(RANGE_KEY, v)
  } catch {
    // Storage can be unavailable; the choice still applies for this visit.
  }
}

// SiteTraffic shows what every site uploaded and downloaded today, yesterday or
// over the last 7 days, with the total over all sites first.
export function SiteTraffic({ sites }: { sites: BrushSite[] | undefined }) {
  const { t } = useTranslation("brush")
  const [range, setRange] = useState(readRange)
  const { data } = useQuery({
    queryKey: ["brush", "sites", "transfer", range],
    queryFn: () => api.getBrushSiteTransfer(range),
    refetchInterval: range === "yesterday" ? false : 60000,
    placeholderData: (prev) => prev,
  })
  const rows = data?.sites ?? []
  const up = rows.reduce((n, s) => n + s.uploaded, 0)
  const down = rows.reduce((n, s) => n + s.downloaded, 0)
  const average = (bytes: number) => data && data.seconds > 0
    ? t("today.mbs", { value: bytesPerSecondToMBs(bytes / data.seconds) })
    : "-"
  const ratio = (u: number, d: number) => d > 0 ? (u / d).toFixed(2) : u > 0 ? "∞" : "-"

  return (
    <div className="space-y-3 border-b p-6">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div>
          <h3 className="text-sm font-medium">{t("performance.traffic.title")}</h3>
          {data && (
            <p className="text-xs text-muted-foreground tabular-nums">
              {data.first === data.last ? data.first : `${data.first} – ${data.last}`}
            </p>
          )}
        </div>
        <Tabs value={range} onValueChange={(v) => { const r = v as BrushSiteTransferRange; setRange(r); writeRange(r) }}>
          <TabsList className="h-8">
            {RANGES.map((r) => <TabsTrigger key={r} value={r} className="text-xs">{t(`performance.traffic.range.${r}`)}</TabsTrigger>)}
          </TabsList>
        </Tabs>
      </div>

      <div className="grid grid-cols-2 gap-4 md:grid-cols-4">
        <Figure label={t("performance.traffic.uploaded")} value={formatBytes(up)} />
        <Figure label={t("performance.traffic.downloaded")} value={formatBytes(down)} />
        <Figure label={t("performance.traffic.averageUpload")} value={average(up)} />
        <Figure label={t("performance.traffic.ratio")} value={ratio(up, down)} />
      </div>

      {rows.length > 0 && (
        <div className="overflow-x-auto rounded-lg border">
          <table className="w-full text-sm">
            <thead className="bg-muted/40 text-xs text-muted-foreground">
              <tr>
                <th className="px-3 py-2 text-left font-normal">{t("performance.traffic.site")}</th>
                <th className="px-3 py-2 text-right font-normal">{t("performance.traffic.uploaded")}</th>
                <th className="px-3 py-2 text-right font-normal">{t("performance.traffic.downloaded")}</th>
                <th className="hidden px-3 py-2 text-right font-normal sm:table-cell">{t("performance.traffic.averageUpload")}</th>
                <th className="px-3 py-2 text-right font-normal">{t("performance.traffic.ratio")}</th>
              </tr>
            </thead>
            <tbody className="divide-y">
              {rows.map((s) => {
                const site = sites?.find((x) => x.id === s.siteId)
                return (
                  <tr key={s.siteId}>
                    <td className="px-3 py-2">
                      <span className="flex min-w-0 items-center gap-2">
                        {site && <SiteIcon site={site} />}
                        <span className="truncate">{s.name}</span>
                      </span>
                    </td>
                    <td className="px-3 py-2 text-right font-medium tabular-nums">{formatBytes(s.uploaded)}</td>
                    <td className="px-3 py-2 text-right tabular-nums">{formatBytes(s.downloaded)}</td>
                    <td className="hidden px-3 py-2 text-right tabular-nums sm:table-cell">{average(s.uploaded)}</td>
                    <td className="px-3 py-2 text-right tabular-nums">{ratio(s.uploaded, s.downloaded)}</td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        </div>
      )}
      <p className="text-xs text-muted-foreground">{t("performance.traffic.footnote")}</p>
    </div>
  )
}

function Figure({ label, value }: { label: string, value: string }) {
  return (
    <div>
      <p className="text-xs text-muted-foreground">{label}</p>
      <p className="mt-1 text-xl font-semibold tabular-nums">{value}</p>
    </div>
  )
}
