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

// Coloured by server, each server keeps its hue for as long as the list keeps its
// order. The hues leave out the upload cyan and download amber, so the two ways of
// colouring never look alike.
const SERVER_COLORS = ["#f472b6", "#a3e635", "#a78bfa", "#fb923c", "#60a5fa", "#f87171", "#34d399", "#e879f9", "#94a3b8"]

export function serverColor(index: number) {
  return SERVER_COLORS[index % SERVER_COLORS.length]
}

export type FlowColorBy = "direction" | "server"

export function countryName(names: Intl.DisplayNames | null, code: string) {
  return names?.of(code.toUpperCase()) ?? code.toUpperCase()
}

// Countries are shaded by how much they move on one violet scale, deep for little
// and pale for the most, so the shading never reads as an arc colour.
const HEAT_STOPS: Array<[number, [number, number, number]]> = [
  [0, [46, 16, 101]],
  [0.45, [124, 58, 237]],
  [0.8, [192, 132, 252]],
  [1, [245, 208, 254]],
]

// heatShare spreads speeds over 0 to 1 by their square root, so countries well
// below the busiest still show.
export function heatShare(speed: number, max: number) {
  return max > 0 ? Math.sqrt(Math.min(1, speed / max)) : 0
}

export function heatColor(share: number, alpha = 1) {
  const s = Math.min(1, Math.max(0, share))
  let i = 1
  while (i < HEAT_STOPS.length - 1 && HEAT_STOPS[i][0] < s) i++
  const [p0, c0] = HEAT_STOPS[i - 1]
  const [p1, c1] = HEAT_STOPS[i]
  const f = (s - p0) / (p1 - p0)
  const mix = (j: number) => Math.round(c0[j] + (c1[j] - c0[j]) * f)
  return `rgba(${mix(0)},${mix(1)},${mix(2)},${alpha})`
}

export const heatGradient = `linear-gradient(90deg, ${HEAT_STOPS.map(([at, c]) => `rgb(${c.join(",")}) ${at * 100}%`).join(", ")})`
