/*
 * Copyright (c) 2026, vertex-qui contributors.
 * SPDX-License-Identifier: GPL-2.0-or-later
 */

import { api } from "@/lib/api"
import type { BrushTransferInstance } from "@/types/brush"
import { useQuery } from "@tanstack/react-query"
import { useTranslation } from "react-i18next"
import { bytesPerSecondToMBs } from "./units"
import { formatBytes } from "@/lib/utils"

export type TodayRow = BrushTransferInstance & { averageUpload: number | null }

export type TodayTransfer = {
  rows: Map<number, TodayRow>
  totals: { uploaded: number, downloaded: number, uploadSpeed: number, averageUpload: number | null }
} | null

// useTodayTransfer reads what each instance moved since midnight on the server and
// how fast it moves now, for the dashboard's server statistics. The average upload
// is today's upload over the time since midnight.
export function useTodayTransfer(): TodayTransfer {
  const { data } = useQuery({
    queryKey: ["brush", "transfer", "today"],
    queryFn: () => api.getBrushTransferToday(),
    refetchInterval: 10000,
  })
  if (!data || data.instances.length === 0) return null
  const average = (bytes: number) => data.elapsedSeconds > 0 ? bytes / data.elapsedSeconds : null
  const rows = new Map<number, TodayRow>()
  let uploaded = 0, downloaded = 0, uploadSpeed = 0
  for (const i of data.instances) {
    rows.set(i.id, { ...i, averageUpload: average(i.uploaded) })
    uploaded += i.uploaded
    downloaded += i.downloaded
    uploadSpeed += i.uploadSpeed ?? 0
  }
  return { rows, totals: { uploaded, downloaded, uploadSpeed, averageUpload: average(uploaded) } }
}

// useTodayMetrics gives one instance's today figures for the mobile detail drawer.
export function useTodayMetrics() {
  const { t } = useTranslation("brush")
  const speed = (bytes: number | null) => bytes === null ? "-" : t("today.mbs", { value: bytesPerSecondToMBs(bytes) })
  return (today: TodayTransfer, instanceId: number) => {
    const row = today?.rows.get(instanceId)
    if (!row) return []
    return [
      { label: t("today.todayUploaded"), value: formatBytes(row.uploaded) },
      { label: t("today.todayDownloaded"), value: formatBytes(row.downloaded) },
      { label: t("today.uploadSpeed"), value: speed(row.uploadSpeed) },
      { label: t("today.averageUploadShort"), value: speed(row.averageUpload) },
    ]
  }
}
