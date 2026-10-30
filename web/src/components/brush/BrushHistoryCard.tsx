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

// splitReason separates the reason code the backend stores from its free-text detail.
function splitReason(reason: string): { code: string, detail: string } {
  const i = reason.indexOf(": ")
  return i === -1 ? { code: reason, detail: "" } : { code: reason.slice(0, i), detail: reason.slice(i + 2) }
}

export function BrushHistoryCard() {
  const { t } = useTranslation("brush")
  const { data: entries } = useQuery({ queryKey: ["brush", "entries"], queryFn: () => api.getBrushEntries(), refetchInterval: 5000 })

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("history.title")}</CardTitle>
        <CardDescription>{t("history.description")}</CardDescription>
      </CardHeader>
      <CardContent>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t("history.time")}</TableHead>
              <TableHead>{t("history.feed")}</TableHead>
              <TableHead>{t("history.torrent")}</TableHead>
              <TableHead>{t("history.size")}</TableHead>
              <TableHead>{t("history.result")}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {entries?.length === 0 && (
              <TableRow>
                <TableCell colSpan={5} className="text-muted-foreground">{t("history.empty")}</TableCell>
              </TableRow>
            )}
            {entries?.map((e) => {
              const { code, detail } = splitReason(e.reason)
              return (
                <TableRow key={e.id}>
                  <TableCell className="whitespace-nowrap">{new Date((e.decidedAt || e.firstSeenAt) * 1000).toLocaleString()}</TableCell>
                  <TableCell>{e.feedName}</TableCell>
                  <TableCell className="max-w-md truncate" title={e.title}>{e.title}</TableCell>
                  <TableCell>{e.sizeBytes ? formatBytes(e.sizeBytes) : "-"}</TableCell>
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
      </CardContent>
    </Card>
  )
}
