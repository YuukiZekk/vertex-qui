/*
 * Copyright (c) 2026, vertex-qui contributors.
 * SPDX-License-Identifier: GPL-2.0-or-later
 */

import { TableCell, TableHead } from "@/components/ui/table"
import { formatBytes } from "@/lib/utils"
import { ChevronDown, ChevronUp } from "lucide-react"
import { useTranslation } from "react-i18next"
import { bytesPerSecondToMBs } from "./units"
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

export function TodayHeadCells({ today }: { today: TodayTransfer }) {
  const { t } = useTranslation("brush")
  if (!today) return null
  return (
    <>
      <TableHead className="text-center">{t("today.todayUploaded")}</TableHead>
      <TableHead className="text-center">{t("today.todayDownloaded")}</TableHead>
      <TableHead className="text-center">{t("today.currentSpeed")}</TableHead>
      <TableHead className="text-center">{t("today.averageUploadShort")}</TableHead>
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
