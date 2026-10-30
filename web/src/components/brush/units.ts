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

// Upload capacity is entered in Mbps as hosting providers sell it: 1 Mbps = 125,000 bytes/s.
const BYTES_PER_SECOND_PER_MBPS = 125_000

export function bytesPerSecondToMbps(bytes: number): string {
  return String(Math.round(bytes / BYTES_PER_SECOND_PER_MBPS))
}

export function mbpsToBytesPerSecond(mbps: string): number {
  return Math.round(Number(mbps) * BYTES_PER_SECOND_PER_MBPS)
}
