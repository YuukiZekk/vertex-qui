/*
 * Copyright (c) 2026, vertex-qui contributors.
 * SPDX-License-Identifier: GPL-2.0-or-later
 */

import { Accordion, AccordionContent, AccordionItem, AccordionTrigger } from "@/components/ui/accordion"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { api } from "@/lib/api"
import { formatBytes } from "@/lib/utils"
import type { BrushTransferInstance } from "@/types/brush"
import { useQuery } from "@tanstack/react-query"
import { ArrowDown, ArrowUp, ArrowUpDown, ChevronDown, ChevronUp, Minus, Plus } from "lucide-react"
import { useState } from "react"
import { useTranslation } from "react-i18next"
import { bytesPerSecondToMBs } from "./units"

const OPEN_KEY = "brush.today.open"

type SortKey = "name" | "uploaded" | "downloaded" | "uploadSpeed" | "downloadSpeed" | "averageUpload"

type Row = BrushTransferInstance & { averageUpload: number | null }

// The card remembers whether it was open, per browser, like the dashboard's own sections.
function readOpen(): boolean {
  try {
    return localStorage.getItem(OPEN_KEY) !== "false"
  } catch {
    return true
  }
}

// Instances the server cannot read now have no speed and sort last either way.
function compare(a: Row, b: Row, key: SortKey): number {
  if (key === "name") return a.name.localeCompare(b.name)
  const x = a[key]
  const y = b[key]
  if (x === null || y === null) return x === y ? 0 : x === null ? 1 : -1
  return x - y
}

function SortIcon({ active, desc }: { active: boolean, desc: boolean }) {
  if (!active) return <ArrowUpDown className="h-3 w-3 text-muted-foreground/50" />
  return desc ? <ArrowDown className="h-3 w-3" /> : <ArrowUp className="h-3 w-3" />
}

// TodayTransferCard shows what each instance uploaded and downloaded since local
// midnight on the server, how fast it moves data right now, and its average upload
// today. It follows the look of the dashboard's server statistics section.
export function TodayTransferCard() {
  const { t } = useTranslation("brush")
  const [open, setOpen] = useState(readOpen)
  const [sort, setSort] = useState<{ key: SortKey, desc: boolean }>({ key: "uploaded", desc: true })
  const { data } = useQuery({
    queryKey: ["brush", "transfer", "today"],
    queryFn: () => api.getBrushTransferToday(),
    refetchInterval: 10000,
  })
  if (!data || data.instances.length === 0) return null

  const average = (bytes: number) => data.elapsedSeconds > 0 ? bytes / data.elapsedSeconds : null
  const rows: Row[] = data.instances.map((i) => ({ ...i, averageUpload: average(i.uploaded) }))
  const sum = (pick: (r: Row) => number | null) => rows.reduce((total, r) => total + (pick(r) ?? 0), 0)
  const speed = (bytes: number | null) => bytes === null ? "-" : t("today.mbs", { value: bytesPerSecondToMBs(bytes) })
  const totalUploaded = sum((r) => r.uploaded)
  const sorted = [...rows].sort((a, b) => {
    const order = compare(a, b, sort.key)
    const bothKnown = sort.key === "name" || (a[sort.key] !== null && b[sort.key] !== null)
    return sort.desc && bothKnown ? -order : order
  })
  const toggleSort = (key: SortKey) => setSort((s) => s.key === key ? { key, desc: !s.desc } : { key, desc: key !== "name" })
  const onOpenChange = (value: string) => {
    const next = value === "today"
    setOpen(next)
    try {
      localStorage.setItem(OPEN_KEY, String(next))
    } catch {
      // Storage can be unavailable; the section still toggles for this visit.
    }
  }
  const header = (key: SortKey, label: string) => (
    <TableHead className={key === "name" ? undefined : "text-right"}>
      <button
        type="button"
        onClick={() => toggleSort(key)}
        className={`flex items-center gap-1.5 ${key === "name" ? "" : "ml-auto"} hover:text-foreground transition-colors rounded px-1 py-0.5 -mx-1 -my-0.5`}
      >
        {label}
        <SortIcon active={sort.key === key} desc={sort.desc} />
      </button>
    </TableHead>
  )

  return (
    <Accordion type="single" collapsible className="rounded-lg border bg-card" value={open ? "today" : ""} onValueChange={onOpenChange}>
      <AccordionItem value="today" className="border-0">
        <AccordionTrigger className="px-4 py-4 hover:no-underline hover:bg-muted/50 transition-colors [&>svg]:hidden group">
          <div className="flex w-full flex-col gap-3 sm:flex-row sm:items-center sm:justify-between sm:gap-4">
            <div className="flex items-center gap-2">
              <Plus className="h-4 w-4 text-muted-foreground group-data-[state=open]:hidden" />
              <Minus className="h-4 w-4 text-muted-foreground group-data-[state=closed]:hidden" />
              <h3 className="text-base font-medium">{t("today.title")}</h3>
              <span className="text-xs text-muted-foreground">{data.day}</span>
            </div>
            <div className="flex flex-wrap items-center gap-x-6 gap-y-2 text-sm">
              <div className="flex items-center gap-2">
                <ChevronUp className="h-4 w-4 text-muted-foreground" />
                <span className="text-lg font-semibold">{formatBytes(totalUploaded)}</span>
              </div>
              <div className="flex items-center gap-2">
                <ChevronDown className="h-4 w-4 text-muted-foreground" />
                <span className="text-lg font-semibold">{formatBytes(sum((r) => r.downloaded))}</span>
              </div>
              <div className="flex items-center gap-2">
                <span className="text-muted-foreground">{t("today.uploadSpeedShort")}</span>
                <span className="text-lg font-semibold tabular-nums">{speed(sum((r) => r.uploadSpeed))}</span>
              </div>
              <div className="flex items-center gap-2">
                <span className="text-muted-foreground">{t("today.averageUploadShort")}</span>
                <span className="text-lg font-semibold tabular-nums">{speed(average(totalUploaded))}</span>
              </div>
            </div>
          </div>
        </AccordionTrigger>
        <AccordionContent className="px-0 pb-0">
          {/* Mobile row list: six columns do not fit on a phone */}
          <div className="sm:hidden divide-y">
            {sorted.map((r) => (
              <div key={r.id} className="px-4 py-2">
                <div className="truncate text-sm font-medium">{r.name}</div>
                <div className="flex flex-wrap items-center gap-x-3 text-xs text-muted-foreground tabular-nums">
                  <span className="flex items-center gap-0.5"><ChevronUp className="h-3 w-3" />{formatBytes(r.uploaded)}</span>
                  <span className="flex items-center gap-0.5"><ChevronDown className="h-3 w-3" />{formatBytes(r.downloaded)}</span>
                  <span>{t("today.uploadSpeedShort")} {speed(r.uploadSpeed)}</span>
                  <span>{t("today.averageUploadShort")} {speed(r.averageUpload)}</span>
                </div>
              </div>
            ))}
          </div>

          <Table className="hidden sm:table">
            <TableHeader>
              <TableRow className="bg-muted/50">
                {header("name", t("today.instance"))}
                {header("uploaded", t("today.uploaded"))}
                {header("downloaded", t("today.downloaded"))}
                {header("uploadSpeed", t("today.uploadSpeed"))}
                {header("downloadSpeed", t("today.downloadSpeed"))}
                {header("averageUpload", t("today.averageUpload"))}
              </TableRow>
            </TableHeader>
            <TableBody>
              {sorted.map((r) => (
                <TableRow key={r.id}>
                  <TableCell className="font-medium">{r.name}</TableCell>
                  <TableCell className="text-right font-semibold">{formatBytes(r.uploaded)}</TableCell>
                  <TableCell className="text-right font-semibold">{formatBytes(r.downloaded)}</TableCell>
                  <TableCell className="text-right font-semibold">{speed(r.uploadSpeed)}</TableCell>
                  <TableCell className="text-right font-semibold">{speed(r.downloadSpeed)}</TableCell>
                  <TableCell className="text-right font-semibold">{speed(r.averageUpload)}</TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </AccordionContent>
      </AccordionItem>
    </Accordion>
  )
}
