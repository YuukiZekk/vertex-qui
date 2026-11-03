/*
 * Copyright (c) 2026, vertex-qui contributors.
 * SPDX-License-Identifier: GPL-2.0-or-later
 */

import { api } from "@/lib/api"
import { useQuery } from "@tanstack/react-query"

export function useBrushGroups() {
  return useQuery({ queryKey: ["brush", "groups"], queryFn: () => api.getBrushGroups() })
}
