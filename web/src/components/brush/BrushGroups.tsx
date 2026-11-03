/*
 * Copyright (c) 2026, vertex-qui contributors.
 * SPDX-License-Identifier: GPL-2.0-or-later
 */

import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { api } from "@/lib/api"
import { cn } from "@/lib/utils"
import { useMutation, useQueryClient } from "@tanstack/react-query"
import { Check, Pencil, Plus, Trash2, X } from "lucide-react"
import { useState } from "react"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"
import { useBrushGroups } from "./useBrushGroups"

// GroupPicker toggles groups on and off as chips, and adds a new group, picked at
// once, from the box after them.
export function GroupPicker({ value, onChange }: { value: number[], onChange: (ids: number[]) => void }) {
  const { t } = useTranslation("brush")
  const queryClient = useQueryClient()
  const { data: groups } = useBrushGroups()
  const [name, setName] = useState("")
  const create = useMutation({
    mutationFn: (n: string) => api.createBrushGroup(n),
    onSuccess: ({ id }) => {
      setName("")
      onChange([...value, id])
      return queryClient.invalidateQueries({ queryKey: ["brush", "groups"] })
    },
    onError: (err: Error) => toast.error(t("groups.saveFailed"), { description: err.message }),
  })
  const toggle = (id: number) => onChange(value.includes(id) ? value.filter((g) => g !== id) : [...value, id])
  const add = () => {
    const n = name.trim()
    if (n) create.mutate(n)
  }

  return (
    <div className="flex flex-wrap items-center gap-2">
      {(groups ?? []).map((g) => {
        const on = value.includes(g.id)
        return (
          <Button key={g.id} type="button" size="sm" variant={on ? "secondary" : "outline"} aria-pressed={on}
            className={cn("h-7 gap-1 px-2.5 text-xs", on && "border border-primary/40")} onClick={() => toggle(g.id)}>
            {on && <Check className="h-3 w-3" />}
            {g.name}
          </Button>
        )
      })}
      <div className="flex items-center gap-1">
        <Input value={name} onChange={(e) => setName(e.target.value)} placeholder={t("groups.newPlaceholder")} maxLength={40}
          className="h-7 w-36 text-xs"
          onKeyDown={(e) => {
            if (e.key === "Enter") {
              e.preventDefault()
              add()
            }
          }} />
        <Button type="button" size="sm" variant="outline" className="h-7 px-2" disabled={!name.trim() || create.isPending}
          aria-label={t("groups.add")} onClick={add}>
          <Plus className="h-3.5 w-3.5" />
        </Button>
      </div>
    </div>
  )
}

// GroupManager lists every group with how many instances it has, to rename or
// delete it.
export function GroupManager() {
  const { t } = useTranslation("brush")
  const queryClient = useQueryClient()
  const { data: groups } = useBrushGroups()
  const [editing, setEditing] = useState<{ id: number, name: string } | null>(null)
  const [confirming, setConfirming] = useState<number | null>(null)
  const refresh = () => queryClient.invalidateQueries({ queryKey: ["brush"] })
  const rename = useMutation({
    mutationFn: ({ id, name }: { id: number, name: string }) => api.renameBrushGroup(id, name),
    onSuccess: () => {
      setEditing(null)
      return refresh()
    },
    onError: (err: Error) => toast.error(t("groups.saveFailed"), { description: err.message }),
  })
  const remove = useMutation({
    mutationFn: (id: number) => api.deleteBrushGroup(id),
    onSuccess: () => {
      setConfirming(null)
      return refresh()
    },
    onError: (err: Error) => toast.error(t("groups.deleteFailed"), { description: err.message }),
  })
  if (!groups?.length) return null

  return (
    <ul className="divide-y rounded-lg border text-sm">
      {groups.map((g) => (
        <li key={g.id} className="flex items-center gap-2 px-3 py-2">
          {editing?.id === g.id ? (
            <>
              <Input value={editing.name} onChange={(e) => setEditing({ id: g.id, name: e.target.value })} maxLength={40} className="h-7 flex-1 text-xs" autoFocus />
              <Button type="button" size="icon" variant="ghost" className="h-7 w-7" aria-label={t("actions.save")}
                disabled={!editing.name.trim() || rename.isPending} onClick={() => rename.mutate({ id: g.id, name: editing.name.trim() })}>
                <Check className="h-3.5 w-3.5" />
              </Button>
              <Button type="button" size="icon" variant="ghost" className="h-7 w-7" aria-label={t("actions.cancel")} onClick={() => setEditing(null)}>
                <X className="h-3.5 w-3.5" />
              </Button>
            </>
          ) : (
            <>
              <span className="flex-1 truncate">{g.name}</span>
              <span className="text-xs text-muted-foreground">{t("groups.members", { count: g.instanceIds.length })}</span>
              <Button type="button" size="icon" variant="ghost" className="h-7 w-7" aria-label={t("actions.edit")} onClick={() => setEditing({ id: g.id, name: g.name })}>
                <Pencil className="h-3.5 w-3.5" />
              </Button>
              {confirming === g.id ? (
                <Button type="button" size="sm" variant="destructive" className="h-7 px-2 text-xs" disabled={remove.isPending} onClick={() => remove.mutate(g.id)}>
                  {t("groups.confirmDelete")}
                </Button>
              ) : (
                <Button type="button" size="icon" variant="ghost" className="h-7 w-7" aria-label={t("actions.delete")} onClick={() => setConfirming(g.id)}>
                  <Trash2 className="h-3.5 w-3.5" />
                </Button>
              )}
            </>
          )}
        </li>
      ))}
    </ul>
  )
}
