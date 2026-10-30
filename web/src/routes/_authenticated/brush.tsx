/*
 * Copyright (c) 2026, vertex-qui contributors.
 * SPDX-License-Identifier: GPL-2.0-or-later
 */

import { Brush } from "@/pages/Brush"
import { createFileRoute } from "@tanstack/react-router"

export const Route = createFileRoute("/_authenticated/brush")({
  component: Brush,
})
