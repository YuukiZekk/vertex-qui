/*
 * Copyright (c) 2026, vertex-qui contributors.
 * SPDX-License-Identifier: GPL-2.0-or-later
 */

import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Switch } from "@/components/ui/switch"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { api } from "@/lib/api"
import { formatBytes } from "@/lib/utils"
import type { BrushInstanceStatus } from "@/types/brush"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { useState } from "react"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"
import { bytesToGB, gbToBytes } from "./units"

export function BrushStatusCard() {
  const { t } = useTranslation("brush")
  const queryClient = useQueryClient()
  const { data, error, isLoading } = useQuery({
    queryKey: ["brush", "status"],
    queryFn: () => api.getBrushStatus(),
    refetchInterval: 5000,
  })
  const toggle = useMutation({
    mutationFn: (enabled: boolean) => api.setBrushEnabled(enabled),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["brush", "status"] }),
    onError: (err: Error) => toast.error(t("status.saveFailed"), { description: err.message }),
  })

  return (
    <Card>
      <CardHeader>
        <div className="flex flex-wrap items-start justify-between gap-4">
          <div className="space-y-1.5">
            <CardTitle>{t("status.title")}</CardTitle>
            <CardDescription>{t("status.description")}</CardDescription>
          </div>
          {data && (
            <div className="flex shrink-0 items-center gap-2">
              <Label htmlFor="brush-enabled" className="whitespace-nowrap">{data.enabled ? t("status.enabled") : t("status.disabled")}</Label>
              <Switch id="brush-enabled" checked={data.enabled} disabled={toggle.isPending} onCheckedChange={(v) => toggle.mutate(v)} />
            </div>
          )}
        </div>
      </CardHeader>
      <CardContent>
        {isLoading && <p className="text-sm text-muted-foreground">{t("status.loading")}</p>}
        {error && <p className="text-sm text-destructive">{t("status.loadFailed", { message: error.message })}</p>}
        {data && (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t("instances.name")}</TableHead>
                <TableHead>{t("instances.torrents")}</TableHead>
                <TableHead>{t("instances.freeSpace")}</TableHead>
                <TableHead>{t("instances.committed")}</TableHead>
                <TableHead>{t("instances.floor")}</TableHead>
                <TableHead>{t("instances.state")}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {data.instances.length === 0 && (
                <TableRow>
                  <TableCell colSpan={6} className="text-muted-foreground">{t("instances.empty")}</TableCell>
                </TableRow>
              )}
              {data.instances.map((inst) => <InstanceRow key={inst.id} inst={inst} />)}
            </TableBody>
          </Table>
        )}
      </CardContent>
    </Card>
  )
}

function InstanceRow({ inst }: { inst: BrushInstanceStatus }) {
  const { t } = useTranslation("brush")
  const queryClient = useQueryClient()
  const [floorGB, setFloorGB] = useState(inst.floorBytes === null ? "" : bytesToGB(inst.floorBytes))
  const save = useMutation({
    mutationFn: () => api.setBrushFloor(inst.id, gbToBytes(floorGB)),
    onSuccess: () => {
      toast.success(t("instances.floorSaved"))
      return queryClient.invalidateQueries({ queryKey: ["brush", "status"] })
    },
    onError: (err: Error) => toast.error(t("instances.floorSaveFailed"), { description: err.message }),
  })
  const healthy = inst.active && !inst.error
  const belowFloor = healthy && inst.floorBytes !== null && inst.committed < inst.floorBytes

  return (
    <TableRow>
      <TableCell>{inst.name}</TableCell>
      <TableCell>{healthy ? inst.torrents : "-"}</TableCell>
      <TableCell>{healthy ? formatBytes(inst.freeSpace) : "-"}</TableCell>
      <TableCell className={belowFloor ? "text-destructive" : undefined}>
        {healthy ? formatBytes(Math.max(inst.committed, 0)) : "-"}
      </TableCell>
      <TableCell>
        <div className="flex items-center gap-2">
          <Input
            className="h-8 w-24"
            type="number"
            min="1"
            step="any"
            value={floorGB}
            placeholder={t("instances.floorPlaceholder")}
            onChange={(e) => setFloorGB(e.target.value)}
          />
          <Button size="sm" variant="outline" disabled={!(Number(floorGB) > 0) || save.isPending} onClick={() => save.mutate()}>
            {t("actions.save")}
          </Button>
        </div>
      </TableCell>
      <TableCell className={inst.error || belowFloor ? "text-destructive" : undefined}>
        {!inst.active
          ? t("instances.inactive")
          : inst.error
            ? t("instances.error", { message: inst.error })
            : inst.floorBytes === null
              ? t("instances.noFloor")
              : belowFloor
                ? t("instances.belowFloor")
                : t("instances.ok")}
      </TableCell>
    </TableRow>
  )
}
