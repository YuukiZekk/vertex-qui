/*
 * Copyright (c) 2026, vertex-qui contributors.
 * SPDX-License-Identifier: GPL-2.0-or-later
 */

import "flag-icons/css/flag-icons.min.css"
import { api } from "@/lib/api"
import { useQuery } from "@tanstack/react-query"
import { isCountryCode } from "./countries"

// CountryFlag draws a country's flag with flag-icons, like MrGlaucus/qui does; the
// browser fetches only the flags in use.
// With iconBox it fills the same 16px box as qui's tracker icons, so names after it
// line up with tracker names, and an empty box keeps that place without a flag.
export function CountryFlag({ code, iconBox }: { code?: string, iconBox?: boolean }) {
  if (iconBox) {
    return (
      <span className="flex h-4 w-4 shrink-0 items-center justify-center">
        {isCountryCode(code) && <span className={`fi fi-${code} rounded-[2px] text-xs`} aria-hidden="true" />}
      </span>
    )
  }
  if (!isCountryCode(code)) return null
  return <span className={`fi fi-${code} shrink-0 rounded-sm text-sm`} aria-hidden="true" />
}

// InstanceFlag is the flag of where an instance runs, as set in its brush settings.
export function InstanceFlag({ instanceId, iconBox }: { instanceId: number, iconBox?: boolean }) {
  const { data } = useQuery({ queryKey: ["brush", "status"], queryFn: () => api.getBrushStatus(), staleTime: 30000 })
  return <CountryFlag code={data?.instances.find((i) => i.id === instanceId)?.countryCode} iconBox={iconBox} />
}
