import type { Edge, Node } from '@xyflow/react'
import { CARD_W, RESULT_ID, roundedPath, tracePoints, type Chip, type Layout, type Point } from '../layout'
import type { RunResult, Step } from '../types'

export interface StepNodeData extends Record<string, unknown> {
  step: Step
  number: number
  index: number
  chips: Chip[]
  carried: Step['uses']
}
export interface StackNodeData extends Record<string, unknown> {
  steps: Step[]
  /** Each step's position in the visible sequence, for replay. */
  indices: number[]
}
export interface ResultNodeData extends Record<string, unknown> {
  result: RunResult
  steps: Step[]
  index: number
}
export interface LaneNodeData extends Record<string, unknown> {
  alternate: boolean
}
export interface WireData extends Record<string, unknown> {
  kind: 'line' | 'trace'
  path: string
  hue: number
  fromIndex: number
  toIndex: number
  /** The start dot and the label: on the first branch of a line only. */
  start?: Point
  label?: Point & { text: string }
}

/** While a chip is hovered: the card it is on, and the steps its values came from. */
export interface Trace {
  target: string
  sources: string[]
}

export type StepFlowNode = Node<StepNodeData, 'step'>
export type StackFlowNode = Node<StackNodeData, 'stack'>
export type ResultFlowNode = Node<ResultNodeData, 'result'>
export type LaneFlowNode = Node<LaneNodeData, 'lane'>
export type CanvasNode = StepFlowNode | StackFlowNode | ResultFlowNode | LaneFlowNode
export type FlowEdge = Edge<WireData, 'wire'>

export const LANE_PREFIX = 'lane:'

const fixed = { draggable: false, selectable: false, connectable: false, focusable: false, deletable: false } as const
const wire = { type: 'wire', selectable: false, focusable: false, deletable: false } as const
// React Flow switches pointer events off on nodes that are neither draggable nor selectable. Cards have buttons.
const clickable = { pointerEvents: 'all' } as const
const NO_CHIPS: Chip[] = []

/**
 * Turns the pure layout into React Flow nodes and edges. Cards that have been measured carry their measured
 * size, so React Flow keeps them visible when we hand it fresh node objects after a re-layout.
 */
export function toFlow(
  layout: Layout,
  heights: Record<string, number>,
  result?: RunResult,
  trace?: Trace | null,
): { nodes: CanvasNode[]; edges: FlowEdge[] } {
  const nodes: CanvasNode[] = layout.lanes.map((lane, i) => ({
    ...fixed,
    id: `${LANE_PREFIX}${lane.id}`,
    type: 'lane',
    position: { x: 0, y: lane.top },
    width: layout.width,
    height: lane.height,
    zIndex: -1,
    data: { alternate: i % 2 === 0 },
  }))

  const sized = (id: string, h: number) => {
    const measured = heights[id]
    return measured === undefined
      ? { initialWidth: CARD_W, initialHeight: h }
      : { initialWidth: CARD_W, initialHeight: h, measured: { width: CARD_W, height: measured } }
  }

  for (const block of layout.blocks) {
    const p = layout.place[block.id]
    const first = block.steps[0]
    if (!p || !first) continue
    const base = { ...fixed, ...sized(block.id, p.h), style: clickable, id: block.id, position: { x: p.x, y: p.y } }
    if (block.steps.length > 1) {
      nodes.push({ ...base, type: 'stack', data: { steps: block.steps, indices: block.steps.map((s) => layout.place[s.id]?.index ?? p.index) } })
      continue
    }
    const carried = layout.lines
      .filter((l) => l.target === first.id)
      .flatMap((l) => l.whats.map((what) => ({ fromStepId: l.source, what })))
    nodes.push({
      ...base,
      type: 'step',
      data: { step: first, number: layout.numbers[first.id] ?? 0, index: p.index, chips: layout.chips[first.id] ?? NO_CHIPS, carried },
    })
  }

  const rp = layout.place[RESULT_ID]
  if (result && rp) {
    nodes.push({
      ...fixed,
      ...sized(RESULT_ID, rp.h),
      style: clickable,
      id: RESULT_ID,
      type: 'result',
      position: { x: rp.x, y: rp.y },
      data: { result, steps: layout.cloud, index: rp.index },
    })
  }

  // One label per line: everything its source made that later steps use.
  const whats = new Map<string, string[]>()
  for (const l of layout.lines) whats.set(l.source, [...new Set([...(whats.get(l.source) ?? []), ...l.whats])])

  const edges: FlowEdge[] = layout.lines.map((l) => {
    const corner = l.points[1] as Point
    return {
      ...wire,
      id: l.id,
      source: l.source,
      target: l.target,
      data: {
        kind: 'line',
        path: l.path,
        hue: l.hue,
        fromIndex: layout.place[l.source]?.index ?? 0,
        toIndex: l.toIndex,
        start: l.first ? l.points[0] : undefined,
        label: l.first ? { x: corner.x + 10, y: corner.y, text: (whats.get(l.source) ?? []).join(', ') } : undefined,
      },
    }
  })

  for (const from of trace?.sources ?? []) {
    const a = layout.place[from]
    const t = trace ? layout.place[trace.target] : undefined
    const points = trace ? tracePoints(layout, from, trace.target) : null
    if (!a || !t || !points) continue
    edges.push({
      ...wire,
      id: `trace:${from}`,
      source: a.node,
      target: t.node,
      data: { kind: 'trace', path: roundedPath(points), hue: 0, fromIndex: a.index, toIndex: t.index },
    })
  }

  return { nodes, edges }
}
