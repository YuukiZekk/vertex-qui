/*
 * Copyright (c) 2026, vertex-qui contributors.
 * SPDX-License-Identifier: GPL-2.0-or-later
 */

import { TableCell, TableHead } from "@/components/ui/table"
import { formatBytes } from "@/lib/utils"
import { ArrowDown, ArrowUp, ArrowUpDown, ChevronDown, ChevronUp } from "lucide-react"
import { useTranslation } from "react-i18next"
import { bytesPerSecondToMBs } from "./units"
import type { ServerSort, ServerSortKey } from "./useServerStatsSort"
import type { TodayTransfer } from "./useTodayTransfer"

// These pieces add today's transfer to qui's server statistics section, styled
// like its own header figures and columns.

function useSpeed() {
  const { t } = useTranslation("brush")
  return (bytes: number | null) => bytes === null ? "-" : t("today.mbs", { value: bytesPerSecondToMBs(bytes) })
}

export function TodayHeaderStats({ today }: { today: TodayTransfer }) {
  const { t } = useTranslation("brush")
  const speed = useSpeed()
  if (!today) return null
  return (
    <>
      <div className="flex items-center gap-2">
        <span className="text-muted-foreground">{t("today.todayShort")}</span>
        <ChevronUp className="h-4 w-4 text-muted-foreground" />
        <span className="text-lg font-semibold">{formatBytes(today.totals.uploaded)}</span>
      </div>
      <div className="flex items-center gap-2">
        <span className="text-muted-foreground">{t("today.uploadSpeedShort")}</span>
        <span className="text-lg font-semibold tabular-nums">{speed(today.totals.uploadSpeed)}</span>
      </div>
    </>
  )
}

export function TodayHeadCells({ today, sort, onSort }: { today: TodayTransfer, sort: ServerSort | null, onSort: (key: ServerSortKey) => void }) {
  const { t } = useTranslation("brush")
  if (!today) return null
  return (
    <>
      <SortHead column="todayUploaded" label={t("today.todayUploaded")} sort={sort} onSort={onSort} />
      <SortHead column="todayDownloaded" label={t("today.todayDownloaded")} sort={sort} onSort={onSort} />
      <SortHead column="uploadSpeed" label={t("today.currentSpeed")} sort={sort} onSort={onSort} />
      <SortHead column="averageUpload" label={t("today.averageUploadShort")} sort={sort} onSort={onSort} />
    </>
  )
}

export function TodayCells({ today, instanceId }: { today: TodayTransfer, instanceId: number }) {
  const speed = useSpeed()
  if (!today) return null
  const row = today.rows.get(instanceId)
  return (
    <>
      <TableCell className="text-center font-semibold">{row ? formatBytes(row.uploaded) : "-"}</TableCell>
      <TableCell className="text-center font-semibold">{row ? formatBytes(row.downloaded) : "-"}</TableCell>
      <TableCell className="text-center font-semibold whitespace-nowrap">
        {row ? (
          <div className="flex flex-col items-center leading-tight">
            <span className="flex items-center gap-0.5"><ChevronUp className="h-3 w-3 text-muted-foreground" />{speed(row.uploadSpeed)}</span>
            <span className="flex items-center gap-0.5 text-xs text-muted-foreground"><ChevronDown className="h-3 w-3" />{speed(row.downloadSpeed)}</span>
          </div>
        ) : "-"}
      </TableCell>
      <TableCell className="text-center font-semibold whitespace-nowrap">{row ? speed(row.averageUpload) : "-"}</TableCell>
    </>
  )
}

// SortHead is a centred, sortable column head with the up-down icons the tracker
// breakdown uses.
export function SortHead({ column, label, sort, onSort, alignLeft }: {
  column: ServerSortKey
  label: string
  sort: ServerSort | null
  onSort: (key: ServerSortKey) => void
  // The name column starts at the left, padded like the tracker breakdown's.
  alignLeft?: boolean
}) {
  const active = sort?.key === column
  return (
    <TableHead className={alignLeft ? "pl-4 text-left" : "text-center"}>
      <button
        type="button"
        onClick={() => onSort(column)}
        className={`${alignLeft ? "" : "mx-auto "}flex items-center gap-1.5 hover:text-foreground transition-colors rounded px-1 py-0.5 -my-0.5`}
      >
        {label}
        {!active ? <ArrowUpDown className="h-3 w-3 text-muted-foreground/50" /> : sort.desc ? <ArrowDown className="h-3 w-3" /> : <ArrowUp className="h-3 w-3" />}
      </button>
    </TableHead>
  )
}
