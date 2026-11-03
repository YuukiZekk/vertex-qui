/*
 * Copyright (c) 2026, vertex-qui contributors.
 * SPDX-License-Identifier: GPL-2.0-or-later
 */

import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { useDebounce } from "@/hooks/useDebounce"
import { Search } from "lucide-react"
import { useState } from "react"
import { useTranslation } from "react-i18next"
import { BrushDeletionsLog } from "./BrushDeletionsLog"
import { BrushHistoryLog } from "./BrushHistoryLog"

// BrushLogPanel is the brush module's own log on qui's logs page, laid out like
// qui's log card above it: what brush decided for each new torrent, and what it
// deleted. A tab's list is only fetched while the tab is open. One search box
// filters both tabs on the server, so it finds older rows the lists leave out.
export function BrushLogPanel() {
  const { t } = useTranslation("brush")
  const [query, setQuery] = useState("")
  const search = useDebounce(query.trim(), 300)

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
          <div className="flex flex-wrap items-center gap-3 px-6 pb-4">
            <TabsList>
              <TabsTrigger value="history">{t("history.title")}</TabsTrigger>
              <TabsTrigger value="deletions">{t("deletions.title")}</TabsTrigger>
            </TabsList>
            <div className="relative w-full sm:ml-auto sm:w-72">
              <Search className="pointer-events-none absolute left-2.5 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
              <Input type="search" value={query} onChange={(e) => setQuery(e.target.value)} placeholder={t("log.searchPlaceholder")}
                aria-label={t("log.searchPlaceholder")} className="h-9 pl-8" />
            </div>
          </div>
          <div className="border-t">
            <TabsContent value="history" className="mt-0">
              <BrushHistoryLog search={search} />
            </TabsContent>
            <TabsContent value="deletions" className="mt-0">
              <BrushDeletionsLog search={search} />
            </TabsContent>
          </div>
        </Tabs>
      </CardContent>
    </Card>
  )
}
