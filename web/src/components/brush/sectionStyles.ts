/*
 * Copyright (c) 2026, vertex-qui contributors.
 * SPDX-License-Identifier: GPL-2.0-or-later
 */

// Tables in a section run edge to edge like qui's overview cards, so the first and
// last columns take the card's 24px padding.
export const sectionTableClass =
  "[&_th:first-child]:pl-6 [&_td:first-child]:pl-6 [&_th:last-child]:pr-6 [&_td:last-child]:pr-6"
export const sectionHeaderRowClass = "bg-muted/50"

export type Tone = "ok" | "warn" | "error" | "neutral"

export const toneClass: Record<Tone, string> = {
  ok: "bg-emerald-500/10 text-emerald-500 border-emerald-500/20",
  warn: "bg-amber-500/10 text-amber-500 border-amber-500/20",
  error: "bg-destructive/10 text-destructive border-destructive/30",
  neutral: "",
}

// readOpen and writeOpen remember a section's collapsed state per browser.
export function readOpen(key: string): boolean {
  try {
    return localStorage.getItem(key) !== "false"
  } catch {
    return true
  }
}

export function writeOpen(key: string, open: boolean) {
  try {
    localStorage.setItem(key, String(open))
  } catch {
    // Storage can be unavailable; the section still toggles for this visit.
  }
}
