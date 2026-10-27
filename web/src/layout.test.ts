import { describe, expect, it } from 'vitest'
import {
  ANCHOR_Y,
  CARD_W,
  COL_GAP,
  GUTTER,
  LANE_PAD_BOTTOM,
  LANE_PAD_TOP,
  MIN_CARD_H,
  RESULT_ID,
  computeLayout,
  estimateCardHeight,
} from './layout'
import { at, deployConv, newsletterConv } from './mock/data'
import type { Step } from './types'

const news = newsletterConv()
const step = (layout: ReturnType<typeof computeLayout>, id: string) => layout.place[id]!
const edge = (layout: ReturnType<typeof computeLayout>, from: string, to: string) =>
  layout.edges.find((e) => e.source === from && e.target === to)

describe('which steps go on the canvas', () => {
  it('keeps only cloud steps, and numbers them among themselves', () => {
    const layout = computeLayout({ steps: news.steps, filter: 'all' })
    expect(news.steps).toHaveLength(11)
    expect(layout.cloud.map((s) => s.id)).toEqual(['n1', 'n2', 'n3', 'n4', 'n5', 'n6', 'n7', 'n8', 'n9'])
    expect(layout.place.l1).toBeUndefined()
    expect(layout.place.l2).toBeUndefined()
    expect(layout.numbers.n7).toBe(7)
    expect(layout.numbers.l1).toBeUndefined()
  })

  it('"Changes only" hides reads, re-lays the columns, and keeps the numbers', () => {
    const layout = computeLayout({ steps: news.steps, filter: 'changes' })
    expect(layout.steps.map((s) => s.id)).toEqual(['n2', 'n3', 'n5', 'n6', 'n7', 'n8', 'n9'])
    expect(step(layout, 'n2').x).toBe(GUTTER)
    expect(step(layout, 'n3').x).toBe(GUTTER + CARD_W + COL_GAP)
    expect(layout.numbers.n2).toBe(2)
    expect(layout.numbers.n4).toBe(4)
  })
})

describe('lanes', () => {
  it('uses one lane per provider, in order of first appearance, when several providers are involved', () => {
    const layout = computeLayout({ steps: news.steps, filter: 'all' })
    expect(layout.lanes.map((l) => l.name)).toEqual(['Resend', 'Cloudflare', 'Stripe', 'Google Cloud'])
  })

  it('says how each provider is reached', () => {
    const layout = computeLayout({ steps: news.steps, filter: 'all' })
    expect(layout.lanes.map((l) => l.sub)).toEqual(['MCP', 'API', 'CLI', 'CLI · no adapter'])
  })

  it('uses one lane per product when the chat uses a single provider', () => {
    const layout = computeLayout({ steps: deployConv().steps, filter: 'all' })
    expect(layout.lanes.map((l) => l.name)).toEqual(['Account', 'S3', 'CloudFront'])
    expect(layout.lanes.every((l) => l.sub === 'AWS')).toBe(true)
  })

  it('puts each step in the lane of its provider', () => {
    const layout = computeLayout({ steps: news.steps, filter: 'all' })
    expect(step(layout, 'n1').lane).toBe(0)
    expect(step(layout, 'n3').lane).toBe(1)
    expect(step(layout, 'n5').lane).toBe(2)
    expect(step(layout, 'n7').lane).toBe(3)
  })

  it('sizes a lane by its tallest measured card plus padding, and stacks the lanes', () => {
    // Every card in the Resend lane is measured, so the tallest one decides the lane height.
    const heights = { n1: 200, n2: 300, n4: 100, n8: 120, n9: 250, n3: 150 }
    const layout = computeLayout({ steps: news.steps, filter: 'all', heights })
    const [resend, cloudflare] = layout.lanes
    expect(resend!.top).toBe(0)
    expect(resend!.height).toBe(300 + LANE_PAD_TOP + LANE_PAD_BOTTOM)
    expect(cloudflare!.top).toBe(resend!.height)
    expect(step(layout, 'n3').y).toBe(cloudflare!.top + LANE_PAD_TOP)
    expect(step(layout, 'n3').h).toBe(150)
  })

  it('falls back to an estimate for cards that have not been measured', () => {
    const layout = computeLayout({ steps: news.steps, filter: 'all' })
    const n1 = news.steps.find((s) => s.id === 'n1')!
    expect(step(layout, 'n1').h).toBe(estimateCardHeight(n1))
  })
})

describe('columns', () => {
  it('puts one step per column in time order', () => {
    const layout = computeLayout({ steps: news.steps, filter: 'all' })
    layout.steps.forEach((s, i) => {
      expect(step(layout, s.id).x).toBe(GUTTER + i * (CARD_W + COL_GAP))
      expect(step(layout, s.id).index).toBe(i)
    })
  })
})

describe('edges', () => {
  const layout = computeLayout({ steps: news.steps, filter: 'all', result: news.result })

  it('joins consecutive steps, skipping local ones', () => {
    expect(edge(layout, 'n1', 'n2')?.kind).toBe('next')
    expect(edge(layout, 'n4', 'n5')?.kind).toBe('next')
    expect(layout.edges.some((e) => e.source === 'l1' || e.target === 'l1')).toBe(false)
  })

  it('makes an edge a blue "flow" edge when the next step uses the one before', () => {
    expect(edge(layout, 'n2', 'n3')?.kind).toBe('flow')
    expect(edge(layout, 'n6', 'n7')?.kind).toBe('flow')
  })

  it('adds a dashed blue edge from a source that is not the step just before', () => {
    const extra = edge(layout, 'n5', 'n7')
    expect(extra).toMatchObject({ kind: 'flow', extra: true })
    expect(layout.edges.filter((e) => e.extra)).toHaveLength(1)
  })

  it('draws the edge into a waiting step red and dashed', () => {
    expect(edge(layout, 'n8', 'n9')?.kind).toBe('gate')
  })

  it('adds a result card after the last step, with an edge from it', () => {
    const withResult = computeLayout({ steps: news.steps, filter: 'all', result: { title: 'Done', line: 'ok', at: at('20:44:00') } })
    const last = step(withResult, 'n9')
    expect(withResult.place[RESULT_ID]).toMatchObject({ x: last.x + CARD_W + COL_GAP, index: 9 })
    expect(edge(withResult, 'n9', RESULT_ID)?.kind).toBe('next')
    expect(computeLayout({ steps: news.steps, filter: 'all' }).place[RESULT_ID]).toBeUndefined()
  })

  it('leaves out a value edge whose source the filter hides', () => {
    const steps: Step[] = news.steps.filter((s) => ['n1', 'n2', 'n3'].includes(s.id)).map((s) => ({ ...s }))
    const n3 = steps.find((s) => s.id === 'n3')!
    n3.uses = [{ fromStepId: 'n1', what: 'domain list' }]
    const layout = computeLayout({ steps, filter: 'changes' })
    expect(layout.edges.filter((e) => e.extra)).toHaveLength(0)
  })

  it('starts each edge at the card edge, level with the anchor', () => {
    const e = edge(layout, 'n1', 'n2')!
    const a = step(layout, 'n1')
    expect(e.path.startsWith(`M${a.x + CARD_W} ${a.y + ANCHOR_Y}`)).toBe(true)
  })
})

describe('world size and empty chats', () => {
  it('has no lanes and no edges when nothing ran on a cloud service', () => {
    const layout = computeLayout({ steps: [], filter: 'all' })
    expect(layout).toMatchObject({ lanes: [], edges: [], steps: [] })
    expect(layout.width).toBeGreaterThan(0)
  })

  it('grows with the number of columns', () => {
    const one = computeLayout({ steps: news.steps.slice(0, 1), filter: 'all' })
    const two = computeLayout({ steps: news.steps.slice(0, 2), filter: 'all' })
    expect(two.width - one.width).toBe(CARD_W + COL_GAP)
  })
})

describe('estimateCardHeight', () => {
  it('is never below the minimum and is taller for a card with buttons and a loss box', () => {
    const done = news.steps.find((s) => s.id === 'n8')!
    const waiting = news.steps.find((s) => s.id === 'n9')!
    expect(estimateCardHeight(done)).toBeGreaterThanOrEqual(MIN_CARD_H)
    expect(estimateCardHeight(waiting)).toBeGreaterThan(estimateCardHeight(done))
  })
})
