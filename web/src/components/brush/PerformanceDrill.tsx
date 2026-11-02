/*
 * Copyright (c) 2026, vertex-qui contributors.
 * SPDX-License-Identifier: GPL-2.0-or-later
 */

import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { api } from "@/lib/api"
import { cn, formatBytes } from "@/lib/utils"
import type { BrushPerfDetail } from "@/types/brush"
import { useQuery } from "@tanstack/react-query"
import { ChevronRight } from "lucide-react"
import { Fragment, useState } from "react"
import { useTranslation } from "react-i18next"
import { StateBadge } from "./section"
import type { Tone } from "./sectionStyles"

export type Drill = { by: "site" | "instance", id: number, name: string, from: number, to: number }

const statusTone: Record<string, Tone> = { added: "ok", failed: "error", adding: "warn", unknown: "warn" }

function delta(from: number, to: number): string {
  const s = to - from
  if (s < 60) return `${s} s`
  return `${Math.floor(s / 60)}m ${String(s % 60).padStart(2, "0")}s`
}

function clock(unix: number): string {
  return new Date(unix * 1000).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit", second: "2-digit" })
}

// PerformanceDrill lists the torrents behind one site or VPS of the overview, for
// a period, and opens each into its timeline from publish to first upload.
export function PerformanceDrill({ drill, onClose }: { drill: Drill, onClose: () => void }) {
  const { t } = useTranslation("brush")
  const [open, setOpen] = useState<number | null>(null)
  const { data, isLoading } = useQuery({
    queryKey: ["brush", "performance", "entries", drill],
    queryFn: () => api.getBrushPerformanceEntries(drill.by, drill.id, drill.from, drill.to),
  })
  const range = `${new Date(drill.from * 1000).toLocaleString()} – ${new Date(drill.to * 1000).toLocaleTimeString()}`

  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="sm:max-w-4xl max-h-[90dvh] flex flex-col">
        <DialogHeader className="flex-shrink-0">
          <DialogTitle>{drill.name}</DialogTitle>
          <DialogDescription>{t("performance.drillDescription", { range, count: data?.length ?? 0 })}</DialogDescription>
        </DialogHeader>
        <div className="flex-1 overflow-y-auto min-h-0 pr-1">
          {isLoading && <p className="py-8 text-center text-sm text-muted-foreground">{t("status.loading")}</p>}
          {data && data.length === 0 && <p className="py-8 text-center text-sm text-muted-foreground">{t("performance.drillEmpty")}</p>}
          {data && data.length > 0 && (
            <Table>
              <TableHeader>
                <TableRow className="bg-muted/50">
                  <TableHead className="w-6" />
                  <TableHead>{t("performance.drillSeen")}</TableHead>
                  <TableHead>{t("history.torrent")}</TableHead>
                  <TableHead className="text-right">{t("history.size")}</TableHead>
                  <TableHead>{t("history.result")}</TableHead>
                  <TableHead className="text-right">{t("performance.discovery")}</TableHead>
                  <TableHead className="text-right">{t("performance.processing")}</TableHead>
                  <TableHead className="text-right">{t("performance.firstUpload")}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {data.map((d) => {
                  const expanded = open === d.id
                  return (
                    <Fragment key={d.id}>
                      <TableRow className="cursor-pointer" onClick={() => setOpen(expanded ? null : d.id)}>
                        <TableCell className="w-6 pr-0">
                          <ChevronRight className={cn("h-4 w-4 text-muted-foreground transition-transform", expanded && "rotate-90")} />
                        </TableCell>
                        <TableCell className="whitespace-nowrap tabular-nums">{clock(d.firstSeenAt)}</TableCell>
                        <TableCell className="max-w-xs truncate font-medium" title={d.title}>{d.title}</TableCell>
                        <TableCell className="whitespace-nowrap text-right">{d.sizeBytes ? formatBytes(d.sizeBytes) : "-"}</TableCell>
                        <TableCell>
                          <StateBadge tone={statusTone[d.status] ?? "neutral"} title={d.reason || undefined}>
                            {t([`entryStatus.${d.status}`, "entryStatus.other"])}
                          </StateBadge>
                        </TableCell>
                        <TableCell className="whitespace-nowrap text-right tabular-nums">
                          {d.publishedAt && d.publishedAt <= d.firstSeenAt ? delta(d.publishedAt, d.firstSeenAt) : "-"}
                        </TableCell>
                        <TableCell className="whitespace-nowrap text-right tabular-nums">
                          {d.addedAt ? delta(d.firstSeenAt, d.addedAt) : "-"}
                        </TableCell>
                        <TableCell className="whitespace-nowrap text-right tabular-nums">
                          {d.addedAt && d.firstUploadAt ? delta(d.addedAt, d.firstUploadAt) : "-"}
                        </TableCell>
                      </TableRow>
                      {expanded && (
                        <TableRow className="hover:bg-transparent">
                          <TableCell colSpan={8} className="bg-muted/30 px-6 py-4">
                            <Timeline detail={d} />
                          </TableCell>
                        </TableRow>
                      )}
                    </Fragment>
                  )
                })}
              </TableBody>
            </Table>
          )}
        </div>
      </DialogContent>
    </Dialog>
  )
}

// Timeline shows each stage a torrent went through, with the time it took since
// the stage before. A stage it skipped, like the free check on a feed without
// filters, or one not reached, is shown muted.
function Timeline({ detail: d }: { detail: BrushPerfDetail }) {
  const { t } = useTranslation("brush")
  const steps: Array<{ key: string, at: number }> = [
    { key: "published", at: d.publishedAt && d.publishedAt <= d.firstSeenAt ? d.publishedAt : 0 },
    { key: "seen", at: d.firstSeenAt },
    { key: "inspected", at: d.inspectedAt },
    { key: "fetched", at: d.fetchedAt },
    { key: "added", at: d.addedAt },
    { key: "uploading", at: d.firstUploadAt },
  ]
  let previous = 0
  return (
    <div className="space-y-3">
      <ol className="grid gap-3 sm:grid-cols-6">
        {steps.map((s) => {
          const reached = s.at > 0
          const since = reached && previous > 0 ? delta(previous, s.at) : null
          if (reached) previous = s.at
          return (
            <li key={s.key} className="relative space-y-1 border-l-2 pl-3 sm:border-l-0 sm:border-t-2 sm:pl-0 sm:pt-2"
              style={{ borderColor: reached ? "var(--color-emerald-500, #10b981)" : undefined }}>
              <p className={cn("text-xs font-medium", !reached && "text-muted-foreground")}>{t(`performance.steps.${s.key}`)}</p>
              <p className="text-xs tabular-nums text-muted-foreground">{reached ? clock(s.at) : "—"}</p>
              {since && <p className="text-xs font-semibold tabular-nums">+{since}</p>}
            </li>
          )
        })}
      </ol>
      <p className="text-xs text-muted-foreground">
        {t("performance.drillFeed", { feed: d.feedName })}
        {d.reason && <> · {d.reason}</>}
      </p>
    </div>
  )
}
