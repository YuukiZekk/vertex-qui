/*
 * Copyright (c) 2026, vertex-qui contributors.
 * SPDX-License-Identifier: GPL-2.0-or-later
 */

// Upload and download keep one hue each in both themes, like NextTrace's flow map.
export const UPLOAD_COLOR = "#10b981"
export const DOWNLOAD_COLOR = "#0ea5e9"

export function countryName(names: Intl.DisplayNames | null, code: string) {
  return names?.of(code.toUpperCase()) ?? code.toUpperCase()
}
