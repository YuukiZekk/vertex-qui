/*
 * Copyright (c) 2026, vertex-qui contributors.
 * SPDX-License-Identifier: GPL-2.0-or-later
 */

import { Button } from "@/components/ui/button"
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
import { EmptyState, SectionCard, StateBadge } from "./section"
import { sectionHeaderRowClass, sectionTableClass } from "./sectionStyles"
import { bytesPerSecondToMbps, bytesToGB, gbToBytes, mbpsToBytesPerSecond } from "./units"

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
    <SectionCard
      title={t("status.title")}
      description={t("status.description")}
      info={t("status.info")}
      actions={data && (
        <div className="flex items-center gap-2">
          <Label htmlFor="brush-enabled" className="whitespace-nowrap text-sm">{data.enabled ? t("status.enabled") : t("status.disabled")}</Label>
          <Switch id="brush-enabled" checked={data.enabled} disabled={toggle.isPending} onCheckedChange={(v) => toggle.mutate(v)} />
        </div>
      )}
    >
      {isLoading && <p className="px-6 py-4 text-sm text-muted-foreground">{t("status.loading")}</p>}
      {error && <p className="px-6 py-4 text-sm text-destructive">{t("status.loadFailed", { message: error.message })}</p>}
      {data && data.instances.length === 0 && <EmptyState>{t("instances.empty")}</EmptyState>}
      {data && data.instances.length > 0 && (
        <Table className={sectionTableClass}>
          <TableHeader>
            <TableRow className={sectionHeaderRowClass}>
              <TableHead>{t("instances.name")}</TableHead>
              <TableHead>{t("instances.state")}</TableHead>
              <TableHead className="text-right">{t("instances.torrents")}</TableHead>
              <TableHead className="text-right">{t("instances.freeSpace")}</TableHead>
              <TableHead className="text-right">{t("instances.committed")}</TableHead>
              <TableHead>{t("instances.floor")}</TableHead>
              <TableHead className="text-right">{t("instances.upload")}</TableHead>
              <TableHead>{t("instances.capacity")}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {data.instances.map((inst) => <InstanceRow key={inst.id} inst={inst} />)}
          </TableBody>
        </Table>
      )}
    </SectionCard>
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
  const [capacityMbps, setCapacityMbps] = useState(inst.uploadCapacity ? bytesPerSecondToMbps(inst.uploadCapacity) : "")
  const saveCapacity = useMutation({
    mutationFn: () => api.setBrushCapacity(inst.id, mbpsToBytesPerSecond(capacityMbps || "0")),
    onSuccess: () => {
      toast.success(t("instances.capacitySaved"))
      return queryClient.invalidateQueries({ queryKey: ["brush", "status"] })
    },
    onError: (err: Error) => toast.error(t("instances.capacitySaveFailed"), { description: err.message }),
  })
  const healthy = inst.active && !inst.error
  const belowFloor = healthy && inst.floorBytes !== null && inst.committed < inst.floorBytes

  const state = !inst.active
    ? { tone: "neutral" as const, label: t("instances.inactive") }
    : inst.error
      ? { tone: "error" as const, label: t("instances.errorShort"), title: inst.error }
      : inst.floorBytes === null
        ? { tone: "warn" as const, label: t("instances.noFloor") }
        : belowFloor
          ? { tone: "error" as const, label: t("instances.belowFloor") }
          : { tone: "ok" as const, label: t("instances.ok") }

  return (
    <TableRow>
      <TableCell className="font-medium">{inst.name}</TableCell>
      <TableCell><StateBadge tone={state.tone} title={state.title}>{state.label}</StateBadge></TableCell>
      <TableCell className="text-right font-semibold">{healthy ? inst.torrents : "-"}</TableCell>
      <TableCell className="text-right font-semibold">{healthy ? formatBytes(inst.freeSpace) : "-"}</TableCell>
      <TableCell className={belowFloor ? "text-right font-semibold text-destructive" : "text-right font-semibold"}>
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
      <TableCell className="whitespace-nowrap text-right font-semibold">
        {inst.uploadRate === null ? "-" : t("instances.mbpsValue", { value: bytesPerSecondToMbps(inst.uploadRate) })}
      </TableCell>
      <TableCell>
        <div className="flex items-center gap-2">
          <Input
            className="h-8 w-24"
            type="number"
            min="0"
            value={capacityMbps}
            placeholder={t("instances.capacityPlaceholder")}
            disabled={inst.floorBytes === null}
            onChange={(e) => setCapacityMbps(e.target.value)}
          />
          <Button
            size="sm"
            variant="outline"
            disabled={inst.floorBytes === null || Number(capacityMbps) < 0 || saveCapacity.isPending}
            onClick={() => saveCapacity.mutate()}
          >
            {t("actions.save")}
          </Button>
        </div>
      </TableCell>
    </TableRow>
  )
}
