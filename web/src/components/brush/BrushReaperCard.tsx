/*
 * Copyright (c) 2026, vertex-qui contributors.
 * SPDX-License-Identifier: GPL-2.0-or-later
 */

import { Badge } from "@/components/ui/badge"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { api } from "@/lib/api"
import { formatBytes } from "@/lib/utils"
import type { BrushReapInstance, BrushReapMode } from "@/types/brush"
import { useQuery } from "@tanstack/react-query"
import { ChevronRight } from "lucide-react"
import { useState } from "react"
import { useTranslation } from "react-i18next"

const OPEN_KEY = "brush.reaper.open"

// The card remembers whether it was open, per browser; it starts open.
function readOpen(): boolean {
  try {
    return localStorage.getItem(OPEN_KEY) !== "false"
  } catch {
    return true
  }
}

function formatAge(seconds: number): string {
  const minutes = Math.floor(seconds / 60)
  return minutes < 60 ? `${minutes}m` : `${Math.floor(minutes / 60)}h${String(minutes % 60).padStart(2, "0")}m`
}

const modeVariant: Record<BrushReapMode, "default" | "secondary" | "destructive" | "outline"> = {
  ok: "secondary",
  watch: "outline",
  need_space: "default",
  emergency: "destructive",
  unavailable: "outline",
}

// BrushReaperCard shows, per instance, whether brush would delete torrents now and
// which, and every torrent it picked with what that torrent uploaded afterwards.
// Deletion runs in preview mode: nothing is deleted.
export function BrushReaperCard() {
  const { t } = useTranslation("brush")
  const [open, setOpen] = useState(readOpen)
  const { data } = useQuery({
    queryKey: ["brush", "reaper"],
    queryFn: () => api.getBrushReaper(),
    refetchInterval: 15000,
    enabled: open,
  })
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
  const instanceName = (id: number) => data?.instances.find((i) => i.instanceId === id)?.name ?? `#${id}`
  const minutes = (data?.horizonSeconds ?? 1800) / 60
  const lookahead = (data?.lookaheadSeconds ?? 1800) / 60
  const picks = data?.instances.flatMap((inst) => inst.picks.map((p) => ({ ...p, instanceId: inst.instanceId }))) ?? []

  const replacementText = (inst: BrushReapInstance) => {
    const r = inst.replacement
    if (r.value === null) return t("reaper.noReplacement")
    return t("reaper.replacementValue", { value: formatBytes(r.value) })
  }
  const replacementDetail = (inst: BrushReapInstance) => {
    const r = inst.replacement
    return t("reaper.replacementDetail", {
      yield: r.yield === null ? "-" : formatBytes(r.yield),
      from: r.yieldFrom === "all" ? t("reaper.yieldFromAll") : t("reaper.yieldFromInstance"),
      rate: r.arrivalRate.toFixed(1),
      probability: Math.round(r.probability * 100),
      minutes: lookahead,
    })
  }

  return (
    <Card>
      <CardHeader>
        <button type="button" className="flex w-full items-start gap-2 text-left" aria-expanded={open} onClick={toggle}>
          <ChevronRight className={`mt-1 size-4 shrink-0 transition-transform ${open ? "rotate-90" : ""}`} />
          <div className="space-y-1.5">
            <CardTitle className="flex items-center gap-2">
              {t("reaper.title")}
              <Badge variant="outline">{t("reaper.preview")}</Badge>
            </CardTitle>
            <CardDescription>
              {t("reaper.description", {
                minutes: lookahead,
                margin: data?.marginPercent ?? 20,
                keep: (data?.minKeepSeconds ?? 1800) / 60,
                rounds: data?.needRounds ?? 2,
              })}
            </CardDescription>
          </div>
        </button>
      </CardHeader>
      {open && data && <CardContent className="space-y-6">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t("reaper.instance")}</TableHead>
              <TableHead>{t("reaper.mode")}</TableHead>
              <TableHead>{t("reaper.committed")}</TableHead>
              <TableHead>{t("reaper.need", { minutes: lookahead })}</TableHead>
              <TableHead>{t("reaper.replacement")}</TableHead>
              <TableHead>{t("reaper.wouldDelete")}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {data.instances.length === 0 && (
              <TableRow>
                <TableCell colSpan={6} className="text-muted-foreground">{t("reaper.empty")}</TableCell>
              </TableRow>
            )}
            {data.instances.map((inst) => (
              <TableRow key={inst.instanceId}>
                <TableCell className="whitespace-nowrap">{inst.name}</TableCell>
                <TableCell>
                  <Badge variant={modeVariant[inst.mode]} title={inst.error}>{t(`reaper.modes.${inst.mode}`)}</Badge>
                </TableCell>
                <TableCell className="whitespace-nowrap">
                  {inst.mode === "unavailable" ? "-" : t("reaper.committedValue", { committed: formatBytes(inst.committed), floor: formatBytes(inst.floorBytes) })}
                </TableCell>
                <TableCell className="whitespace-nowrap">{formatBytes(inst.need)}</TableCell>
                <TableCell className="whitespace-nowrap">
                  <div>{replacementText(inst)}</div>
                  <div className="text-xs text-muted-foreground">{replacementDetail(inst)}</div>
                </TableCell>
                <TableCell className="whitespace-nowrap">
                  {inst.picks.length === 0
                    ? "-"
                    : t("reaper.wouldDeleteValue", {
                      count: inst.picks.length,
                      size: formatBytes(inst.picks.reduce((sum, p) => sum + p.sizeBytes, 0)),
                      shortfall: formatBytes(inst.shortfall),
                    })}
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>

        {picks.length > 0 && (
          <div className="space-y-2">
            <h3 className="text-sm font-medium">{t("reaper.picksTitle")}</h3>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t("reaper.instance")}</TableHead>
                  <TableHead>{t("reaper.torrent")}</TableHead>
                  <TableHead>{t("reaper.size")}</TableHead>
                  <TableHead>{t("reaper.age")}</TableHead>
                  <TableHead>{t("reaper.predicted", { minutes })}</TableHead>
                  <TableHead>{t("reaper.density")}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {picks.map((p) => (
                  <TableRow key={`${p.instanceId}-${p.hash}`}>
                    <TableCell className="whitespace-nowrap">{instanceName(p.instanceId)}</TableCell>
                    <TableCell className="max-w-md truncate" title={p.name}>{p.name}</TableCell>
                    <TableCell className="whitespace-nowrap">{formatBytes(p.sizeBytes)}</TableCell>
                    <TableCell className="whitespace-nowrap">{formatAge(p.ageSeconds)}</TableCell>
                    <TableCell className="whitespace-nowrap">{p.predicted === null ? t("reaper.unknown") : formatBytes(p.predicted)}</TableCell>
                    <TableCell className="whitespace-nowrap">{p.density === null ? "-" : formatBytes(p.density)}</TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
        )}

        <div className="space-y-2">
          <h3 className="text-sm font-medium">{t("reaper.logTitle")}</h3>
          <p className="text-sm text-muted-foreground">{t("reaper.logDescription", { minutes })}</p>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t("reaper.pickedAt")}</TableHead>
                <TableHead>{t("reaper.instance")}</TableHead>
                <TableHead>{t("reaper.torrent")}</TableHead>
                <TableHead>{t("reaper.reason")}</TableHead>
                <TableHead>{t("reaper.size")}</TableHead>
                <TableHead>{t("reaper.predictedShort")}</TableHead>
                <TableHead>{t("reaper.uploadedAfter")}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {data.recent.length === 0 && (
                <TableRow>
                  <TableCell colSpan={7} className="text-muted-foreground">{t("reaper.logEmpty")}</TableCell>
                </TableRow>
              )}
              {data.recent.map((r) => (
                <TableRow key={r.id}>
                  <TableCell className="whitespace-nowrap">{new Date(r.firstAt * 1000).toLocaleString()}</TableCell>
                  <TableCell className="whitespace-nowrap">{instanceName(r.instanceId)}</TableCell>
                  <TableCell className="max-w-md truncate" title={r.name}>{r.name}</TableCell>
                  <TableCell className="whitespace-nowrap">{t(`reaper.modes.${r.reason}`)}</TableCell>
                  <TableCell className="whitespace-nowrap">{formatBytes(r.sizeBytes)}</TableCell>
                  <TableCell className="whitespace-nowrap">{r.predicted === null ? t("reaper.unknown") : formatBytes(r.predicted)}</TableCell>
                  <TableCell className="whitespace-nowrap">
                    {t(r.gone ? "reaper.uploadedAfterGone" : "reaper.uploadedAfterValue", {
                      value: formatBytes(Math.max(0, r.uploadedLast - r.uploadedFirst)),
                      minutes: Math.round((r.measuredAt - r.firstAt) / 60),
                    })}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      </CardContent>}
    </Card>
  )
}
