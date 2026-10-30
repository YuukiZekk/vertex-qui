/*
 * Copyright (c) 2026, vertex-qui contributors.
 * SPDX-License-Identifier: GPL-2.0-or-later
 */

import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { api } from "@/lib/api"
import { formatBytes } from "@/lib/utils"
import { useQuery } from "@tanstack/react-query"
import { ChevronDown, ChevronUp } from "lucide-react"
import { useTranslation } from "react-i18next"
import { bytesPerSecondToMbps } from "./units"

// TodayTransferCard shows what each instance uploaded and downloaded since local
// midnight on the server, and the day's average upload speed.
export function TodayTransferCard() {
  const { t } = useTranslation("brush")
  const { data } = useQuery({
    queryKey: ["brush", "transfer", "today"],
    queryFn: () => api.getBrushTransferToday(),
    refetchInterval: 60000,
  })
  if (!data || data.instances.length === 0) return null

  const uploaded = data.instances.reduce((sum, i) => sum + i.uploaded, 0)
  const downloaded = data.instances.reduce((sum, i) => sum + i.downloaded, 0)
  const average = (bytes: number) => data.elapsed > 0 ? t("today.mbps", { value: bytesPerSecondToMbps(bytes / data.elapsed) }) : "-"

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("today.title")}</CardTitle>
        <CardDescription>{t("today.description", { day: data.day })}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="grid grid-cols-2 gap-4 sm:grid-cols-3">
          <div>
            <p className="text-xs text-muted-foreground">{t("today.uploaded")}</p>
            <p className="flex items-center gap-1 text-lg font-semibold"><ChevronUp className="size-4" />{formatBytes(uploaded)}</p>
          </div>
          <div>
            <p className="text-xs text-muted-foreground">{t("today.downloaded")}</p>
            <p className="flex items-center gap-1 text-lg font-semibold"><ChevronDown className="size-4" />{formatBytes(downloaded)}</p>
          </div>
          <div>
            <p className="text-xs text-muted-foreground">{t("today.averageUpload")}</p>
            <p className="text-lg font-semibold">{average(uploaded)}</p>
          </div>
        </div>
        {data.instances.length > 1 && (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t("today.instance")}</TableHead>
                <TableHead>{t("today.uploaded")}</TableHead>
                <TableHead>{t("today.downloaded")}</TableHead>
                <TableHead>{t("today.averageUpload")}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {data.instances.map((inst) => (
                <TableRow key={inst.id}>
                  <TableCell>{inst.name}</TableCell>
                  <TableCell>{formatBytes(inst.uploaded)}</TableCell>
                  <TableCell>{formatBytes(inst.downloaded)}</TableCell>
                  <TableCell>{average(inst.uploaded)}</TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        )}
      </CardContent>
    </Card>
  )
}
