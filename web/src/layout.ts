import type { RunResult, Step } from './types'
import { VIA_LABELS } from './vocab'

// The canvas geometry from the approved mockup. Everything here is pure so it can be unit-tested.
export const CARD_W = 244
export const COL_GAP = 64
export const GUTTER = 140
export const LANE_PAD_TOP = 50
export const LANE_PAD_BOTTOM = 28
export const MIN_CARD_H = 110
export const ANCHOR_Y = 22
export const RESULT_ID = 'result'
const RESULT_ESTIMATE_H = 190

export type StepFilter = 'all' | 'changes'
export type EdgeKind = 'next' | 'flow' | 'gate'

export interface Lane {
  id: string
  name: string
  sub: string
  top: number
  height: number
}

export interface Placement {
  id: string
  x: number
  y: number
  h: number
  /** Position in the visible sequence. The result card comes after the last step. */
  index: number
  lane: number
}

export interface LayoutEdge {
  id: string
  source: string
  target: string
  kind: EdgeKind
  /** A dashed value arrow from a step that is not the one just before. */
  extra: boolean
  toIndex: number
  path: string
}

export interface Layout {
  /** Every step that belongs on the canvas, before the filter. */
  cloud: Step[]
  /** The steps shown, in column order. */
  steps: Step[]
  /** Canvas step numbers (1-based among cloud steps), also for steps the filter hides. */
  numbers: Record<string, number>
  lanes: Lane[]
  place: Record<string, Placement>
  hasResult: boolean
  edges: LayoutEdge[]
  width: number
  height: number
}

export interface LayoutInput {
  steps: Step[]
  result?: RunResult
  filter: StepFilter
  /** Measured card heights by id. Missing ones fall back to an estimate. */
  heights?: Record<string, number>
}

const lines = (text: string, perLine: number) => Math.max(1, Math.ceil(text.length / perLine))

/** A rough card height, used until React Flow has measured the real one. */
export function estimateCardHeight(step: Step): number {
  const waiting = step.status === 'waiting'
  const parts: number[] = [18, 19 * lines(step.title, 27)]
  if (waiting) parts.unshift(22)
  if (step.resource) parts.push(17 * lines(step.resource, 30))
  if (step.tags.length) parts.push(22)
  if (step.generic) parts.push(46)
  if (step.uses.length) parts.push(30 * step.uses.length)
  if (step.facts.length) parts.push(18 * step.facts.length)
  if (waiting) {
    const loss = step.approval?.loss?.length ?? 0
    if (loss) parts.push(34 + 18 * loss)
    parts.push(18 + 18 * lines(step.why, 40), 34)
    if (step.approval?.warning) parts.push(50)
  } else if (step.approval) {
    parts.push(24)
  }
  if (step.status === 'running') parts.push(3)
  parts.push(27)
  const gaps = 8 * (parts.length - 1)
  return Math.max(MIN_CARD_H, Math.round(24 + parts.reduce((a, b) => a + b, 0) + gaps))
}

export function edgePath(a: { x: number; y: number }, b: { x: number; y: number }): string {
  const x1 = a.x + CARD_W
  const y1 = a.y + ANCHOR_Y
  const x2 = b.x - 2
  const y2 = b.y + ANCHOR_Y
  if (Math.abs(y1 - y2) < 1) return `M${x1} ${y1} L${x2} ${y2}`
  const mx = (x1 + x2) / 2
  return `M${x1} ${y1} C${mx} ${y1}, ${mx} ${y2}, ${x2} ${y2}`
}

export function valuePath(a: { x: number; y: number; h: number }, b: { x: number; y: number }): string {
  const sx = a.x + CARD_W / 2
  const sy = a.y + a.h
  const tx = b.x - 2
  const ty = b.y + ANCHOR_Y + 22
  return `M${sx} ${sy} C${sx} ${sy + 80}, ${tx - 90} ${ty}, ${tx} ${ty}`
}

function laneNameAndSub(id: string, multi: boolean, cloud: Step[]): { name: string; sub: string } {
  if (!multi) {
    const first = cloud.find((s) => s.product === id)
    return { name: id, sub: first?.productSub ?? first?.provider ?? '' }
  }
  const mine = cloud.filter((s) => s.provider === id)
  const vias = [...new Set(mine.map((s) => VIA_LABELS[s.via]))]
  const noAdapter = mine.length > 0 && mine.every((s) => s.generic)
  return { name: id, sub: [...vias, ...(noAdapter ? ['no adapter'] : [])].join(' · ') }
}

export function computeLayout({ steps, result, filter, heights = {} }: LayoutInput): Layout {
  const cloud = steps.filter((s) => s.kind === 'cloud')
  const numbers: Record<string, number> = {}
  cloud.forEach((s, i) => {
    numbers[s.id] = i + 1
  })
  const visible = filter === 'changes' ? cloud.filter((s) => s.action !== 'read') : cloud

  // One lane per provider when the chat touches several; otherwise one per product. First appearance sets the order.
  const multi = new Set(cloud.map((s) => s.provider)).size > 1
  const laneOf = (s: Step) => (multi ? s.provider : s.product)
  const laneIds = [...new Set(visible.map(laneOf))]
  const heightOf = (s: Step) => heights[s.id] ?? estimateCardHeight(s)

  const lanes: Lane[] = []
  let y = 0
  for (const id of laneIds) {
    const tallest = Math.max(MIN_CARD_H, ...visible.filter((s) => laneOf(s) === id).map(heightOf))
    const height = tallest + LANE_PAD_TOP + LANE_PAD_BOTTOM
    lanes.push({ id, ...laneNameAndSub(id, multi, cloud), top: y, height })
    y += height
  }

  const place: Record<string, Placement> = {}
  visible.forEach((s, i) => {
    const lane = laneIds.indexOf(laneOf(s))
    place[s.id] = {
      id: s.id,
      x: GUTTER + i * (CARD_W + COL_GAP),
      y: (lanes[lane]?.top ?? 0) + LANE_PAD_TOP,
      h: heightOf(s),
      index: i,
      lane,
    }
  })

  let worldW = GUTTER + visible.length * (CARD_W + COL_GAP)
  const hasResult = result !== undefined
  let worldH = y
  if (hasResult) {
    const h = heights[RESULT_ID] ?? RESULT_ESTIMATE_H
    place[RESULT_ID] = { id: RESULT_ID, x: worldW, y: LANE_PAD_TOP, h, index: visible.length, lane: 0 }
    worldW += CARD_W + COL_GAP
    worldH = Math.max(worldH, h + LANE_PAD_TOP + LANE_PAD_BOTTOM)
  }
  worldW = worldW - COL_GAP + 80
  const last = lanes[lanes.length - 1]
  if (last) last.height = worldH - last.top

  const seq = visible.map((s) => s.id)
  if (hasResult) seq.push(RESULT_ID)
  const byId = new Map(visible.map((s) => [s.id, s]))
  const edges: LayoutEdge[] = []
  const seen = new Set<string>()

  for (let i = 0; i < seq.length - 1; i++) {
    const [from, to] = [seq[i] as string, seq[i + 1] as string]
    const target = byId.get(to)
    const kind: EdgeKind =
      target?.status === 'waiting' ? 'gate' : target?.uses.some((u) => u.fromStepId === from) ? 'flow' : 'next'
    const id = `${from}->${to}`
    seen.add(id)
    edges.push({
      id,
      source: from,
      target: to,
      kind,
      extra: false,
      toIndex: i + 1,
      path: edgePath(place[from] as Placement, place[to] as Placement),
    })
  }
  for (const s of visible) {
    for (const u of s.uses) {
      const from = seq.indexOf(u.fromStepId)
      const to = seq.indexOf(s.id)
      const id = `${u.fromStepId}=>${s.id}`
      if (from < 0 || to - from < 2 || seen.has(id)) continue
      seen.add(id)
      edges.push({
        id,
        source: u.fromStepId,
        target: s.id,
        kind: 'flow',
        extra: true,
        toIndex: to,
        path: valuePath(place[u.fromStepId] as Placement, place[s.id] as Placement),
      })
    }
  }

  return { cloud, steps: visible, numbers, lanes, place, hasResult, edges, width: worldW, height: worldH }
}
