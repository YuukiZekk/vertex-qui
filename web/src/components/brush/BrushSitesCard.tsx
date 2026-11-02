/*
 * Copyright (c) 2026, vertex-qui contributors.
 * SPDX-License-Identifier: GPL-2.0-or-later
 */

import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { api } from "@/lib/api"
import { formatBytes } from "@/lib/utils"
import type { BrushSite } from "@/types/brush"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { ArrowDown, ArrowUp, ArrowUpDown, ChevronDown, ChevronUp, Pencil, Plug, Trash2 } from "lucide-react"
import { useState } from "react"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"
import { bytesPerSecondToMBs, mbsToBytesPerSecond } from "./units"

type SortKey = "name" | "todayUploaded" | "todayDownloaded" | "uploadSpeed" | "torrents" | "sizeBytes"

function SortIcon({ active, desc }: { active: boolean, desc: boolean }) {
  if (!active) return <ArrowUpDown className="h-3 w-3 text-muted-foreground/50" />
  return desc ? <ArrowDown className="h-3 w-3" /> : <ArrowUp className="h-3 w-3" />
}

// BrushSitesCard lists the sites with what their torrents did today on every
// instance, and is where sites are added and edited. Feeds only pick a site.
export function BrushSitesCard() {
  const { t } = useTranslation("brush")
  const queryClient = useQueryClient()
  const { data: sites } = useQuery({ queryKey: ["brush", "sites"], queryFn: () => api.getBrushSites(), refetchInterval: 10000 })
  const [editing, setEditing] = useState<BrushSite | "new" | null>(null)
  const [sort, setSort] = useState<{ key: SortKey, desc: boolean }>({ key: "todayUploaded", desc: true })
  const remove = useMutation({
    mutationFn: (site: BrushSite) => api.deleteBrushSite(site.id),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["brush"] }),
    onError: (err: Error) => toast.error(t("sitesPage.deleteFailed"), { description: err.message }),
  })
  const test = useMutation({
    mutationFn: (site: BrushSite) => api.testBrushSite(site.id),
    onSuccess: (result, site) => result.ok
      ? toast.success(t("sitesPage.loginOk", { name: site.name }))
      : toast.error(t("sitesPage.loginFailed", { name: site.name }), { description: result.error }),
    onError: (err: Error) => toast.error(t("sitesPage.testFailed"), { description: err.message }),
  })

  const list = sites ?? []
  const sorted = [...list].sort((a, b) => {
    const order = sort.key === "name" ? a.name.localeCompare(b.name) : a[sort.key] - b[sort.key]
    return sort.desc ? -order : order
  })
  const sum = (pick: (s: BrushSite) => number) => list.reduce((total, s) => total + pick(s), 0)
  const speed = (bytes: number) => t("today.mbs", { value: bytesPerSecondToMBs(bytes) })
  const toggleSort = (key: SortKey) => setSort((s) => s.key === key ? { key, desc: !s.desc } : { key, desc: key !== "name" })
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
    <div className="rounded-lg border bg-card">
      <div className="flex flex-col gap-3 px-4 py-4 sm:flex-row sm:items-center sm:justify-between">
        <div className="flex items-center gap-2">
          <h3 className="text-base font-medium">{t("sitesPage.today")}</h3>
          <span className="text-xs text-muted-foreground">{t("sitesPage.count", { count: list.length })}</span>
        </div>
        <div className="flex flex-wrap items-center gap-x-6 gap-y-2 text-sm">
          <div className="flex items-center gap-2">
            <ChevronUp className="h-4 w-4 text-muted-foreground" />
            <span className="text-lg font-semibold">{formatBytes(sum((s) => s.todayUploaded))}</span>
          </div>
          <div className="flex items-center gap-2">
            <ChevronDown className="h-4 w-4 text-muted-foreground" />
            <span className="text-lg font-semibold">{formatBytes(sum((s) => s.todayDownloaded))}</span>
          </div>
          <div className="flex items-center gap-2">
            <span className="text-muted-foreground">{t("today.uploadSpeedShort")}</span>
            <span className="text-lg font-semibold tabular-nums">{speed(sum((s) => s.uploadSpeed))}</span>
          </div>
          <Button size="sm" onClick={() => setEditing("new")}>{t("sitesPage.add")}</Button>
        </div>
      </div>

      {/* Mobile row list */}
      <div className="sm:hidden divide-y border-t">
        {sorted.map((s) => (
          <div key={s.id} className="flex items-center gap-2 px-4 py-2">
            <div className="min-w-0 flex-1">
              <div className="truncate text-sm font-medium">{s.name}</div>
              <div className="flex flex-wrap items-center gap-x-3 text-xs text-muted-foreground tabular-nums">
                <span className="flex items-center gap-0.5"><ChevronUp className="h-3 w-3" />{formatBytes(s.todayUploaded)}</span>
                <span className="flex items-center gap-0.5"><ChevronDown className="h-3 w-3" />{formatBytes(s.todayDownloaded)}</span>
                <span>{speed(s.uploadSpeed)}</span>
                <span>{t("sitesPage.torrentsValue", { count: s.torrents })}</span>
              </div>
            </div>
            <Button variant="ghost" size="icon" className="size-11" aria-label={t("actions.edit")} onClick={() => setEditing(s)}>
              <Pencil className="h-4 w-4" />
            </Button>
          </div>
        ))}
      </div>

      <Table className="hidden sm:table">
        <TableHeader>
          <TableRow className="bg-muted/50">
            {header("name", t("sitesPage.site"))}
            <TableHead>{t("sitesPage.rule")}</TableHead>
            {header("todayUploaded", t("sitesPage.todayUploaded"))}
            {header("todayDownloaded", t("sitesPage.todayDownloaded"))}
            {header("uploadSpeed", t("today.uploadSpeed"))}
            {header("torrents", t("sitesPage.torrents"))}
            {header("sizeBytes", t("sitesPage.size"))}
            <TableHead />
          </TableRow>
        </TableHeader>
        <TableBody>
          {list.length === 0 && (
            <TableRow>
              <TableCell colSpan={8} className="text-muted-foreground">{t("sitesPage.empty")}</TableCell>
            </TableRow>
          )}
          {sorted.map((s) => (
            <TableRow key={s.id}>
              <TableCell>
                <div className="font-medium">{s.name}</div>
                <div className="text-xs text-muted-foreground">
                  {s.baseUrl}
                  {!s.hasCookie && <span className="ml-2 text-destructive">{t("sites.cookieMissing")}</span>}
                  {s.torrentUploadLimit > 0 && <span className="ml-2">{t("sites.uploadLimitValue", { value: bytesPerSecondToMBs(s.torrentUploadLimit) })}</span>}
                </div>
              </TableCell>
              <TableCell>
                <Badge variant="outline">{s.rule}</Badge>
                <div className="mt-1 text-xs text-muted-foreground">{t("sitesPage.feedsValue", { count: s.feeds })}</div>
              </TableCell>
              <TableCell className="text-right font-semibold">{formatBytes(s.todayUploaded)}</TableCell>
              <TableCell className="text-right font-semibold">{formatBytes(s.todayDownloaded)}</TableCell>
              <TableCell className="text-right font-semibold">{speed(s.uploadSpeed)}</TableCell>
              <TableCell className="text-right font-semibold">{s.torrents}</TableCell>
              <TableCell className="text-right font-semibold">{formatBytes(s.sizeBytes)}</TableCell>
              <TableCell className="text-right whitespace-nowrap">
                <Button variant="ghost" size="icon" title={t("sitesPage.test")} disabled={test.isPending} onClick={() => test.mutate(s)}>
                  <Plug className="h-4 w-4" />
                </Button>
                <Button variant="ghost" size="icon" title={t("actions.edit")} onClick={() => setEditing(s)}>
                  <Pencil className="h-4 w-4" />
                </Button>
                <Button
                  variant="ghost"
                  size="icon"
                  title={s.feeds > 0 ? t("sitesPage.deleteHasFeeds") : t("actions.delete")}
                  disabled={s.feeds > 0}
                  onClick={() => window.confirm(t("sitesPage.confirmDelete", { name: s.name })) && remove.mutate(s)}
                >
                  <Trash2 className="h-4 w-4" />
                </Button>
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
      <p className="border-t px-4 py-3 text-xs text-muted-foreground">{t("sitesPage.footnote")}</p>
      {editing !== null && <SiteDialog site={editing === "new" ? null : editing} onClose={() => setEditing(null)} />}
    </div>
  )
}

function SiteDialog({ site, onClose }: { site: BrushSite | null, onClose: () => void }) {
  const { t } = useTranslation("brush")
  const queryClient = useQueryClient()
  const [form, setForm] = useState({
    name: site?.name ?? "",
    baseUrl: site?.baseUrl ?? "",
    cookie: "",
    uploadLimitMBs: site?.torrentUploadLimit ? bytesPerSecondToMBs(site.torrentUploadLimit) : "",
  })
  const set = <K extends keyof typeof form>(key: K, value: (typeof form)[K]) => setForm((prev) => ({ ...prev, [key]: value }))
  const save = useMutation({
    mutationFn: () => api.saveBrushSite(site?.id ?? null, {
      name: form.name.trim(),
      baseUrl: form.baseUrl.trim(),
      cookie: form.cookie,
      torrentUploadLimit: form.uploadLimitMBs ? mbsToBytesPerSecond(form.uploadLimitMBs) : 0,
    }),
    onSuccess: () => {
      onClose()
      return queryClient.invalidateQueries({ queryKey: ["brush"] })
    },
    onError: (err: Error) => toast.error(t("sitesPage.saveFailed"), { description: err.message }),
  })

  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{site ? t("sitesPage.editTitle") : t("sitesPage.addTitle")}</DialogTitle>
        </DialogHeader>
        <div className="space-y-4">
          <div className="space-y-2">
            <Label htmlFor="site-name">{t("sitesPage.site")}</Label>
            <Input id="site-name" value={form.name} onChange={(e) => set("name", e.target.value)} />
          </div>
          <div className="space-y-2">
            <Label htmlFor="site-url">{t("sites.baseUrl")}</Label>
            <Input id="site-url" value={form.baseUrl} placeholder={t("sites.baseUrlPlaceholder")} onChange={(e) => set("baseUrl", e.target.value)} />
            <p className="text-xs text-muted-foreground">{t("sitesPage.baseUrlHelp")}</p>
          </div>
          <div className="space-y-2">
            <Label htmlFor="site-cookie">{t("sites.cookie")}</Label>
            <Input
              id="site-cookie"
              type="password"
              autoComplete="off"
              value={form.cookie}
              placeholder={site?.hasCookie ? t("sites.cookieKeep") : ""}
              onChange={(e) => set("cookie", e.target.value)}
            />
            <p className="text-xs text-muted-foreground">{t("sites.cookieHelp")}</p>
          </div>
          <div className="space-y-2">
            <Label htmlFor="site-limit">{t("sites.uploadLimit")}</Label>
            <Input
              id="site-limit"
              className="w-40"
              type="number"
              min="0"
              step="any"
              value={form.uploadLimitMBs}
              placeholder={t("sites.uploadLimitPlaceholder")}
              onChange={(e) => set("uploadLimitMBs", e.target.value)}
            />
            <p className="text-xs text-muted-foreground">{t("sites.uploadLimitHelp")}</p>
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={onClose}>{t("actions.cancel")}</Button>
          <Button disabled={save.isPending || !form.name.trim() || !form.baseUrl.trim()} onClick={() => save.mutate()}>
            {t("actions.save")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
