/*
 * Copyright (c) 2026, vertex-qui contributors.
 * SPDX-License-Identifier: GPL-2.0-or-later
 */

import { cn } from "@/lib/utils"
import type { InstanceResponse } from "@/types"
import type { DragEndEvent } from "@dnd-kit/core"
import { closestCenter, DndContext, KeyboardSensor, PointerSensor, TouchSensor, useSensor, useSensors } from "@dnd-kit/core"
import { arrayMove, rectSortingStrategy, SortableContext, sortableKeyboardCoordinates, useSortable } from "@dnd-kit/sortable"
import { CSS } from "@dnd-kit/utilities"
import type { CSSProperties, ReactNode } from "react"

// SortableInstanceGrid lets the instance cards in settings be dragged into a new
// order. A drag starts after the pointer moves 8px (or a 250ms press on touch), so
// clicks on the card's own buttons still work; the move up and down buttons stay.
export function SortableInstanceGrid({ instances, disabled, onReorder, renderItem }: {
  instances: InstanceResponse[]
  disabled?: boolean
  onReorder: (orderedIds: number[]) => void
  renderItem: (instance: InstanceResponse, index: number) => ReactNode
}) {
  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 8 } }),
    useSensor(TouchSensor, { activationConstraint: { delay: 250, tolerance: 8 } }),
    useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates })
  )
  const ids = instances.map((instance) => instance.id)
  const onDragEnd = ({ active, over }: DragEndEvent) => {
    if (!over || active.id === over.id) return
    const from = ids.indexOf(Number(active.id))
    const to = ids.indexOf(Number(over.id))
    if (from < 0 || to < 0) return
    onReorder(arrayMove(ids, from, to))
  }

  return (
    <DndContext sensors={sensors} collisionDetection={closestCenter} onDragEnd={onDragEnd}>
      <SortableContext items={ids} strategy={rectSortingStrategy} disabled={disabled}>
        <div className="grid gap-4 lg:grid-cols-2">
          {instances.map((instance, index) => (
            <SortableItem key={instance.id} id={instance.id}>
              {renderItem(instance, index)}
            </SortableItem>
          ))}
        </div>
      </SortableContext>
    </DndContext>
  )
}

function SortableItem({ id, children }: { id: number, children: ReactNode }) {
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({ id })
  const style: CSSProperties = {
    transform: CSS.Transform.toString(transform),
    transition,
  }
  return (
    <div
      ref={setNodeRef}
      style={style}
      className={cn("cursor-grab active:cursor-grabbing", isDragging && "relative z-10 opacity-70")}
      {...attributes}
      {...listeners}
    >
      {children}
    </div>
  )
}
