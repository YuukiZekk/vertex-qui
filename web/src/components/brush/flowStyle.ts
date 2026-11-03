/*
 * Copyright (c) 2026, vertex-qui contributors.
 * SPDX-License-Identifier: GPL-2.0-or-later
 */

// The map is always dark, like NextTrace's flow table: outbound arcs are cyan and
// inbound ones amber.
export const UPLOAD_COLOR = "#22d3ee"
export const DOWNLOAD_COLOR = "#fbbf24"

// Text on the page follows the theme, so it uses darker shades in light mode.
export const uploadText = "text-cyan-600 dark:text-cyan-400"
export const downloadText = "text-amber-600 dark:text-amber-400"

// Coloured by server, each server keeps its hue for as long as the list keeps its order.
const SERVER_COLORS = ["#22d3ee", "#f472b6", "#a3e635", "#fbbf24", "#a78bfa", "#fb923c", "#34d399", "#60a5fa", "#f87171", "#e879f9"]

export function serverColor(index: number) {
  return SERVER_COLORS[index % SERVER_COLORS.length]
}

export type FlowColorBy = "direction" | "server"

export function countryName(names: Intl.DisplayNames | null, code: string) {
  return names?.of(code.toUpperCase()) ?? code.toUpperCase()
}
