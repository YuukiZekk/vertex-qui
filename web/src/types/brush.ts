/*
 * Copyright (c) 2026, vertex-qui contributors.
 * SPDX-License-Identifier: GPL-2.0-or-later
 */

export interface BrushInstanceStatus {
  id: number
  name: string
  active: boolean
  torrents: number
  freeSpace: number
  error?: string
}

export interface BrushStatus {
  enabled: boolean
  instances: BrushInstanceStatus[]
}
