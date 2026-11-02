/*
 * Copyright (c) 2026, vertex-qui contributors.
 * SPDX-License-Identifier: GPL-2.0-or-later
 */

import { Label } from "@/components/ui/label"
import { Switch } from "@/components/ui/switch"
import { api } from "@/lib/api"
import { formatBytes } from "@/lib/utils"
import type { BrushInstanceStatus } from "@/types/brush"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { Link } from "@tanstack/react-router"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"
import { StateBadge } from "./section"
import type { Tone } from "./sectionStyles"

function stateOf(inst: BrushInstanceStatus): { tone: Tone, key: string } {
  if (!inst.active) return { tone: "neutral", key: "instances.inactive" }
  if (inst.error) return { tone: "error", key: "instances.errorShort" }
  if (inst.floorBytes === null) return { tone: "warn", key: "instances.noFloor" }
  if (inst.committed < inst.floorBytes) return { tone: "error", key: "instances.belowFloor" }
  return { tone: "ok", key: "instances.ok" }
}

// BrushHeader is the brush page's title with the brush switch beside it and one
// badge per downloader under it, green while it can take torrents. Each badge's
// tooltip has its space; the floor and capacity are set in the instance settings.
export function BrushHeader() {
  const { t } = useTranslation("brush")
  const queryClient = useQueryClient()
  const { data, error } = useQuery({ queryKey: ["brush", "status"], queryFn: () => api.getBrushStatus(), refetchInterval: 5000 })
  const toggle = useMutation({
    mutationFn: (enabled: boolean) => api.setBrushEnabled(enabled),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["brush", "status"] }),
    onError: (err: Error) => toast.error(t("status.saveFailed"), { description: err.message }),
  })

  return (
    <div className="space-y-3">
      <div className="flex flex-col gap-4 md:flex-row md:items-start md:justify-between">
        <div className="space-y-2">
          <h1 className="text-2xl font-semibold">{t("page.title")}</h1>
          <p className="text-sm text-muted-foreground">{t("page.description")}</p>
        </div>
        {data && (
          <div className="flex shrink-0 items-center gap-2 rounded-lg border bg-muted/40 p-2">
            <Label htmlFor="brush-enabled" className="cursor-pointer text-sm font-medium">
              {data.enabled ? t("status.enabled") : t("status.disabled")}
            </Label>
            <Switch id="brush-enabled" checked={data.enabled} disabled={toggle.isPending} onCheckedChange={(v) => toggle.mutate(v)} />
          </div>
        )}
      </div>
      {error && <p className="text-sm text-destructive">{t("status.loadFailed", { message: error.message })}</p>}
      {data && (
        <div className="flex flex-wrap items-center gap-2">
          {data.instances.length === 0 && <span className="text-sm text-muted-foreground">{t("instances.empty")}</span>}
          {data.instances.map((inst) => {
            const state = stateOf(inst)
            const title = inst.error || (inst.active
              ? t("status.badgeTitle", { free: formatBytes(inst.freeSpace), committed: formatBytes(Math.max(inst.committed, 0)), floor: inst.floorBytes === null ? "-" : formatBytes(inst.floorBytes) })
              : undefined)
            return (
              <Link key={inst.id} to="/settings" search={{ tab: "instances" }}>
                <StateBadge tone={state.tone} title={title}>
                  {inst.name} · {t(state.key)}
                </StateBadge>
              </Link>
            )
          })}
          <span className="text-xs text-muted-foreground">{t("status.settingsHint")}</span>
        </div>
      )}
      {data?.today && (
        <div className="flex flex-wrap items-center gap-x-5 gap-y-1 text-sm text-muted-foreground">
          <span>{t("status.todayLabel")}</span>
          <span>{t("status.todayAdded")} <span className="font-semibold text-foreground tabular-nums">{data.today.added}</span></span>
          <span>{t("status.todayDeleted")} <span className="font-semibold text-foreground tabular-nums">{data.today.deleted}</span></span>
          <span title={t("status.missedSpaceHint")}>
            {t("status.missedSpace")} <span className={`font-semibold tabular-nums ${data.today.missedSpace > 0 ? "text-amber-500" : "text-foreground"}`}>{data.today.missedSpace}</span>
          </span>
          <span title={t("status.missedWindowHint")}>
            {t("status.missedWindow")} <span className={`font-semibold tabular-nums ${data.today.missedWindow > 0 ? "text-amber-500" : "text-foreground"}`}>{data.today.missedWindow}</span>
          </span>
        </div>
      )}
    </div>
  )
}
