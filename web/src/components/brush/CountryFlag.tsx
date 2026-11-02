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
export function CountryFlag({ code }: { code?: string }) {
  if (!isCountryCode(code)) return null
  return <span className={`fi fi-${code} shrink-0 rounded-sm text-sm`} aria-hidden="true" />
}

// InstanceFlag is the flag of where an instance runs, as set in its brush settings.
export function InstanceFlag({ instanceId }: { instanceId: number }) {
  const { data } = useQuery({ queryKey: ["brush", "status"], queryFn: () => api.getBrushStatus(), staleTime: 30000 })
  return <CountryFlag code={data?.instances.find((i) => i.id === instanceId)?.countryCode} />
}
