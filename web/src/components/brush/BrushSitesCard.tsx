/*
 * Copyright (c) 2026, vertex-qui contributors.
 * SPDX-License-Identifier: GPL-2.0-or-later
 */

import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { api } from "@/lib/api"
import type { BrushSite } from "@/types/brush"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { useState } from "react"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"

export function BrushSitesCard() {
  const { t } = useTranslation("brush")
  const queryClient = useQueryClient()
  const { data: sites } = useQuery({ queryKey: ["brush", "sites"], queryFn: () => api.getBrushSites() })
  const [editing, setEditing] = useState<BrushSite | "new" | null>(null)
  const remove = useMutation({
    mutationFn: (id: number) => api.deleteBrushSite(id),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["brush"] }),
    onError: (err: Error) => toast.error(t("sites.deleteFailed"), { description: err.message }),
  })

  return (
    <Card>
      <CardHeader>
        <div className="flex flex-wrap items-start justify-between gap-4">
          <div className="space-y-1.5">
            <CardTitle>{t("sites.title")}</CardTitle>
            <CardDescription>{t("sites.description")}</CardDescription>
          </div>
          <Button size="sm" className="shrink-0" onClick={() => setEditing("new")}>{t("sites.add")}</Button>
        </div>
      </CardHeader>
      <CardContent>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t("sites.name")}</TableHead>
              <TableHead>{t("sites.baseUrl")}</TableHead>
              <TableHead>{t("sites.cookie")}</TableHead>
              <TableHead />
            </TableRow>
          </TableHeader>
          <TableBody>
            {sites?.length === 0 && (
              <TableRow>
                <TableCell colSpan={4} className="text-muted-foreground">{t("sites.empty")}</TableCell>
              </TableRow>
            )}
            {sites?.map((site) => (
              <TableRow key={site.id}>
                <TableCell>{site.name}</TableCell>
                <TableCell>{site.baseUrl}</TableCell>
                <TableCell>{site.hasCookie ? t("sites.cookieSet") : t("sites.cookieMissing")}</TableCell>
                <TableCell className="text-right space-x-2">
                  <Button size="sm" variant="outline" onClick={() => setEditing(site)}>{t("actions.edit")}</Button>
                  <Button
                    size="sm"
                    variant="outline"
                    onClick={() => {
                      if (window.confirm(t("sites.confirmDelete", { name: site.name }))) remove.mutate(site.id)
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
      {editing !== null && <SiteDialog site={editing === "new" ? null : editing} onClose={() => setEditing(null)} />}
    </Card>
  )
}

function SiteDialog({ site, onClose }: { site: BrushSite | null, onClose: () => void }) {
  const { t } = useTranslation("brush")
  const queryClient = useQueryClient()
  const [name, setName] = useState(site?.name ?? "")
  const [baseUrl, setBaseUrl] = useState(site?.baseUrl ?? "")
  const [cookie, setCookie] = useState("")
  const save = useMutation({
    mutationFn: () => api.saveBrushSite(site?.id ?? null, { name, baseUrl, cookie }),
    onSuccess: () => {
      onClose()
      return queryClient.invalidateQueries({ queryKey: ["brush", "sites"] })
    },
    onError: (err: Error) => toast.error(t("sites.saveFailed"), { description: err.message }),
  })

  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{site ? t("sites.editTitle") : t("sites.addTitle")}</DialogTitle>
        </DialogHeader>
        <div className="space-y-4">
          <div className="space-y-2">
            <Label htmlFor="site-name">{t("sites.name")}</Label>
            <Input id="site-name" value={name} onChange={(e) => setName(e.target.value)} />
          </div>
          <div className="space-y-2">
            <Label htmlFor="site-url">{t("sites.baseUrl")}</Label>
            <Input id="site-url" value={baseUrl} placeholder={t("sites.baseUrlPlaceholder")} onChange={(e) => setBaseUrl(e.target.value)} />
          </div>
          <div className="space-y-2">
            <Label htmlFor="site-cookie">{t("sites.cookie")}</Label>
            <Input
              id="site-cookie"
              type="password"
              autoComplete="off"
              value={cookie}
              placeholder={site?.hasCookie ? t("sites.cookieKeep") : ""}
              onChange={(e) => setCookie(e.target.value)}
            />
            <p className="text-xs text-muted-foreground">{t("sites.cookieHelp")}</p>
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
