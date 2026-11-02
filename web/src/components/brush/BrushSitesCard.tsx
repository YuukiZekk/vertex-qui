/*
 * Copyright (c) 2026, vertex-qui contributors.
 * SPDX-License-Identifier: GPL-2.0-or-later
 */

import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { api } from "@/lib/api"
import { formatBytes } from "@/lib/utils"
import type { BrushSite } from "@/types/brush"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { ArrowDown, ArrowUp, ArrowUpDown, ChevronDown, ChevronUp, Pencil, Plug, Plus, Trash2 } from "lucide-react"
import { useState } from "react"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"
import { EmptyState, SectionCard, StateBadge } from "./section"
import { sectionHeaderRowClass, sectionTableClass } from "./sectionStyles"
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
  const totals = (
    <span className="flex flex-wrap items-center gap-x-4 gap-y-1">
      <span>{t("sitesPage.count", { count: list.length })}</span>
      <span className="flex items-center gap-1"><ChevronUp className="h-3.5 w-3.5" />{formatBytes(sum((s) => s.todayUploaded))}</span>
      <span className="flex items-center gap-1"><ChevronDown className="h-3.5 w-3.5" />{formatBytes(sum((s) => s.todayDownloaded))}</span>
      <span>{t("today.uploadSpeedShort")} {speed(sum((s) => s.uploadSpeed))}</span>
    </span>
  )

  return (
    <SectionCard
      title={t("sitesPage.today")}
      description={totals}
      info={t("sitesPage.footnote")}
      actions={(
        <Button variant="outline" size="sm" onClick={() => setEditing("new")}>
          <Plus className="h-4 w-4 mr-2" />
          {t("sitesPage.add")}
        </Button>
      )}
    >
      {sites?.length === 0 && <EmptyState>{t("sitesPage.empty")}</EmptyState>}
      {list.length > 0 && (
        <>
          {/* Mobile row list */}
          <div className="sm:hidden divide-y">
            {sorted.map((s) => (
              <div key={s.id} className="flex items-center gap-2 px-6 py-2">
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

          <Table className={`hidden sm:table ${sectionTableClass}`}>
            <TableHeader>
              <TableRow className={sectionHeaderRowClass}>
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
              {sorted.map((s) => (
                <TableRow key={s.id}>
                  <TableCell>
                    <div className="flex items-center gap-2">
                      <span className="font-medium">{s.name}</span>
                      {!s.hasCookie && <StateBadge tone="error">{t("sites.cookieMissing")}</StateBadge>}
                    </div>
                    <div className="text-xs text-muted-foreground">
                      {s.baseUrl}
                      {s.torrentUploadLimit > 0 && <span className="ml-2">{t("sites.uploadLimitValue", { value: bytesPerSecondToMBs(s.torrentUploadLimit) })}</span>}
                    </div>
                  </TableCell>
                  <TableCell>
                    <div className="flex flex-wrap items-center gap-1">
                      <StateBadge tone="neutral">{s.rule}</StateBadge>
                      <span className="text-xs text-muted-foreground">{t("sitesPage.feedsValue", { count: s.feeds })}</span>
                    </div>
                  </TableCell>
                  <TableCell className="text-right font-semibold">{formatBytes(s.todayUploaded)}</TableCell>
                  <TableCell className="text-right font-semibold">{formatBytes(s.todayDownloaded)}</TableCell>
                  <TableCell className="text-right font-semibold">{speed(s.uploadSpeed)}</TableCell>
                  <TableCell className="text-right font-semibold">{s.torrents}</TableCell>
                  <TableCell className="text-right font-semibold">{formatBytes(s.sizeBytes)}</TableCell>
                  <TableCell className="text-right whitespace-nowrap">
                    <Button variant="ghost" size="icon" className="h-7 w-7" title={t("sitesPage.test")} disabled={test.isPending} onClick={() => test.mutate(s)}>
                      <Plug className="h-4 w-4" />
                    </Button>
                    <Button variant="ghost" size="icon" className="h-7 w-7" title={t("actions.edit")} onClick={() => setEditing(s)}>
                      <Pencil className="h-4 w-4" />
                    </Button>
                    <Button
                      variant="ghost"
                      size="icon"
                      className="h-7 w-7"
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
        </>
      )}
      {editing !== null && <SiteDialog site={editing === "new" ? null : editing} onClose={() => setEditing(null)} />}
    </SectionCard>
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
