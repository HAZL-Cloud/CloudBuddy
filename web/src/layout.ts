import type { RunResult, Step } from './types'
import { VIA_LABELS } from './vocab'

// The canvas geometry (D-053). Everything here is pure so it can be unit-tested.
export const CARD_W = 244
export const COL_GAP = 64
export const GUTTER = 140
export const LANE_PAD_TOP = 50
export const LANE_PAD_BOTTOM = 28
export const MIN_CARD_H = 110
export const ANCHOR_Y = 22
export const RESULT_ID = 'result'
export const STACK_PREFIX = 'stack:'
/** Rows a stack of reads shows before "Show more". */
export const STACK_ROWS = 3
/** How many line colours there are. */
export const HUES = 4
const RESULT_ESTIMATE_H = 190
/** The first line under a lane's cards runs this far below the tallest one. */
const CHANNEL_TOP = 16
/** Spacing between lines that run side by side, along a lane or down a gap. */
const TRACK_GAP = 10
/** Spacing between lines that enter the same card. */
const ENTRY_GAP = 12
/** A hover trace runs closer to the cards than the lines, so the two never sit on top of each other. */
const TRACE_TOP = 7

export type StepFilter = 'all' | 'changes'

export interface Lane {
  id: string
  name: string
  sub: string
  top: number
  height: number
  /** Where the lane's tallest card ends. Lines run along the lane below this. */
  floor: number
}

export interface Column {
  x: number
  label: string
  /** Index in the visible steps of the column's first step (the result's index for the result column). */
  index: number
}

/** One card on the canvas: a step, or a stack of back-to-back reads in one lane. */
export interface Block {
  /** The card's node id: the step's id, or STACK_PREFIX + the first step's id. */
  id: string
  steps: Step[]
  lane: number
  col: number
}

export interface Placement {
  id: string
  /** The card this step is drawn on. Differs from id for a step inside a stack. */
  node: string
  x: number
  y: number
  h: number
  /** Position in the visible sequence. The result card comes after the last step. */
  index: number
  lane: number
  col: number
}

export interface Point {
  x: number
  y: number
}

/** Something a step created, carried to a later step that builds on it. */
export interface Line {
  id: string
  source: string
  target: string
  /** What is carried, like "distribution ID". */
  whats: string[]
  hue: number
  /** The first branch of a source's line carries its start dot and its label. */
  first: boolean
  toIndex: number
  points: Point[]
  path: string
}

/** Values a card uses that no line carries, grouped by where they first appeared. */
export interface Chip {
  /** Cloud steps the values came from, in step order. */
  from: string[]
  /** True when some came from local work, which is not on the canvas. */
  local: boolean
  whats: string[]
}

export interface Layout {
  /** Every step that belongs on the canvas, before the filter. */
  cloud: Step[]
  /** The steps shown, in order. */
  steps: Step[]
  /** Canvas step numbers (1-based among cloud steps), also for steps the filter hides. */
  numbers: Record<string, number>
  lanes: Lane[]
  columns: Column[]
  blocks: Block[]
  /** By step id and by card id, plus RESULT_ID. A step in a stack has its stack's position. */
  place: Record<string, Placement>
  hasResult: boolean
  lines: Line[]
  chips: Record<string, Chip[]>
  width: number
  height: number
}

export interface LayoutInput {
  steps: Step[]
  result?: RunResult
  filter: StepFilter
  /** Measured card heights by card id. Missing ones fall back to an estimate. */
  heights?: Record<string, number>
}

const lines = (text: string, perLine: number) => Math.max(1, Math.ceil(text.length / perLine))

/** A rough card height, used until React Flow has measured the real one. */
export function estimateCardHeight(step: Step, chips = 0): number {
  const waiting = step.status === 'waiting'
  const parts: number[] = [18, 19 * lines(step.title, 27)]
  if (waiting) parts.unshift(22)
  if (step.resource) parts.push(17 * lines(step.resource, 30))
  if (step.tags.length) parts.push(22)
  if (step.generic) parts.push(46)
  if (chips) parts.push(24 * chips)
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

export function estimateStackHeight(count: number): number {
  const rows = Math.min(count, STACK_ROWS)
  const more = count > STACK_ROWS ? 30 : 0
  return Math.max(MIN_CARD_H, 24 + 18 + 8 + rows * 30 + more)
}

/** Reads that finished, or are running, sit together. A read that failed or asks first keeps its own column. */
const canStack = (s: Step) => s.action === 'read' && (s.status === 'done' || s.status === 'running')

/** "1", "1–3", or "1, 3–5" for the steps in a column or a stack. */
export function rangeLabel(nums: number[]): string {
  const sorted = [...nums].sort((a, b) => a - b)
  const parts: string[] = []
  let i = 0
  while (i < sorted.length) {
    let j = i
    while (j + 1 < sorted.length && sorted[j + 1] === (sorted[j] as number) + 1) j++
    parts.push(i === j ? `${sorted[i]}` : `${sorted[i]}–${sorted[j]}`)
    i = j + 1
  }
  return parts.join(', ')
}

/** An orthogonal path with rounded corners. Repeated and in-line points are dropped. */
export function roundedPath(points: Point[], r = 8): string {
  const pts: Point[] = []
  for (const p of points) {
    const last = pts[pts.length - 1]
    if (last && Math.abs(last.x - p.x) < 0.5 && Math.abs(last.y - p.y) < 0.5) continue
    const prev = pts[pts.length - 2]
    if (last && prev && ((prev.x === last.x && last.x === p.x) || (prev.y === last.y && last.y === p.y))) pts.pop()
    pts.push(p)
  }
  const first = pts[0]
  if (!first) return ''
  let d = `M${first.x} ${first.y}`
  for (let i = 1; i < pts.length - 1; i++) {
    const [a, b, c] = [pts[i - 1] as Point, pts[i] as Point, pts[i + 1] as Point]
    const r1 = Math.min(r, Math.hypot(b.x - a.x, b.y - a.y) / 2, Math.hypot(c.x - b.x, c.y - b.y) / 2)
    const towards = (p: Point, q: Point) => {
      const len = Math.hypot(q.x - p.x, q.y - p.y) || 1
      return { x: p.x + ((q.x - p.x) / len) * r1, y: p.y + ((q.y - p.y) / len) * r1 }
    }
    const p1 = towards(b, a)
    const p2 = towards(b, c)
    d += ` L${p1.x} ${p1.y} Q${b.x} ${b.y} ${p2.x} ${p2.y}`
  }
  const end = pts[pts.length - 1] as Point
  if (pts.length > 1) d += ` L${end.x} ${end.y}`
  return d
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

function groupChips(found: { from: string | null; what: string }[], numbers: Record<string, number>): Chip[] {
  const byWhat = new Map<string, Set<string>>()
  for (const f of found) {
    const set = byWhat.get(f.what) ?? new Set<string>()
    set.add(f.from ?? '')
    byWhat.set(f.what, set)
  }
  const groups = new Map<string, Chip>()
  for (const [what, set] of byWhat) {
    const from = [...set].filter(Boolean).sort((a, b) => (numbers[a] ?? 0) - (numbers[b] ?? 0))
    const local = set.has('')
    const key = `${from.join(',')}|${local}`
    const chip = groups.get(key)
    if (chip) chip.whats.push(what)
    else groups.set(key, { from, local, whats: [what] })
  }
  return [...groups.values()]
}

export function computeLayout({ steps, result, filter, heights = {} }: LayoutInput): Layout {
  const cloud = steps.filter((s) => s.kind === 'cloud')
  const numbers: Record<string, number> = {}
  cloud.forEach((s, i) => {
    numbers[s.id] = i + 1
  })
  const visible = filter === 'changes' ? cloud.filter((s) => s.action !== 'read') : cloud
  const indexOf = new Map(visible.map((s, i) => [s.id, i]))

  // One lane per provider when the chat touches several; otherwise one per product. First appearance sets the order.
  const multi = new Set(cloud.map((s) => s.provider)).size > 1
  const laneOf = (s: Step) => (multi ? s.provider : s.product)
  const laneIds = [...new Set(visible.map(laneOf))]

  // Left to right is the order. Back-to-back reads share one column, and the reads in each lane stack into one card.
  const blocks: Block[] = []
  const blockOf = new Map<string, Block>()
  let col = -1
  let inReads = false
  for (const s of visible) {
    const stack = canStack(s)
    if (!(stack && inReads)) col++
    inReads = stack
    const lane = laneIds.indexOf(laneOf(s))
    let block = stack ? blocks.find((b) => b.col === col && b.lane === lane) : undefined
    if (!block) {
      block = { id: s.id, steps: [], lane, col }
      blocks.push(block)
    }
    block.steps.push(s)
    blockOf.set(s.id, block)
  }
  for (const b of blocks) if (b.steps.length > 1) b.id = STACK_PREFIX + (b.steps[0] as Step).id
  const stepCols = col + 1

  // A line carries something a step created to each later step that builds on it. Everything else is a chip.
  const byId = new Map(cloud.map((s) => [s.id, s]))
  const branches: { source: string; target: string; whats: string[] }[] = []
  const chips: Record<string, Chip[]> = {}
  for (const s of visible) {
    const carried = new Map<string, string[]>()
    const found: { from: string | null; what: string }[] = []
    for (const u of s.uses) {
      const from = byId.get(u.fromStepId)
      const fromIndex = from ? indexOf.get(from.id) : undefined
      const builds = s.action !== 'read' && from?.action === 'create' && fromIndex !== undefined && fromIndex < (indexOf.get(s.id) as number)
      if (builds && from) carried.set(from.id, [...(carried.get(from.id) ?? []), u.what])
      else found.push({ from: from ? from.id : null, what: u.what })
    }
    for (const [source, whats] of carried) branches.push({ source, target: s.id, whats: [...new Set(whats)] })
    const grouped = groupChips(found, numbers)
    if (grouped.length) chips[s.id] = grouped
  }

  const heightOf = (b: Block) => {
    const first = b.steps[0] as Step
    return heights[b.id] ?? (b.steps.length > 1 ? estimateStackHeight(b.steps.length) : estimateCardHeight(first, chips[first.id]?.length ?? 0))
  }
  const colX = (c: number) => GUTTER + c * (CARD_W + COL_GAP)

  // Each source runs along its own track under its lane, so two lines never share a stretch.
  const sources = [...new Set(branches.map((b) => b.source))].sort((a, b) => (indexOf.get(a) as number) - (indexOf.get(b) as number))
  const trackOf = new Map<string, number>()
  const laneTracks: number[][] = laneIds.map(() => [])
  for (const src of sources) {
    const b = blockOf.get(src) as Block
    const start = colX(b.col) + CARD_W / 2
    const end = Math.max(...branches.filter((x) => x.source === src).map((x) => colX((blockOf.get(x.target) as Block).col)))
    const ends = laneTracks[b.lane] as number[]
    let t = ends.findIndex((e) => e < start)
    if (t < 0) t = ends.length
    ends[t] = end
    trackOf.set(src, t)
  }

  const lanes: Lane[] = []
  let y = 0
  laneIds.forEach((id, li) => {
    const tallest = Math.max(MIN_CARD_H, ...blocks.filter((b) => b.lane === li).map(heightOf))
    const tracks = (laneTracks[li] as number[]).length
    const channel = tracks ? CHANNEL_TOP + (tracks - 1) * TRACK_GAP + 14 : 0
    const height = LANE_PAD_TOP + tallest + Math.max(LANE_PAD_BOTTOM, channel)
    lanes.push({ id, ...laneNameAndSub(id, multi, cloud), top: y, height, floor: y + LANE_PAD_TOP + tallest })
    y += height
  })

  const place: Record<string, Placement> = {}
  for (const b of blocks) {
    const first = b.steps[0] as Step
    const p: Placement = {
      id: b.id,
      node: b.id,
      x: colX(b.col),
      y: (lanes[b.lane]?.top ?? 0) + LANE_PAD_TOP,
      h: heightOf(b),
      index: indexOf.get(first.id) as number,
      lane: b.lane,
      col: b.col,
    }
    place[b.id] = p
    for (const s of b.steps) place[s.id] = { ...p, id: s.id, index: indexOf.get(s.id) as number }
  }

  const columns: Column[] = []
  for (let c = 0; c < stepCols; c++) {
    const inCol = visible.filter((s) => blockOf.get(s.id)?.col === c)
    columns.push({ x: colX(c), label: rangeLabel(inCol.map((s) => numbers[s.id] as number)), index: indexOf.get((inCol[0] as Step).id) as number })
  }

  let worldW = colX(stepCols)
  const hasResult = result !== undefined
  let worldH = y
  if (hasResult) {
    const h = heights[RESULT_ID] ?? RESULT_ESTIMATE_H
    place[RESULT_ID] = { id: RESULT_ID, node: RESULT_ID, x: worldW, y: LANE_PAD_TOP, h, index: visible.length, lane: 0, col: stepCols }
    columns.push({ x: worldW, label: 'Result', index: visible.length })
    worldW += CARD_W + COL_GAP
    worldH = Math.max(worldH, h + LANE_PAD_TOP + LANE_PAD_BOTTOM)
  }
  worldW = worldW - COL_GAP + 80
  const last = lanes[lanes.length - 1]
  if (last) last.height = worldH - last.top

  // Route each branch: down from the source to its track, along the lane, then up or down the gap before the
  // target's column, and in from the left. Gaps and the space under the cards are never covered by a card.
  const gapSlots = new Map<number, string[]>()
  for (const b of branches) {
    const c = (place[b.target] as Placement).col
    const list = gapSlots.get(c) ?? []
    if (!list.includes(b.source)) list.push(b.source)
    gapSlots.set(c, list)
  }
  const entries = new Map<string, string[]>()
  for (const b of branches) entries.set(b.target, [...(entries.get(b.target) ?? []), b.source])

  const drawn = new Set<string>()
  const lines: Line[] = branches.map((b) => {
    const a = place[b.source] as Placement
    const t = place[b.target] as Placement
    const lane = lanes[a.lane] as Lane
    const cy = lane.floor + CHANNEL_TOP + (trackOf.get(b.source) as number) * TRACK_GAP
    const slots = gapSlots.get(t.col) as string[]
    const gx = t.x - COL_GAP / 2 + (slots.indexOf(b.source) - (slots.length - 1) / 2) * TRACK_GAP
    const ty = t.y + ANCHOR_Y + (entries.get(b.target) as string[]).indexOf(b.source) * ENTRY_GAP
    const sx = a.x + CARD_W / 2
    const points = [
      { x: sx, y: a.y + a.h },
      { x: sx, y: cy },
      { x: gx, y: cy },
      { x: gx, y: ty },
      { x: t.x - 2, y: ty },
    ]
    const first = !drawn.has(b.source)
    drawn.add(b.source)
    return {
      id: `${b.source}=>${b.target}`,
      source: b.source,
      target: b.target,
      whats: b.whats,
      hue: sources.indexOf(b.source) % HUES,
      first,
      toIndex: t.index,
      points,
      path: roundedPath(points),
    }
  })

  return { cloud, steps: visible, numbers, lanes, columns, blocks, place, hasResult, lines, chips, width: worldW, height: worldH }
}

/** The dashed line drawn while you hover a chip: from where a value came from to the card that uses it. */
export function tracePoints(layout: Layout, from: string, to: string): Point[] | null {
  const a = layout.place[from]
  const t = layout.place[to]
  const lane = a ? layout.lanes[a.lane] : undefined
  if (!a || !t || !lane || a.col >= t.col) return null
  const cy = lane.floor + TRACE_TOP
  const gx = t.x - COL_GAP / 2
  const ty = t.y + ANCHOR_Y
  const sx = a.x + CARD_W / 2
  return [
    { x: sx, y: a.y + a.h },
    { x: sx, y: cy },
    { x: gx, y: cy },
    { x: gx, y: ty },
    { x: t.x - 2, y: ty },
  ]
}
