/*
 * Copyright (c) 2026, vertex-qui contributors.
 * SPDX-License-Identifier: GPL-2.0-or-later
 */

// Sizes are entered in GB as NexusPHP sites show them, which count 1 GB as 1024^3 bytes.
export const BYTES_PER_GB = 1024 ** 3

export function bytesToGB(bytes: number): string {
  return String(Math.round((bytes / BYTES_PER_GB) * 100) / 100)
}

export function gbToBytes(gb: string): number {
  return Math.round(Number(gb) * BYTES_PER_GB)
}
