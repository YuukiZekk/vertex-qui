/*
 * Copyright (c) 2026, vertex-qui contributors.
 * SPDX-License-Identifier: GPL-2.0-or-later
 */

import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"
import { cn } from "@/lib/utils"
import { ChevronDown, Info } from "lucide-react"
import type { ReactNode } from "react"
import { toneClass, type Tone } from "./sectionStyles"

// SectionCard is the card qui's automation overviews use: a large title with an
// info tooltip for the longer explanation, a one-line description, actions on the
// right, and content under a border with no padding of its own. With onOpenChange
// it collapses, and its content is not rendered while closed.
export function SectionCard({
  title,
  description,
  info,
  actions,
  open = true,
  onOpenChange,
  children,
}: {
  title: ReactNode
  description?: ReactNode
  info?: ReactNode
  actions?: ReactNode
  open?: boolean
  onOpenChange?: (open: boolean) => void
  children?: ReactNode
}) {
  return (
    <Card>
      <CardHeader className="space-y-2">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div className="flex items-center gap-2">
            <CardTitle className="text-lg font-semibold">{title}</CardTitle>
            {info && (
              <Tooltip>
                <TooltipTrigger asChild>
                  <Info className="h-4 w-4 text-muted-foreground cursor-help" />
                </TooltipTrigger>
                <TooltipContent className="max-w-[340px]">
                  <p>{info}</p>
                </TooltipContent>
              </Tooltip>
            )}
          </div>
          <div className="flex items-center gap-2">
            {open && actions}
            {onOpenChange && (
              <Button variant="ghost" size="icon" className="h-7 w-7" aria-expanded={open} onClick={() => onOpenChange(!open)}>
                <ChevronDown className={cn("h-4 w-4 text-muted-foreground transition-transform duration-200", open && "rotate-180")} />
              </Button>
            )}
          </div>
        </div>
        {description && <CardDescription>{description}</CardDescription>}
      </CardHeader>
      {open && children && <CardContent className="p-0"><div className="border-t">{children}</div></CardContent>}
    </Card>
  )
}

// EmptyState is qui's dashed placeholder for a list with nothing in it yet.
export function EmptyState({ children, action }: { children: ReactNode, action?: ReactNode }) {
  return (
    <div className="m-6 flex flex-col items-center justify-center gap-2 rounded-lg border border-dashed py-6 text-center">
      <p className="text-sm text-muted-foreground">{children}</p>
      {action}
    </div>
  )
}

// StateBadge is the tinted outline badge qui uses for states and counts.
export function StateBadge({ tone, children, title }: { tone: Tone, children: ReactNode, title?: string }) {
  return (
    <Badge variant="outline" className={cn("text-xs", toneClass[tone])} title={title}>
      {children}
    </Badge>
  )
}
