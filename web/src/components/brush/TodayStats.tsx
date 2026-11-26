/*
 * Copyright (c) 2026, vertex-qui contributors.
 * SPDX-License-Identifier: GPL-2.0-or-later
 */

import { TableCell, TableHead } from "@/components/ui/table"
import { formatBytes } from "@/lib/utils"
import { ArrowDown, ArrowUp, ArrowUpDown } from "lucide-react"
import { useTranslation } from "react-i18next"
import { bytesPerSecondToMBs } from "./units"
import type { ServerSort, ServerSortKey } from "./useServerStatsSort"
import type { TodayTransfer } from "./useTodayTransfer"

// These pieces add today's transfer to the columns of qui's dashboard instance list.

function useSpeed() {
  const { t } = useTranslation("brush")
  return (bytes: number | null) => bytes === null ? "-" : t("today.mbs", { value: bytesPerSecondToMBs(bytes) })
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
    <TableHead className={alignLeft ? "text-left" : "text-center"}>
      <button
        type="button"
        onClick={() => onSort(column)}
        className={`${alignLeft ? "-mx-1 " : "mx-auto "}flex items-center gap-1.5 hover:text-foreground transition-colors rounded px-1 py-0.5 -my-0.5`}
      >
        {label}
        {!active ? <ArrowUpDown className="h-3 w-3 text-muted-foreground/50" /> : sort.desc ? <ArrowDown className="h-3 w-3" /> : <ArrowUp className="h-3 w-3" />}
      </button>
    </TableHead>
  )
}

// The dashboard's instance list shows its own speeds, so it takes only the day's
// transfer and the average upload from the brush module.
export const DASHBOARD_TODAY_COLUMNS = 3

export function DashboardTodayHeads({ today, sort, onSort }: { today: TodayTransfer, sort: ServerSort | null, onSort: (key: ServerSortKey) => void }) {
  const { t } = useTranslation("brush")
  if (!today) return null
  return (
    <>
      <SortHead column="todayUploaded" label={t("today.todayUploaded")} sort={sort} onSort={onSort} />
      <SortHead column="todayDownloaded" label={t("today.todayDownloaded")} sort={sort} onSort={onSort} />
      <SortHead column="averageUpload" label={t("today.averageUploadShort")} sort={sort} onSort={onSort} />
    </>
  )
}

export function DashboardTodayCells({ today, instanceId }: { today: TodayTransfer, instanceId: number }) {
  const speed = useSpeed()
  if (!today) return null
  const row = today.rows.get(instanceId)
  return (
    <>
      <TableCell className="text-center">{row ? formatBytes(row.uploaded) : "-"}</TableCell>
      <TableCell className="text-center text-muted-foreground">{row ? formatBytes(row.downloaded) : "-"}</TableCell>
      <TableCell className="text-center whitespace-nowrap text-muted-foreground">{row ? speed(row.averageUpload) : "-"}</TableCell>
    </>
  )
}
