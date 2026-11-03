/*
 * Copyright (c) 2026, vertex-qui contributors.
 * SPDX-License-Identifier: GPL-2.0-or-later
 */

import { Button } from "@/components/ui/button"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { api } from "@/lib/api"
import { formatBytes } from "@/lib/utils"
import type { BrushPlacement } from "@/types/brush"
import { useQuery } from "@tanstack/react-query"
import type { TFunction } from "i18next"
import { useState } from "react"
import { useTranslation } from "react-i18next"
import { EmptyState, StateBadge } from "./section"
import { sectionHeaderRowClass, sectionTableClass, type Tone } from "./sectionStyles"
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
const statusTone: Record<string, Tone> = { added: "ok", rejected: "neutral", pending: "neutral", adding: "warn" }

// BrushHistoryLog is what brush decided for each new torrent, newest first, in the
// brush log under qui's logs, only those matching the search when there is one.
export function BrushHistoryLog({ search }: { search: string }) {
  const { t } = useTranslation("brush")
  const [page, setPage] = useState(0)
  const [searched, setSearched] = useState(search)
  // A new search starts again from its first page.
  if (searched !== search) {
    setSearched(search)
    setPage(0)
  }
  const { data: entries } = useQuery({
    queryKey: ["brush", "entries", search],
    queryFn: () => api.getBrushEntries(search),
    placeholderData: (previous) => previous,
    refetchInterval: 5000,
  })
  const pageCount = Math.max(1, Math.ceil((entries?.length ?? 0) / PAGE_SIZE))
  // New entries arrive at the top; a page past the end after a refetch falls back to the last one.
  const current = Math.min(page, pageCount - 1)
  const shown = entries?.slice(current * PAGE_SIZE, (current + 1) * PAGE_SIZE)

  return (
    <>
      <p className="px-6 py-3 text-sm text-muted-foreground">{t("history.description")}</p>
      {entries?.length === 0 && <EmptyState>{search ? t("log.noMatch") : t("history.empty")}</EmptyState>}
      {(entries?.length ?? 0) > 0 && (
        <Table className={sectionTableClass}>
          <TableHeader>
            <TableRow className={sectionHeaderRowClass}>
              <TableHead>{t("history.time")}</TableHead>
              <TableHead>{t("history.feed")}</TableHead>
              <TableHead>{t("history.torrent")}</TableHead>
              <TableHead className="text-right">{t("history.size")}</TableHead>
              <TableHead>{t("history.instance")}</TableHead>
              <TableHead>{t("history.result")}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
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
                  <TableCell className="max-w-md truncate font-medium" title={e.title}>{e.title}</TableCell>
                  <TableCell className="whitespace-nowrap text-right font-semibold">{e.sizeBytes ? formatBytes(e.sizeBytes) : "-"}</TableCell>
                  <TableCell
                    className={placement ? "whitespace-nowrap underline decoration-dotted cursor-help" : "whitespace-nowrap"}
                    title={placement ? placementSummary(placement, t) : undefined}
                  >
                    {chosen ? chosen.name : placement ? t("history.noInstance") : "-"}
                  </TableCell>
                  <TableCell>
                    <div className="flex flex-wrap items-center gap-1">
                      <StateBadge tone={statusTone[e.status] ?? "error"}>{t([`entryStatus.${e.status}`, "entryStatus.other"])}</StateBadge>
                      {code && <span className="text-xs text-muted-foreground">{t([`reasons.${code}`, "reasons.other"])}</span>}
                      {detail && <span className="text-xs text-muted-foreground">({detail})</span>}
                    </div>
                  </TableCell>
                </TableRow>
              )
            })}
          </TableBody>
        </Table>
      )}
      {pageCount > 1 && (
        <div className="flex items-center justify-end gap-2 border-t px-6 py-3 text-sm">
          <span className="text-muted-foreground">{t("history.page", { page: current + 1, pages: pageCount })}</span>
          <Button size="sm" variant="outline" disabled={current === 0} onClick={() => setPage(current - 1)}>
            {t("history.previous")}
          </Button>
          <Button size="sm" variant="outline" disabled={current >= pageCount - 1} onClick={() => setPage(current + 1)}>
            {t("history.next")}
          </Button>
        </div>
      )}
    </>
  )
}
