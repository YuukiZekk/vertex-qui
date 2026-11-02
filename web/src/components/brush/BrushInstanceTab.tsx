/*
 * Copyright (c) 2026, vertex-qui contributors.
 * SPDX-License-Identifier: GPL-2.0-or-later
 */

import { PreferencesFormShell } from "@/components/instances/preferences/PreferencesFormShell"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { TabsContent } from "@/components/ui/tabs"
import { api } from "@/lib/api"
import { formatBytes } from "@/lib/utils"
import type { BrushInstanceStatus } from "@/types/brush"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { useState, type FormEvent } from "react"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"
import { countryOptions } from "./countries"
import { CountryFlag } from "./CountryFlag"
import { bytesPerSecondToMbps, bytesToGB, gbToBytes, mbpsToBytesPerSecond } from "./units"

// BrushInstanceTab is the brush tab of qui's instance settings dialog: the safety
// floor and upload capacity brush uses for this instance, with its space and upload
// now. It is laid out like qui's own preference tabs.
export function BrushInstanceTab({ instanceId }: { instanceId: number }) {
  const { t } = useTranslation("brush")
  const { data } = useQuery({ queryKey: ["brush", "status"], queryFn: () => api.getBrushStatus(), refetchInterval: 5000 })
  const inst = data?.instances.find((i) => i.id === instanceId)

  return (
    <TabsContent value="brush" className="mt-6 flex min-h-0 flex-1 flex-col overflow-hidden">
      {inst ? <BrushInstanceForm key={`${inst.floorBytes}-${inst.uploadCapacity}-${inst.countryCode}`} inst={inst} /> : (
        <p className="py-8 text-center text-sm text-muted-foreground">{t("status.loading")}</p>
      )}
    </TabsContent>
  )
}

function BrushInstanceForm({ inst }: { inst: BrushInstanceStatus }) {
  const { t, i18n } = useTranslation("brush")
  const queryClient = useQueryClient()
  const [country, setCountry] = useState(inst.countryCode)
  const [floorGB, setFloorGB] = useState(inst.floorBytes === null ? "" : bytesToGB(inst.floorBytes))
  const [capacityMbps, setCapacityMbps] = useState(inst.uploadCapacity ? bytesPerSecondToMbps(inst.uploadCapacity) : "")
  // The capacity is stored with the floor, so the floor is saved first.
  const save = useMutation({
    mutationFn: async () => {
      await api.setBrushFloor(inst.id, gbToBytes(floorGB))
      await api.setBrushCapacity(inst.id, mbpsToBytesPerSecond(capacityMbps || "0"))
      await api.setBrushCountry(inst.id, country)
    },
    onSuccess: () => {
      toast.success(t("instanceTab.saved"))
      return queryClient.invalidateQueries({ queryKey: ["brush"] })
    },
    onError: (err: Error) => toast.error(t("instanceTab.saveFailed"), { description: err.message }),
  })
  const onSubmit = (e: FormEvent) => {
    e.preventDefault()
    save.mutate()
  }
  const healthy = inst.active && !inst.error
  const facts = [
    { label: t("instances.freeSpace"), value: healthy ? formatBytes(inst.freeSpace) : "-" },
    { label: t("instances.committed"), value: healthy ? formatBytes(Math.max(inst.committed, 0)) : "-" },
    { label: t("instances.torrents"), value: healthy ? String(inst.torrents) : "-" },
    { label: t("instances.upload"), value: inst.uploadRate === null ? "-" : t("instances.mbpsValue", { value: bytesPerSecondToMbps(inst.uploadRate) }) },
  ]

  return (
    <PreferencesFormShell
      onSubmit={onSubmit}
      footer={(
        <Button type="submit" disabled={save.isPending || !(Number(floorGB) > 0) || Number(capacityMbps) < 0} className="min-w-32">
          {t("actions.save")}
        </Button>
      )}
    >
      <div className="space-y-8">
        <section className="space-y-4">
          <div className="space-y-1">
            <h3 className="text-base font-medium">{t("instanceTab.limitsTitle")}</h3>
            <p className="text-sm text-muted-foreground">{t("instanceTab.limitsDescription")}</p>
          </div>
          <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
            <div className="space-y-2">
              <Label htmlFor="brush-floor">{t("instances.floor")}</Label>
              <Input id="brush-floor" type="number" min="1" step="any" value={floorGB} placeholder={t("instances.floorPlaceholder")}
                onChange={(e) => setFloorGB(e.target.value)} />
              <p className="text-xs text-muted-foreground">{t("instanceTab.floorHelp")}</p>
            </div>
            <div className="space-y-2">
              <Label htmlFor="brush-capacity">{t("instances.capacity")}</Label>
              <Input id="brush-capacity" type="number" min="0" value={capacityMbps} placeholder={t("instances.capacityPlaceholder")}
                onChange={(e) => setCapacityMbps(e.target.value)} />
              <p className="text-xs text-muted-foreground">{t("instanceTab.capacityHelp")}</p>
            </div>
            <div className="space-y-2">
              <Label htmlFor="brush-country" className="flex items-center gap-2">{t("instanceTab.country")}<CountryFlag code={country} /></Label>
              <select
                id="brush-country"
                value={country}
                onChange={(e) => setCountry(e.target.value)}
                className="flex h-9 w-full rounded-md border border-input bg-background px-3 py-1 text-sm shadow-xs"
              >
                <option value="">—</option>
                {countryOptions(i18n.language).map((c) => <option key={c.code} value={c.code}>{c.label}</option>)}
              </select>
              <p className="text-xs text-muted-foreground">{t("instanceTab.countryHelp")}</p>
            </div>
          </div>
        </section>
        <section className="space-y-4">
          <div className="space-y-1">
            <h3 className="text-base font-medium">{t("instanceTab.nowTitle")}</h3>
            <p className="text-sm text-muted-foreground">{t("instanceTab.nowDescription")}</p>
          </div>
          <div className="grid grid-cols-2 gap-4 md:grid-cols-4">
            {facts.map((f) => (
              <div key={f.label} className="rounded-lg border p-3 bg-muted/40">
                <p className="text-xs text-muted-foreground">{f.label}</p>
                <p className="text-lg font-semibold tabular-nums">{f.value}</p>
              </div>
            ))}
          </div>
          {inst.error && <p className="text-sm text-destructive">{inst.error}</p>}
        </section>
      </div>
    </PreferencesFormShell>
  )
}
