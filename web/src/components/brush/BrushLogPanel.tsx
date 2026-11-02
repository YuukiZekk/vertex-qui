/*
 * Copyright (c) 2026, vertex-qui contributors.
 * SPDX-License-Identifier: GPL-2.0-or-later
 */

import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { useTranslation } from "react-i18next"
import { BrushDeletionsLog } from "./BrushDeletionsLog"
import { BrushHistoryLog } from "./BrushHistoryLog"

// BrushLogPanel is the brush module's own log on qui's logs page, laid out like
// qui's log card above it: what brush decided for each new torrent, and what it
// deleted. A tab's list is only fetched while the tab is open.
export function BrushLogPanel() {
  const { t } = useTranslation("brush")

  return (
    <Card>
      <CardHeader>
        <div className="space-y-1">
          <CardTitle>{t("log.title")}</CardTitle>
          <CardDescription>{t("log.description")}</CardDescription>
        </div>
      </CardHeader>
      <CardContent className="p-0">
        <Tabs defaultValue="history">
          <div className="px-6 pb-4">
            <TabsList>
              <TabsTrigger value="history">{t("history.title")}</TabsTrigger>
              <TabsTrigger value="deletions">{t("deletions.title")}</TabsTrigger>
            </TabsList>
          </div>
          <div className="border-t">
            <TabsContent value="history" className="mt-0">
              <BrushHistoryLog />
            </TabsContent>
            <TabsContent value="deletions" className="mt-0">
              <BrushDeletionsLog />
            </TabsContent>
          </div>
        </Tabs>
      </CardContent>
    </Card>
  )
}
