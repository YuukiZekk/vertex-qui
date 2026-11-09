/*
 * Copyright (c) 2026, vertex-qui contributors.
 * SPDX-License-Identifier: GPL-2.0-or-later
 */

import { Button } from "@/components/ui/button"
import { Checkbox } from "@/components/ui/checkbox"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { api } from "@/lib/api"
import { formatBytes } from "@/lib/utils"
import type { BrushFeed, BrushTestItem } from "@/types/brush"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { Link } from "@tanstack/react-router"
import { FlaskConical, Pencil, Plus, Trash2 } from "lucide-react"
import { useState } from "react"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"
import { GroupPicker } from "./BrushGroups"
import { useBrushGroups } from "./useBrushGroups"
import { EmptyState, Field, FormSection, SectionCard, StateBadge, ToggleRow } from "./section"
import { SiteIcon } from "./SiteIcon"
import { sectionHeaderRowClass, sectionTableClass } from "./sectionStyles"
import { bytesPerSecondToMBs, bytesToGB, gbToBytes, ratioToPercent } from "./units"

export function BrushFeedsCard() {
  const { t } = useTranslation("brush")
  const queryClient = useQueryClient()
  const { data: feeds } = useQuery({ queryKey: ["brush", "feeds"], queryFn: () => api.getBrushFeeds(), refetchInterval: 10000 })
  const { data: sites } = useQuery({ queryKey: ["brush", "sites"], queryFn: () => api.getBrushSites() })
  const { data: status } = useQuery({ queryKey: ["brush", "status"], queryFn: () => api.getBrushStatus() })
  const [editing, setEditing] = useState<BrushFeed | "new" | null>(null)
  const [testing, setTesting] = useState<BrushFeed | null>(null)
  // Sites are kept on the sites page; deleting a feed leaves its site.
  const remove = useMutation({
    mutationFn: (feed: BrushFeed) => api.deleteBrushFeed(feed.id),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["brush"] }),
    onError: (err: Error) => toast.error(t("feeds.deleteFailed"), { description: err.message }),
  })
  const { data: groups } = useBrushGroups()
  // A feed lists its groups first, then the instances picked one by one.
  const targetNames = (feed: BrushFeed) => feed.instanceIds.length === 0 && feed.groupIds.length === 0
    ? t("feeds.allInstances")
    : [
      ...feed.groupIds.map((id) => t("groups.label", { name: groups?.find((g) => g.id === id)?.name ?? `#${id}` })),
      ...feed.instanceIds.map((id) => status?.instances.find((i) => i.id === id)?.name ?? `#${id}`),
    ].join(", ")
  const siteOf = (feed: BrushFeed) => sites?.find((s) => s.id === feed.siteId)

  return (
    <SectionCard
      title={t("feeds.title")}
      description={t("feeds.description")}
      info={t("feeds.info")}
      actions={(
        <Button variant="outline" size="sm" onClick={() => setEditing("new")}>
          <Plus className="h-4 w-4 mr-2" />
          {t("feeds.add")}
        </Button>
      )}
    >
      {feeds?.length === 0 && <EmptyState>{t("feeds.empty")}</EmptyState>}
      {(feeds?.length ?? 0) > 0 && (
        <Table className={sectionTableClass}>
          <TableHeader>
            <TableRow className={sectionHeaderRowClass}>
              <TableHead>{t("feeds.name")}</TableHead>
              <TableHead>{t("feeds.site")}</TableHead>
              <TableHead>{t("feeds.instance")}</TableHead>
              <TableHead>{t("feeds.sizeRange")}</TableHead>
              <TableHead>{t("feeds.filters")}</TableHead>
              <TableHead>{t("feeds.lastFetch")}</TableHead>
              <TableHead />
            </TableRow>
          </TableHeader>
          <TableBody>
            {feeds?.map((feed) => {
              const site = siteOf(feed)
              return (
                <TableRow key={feed.id}>
                  <TableCell>
                    <div className="flex items-center gap-2">
                      <span className="font-medium">{feed.name}</span>
                      {!feed.enabled && <StateBadge tone="neutral">{t("feeds.off")}</StateBadge>}
                      {site && !site.hasCookie && <StateBadge tone="error">{t("sites.cookieMissing")}</StateBadge>}
                    </div>
                  </TableCell>
                  <TableCell>
                    {site ? (
                      <div className="flex items-center gap-2">
                        <SiteIcon site={site} />
                        <span>{site.name}</span>
                      </div>
                    ) : "-"}
                  </TableCell>
                  <TableCell>{targetNames(feed)}</TableCell>
                  <TableCell>{t("feeds.sizeRangeValue", { min: bytesToGB(feed.minSizeBytes), max: bytesToGB(feed.maxSizeBytes) })}</TableCell>
                  <TableCell className="text-muted-foreground">
                    {[
                      feed.freeOnly && t("feeds.freeOnly"),
                      feed.skipHr && t("feeds.skipHr"),
                      feed.ageFromFirstSeen && t("feeds.ageFromFirstSeenShort"),
                      feed.category && t("feeds.categoryValue", { category: feed.category }),
                      feed.earlyCheckSeconds > 0 && t("feeds.earlyValue", {
                        minutes: feed.earlyCheckSeconds / 60,
                        percent: ratioToPercent(feed.earlyMinRatio),
                      }),
                      t("feeds.windowValue", { minutes: feed.windowSeconds / 60 }),
                      site?.torrentUploadLimit ? t("sites.uploadLimitValue", { value: bytesPerSecondToMBs(site.torrentUploadLimit) }) : "",
                      site?.torrentDownloadLimit ? t("sites.downloadLimitValue", { value: bytesPerSecondToMBs(site.torrentDownloadLimit) }) : "",
                    ]
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
                  <TableCell className="text-right whitespace-nowrap">
                    <Button variant="ghost" size="icon" className="h-7 w-7" title={t("feeds.test")} onClick={() => setTesting(feed)}>
                      <FlaskConical className="h-4 w-4" />
                    </Button>
                    <Button variant="ghost" size="icon" className="h-7 w-7" title={t("actions.edit")} onClick={() => setEditing(feed)}>
                      <Pencil className="h-4 w-4" />
                    </Button>
                    <Button
                      variant="ghost"
                      size="icon"
                      className="h-7 w-7"
                      title={t("actions.delete")}
                      onClick={() => {
                        if (window.confirm(t("feeds.confirmDelete", { name: feed.name }))) remove.mutate(feed)
                      }}
                    >
                      <Trash2 className="h-4 w-4" />
                    </Button>
                  </TableCell>
                </TableRow>
              )
            })}
          </TableBody>
        </Table>
      )}
      {editing !== null && <FeedDialog feed={editing === "new" ? null : editing} onClose={() => setEditing(null)} />}
      {testing !== null && <TestDialog feed={testing} onClose={() => setTesting(null)} />}
    </SectionCard>
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
    // Sites are added on the sites page; a new feed starts on the first one.
    siteId: feed ? String(feed.siteId) : sites?.[0] ? String(sites[0].id) : "",
    // A new feed, or one saved when it meant every instance, starts with every
    // instance that has a floor checked.
    instanceIds: feed && (feed.instanceIds.length || feed.groupIds.length)
      ? feed.instanceIds
      : status?.instances.filter((i) => i.floorBytes !== null).map((i) => i.id) ?? [],
    groupIds: feed?.groupIds ?? [],
    // NexusPHP sites generally allow one RSS request a minute.
    intervalSeconds: String(feed?.intervalSeconds ?? 60),
    minGB: feed ? bytesToGB(feed.minSizeBytes) : "5",
    maxGB: feed ? bytesToGB(feed.maxSizeBytes) : "50",
    windowMinutes: String((feed?.windowSeconds ?? 300) / 60),
    freeOnly: feed?.freeOnly ?? true,
    skipHr: feed?.skipHr ?? false,
    ageFromFirstSeen: feed?.ageFromFirstSeen ?? false,
    category: feed?.category ?? "",
    early: (feed?.earlyCheckSeconds ?? 0) > 0,
    earlyMinutes: feed?.earlyCheckSeconds ? String(feed.earlyCheckSeconds / 60) : "15",
    earlyPercent: feed?.earlyCheckSeconds ? ratioToPercent(feed.earlyMinRatio) : "2",
    enabled: feed?.enabled ?? true,
  })
  const set = <K extends keyof typeof form>(key: K, value: (typeof form)[K]) => setForm((prev) => ({ ...prev, [key]: value }))
  const selectedSite = sites?.find((s) => String(s.id) === form.siteId)
  const save = useMutation({
    mutationFn: async () => {
      if (!selectedSite) throw new Error(t("feeds.noSite"))
      await api.saveBrushFeed(feed?.id ?? null, {
        name: form.name,
        url: form.url,
        siteId: selectedSite.id,
        instanceIds: form.instanceIds,
        groupIds: form.groupIds,
        intervalSeconds: Number(form.intervalSeconds),
        minSizeBytes: gbToBytes(form.minGB),
        maxSizeBytes: gbToBytes(form.maxGB),
        windowSeconds: Math.round(Number(form.windowMinutes) * 60),
        freeOnly: form.freeOnly,
        skipHr: form.skipHr,
        ageFromFirstSeen: form.ageFromFirstSeen,
        category: form.category.trim(),
        earlyCheckSeconds: form.early ? Math.round(Number(form.earlyMinutes) * 60) : 0,
        earlyMinRatio: form.early ? Number(form.earlyPercent) / 100 : 0,
        enabled: form.enabled,
      })
    },
    onSuccess: () => {
      onClose()
      return queryClient.invalidateQueries({ queryKey: ["brush"] })
    },
    onError: (err: Error) => toast.error(t("feeds.saveFailed"), { description: err.message }),
  })

  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="sm:max-w-2xl max-h-[90dvh] flex flex-col">
        <DialogHeader className="flex-shrink-0">
          <DialogTitle>{feed ? t("feeds.editTitle") : t("feeds.addTitle")}</DialogTitle>
          <DialogDescription>{feed ? feed.name : t("feeds.dialogDescription")}</DialogDescription>
        </DialogHeader>
        <div className="flex-1 overflow-y-auto min-h-0 pr-1 space-y-6">
          <FormSection title={t("feeds.sectionFeed")}>
            <div className="grid gap-4 sm:grid-cols-2">
              <Field id="feed-name" label={t("feeds.name")}>
                <Input id="feed-name" value={form.name} onChange={(e) => set("name", e.target.value)} />
              </Field>
              <Field
                label={t("feeds.site")}
                help={<>{t("feeds.siteHelp")} <Link to="/sites" className="underline">{t("sitesNav")}</Link></>}
              >
                {(sites?.length ?? 0) > 0 ? (
                  <Select value={form.siteId} onValueChange={(v) => set("siteId", v)}>
                    <SelectTrigger className="w-full"><SelectValue /></SelectTrigger>
                    <SelectContent>
                      {sites?.map((s) => <SelectItem key={s.id} value={String(s.id)}>{s.name}</SelectItem>)}
                    </SelectContent>
                  </Select>
                ) : (
                  <p className="text-sm text-destructive">{t("feeds.noSite")}</p>
                )}
              </Field>
            </div>
            <Field id="feed-category" label={t("feeds.category")} help={t("feeds.categoryHelp")}>
              <Input id="feed-category" value={form.category} placeholder={t("feeds.categoryPlaceholder")} onChange={(e) => set("category", e.target.value)} />
            </Field>
            <Field id="feed-url" label={t("feeds.url")} help={t("feeds.urlHelp")}>
              <Input
                id="feed-url"
                type="password"
                autoComplete="off"
                value={form.url}
                placeholder={feed ? t("feeds.urlKeep") : ""}
                onChange={(e) => set("url", e.target.value)}
              />
            </Field>
          </FormSection>

          <FormSection title={t("feeds.instances")}>
            <div className="space-y-2">
              <p className="text-sm font-medium">{t("groups.title")}</p>
              <GroupPicker value={form.groupIds} onChange={(ids) => set("groupIds", ids)} />
              <p className="text-xs text-muted-foreground">{t("feeds.groupsHelp")}</p>
            </div>
            <div className="grid gap-2 sm:grid-cols-2">
              {status?.instances.map((i) => (
                <label
                  key={i.id}
                  htmlFor={`feed-instance-${i.id}`}
                  className="flex cursor-pointer items-center gap-3 rounded-lg border p-3 bg-muted/40"
                >
                  <Checkbox
                    id={`feed-instance-${i.id}`}
                    checked={form.instanceIds.includes(i.id)}
                    onCheckedChange={(v) => set("instanceIds", v === true
                      ? [...form.instanceIds, i.id].sort((a, b) => a - b)
                      : form.instanceIds.filter((id) => id !== i.id))}
                  />
                  <span className="min-w-0 truncate text-sm font-medium">{i.name}</span>
                  {i.floorBytes === null && <StateBadge tone="warn">{t("instances.noFloor")}</StateBadge>}
                </label>
              ))}
            </div>
            <p className="text-xs text-muted-foreground">{t("feeds.instancesHelp")}</p>
          </FormSection>

          <FormSection title={t("feeds.sectionFilters")}>
            <div className="grid gap-4 sm:grid-cols-2">
              <Field id="feed-min" label={t("feeds.minSize")}>
                <Input id="feed-min" type="number" min="0" step="any" value={form.minGB} onChange={(e) => set("minGB", e.target.value)} />
              </Field>
              <Field id="feed-max" label={t("feeds.maxSize")}>
                <Input id="feed-max" type="number" min="0" step="any" value={form.maxGB} onChange={(e) => set("maxGB", e.target.value)} />
              </Field>
              <Field id="feed-interval" label={t("feeds.interval")}>
                <Input id="feed-interval" type="number" min="10" value={form.intervalSeconds} onChange={(e) => set("intervalSeconds", e.target.value)} />
              </Field>
              <Field id="feed-window" label={t("feeds.window")}>
                <Input id="feed-window" type="number" min="1" step="any" value={form.windowMinutes} onChange={(e) => set("windowMinutes", e.target.value)} />
              </Field>
            </div>
          </FormSection>

          <FormSection title={t("feeds.sectionRules")}>
            <div className="space-y-3">
              <ToggleRow
                id="feed-free"
                label={t("feeds.freeOnly")}
                description={t("feeds.freeOnlyHelp")}
                checked={form.freeOnly}
                onCheckedChange={(v) => set("freeOnly", v)}
              />
              <ToggleRow
                id="feed-hr"
                label={t("feeds.skipHr")}
                description={t("feeds.skipHrHelp")}
                checked={form.skipHr}
                onCheckedChange={(v) => set("skipHr", v)}
              />
              <ToggleRow
                id="feed-first-seen"
                label={t("feeds.ageFromFirstSeen")}
                description={t("feeds.ageFromFirstSeenHelp")}
                checked={form.ageFromFirstSeen}
                onCheckedChange={(v) => set("ageFromFirstSeen", v)}
              />
              <ToggleRow
                id="feed-early"
                label={t("feeds.early")}
                description={t("feeds.earlyHelp")}
                checked={form.early}
                onCheckedChange={(v) => set("early", v)}
              />
              {form.early && (
                <div className="grid gap-4 sm:grid-cols-2">
                  <Field id="feed-early-minutes" label={t("feeds.earlyMinutes")}>
                    <Input id="feed-early-minutes" type="number" min="5" max="360" step="any" value={form.earlyMinutes} onChange={(e) => set("earlyMinutes", e.target.value)} />
                  </Field>
                  <Field id="feed-early-percent" label={t("feeds.earlyPercent")}>
                    <Input id="feed-early-percent" type="number" min="0" max="100" step="any" value={form.earlyPercent} onChange={(e) => set("earlyPercent", e.target.value)} />
                  </Field>
                </div>
              )}
              <ToggleRow
                id="feed-enabled"
                label={t("feeds.enabled")}
                description={t("feeds.enabledHelp")}
                checked={form.enabled}
                onCheckedChange={(v) => set("enabled", v)}
              />
            </div>
          </FormSection>
        </div>
        <DialogFooter className="flex-shrink-0 border-t pt-4">
          <Button variant="outline" onClick={onClose}>{t("actions.cancel")}</Button>
          <Button disabled={save.isPending || (form.instanceIds.length === 0 && form.groupIds.length === 0) || !selectedSite} onClick={() => save.mutate()}>{t("actions.save")}</Button>
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
                <TableHead>{t("test.rule")}</TableHead>
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
                  <TableCell className="text-muted-foreground">{item.rule || "-"}</TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        )}
      </DialogContent>
    </Dialog>
  )
}
