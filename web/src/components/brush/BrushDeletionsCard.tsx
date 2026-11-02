/*
 * Copyright (c) 2026, vertex-qui contributors.
 * SPDX-License-Identifier: GPL-2.0-or-later
 */

import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { api } from "@/lib/api"
import { formatBytes } from "@/lib/utils"
import { useQuery } from "@tanstack/react-query"
import { useTranslation } from "react-i18next"

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
    <Card>
      <CardHeader>
        <CardTitle>{t("deletions.title")}</CardTitle>
        <CardDescription>{t("deletions.description")}</CardDescription>
      </CardHeader>
      <CardContent>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t("deletions.time")}</TableHead>
              <TableHead>{t("deletions.instance")}</TableHead>
              <TableHead>{t("deletions.torrent")}</TableHead>
              <TableHead>{t("deletions.size")}</TableHead>
              <TableHead>{t("deletions.age")}</TableHead>
              <TableHead>{t("deletions.uploaded")}</TableHead>
              <TableHead>{t("deletions.ratio")}</TableHead>
              <TableHead>{t("deletions.reason")}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {(!data || data.length === 0) && (
              <TableRow>
                <TableCell colSpan={8} className="text-muted-foreground">{t("deletions.empty")}</TableCell>
              </TableRow>
            )}
            {data?.map((d) => (
              <TableRow key={d.id}>
                <TableCell className="whitespace-nowrap">{new Date(d.deletedAt * 1000).toLocaleString()}</TableCell>
                <TableCell className="whitespace-nowrap">{instanceName(d.instanceId)}</TableCell>
                <TableCell className="max-w-md truncate" title={d.name}>{d.name}</TableCell>
                <TableCell className="whitespace-nowrap">{formatBytes(d.sizeBytes)}</TableCell>
                <TableCell className="whitespace-nowrap">{formatAge(d.ageSeconds)}</TableCell>
                <TableCell className="whitespace-nowrap">{formatBytes(d.uploaded)}</TableCell>
                <TableCell>{d.ratio.toFixed(2)}</TableCell>
                <TableCell className="whitespace-nowrap">
                  {t(`deletions.reasons.${d.reason}`)}
                  {d.filesKept && <span className="text-muted-foreground">{t("deletions.filesKept")}</span>}
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </CardContent>
    </Card>
  )
}
