/*
 * Copyright (c) 2026, vertex-qui contributors.
 * SPDX-License-Identifier: GPL-2.0-or-later
 */

import { Badge } from "@/components/ui/badge"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { api } from "@/lib/api"
import { formatBytes } from "@/lib/utils"
import { useQuery } from "@tanstack/react-query"
import { useTranslation } from "react-i18next"

export function Brush() {
  const { t } = useTranslation("brush")
  const { data, error, isLoading } = useQuery({
    queryKey: ["brush", "status"],
    queryFn: () => api.getBrushStatus(),
    refetchInterval: 5000,
  })

  return (
    <div className="container mx-auto px-6 space-y-6 py-6 overflow-x-hidden">
      <div className="space-y-2">
        <h1 className="text-2xl font-semibold">{t("page.title")}</h1>
        <p className="text-sm text-muted-foreground">{t("page.description")}</p>
      </div>

      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            {t("status.title")}
            {data && (
              <Badge variant={data.enabled ? "default" : "secondary"}>
                {data.enabled ? t("status.enabled") : t("status.disabled")}
              </Badge>
            )}
          </CardTitle>
          <CardDescription>{t("status.description")}</CardDescription>
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
                  <TableHead>{t("instances.state")}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {data.instances.length === 0 && (
                  <TableRow>
                    <TableCell colSpan={4} className="text-muted-foreground">{t("instances.empty")}</TableCell>
                  </TableRow>
                )}
                {data.instances.map((inst) => (
                  <TableRow key={inst.id}>
                    <TableCell>{inst.name}</TableCell>
                    <TableCell>{inst.active && !inst.error ? inst.torrents : "-"}</TableCell>
                    <TableCell>{inst.active && !inst.error ? formatBytes(inst.freeSpace) : "-"}</TableCell>
                    <TableCell className={inst.error ? "text-destructive" : undefined}>
                      {!inst.active ? t("instances.inactive") : inst.error ? t("instances.error", { message: inst.error }) : t("instances.ok")}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </CardContent>
      </Card>
    </div>
  )
}
