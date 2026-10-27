import type { Edge, Node } from '@xyflow/react'
import { CARD_W, RESULT_ID, type EdgeKind, type Layout } from '../layout'
import type { RunResult, Step } from '../types'

export interface StepNodeData extends Record<string, unknown> {
  step: Step
  number: number
  index: number
}
export interface ResultNodeData extends Record<string, unknown> {
  result: RunResult
  steps: Step[]
  index: number
}
export interface LaneNodeData extends Record<string, unknown> {
  alternate: boolean
}
export interface FlowEdgeData extends Record<string, unknown> {
  path: string
  kind: EdgeKind
  extra: boolean
  toIndex: number
}

export type StepFlowNode = Node<StepNodeData, 'step'>
export type ResultFlowNode = Node<ResultNodeData, 'result'>
export type LaneFlowNode = Node<LaneNodeData, 'lane'>
export type CanvasNode = StepFlowNode | ResultFlowNode | LaneFlowNode
export type FlowEdge = Edge<FlowEdgeData, 'flow'>

export const LANE_PREFIX = 'lane:'

const fixed = { draggable: false, selectable: false, connectable: false, focusable: false, deletable: false } as const
// React Flow switches pointer events off on nodes that are neither draggable nor selectable. Cards have buttons.
const clickable = { pointerEvents: 'all' } as const

/**
 * Turns the pure layout into React Flow nodes and edges. Cards that have been measured carry their measured
 * size, so React Flow keeps them visible when we hand it fresh node objects after a re-layout.
 */
export function toFlow(layout: Layout, heights: Record<string, number>, result?: RunResult): { nodes: CanvasNode[]; edges: FlowEdge[] } {
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

  for (const step of layout.steps) {
    const p = layout.place[step.id]
    if (!p) continue
    nodes.push({
      ...fixed,
      ...sized(step.id, p.h),
      style: clickable,
      id: step.id,
      type: 'step',
      position: { x: p.x, y: p.y },
      data: { step, number: layout.numbers[step.id] ?? 0, index: p.index },
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

  const edges: FlowEdge[] = layout.edges.map((e) => ({
    id: e.id,
    source: e.source,
    target: e.target,
    type: 'flow',
    selectable: false,
    focusable: false,
    deletable: false,
    data: { path: e.path, kind: e.kind, extra: e.extra, toIndex: e.toIndex },
  }))

  return { nodes, edges }
}
