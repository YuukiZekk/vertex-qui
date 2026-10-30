/*
 * Copyright (c) 2026, vertex-qui contributors.
 * SPDX-License-Identifier: GPL-2.0-or-later
 */

import { BrushFeedsCard } from "@/components/brush/BrushFeedsCard"
import { BrushHistoryCard } from "@/components/brush/BrushHistoryCard"
import { BrushSitesCard } from "@/components/brush/BrushSitesCard"
import { BrushStatusCard } from "@/components/brush/BrushStatusCard"
import { useTranslation } from "react-i18next"

export function Brush() {
  const { t } = useTranslation("brush")

  return (
    <div className="container mx-auto px-6 space-y-6 py-6 overflow-x-hidden">
      <div className="space-y-2">
        <h1 className="text-2xl font-semibold">{t("page.title")}</h1>
        <p className="text-sm text-muted-foreground">{t("page.description")}</p>
      </div>
      <BrushStatusCard />
      <BrushSitesCard />
      <BrushFeedsCard />
      <BrushHistoryCard />
    </div>
  )
}
