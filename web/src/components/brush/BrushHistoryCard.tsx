/*
 * Copyright (c) 2026, vertex-qui contributors.
 * SPDX-License-Identifier: GPL-2.0-or-later
 */

import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { api } from "@/lib/api"
import { formatBytes } from "@/lib/utils"
import type { BrushPlacement } from "@/types/brush"
import { useQuery } from "@tanstack/react-query"
import type { TFunction } from "i18next"
import { ChevronRight } from "lucide-react"
import { useState } from "react"
import { useTranslation } from "react-i18next"
import { bytesPerSecondToMbps } from "./units"

// splitReason separates the reason code the backend stores from its free-text detail.
function splitReason(reason: string): { code: string, detail: string } {
  const i = reason.indexOf(": ")
  return i === -1 ? { code: reason, detail: "" } : { code: reason.slice(0, i), detail: reason.slice(i + 2) }
}

// placementSummary is one line per candidate instance: its cost and the parts of
// it, or why it could not take the torrent.
function placementSummary(placement: BrushPlacement, t: TFunction<"brush">): string {
  return placement.candidates.map((c) => {
    if (c.reason) {
      return t("history.candidateRefused", { name: c.name, reason: t([`reasons.${c.reason}`, "reasons.other"]) })
    }
    return t("history.candidateCost", {
      name: c.name,
      cost: c.cost.toFixed(2),
      space: c.spaceCost.toFixed(2),
      upload: c.uploadCost.toFixed(2),
      rate: c.uploadKnown ? bytesPerSecondToMbps(c.upload) : "?",
      capacity: c.capacity ? bytesPerSecondToMbps(c.capacity) : "?",
      fresh: c.fresh,
    })
  }).join("\n")
}

const PAGE_SIZE = 10
const OPEN_KEY = "brush.history.open"

// The card remembers whether it was open, per browser; it starts open.
function readOpen(): boolean {
  try {
    return localStorage.getItem(OPEN_KEY) !== "false"
  } catch {
    return true
  }
}

export function BrushHistoryCard() {
  const { t } = useTranslation("brush")
  const [open, setOpen] = useState(readOpen)
  const [page, setPage] = useState(0)
  const { data: entries } = useQuery({
    queryKey: ["brush", "entries"],
    queryFn: () => api.getBrushEntries(),
    refetchInterval: 5000,
    enabled: open,
  })
  const pageCount = Math.max(1, Math.ceil((entries?.length ?? 0) / PAGE_SIZE))
  // New entries arrive at the top; a page past the end after a refetch falls back to the last one.
  const current = Math.min(page, pageCount - 1)
  const shown = entries?.slice(current * PAGE_SIZE, (current + 1) * PAGE_SIZE)
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

  return (
    <Card>
      <CardHeader>
        <button type="button" className="flex w-full items-start gap-2 text-left" aria-expanded={open} onClick={toggle}>
          <ChevronRight className={`mt-1 size-4 shrink-0 transition-transform ${open ? "rotate-90" : ""}`} />
          <div className="space-y-1.5">
            <CardTitle>{t("history.title")}</CardTitle>
            <CardDescription>{t("history.description")}</CardDescription>
          </div>
        </button>
      </CardHeader>
      {open && <CardContent className="space-y-4">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t("history.time")}</TableHead>
              <TableHead>{t("history.feed")}</TableHead>
              <TableHead>{t("history.torrent")}</TableHead>
              <TableHead>{t("history.size")}</TableHead>
              <TableHead>{t("history.instance")}</TableHead>
              <TableHead>{t("history.result")}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {entries?.length === 0 && (
              <TableRow>
                <TableCell colSpan={6} className="text-muted-foreground">{t("history.empty")}</TableCell>
              </TableRow>
            )}
            {shown?.map((e) => {
              const placement = e.placement
              const chosen = placement?.candidates.find((c) => c.instanceId === placement.chosen)
              const reason = splitReason(e.reason)
              const code = reason.code
              // With several candidates the stored detail lists raw codes; the placement tooltip says it better.
              const detail = placement && placement.candidates.length > 1 && !chosen ? "" : reason.detail
              return (
                <TableRow key={e.id}>
                  <TableCell className="whitespace-nowrap">{new Date((e.decidedAt || e.firstSeenAt) * 1000).toLocaleString()}</TableCell>
                  <TableCell>{e.feedName}</TableCell>
                  <TableCell className="max-w-md truncate" title={e.title}>{e.title}</TableCell>
                  <TableCell>{e.sizeBytes ? formatBytes(e.sizeBytes) : "-"}</TableCell>
                  <TableCell
                    className={placement ? "whitespace-nowrap underline decoration-dotted cursor-help" : "whitespace-nowrap"}
                    title={placement ? placementSummary(placement, t) : undefined}
                  >
                    {chosen ? chosen.name : placement ? t("history.noInstance") : "-"}
                  </TableCell>
                  <TableCell className={e.status === "added" ? "text-green-600" : e.status === "rejected" ? "text-muted-foreground" : "text-destructive"}>
                    {t([`entryStatus.${e.status}`, "entryStatus.other"])}
                    {code && <> · {t([`reasons.${code}`, "reasons.other"])}</>}
                    {detail && <span className="ml-1 text-xs">({detail})</span>}
                  </TableCell>
                </TableRow>
              )
            })}
          </TableBody>
        </Table>
        {pageCount > 1 && (
          <div className="flex items-center justify-end gap-2 text-sm">
            <span className="text-muted-foreground">{t("history.page", { page: current + 1, pages: pageCount })}</span>
            <Button size="sm" variant="outline" disabled={current === 0} onClick={() => setPage(current - 1)}>
              {t("history.previous")}
            </Button>
            <Button size="sm" variant="outline" disabled={current >= pageCount - 1} onClick={() => setPage(current + 1)}>
              {t("history.next")}
            </Button>
          </div>
        )}
      </CardContent>}
    </Card>
  )
}
