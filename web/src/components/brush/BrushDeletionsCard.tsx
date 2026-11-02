/*
 * Copyright (c) 2026, vertex-qui contributors.
 * SPDX-License-Identifier: GPL-2.0-or-later
 */

import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { api } from "@/lib/api"
import { formatBytes } from "@/lib/utils"
import { useQuery } from "@tanstack/react-query"
import { useTranslation } from "react-i18next"
import { EmptyState, SectionCard, StateBadge } from "./section"
import { sectionHeaderRowClass, sectionTableClass } from "./sectionStyles"

function formatAge(seconds: number): string {
  const minutes = Math.floor(seconds / 60)
  return minutes < 60 ? `${minutes}m` : `${Math.floor(minutes / 60)}h${String(minutes % 60).padStart(2, "0")}m`
}

// BrushDeletionsCard lists the torrents brush deleted to make room, newest first.
export function BrushDeletionsCard() {
  const { t } = useTranslation("brush")
  const { data } = useQuery({
    queryKey: ["brush", "deletions"],
    queryFn: () => api.getBrushDeletions(),
    refetchInterval: 30000,
  })
  const { data: status } = useQuery({ queryKey: ["brush", "status"], queryFn: () => api.getBrushStatus() })
  const instanceName = (id: number) => status?.instances.find((i) => i.id === id)?.name ?? `#${id}`

  return (
    <SectionCard title={t("deletions.title")} description={t("deletions.description")} info={t("deletions.info")}>
      {data?.length === 0 && <EmptyState>{t("deletions.empty")}</EmptyState>}
      {(data?.length ?? 0) > 0 && (
        <Table className={sectionTableClass}>
          <TableHeader>
            <TableRow className={sectionHeaderRowClass}>
              <TableHead>{t("deletions.time")}</TableHead>
              <TableHead>{t("deletions.instance")}</TableHead>
              <TableHead>{t("deletions.torrent")}</TableHead>
              <TableHead className="text-right">{t("deletions.size")}</TableHead>
              <TableHead className="text-right">{t("deletions.age")}</TableHead>
              <TableHead className="text-right">{t("deletions.uploaded")}</TableHead>
              <TableHead className="text-right">{t("deletions.ratio")}</TableHead>
              <TableHead>{t("deletions.reason")}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {data?.map((d) => (
              <TableRow key={d.id}>
                <TableCell className="whitespace-nowrap">{new Date(d.deletedAt * 1000).toLocaleString()}</TableCell>
                <TableCell className="whitespace-nowrap">{instanceName(d.instanceId)}</TableCell>
                <TableCell className="max-w-md truncate font-medium" title={d.name}>{d.name}</TableCell>
                <TableCell className="whitespace-nowrap text-right font-semibold">{formatBytes(d.sizeBytes)}</TableCell>
                <TableCell className="whitespace-nowrap text-right">{formatAge(d.ageSeconds)}</TableCell>
                <TableCell className="whitespace-nowrap text-right font-semibold">{formatBytes(d.uploaded)}</TableCell>
                <TableCell className="text-right">{d.ratio.toFixed(2)}</TableCell>
                <TableCell>
                  <div className="flex flex-wrap items-center gap-1">
                    <StateBadge tone={d.reason === "emergency" ? "error" : "warn"}>{t(`deletions.reasons.${d.reason}`)}</StateBadge>
                    {d.filesKept && <StateBadge tone="neutral">{t("deletions.filesKept")}</StateBadge>}
                  </div>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      )}
    </SectionCard>
  )
}
