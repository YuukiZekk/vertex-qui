/*
 * Copyright (c) 2026, vertex-qui contributors.
 * SPDX-License-Identifier: GPL-2.0-or-later
 */

import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { api } from "@/lib/api"
import { formatBytes } from "@/lib/utils"
import type { BrushTorrentValue } from "@/types/brush"
import { useQuery } from "@tanstack/react-query"
import { ChevronRight } from "lucide-react"
import { useState } from "react"
import { useTranslation } from "react-i18next"
import { bytesPerSecondToMBs } from "./units"

const OPEN_KEY = "brush.values.open"

// The card remembers whether it was open, per browser; it starts open.
function readOpen(): boolean {
  try {
    return localStorage.getItem(OPEN_KEY) !== "false"
  } catch {
    return true
  }
}

// Highest predicted upload first; torrents still being measured go last.
function byPredicted(a: BrushTorrentValue, b: BrushTorrentValue): number {
  if (a.predicted === null || b.predicted === null) return a.predicted === b.predicted ? 0 : a.predicted === null ? 1 : -1
  return b.predicted - a.predicted
}

function formatAge(seconds: number): string {
  const minutes = Math.floor(seconds / 60)
  return minutes < 60 ? `${minutes}m` : `${Math.floor(minutes / 60)}h${String(minutes % 60).padStart(2, "0")}m`
}

// BrushValuesCard shows what each brush torrent is predicted to upload next and how
// close the predictions checked so far came.
export function BrushValuesCard() {
  const { t } = useTranslation("brush")
  const [open, setOpen] = useState(readOpen)
  const { data } = useQuery({
    queryKey: ["brush", "values"],
    queryFn: () => api.getBrushValues(),
    refetchInterval: 10000,
    enabled: open,
  })
  const { data: status } = useQuery({ queryKey: ["brush", "status"], queryFn: () => api.getBrushStatus() })
  const instanceName = (id: number) => status?.instances.find((i) => i.id === id)?.name ?? `#${id}`
  const toggle = () => {
    setOpen((prev) => {
      try {
        localStorage.setItem(OPEN_KEY, String(!prev))
      } catch {
        // Storage can be unavailable; the card still toggles for this visit.
      }
      return !prev
    })
  }
  const minutes = (data?.horizonSeconds ?? 1800) / 60
  const accuracy = data?.accuracy
  const error = accuracy && accuracy.actual > 0 ? Math.round((accuracy.absError / accuracy.actual) * 100) : null

  return (
    <Card>
      <CardHeader>
        <button type="button" className="flex w-full items-start gap-2 text-left" aria-expanded={open} onClick={toggle}>
          <ChevronRight className={`mt-1 size-4 shrink-0 transition-transform ${open ? "rotate-90" : ""}`} />
          <div className="space-y-1.5">
            <CardTitle>{t("values.title")}</CardTitle>
            <CardDescription>{t("values.description", { minutes })}</CardDescription>
          </div>
        </button>
      </CardHeader>
      {open && data && <CardContent className="space-y-6">
        <p className="text-sm">
          {accuracy && accuracy.count > 0
            ? t("values.accuracy", {
              count: accuracy.count,
              predicted: formatBytes(accuracy.predicted),
              actual: formatBytes(accuracy.actual),
              error: error ?? "-",
              gone: accuracy.gone,
            })
            : t("values.noChecks", { minutes })}
        </p>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t("values.instance")}</TableHead>
              <TableHead>{t("values.torrent")}</TableHead>
              <TableHead>{t("values.age")}</TableHead>
              <TableHead>{t("values.leechers")}</TableHead>
              <TableHead>{t("values.rate")}</TableHead>
              <TableHead>{t("values.predicted", { minutes })}</TableHead>
              <TableHead>{t("values.density")}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {data.torrents.length === 0 && (
              <TableRow>
                <TableCell colSpan={7} className="text-muted-foreground">{t("values.empty")}</TableCell>
              </TableRow>
            )}
            {[...data.torrents].sort(byPredicted).map((v) => (
              <TableRow key={`${v.instanceId}-${v.hash}`}>
                <TableCell className="whitespace-nowrap">{instanceName(v.instanceId)}</TableCell>
                <TableCell className="max-w-md truncate" title={v.name}>{v.name}</TableCell>
                <TableCell className="whitespace-nowrap">{formatAge(v.ageSeconds)}</TableCell>
                <TableCell>{v.leechers}</TableCell>
                <TableCell className="whitespace-nowrap">
                  {v.rate === null ? t("values.measuring") : t("values.mbs", { value: bytesPerSecondToMBs(v.rate) })}
                </TableCell>
                <TableCell className="whitespace-nowrap">{v.predicted === null ? "-" : formatBytes(v.predicted)}</TableCell>
                <TableCell className="whitespace-nowrap">{v.density === null ? "-" : formatBytes(v.density)}</TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
        {data.recent.length > 0 && (
          <div className="space-y-2">
            <h3 className="text-sm font-medium">{t("values.recentTitle")}</h3>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t("values.checkedAt")}</TableHead>
                  <TableHead>{t("values.instance")}</TableHead>
                  <TableHead>{t("values.torrent")}</TableHead>
                  <TableHead>{t("values.predictedShort")}</TableHead>
                  <TableHead>{t("values.actual")}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {data.recent.map((p) => (
                  <TableRow key={p.id}>
                    <TableCell className="whitespace-nowrap">{new Date(p.resolvedAt * 1000).toLocaleTimeString()}</TableCell>
                    <TableCell className="whitespace-nowrap">{instanceName(p.instanceId)}</TableCell>
                    <TableCell className="max-w-md truncate" title={p.title || p.infoHash}>{p.title || p.infoHash}</TableCell>
                    <TableCell className="whitespace-nowrap">{formatBytes(p.predicted)}</TableCell>
                    <TableCell className="whitespace-nowrap">{formatBytes(p.actual)}</TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
        )}
      </CardContent>}
    </Card>
  )
}
