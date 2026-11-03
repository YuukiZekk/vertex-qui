/*
 * Copyright (c) 2026, vertex-qui contributors.
 * SPDX-License-Identifier: GPL-2.0-or-later
 */

import { Flows } from "@/pages/Flows"
import { createFileRoute } from "@tanstack/react-router"

export const Route = createFileRoute("/_authenticated/flows")({
  component: Flows,
})
