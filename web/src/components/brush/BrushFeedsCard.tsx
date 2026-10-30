/*
 * Copyright (c) 2026, vertex-qui contributors.
 * SPDX-License-Identifier: GPL-2.0-or-later
 */

import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Checkbox } from "@/components/ui/checkbox"
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Switch } from "@/components/ui/switch"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { api } from "@/lib/api"
import { formatBytes } from "@/lib/utils"
import type { BrushFeed, BrushTestItem } from "@/types/brush"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { useState } from "react"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"
import { bytesToGB, gbToBytes } from "./units"

export function BrushFeedsCard() {
  const { t } = useTranslation("brush")
  const queryClient = useQueryClient()
  const { data: feeds } = useQuery({ queryKey: ["brush", "feeds"], queryFn: () => api.getBrushFeeds(), refetchInterval: 10000 })
  const { data: sites } = useQuery({ queryKey: ["brush", "sites"], queryFn: () => api.getBrushSites() })
  const { data: status } = useQuery({ queryKey: ["brush", "status"], queryFn: () => api.getBrushStatus() })
  const [editing, setEditing] = useState<BrushFeed | "new" | null>(null)
  const [testing, setTesting] = useState<BrushFeed | null>(null)
  const remove = useMutation({
    mutationFn: (id: number) => api.deleteBrushFeed(id),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["brush"] }),
    onError: (err: Error) => toast.error(t("feeds.deleteFailed"), { description: err.message }),
  })
  const instanceName = (id: number) => id === 0 ? t("feeds.autoInstance") : status?.instances.find((i) => i.id === id)?.name ?? `#${id}`

  return (
    <Card>
      <CardHeader>
        <div className="flex flex-wrap items-start justify-between gap-4">
          <div className="space-y-1.5">
            <CardTitle>{t("feeds.title")}</CardTitle>
            <CardDescription>{t("feeds.description")}</CardDescription>
          </div>
          <Button size="sm" className="shrink-0" disabled={!sites?.length} onClick={() => setEditing("new")}>{t("feeds.add")}</Button>
        </div>
      </CardHeader>
      <CardContent>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t("feeds.name")}</TableHead>
              <TableHead>{t("feeds.instance")}</TableHead>
              <TableHead>{t("feeds.sizeRange")}</TableHead>
              <TableHead>{t("feeds.filters")}</TableHead>
              <TableHead>{t("feeds.lastFetch")}</TableHead>
              <TableHead />
            </TableRow>
          </TableHeader>
          <TableBody>
            {feeds?.length === 0 && (
              <TableRow>
                <TableCell colSpan={6} className="text-muted-foreground">{t("feeds.empty")}</TableCell>
              </TableRow>
            )}
            {feeds?.map((feed) => (
              <TableRow key={feed.id}>
                <TableCell>
                  {feed.name}
                  {!feed.enabled && <span className="ml-2 text-xs text-muted-foreground">{t("feeds.off")}</span>}
                </TableCell>
                <TableCell>{instanceName(feed.instanceId)}</TableCell>
                <TableCell>{t("feeds.sizeRangeValue", { min: bytesToGB(feed.minSizeBytes), max: bytesToGB(feed.maxSizeBytes) })}</TableCell>
                <TableCell>
                  {[feed.freeOnly && t("feeds.freeOnly"), feed.skipHr && t("feeds.skipHr"), t("feeds.windowValue", { minutes: feed.windowSeconds / 60 })]
                    .filter(Boolean)
                    .join(" · ")}
                </TableCell>
                <TableCell className={feed.lastError ? "text-destructive" : undefined}>
                  {feed.lastError
                    ? t("feeds.fetchError", { message: feed.lastError })
                    : feed.lastFetchAt
                      ? new Date(feed.lastFetchAt * 1000).toLocaleTimeString()
                      : t("feeds.neverFetched")}
                </TableCell>
                <TableCell className="text-right space-x-2 whitespace-nowrap">
                  <Button size="sm" variant="outline" onClick={() => setTesting(feed)}>{t("feeds.test")}</Button>
                  <Button size="sm" variant="outline" onClick={() => setEditing(feed)}>{t("actions.edit")}</Button>
                  <Button
                    size="sm"
                    variant="outline"
                    onClick={() => {
                      if (window.confirm(t("feeds.confirmDelete", { name: feed.name }))) remove.mutate(feed.id)
                    }}
                  >
                    {t("actions.delete")}
                  </Button>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </CardContent>
      {editing !== null && <FeedDialog feed={editing === "new" ? null : editing} onClose={() => setEditing(null)} />}
      {testing !== null && <TestDialog feed={testing} onClose={() => setTesting(null)} />}
    </Card>
  )
}

function FeedDialog({ feed, onClose }: { feed: BrushFeed | null, onClose: () => void }) {
  const { t } = useTranslation("brush")
  const queryClient = useQueryClient()
  const { data: sites } = useQuery({ queryKey: ["brush", "sites"], queryFn: () => api.getBrushSites() })
  const { data: status } = useQuery({ queryKey: ["brush", "status"], queryFn: () => api.getBrushStatus() })
  const [form, setForm] = useState({
    name: feed?.name ?? "",
    url: "",
    siteId: String(feed?.siteId ?? sites?.[0]?.id ?? ""),
    // 0 lets brush pick the least busy instance for each torrent.
    instanceId: String(feed?.instanceId ?? 0),
    // NexusPHP sites generally allow one RSS request a minute.
    intervalSeconds: String(feed?.intervalSeconds ?? 60),
    minGB: feed ? bytesToGB(feed.minSizeBytes) : "5",
    maxGB: feed ? bytesToGB(feed.maxSizeBytes) : "50",
    windowMinutes: String((feed?.windowSeconds ?? 300) / 60),
    freeOnly: feed?.freeOnly ?? true,
    skipHr: feed?.skipHr ?? false,
    enabled: feed?.enabled ?? true,
  })
  const set = <K extends keyof typeof form>(key: K, value: (typeof form)[K]) => setForm((prev) => ({ ...prev, [key]: value }))
  const save = useMutation({
    mutationFn: () => api.saveBrushFeed(feed?.id ?? null, {
      name: form.name,
      url: form.url,
      siteId: Number(form.siteId),
      instanceId: Number(form.instanceId),
      intervalSeconds: Number(form.intervalSeconds),
      minSizeBytes: gbToBytes(form.minGB),
      maxSizeBytes: gbToBytes(form.maxGB),
      windowSeconds: Math.round(Number(form.windowMinutes) * 60),
      freeOnly: form.freeOnly,
      skipHr: form.skipHr,
      enabled: form.enabled,
    }),
    onSuccess: () => {
      onClose()
      return queryClient.invalidateQueries({ queryKey: ["brush", "feeds"] })
    },
    onError: (err: Error) => toast.error(t("feeds.saveFailed"), { description: err.message }),
  })

  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="sm:max-w-xl">
        <DialogHeader>
          <DialogTitle>{feed ? t("feeds.editTitle") : t("feeds.addTitle")}</DialogTitle>
        </DialogHeader>
        <div className="grid grid-cols-2 gap-4">
          <div className="col-span-2 space-y-2">
            <Label htmlFor="feed-name">{t("feeds.name")}</Label>
            <Input id="feed-name" value={form.name} onChange={(e) => set("name", e.target.value)} />
          </div>
          <div className="col-span-2 space-y-2">
            <Label htmlFor="feed-url">{t("feeds.url")}</Label>
            <Input
              id="feed-url"
              type="password"
              autoComplete="off"
              value={form.url}
              placeholder={feed ? t("feeds.urlKeep") : ""}
              onChange={(e) => set("url", e.target.value)}
            />
            <p className="text-xs text-muted-foreground">{t("feeds.urlHelp")}</p>
          </div>
          <div className="space-y-2">
            <Label>{t("feeds.site")}</Label>
            <Select value={form.siteId} onValueChange={(v) => set("siteId", v)}>
              <SelectTrigger><SelectValue /></SelectTrigger>
              <SelectContent>
                {sites?.map((s) => <SelectItem key={s.id} value={String(s.id)}>{s.name}</SelectItem>)}
              </SelectContent>
            </Select>
          </div>
          <div className="space-y-2">
            <Label>{t("feeds.instance")}</Label>
            <Select value={form.instanceId} onValueChange={(v) => set("instanceId", v)}>
              <SelectTrigger><SelectValue /></SelectTrigger>
              <SelectContent>
                <SelectItem value="0">{t("feeds.autoInstance")}</SelectItem>
                {status?.instances.map((i) => <SelectItem key={i.id} value={String(i.id)}>{i.name}</SelectItem>)}
              </SelectContent>
            </Select>
            <p className="text-xs text-muted-foreground">
              {form.instanceId === "0" ? t("feeds.autoInstanceHelp") : t("feeds.fixedInstanceHelp")}
            </p>
          </div>
          <div className="space-y-2">
            <Label htmlFor="feed-min">{t("feeds.minSize")}</Label>
            <Input id="feed-min" type="number" min="0" step="any" value={form.minGB} onChange={(e) => set("minGB", e.target.value)} />
          </div>
          <div className="space-y-2">
            <Label htmlFor="feed-max">{t("feeds.maxSize")}</Label>
            <Input id="feed-max" type="number" min="0" step="any" value={form.maxGB} onChange={(e) => set("maxGB", e.target.value)} />
          </div>
          <div className="space-y-2">
            <Label htmlFor="feed-interval">{t("feeds.interval")}</Label>
            <Input id="feed-interval" type="number" min="10" value={form.intervalSeconds} onChange={(e) => set("intervalSeconds", e.target.value)} />
          </div>
          <div className="space-y-2">
            <Label htmlFor="feed-window">{t("feeds.window")}</Label>
            <Input id="feed-window" type="number" min="1" step="any" value={form.windowMinutes} onChange={(e) => set("windowMinutes", e.target.value)} />
          </div>
          <div className="flex items-center gap-2">
            <Checkbox id="feed-free" checked={form.freeOnly} onCheckedChange={(v) => set("freeOnly", v === true)} />
            <Label htmlFor="feed-free">{t("feeds.freeOnly")}</Label>
          </div>
          <div className="flex items-center gap-2">
            <Checkbox id="feed-hr" checked={form.skipHr} onCheckedChange={(v) => set("skipHr", v === true)} />
            <Label htmlFor="feed-hr">{t("feeds.skipHr")}</Label>
          </div>
          <div className="col-span-2 flex items-center gap-2">
            <Switch id="feed-enabled" checked={form.enabled} onCheckedChange={(v) => set("enabled", v)} />
            <Label htmlFor="feed-enabled">{t("feeds.enabled")}</Label>
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={onClose}>{t("actions.cancel")}</Button>
          <Button disabled={save.isPending} onClick={() => save.mutate()}>{t("actions.save")}</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function TestDialog({ feed, onClose }: { feed: BrushFeed, onClose: () => void }) {
  const { t } = useTranslation("brush")
  const { data, error, isFetching } = useQuery({
    queryKey: ["brush", "feeds", feed.id, "test"],
    queryFn: () => api.testBrushFeed(feed.id),
    gcTime: 0,
  })
  const yesNo = (v: boolean | null) => (v === null ? "-" : v ? t("test.yes") : t("test.no"))

  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="sm:max-w-4xl max-h-[90dvh] overflow-y-auto">
        <DialogHeader>
          <DialogTitle>{t("test.title", { name: feed.name })}</DialogTitle>
        </DialogHeader>
        <p className="text-sm text-muted-foreground">{t("test.description")}</p>
        {isFetching && <p className="text-sm text-muted-foreground">{t("test.running")}</p>}
        {error && <p className="text-sm text-destructive">{t("test.failed", { message: error.message })}</p>}
        {data && (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t("test.torrent")}</TableHead>
                <TableHead>{t("test.size")}</TableHead>
                <TableHead>{t("test.age")}</TableHead>
                <TableHead>{t("test.free")}</TableHead>
                <TableHead>{t("test.hr")}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {data.map((item: BrushTestItem, i) => (
                <TableRow key={i}>
                  <TableCell className="max-w-md truncate" title={item.title}>{item.title}</TableCell>
                  <TableCell className={item.sizeOk ? undefined : "text-muted-foreground line-through"}>
                    {item.sizeBytes ? formatBytes(item.sizeBytes) : "-"}
                  </TableCell>
                  <TableCell className={item.inWindow ? undefined : "text-muted-foreground"}>
                    {t("test.ageValue", { minutes: Math.floor(item.ageSeconds / 60) })}
                  </TableCell>
                  <TableCell>{item.error ? <span className="text-destructive">{item.error}</span> : yesNo(item.free)}</TableCell>
                  <TableCell>{item.error ? "-" : yesNo(item.hr)}</TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        )}
      </DialogContent>
    </Dialog>
  )
}
