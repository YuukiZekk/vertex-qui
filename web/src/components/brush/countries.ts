/*
 * Copyright (c) 2026, vertex-qui contributors.
 * SPDX-License-Identifier: GPL-2.0-or-later
 */

import countryData from "flag-icons/country.json"

// countryOptions lists the ISO countries flag-icons has a flag for, named in the
// interface language and sorted by that name.
export function countryOptions(language: string): Array<{ code: string, label: string }> {
  let names: Intl.DisplayNames | null = null
  try {
    names = new Intl.DisplayNames([language], { type: "region" })
  } catch {
    names = null
  }
  return countryData
    .filter((c) => c.iso)
    .map((c) => ({ code: c.code, label: names?.of(c.code.toUpperCase()) ?? c.name }))
    .sort((a, b) => a.label.localeCompare(b.label, language))
}

export function isCountryCode(code?: string): code is string {
  return !!code && /^[a-z]{2}$/.test(code)
}
