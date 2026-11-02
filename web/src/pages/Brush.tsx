/*
 * Copyright (c) 2026, vertex-qui contributors.
 * SPDX-License-Identifier: GPL-2.0-or-later
 */

import { BrushFeedsCard } from "@/components/brush/BrushFeedsCard"
import { BrushHeader } from "@/components/brush/BrushHeader"
import { BrushPerformanceCard } from "@/components/brush/BrushPerformanceCard"

export function Brush() {
  return (
    <div className="container mx-auto px-6 space-y-6 py-6 overflow-x-hidden">
      <BrushHeader />
      <BrushPerformanceCard />
      <BrushFeedsCard />
    </div>
  )
}
