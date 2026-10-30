/*
 * Copyright (c) 2026, vertex-qui contributors.
 * SPDX-License-Identifier: GPL-2.0-or-later
 */

import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { api } from "@/lib/api"
import { formatBytes } from "@/lib/utils"
import type { BrushTransferInstance } from "@/types/brush"
import { useQuery } from "@tanstack/react-query"
import { ArrowDown, ArrowUp, ChevronDown, ChevronUp } from "lucide-react"
import { useState } from "react"
import { useTranslation } from "react-i18next"
import { bytesPerSecondToMBs } from "./units"

type SortKey = "name" | "uploaded" | "downloaded" | "uploadSpeed" | "downloadSpeed"

// Instances the server cannot read now have no speed and sort last either way.
function compare(a: BrushTransferInstance, b: BrushTransferInstance, key: SortKey): number {
  if (key === "name") return a.name.localeCompare(b.name)
  const x = a[key]
  const y = b[key]
  if (x === null || y === null) return x === y ? 0 : x === null ? 1 : -1
  return x - y
}

// TodayTransferCard shows what each instance uploaded and downloaded since local
// midnight on the server, and how fast it moves data right now.
export function TodayTransferCard() {
  const { t } = useTranslation("brush")
  const [sort, setSort] = useState<{ key: SortKey, desc: boolean }>({ key: "uploaded", desc: true })
  const { data } = useQuery({
    queryKey: ["brush", "transfer", "today"],
    queryFn: () => api.getBrushTransferToday(),
    refetchInterval: 10000,
  })
  if (!data || data.instances.length === 0) return null

  const sum = (pick: (i: BrushTransferInstance) => number | null) => data.instances.reduce((total, i) => total + (pick(i) ?? 0), 0)
  const speed = (bytes: number | null) => bytes === null ? "-" : t("today.mbs", { value: bytesPerSecondToMBs(bytes) })
  const sorted = [...data.instances].sort((a, b) => {
    const order = compare(a, b, sort.key)
    const bothKnown = sort.key === "name" || (a[sort.key] !== null && b[sort.key] !== null)
    return sort.desc && bothKnown ? -order : order
  })
  const toggle = (key: SortKey) => setSort((s) => s.key === key ? { key, desc: !s.desc } : { key, desc: key !== "name" })
  const header = (key: SortKey, label: string) => (
    <TableHead>
      <button type="button" className="flex items-center gap-1" onClick={() => toggle(key)}>
        {label}
        {sort.key === key && (sort.desc ? <ArrowDown className="size-3" /> : <ArrowUp className="size-3" />)}
      </button>
    </TableHead>
  )

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("today.title")}</CardTitle>
        <CardDescription>{t("today.description", { day: data.day })}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="grid grid-cols-2 gap-4 sm:grid-cols-4">
          <div>
            <p className="text-xs text-muted-foreground">{t("today.uploaded")}</p>
            <p className="flex items-center gap-1 text-lg font-semibold"><ChevronUp className="size-4" />{formatBytes(sum((i) => i.uploaded))}</p>
          </div>
          <div>
            <p className="text-xs text-muted-foreground">{t("today.downloaded")}</p>
            <p className="flex items-center gap-1 text-lg font-semibold"><ChevronDown className="size-4" />{formatBytes(sum((i) => i.downloaded))}</p>
          </div>
          <div>
            <p className="text-xs text-muted-foreground">{t("today.uploadSpeed")}</p>
            <p className="text-lg font-semibold">{speed(sum((i) => i.uploadSpeed))}</p>
          </div>
          <div>
            <p className="text-xs text-muted-foreground">{t("today.downloadSpeed")}</p>
            <p className="text-lg font-semibold">{speed(sum((i) => i.downloadSpeed))}</p>
          </div>
        </div>
        {data.instances.length > 1 && (
          <Table>
            <TableHeader>
              <TableRow>
                {header("name", t("today.instance"))}
                {header("uploaded", t("today.uploaded"))}
                {header("downloaded", t("today.downloaded"))}
                {header("uploadSpeed", t("today.uploadSpeed"))}
                {header("downloadSpeed", t("today.downloadSpeed"))}
              </TableRow>
            </TableHeader>
            <TableBody>
              {sorted.map((inst) => (
                <TableRow key={inst.id}>
                  <TableCell>{inst.name}</TableCell>
                  <TableCell>{formatBytes(inst.uploaded)}</TableCell>
                  <TableCell>{formatBytes(inst.downloaded)}</TableCell>
                  <TableCell>{speed(inst.uploadSpeed)}</TableCell>
                  <TableCell>{speed(inst.downloadSpeed)}</TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        )}
      </CardContent>
    </Card>
  )
}
