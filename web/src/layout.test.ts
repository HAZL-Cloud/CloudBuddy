import { describe, expect, it } from 'vitest'
import {
  CARD_W,
  COL_GAP,
  GUTTER,
  LANE_PAD_BOTTOM,
  LANE_PAD_TOP,
  MIN_CARD_H,
  RESULT_ID,
  STACK_PREFIX,
  computeLayout,
  estimateCardHeight,
  rangeLabel,
  roundedPath,
  tracePoints,
  type Layout,
  type Point,
} from './layout'
import { alarmConv, at, deployConv, ec2Conv, newsletterConv } from './mock/data'
import type { Step } from './types'

const news = newsletterConv()
const deploy = deployConv()
const step = (layout: Layout, id: string) => layout.place[id]!
const line = (layout: Layout, from: string, to: string) => layout.lines.find((l) => l.source === from && l.target === to)
const pairs = (layout: Layout) => layout.lines.map((l) => `${l.source}>${l.target}`)

/** A chat like "check my EC2 server": reads, one of them in another product, then changes. */
function ec2Check(): Step[] {
  const base = ec2Conv().steps.find((s) => s.id === 'e1')!
  const mk = (id: string, over: Partial<Step>): Step => ({ ...base, id, uses: [], approval: undefined, ...over })
  return [
    mk('r1', { title: 'List EC2 instances' }),
    mk('r2', { title: 'Check health of web-green' }),
    mk('r3', { title: 'Check state of all instances' }),
    mk('r4', { product: 'SSM', title: 'Is web-green managed by Systems Manager?' }),
    mk('r5', { title: "Inbound ports on web-green's security group" }),
    mk('c1', { action: 'change', title: 'Move the Elastic IP to web-green', uses: [{ fromStepId: 'r1', what: 'resource ID' }] }),
    mk('c2', {
      action: 'delete',
      status: 'waiting',
      title: 'Terminate web-blue',
      // Old links: every step that printed the ID.
      uses: [
        { fromStepId: 'r1', what: 'resource ID' },
        { fromStepId: 'r2', what: 'resource ID' },
        { fromStepId: 'r3', what: 'resource ID' },
      ],
    }),
  ]
}

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
    expect(resend!.floor).toBe(300 + LANE_PAD_TOP)
    expect(resend!.height).toBeGreaterThanOrEqual(300 + LANE_PAD_TOP + LANE_PAD_BOTTOM)
    expect(cloudflare!.top).toBe(resend!.height)
    // Nothing runs under the Cloudflare lane, so it is just its card plus padding.
    expect(cloudflare!.height).toBe(150 + LANE_PAD_TOP + LANE_PAD_BOTTOM)
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
    expect(layout.columns.map((c) => c.label)).toEqual(['1', '2', '3', '4', '5', '6', '7', '8', '9'])
  })

  it('puts back-to-back reads in one column, one card per lane', () => {
    const layout = computeLayout({ steps: deploy.steps, filter: 'all' })
    expect(step(layout, 'd1').col).toBe(0)
    expect(step(layout, 'd2').col).toBe(0)
    expect(step(layout, 'd1').lane).not.toBe(step(layout, 'd2').lane)
    expect(step(layout, 'd3').col).toBe(1)
    expect(layout.columns[0]!.label).toBe('1–2')
  })

  it('stacks back-to-back reads in the same lane into one card', () => {
    const layout = computeLayout({ steps: ec2Check(), filter: 'all' })
    const stack = layout.blocks.find((b) => b.steps.length > 1)!
    expect(stack.id).toBe(`${STACK_PREFIX}r1`)
    expect(stack.steps.map((s) => s.id)).toEqual(['r1', 'r2', 'r3', 'r5'])
    // The read in another product sits in the same column, in its own lane.
    expect(step(layout, 'r4')).toMatchObject({ col: 0, node: 'r4' })
    // Every step in the stack is found at the stack's place, with its own replay index.
    expect(step(layout, 'r3')).toMatchObject({ node: stack.id, x: step(layout, stack.id).x, index: 2 })
    expect(layout.columns.map((c) => c.label)).toEqual(['1–5', '6', '7'])
  })

  it('keeps a read that failed, or asks first, in its own column', () => {
    const steps = ec2Check().map((s) => (s.id === 'r2' ? { ...s, status: 'failed' as const } : s))
    const layout = computeLayout({ steps, filter: 'all' })
    expect([step(layout, 'r1').col, step(layout, 'r2').col, step(layout, 'r3').col]).toEqual([0, 1, 2])
  })
})

describe('lines', () => {
  const layout = computeLayout({ steps: news.steps, filter: 'all', result: news.result })
  const deployed = computeLayout({ steps: deploy.steps, filter: 'all', result: deploy.result })

  it('has no next-step lines: only from a step that created something to a step that builds on it', () => {
    expect(pairs(layout)).toEqual(['n2>n3', 'n5>n7', 'n6>n7'])
    expect(pairs(deployed)).toEqual(['d3>d4', 'd3>d5', 'd6>d7'])
    expect(line(layout, 'n5', 'n7')?.whats).toEqual(['plan ID'])
  })

  it('shows a value found earlier as a chip, not a line', () => {
    expect(deployed.chips.d8).toEqual([{ from: ['d2'], local: false, whats: ['bucket'] }])
    const cleanup = computeLayout({ steps: ec2Conv().steps, filter: 'all' })
    expect(cleanup.lines).toHaveLength(0)
    expect(cleanup.chips.e4).toEqual([{ from: ['e1'], local: false, whats: ['server ID'] }])
  })

  it('gives a read that only looks at something created a chip, not a line', () => {
    expect(line(deployed, 'd6', 'd9')).toBeUndefined()
    expect(deployed.chips.d9).toEqual([{ from: ['d6'], local: false, whats: ['distribution ID'] }])
  })

  it('folds one value seen in several steps into one chip', () => {
    const checked = computeLayout({ steps: ec2Check(), filter: 'all' })
    expect(checked.lines).toHaveLength(0)
    expect(checked.chips.c2).toEqual([{ from: ['r1', 'r2', 'r3'], local: false, whats: ['resource ID'] }])
    // Reads inside a stack never show chips, so nothing is computed for them to show.
    expect(checked.chips.r2).toBeUndefined()
  })

  it('says a value came from local work when its step is not on the canvas', () => {
    const steps = news.steps.map((s) => (s.id === 'n3' ? { ...s, uses: [{ fromStepId: 'l1', what: '3 DNS records' }] } : s))
    expect(computeLayout({ steps, filter: 'all' }).chips.n3).toEqual([{ from: [], local: true, whats: ['3 DNS records'] }])
  })

  it('makes a value from a step the filter hides a chip', () => {
    const steps = news.steps.map((s) => (s.id === 'n3' ? { ...s, uses: [{ fromStepId: 'n1', what: 'domain list' }] } : s))
    const filtered = computeLayout({ steps, filter: 'changes' })
    expect(filtered.lines.some((l) => l.target === 'n3')).toBe(false)
    expect(filtered.chips.n3?.[0]?.from).toEqual(['n1'])
  })

  it('gives each source one colour, and its label and start dot to its first branch only', () => {
    const [a, b] = deployed.lines.filter((l) => l.source === 'd3')
    expect(a!.hue).toBe(b!.hue)
    expect([a!.first, b!.first]).toEqual([true, false])
    expect(line(deployed, 'd6', 'd7')!.hue).not.toBe(a!.hue)
  })

  it('starts at the bottom of the source and ends at the left edge of the target', () => {
    const l = line(layout, 'n2', 'n3')!
    const a = step(layout, 'n2')
    const t = step(layout, 'n3')
    expect(l.points[0]).toEqual({ x: a.x + CARD_W / 2, y: a.y + a.h })
    expect(l.points[l.points.length - 1]).toEqual({ x: t.x - 2, y: expect.any(Number) as number })
    expect(l.path.startsWith(`M${a.x + CARD_W / 2} ${a.y + a.h}`)).toBe(true)
  })

  it('runs lines from different sources on different tracks, and into a shared card at different heights', () => {
    const a = line(layout, 'n5', 'n7')!
    const b = line(layout, 'n6', 'n7')!
    expect(a.points[1]!.y).not.toBe(b.points[1]!.y)
    expect(a.points[3]!.x).not.toBe(b.points[3]!.x)
    expect(a.points[4]!.y).not.toBe(b.points[4]!.y)
  })

  it('makes room under a lane for the lines that run along it', () => {
    const stripe = layout.lanes.find((l) => l.name === 'Stripe')!
    const lowest = Math.max(...layout.lines.filter((l) => step(layout, l.source).lane === 2).map((l) => l.points[1]!.y))
    expect(lowest).toBeLessThan(stripe.top + stripe.height)
    expect(lowest).toBeGreaterThan(stripe.floor)
  })

  it('never draws a line across a card', () => {
    const inside = (p: Point, q: Point, r: { x: number; y: number; h: number }) => {
      const [x0, x1] = [Math.min(p.x, q.x), Math.max(p.x, q.x)]
      const [y0, y1] = [Math.min(p.y, q.y), Math.max(p.y, q.y)]
      return Math.max(x0, r.x) < Math.min(x1, r.x + CARD_W) && Math.max(y0, r.y) < Math.min(y1, r.y + r.h) ||
        (x0 === x1 && x0 > r.x && x0 < r.x + CARD_W && Math.max(y0, r.y) < Math.min(y1, r.y + r.h)) ||
        (y0 === y1 && y0 > r.y && y0 < r.y + r.h && Math.max(x0, r.x) < Math.min(x1, r.x + CARD_W))
    }
    for (const conv of [news, deploy, alarmConv(), ec2Conv()]) {
      for (const heights of [{}, Object.fromEntries(conv.steps.map((s, i) => [s.id, 120 + 70 * (i % 4)]))]) {
        const l = computeLayout({ steps: conv.steps, filter: 'all', result: conv.result, heights })
        const cards = [...l.blocks.map((b) => l.place[b.id]!), ...(l.hasResult ? [l.place[RESULT_ID]!] : [])]
        for (const ln of l.lines) {
          for (let i = 1; i < ln.points.length; i++) {
            for (const card of cards) {
              expect(inside(ln.points[i - 1]!, ln.points[i]!, card), `${conv.id} ${ln.id} segment ${i} crosses ${card.id}`).toBe(false)
            }
          }
        }
      }
    }
  })

  it('adds a result card after the last column, with no line to it', () => {
    const withResult = computeLayout({ steps: news.steps, filter: 'all', result: { title: 'Done', line: 'ok', at: at('20:44:00') } })
    const last = step(withResult, 'n9')
    expect(withResult.place[RESULT_ID]).toMatchObject({ x: last.x + CARD_W + COL_GAP, index: 9 })
    expect(withResult.lines.some((l) => l.target === RESULT_ID)).toBe(false)
    expect(withResult.columns[withResult.columns.length - 1]!.label).toBe('Result')
    expect(computeLayout({ steps: news.steps, filter: 'all' }).place[RESULT_ID]).toBeUndefined()
  })
})

describe('hover traces', () => {
  it('runs from where a value came from to the card with the chip', () => {
    const layout = computeLayout({ steps: deploy.steps, filter: 'all' })
    const pts = tracePoints(layout, 'd2', 'd8')!
    const a = step(layout, 'd2')
    const t = step(layout, 'd8')
    expect(pts[0]).toEqual({ x: a.x + CARD_W / 2, y: a.y + a.h })
    expect(pts[pts.length - 1]).toEqual({ x: t.x - 2, y: expect.any(Number) as number })
  })

  it('has nothing to draw within one column', () => {
    const layout = computeLayout({ steps: deploy.steps, filter: 'all' })
    expect(tracePoints(layout, 'd1', 'd2')).toBeNull()
  })
})

describe('helpers', () => {
  it('writes step ranges compactly', () => {
    expect(rangeLabel([3])).toBe('3')
    expect(rangeLabel([5, 1, 2, 3])).toBe('1–3, 5')
  })

  it('drops repeated and in-line points from a path, and rounds the corners', () => {
    expect(roundedPath([{ x: 0, y: 0 }, { x: 0, y: 0 }, { x: 0, y: 10 }, { x: 0, y: 20 }])).toBe('M0 0 L0 20')
    expect(roundedPath([{ x: 0, y: 0 }, { x: 0, y: 40 }, { x: 40, y: 40 }])).toBe('M0 0 L0 32 Q0 40 8 40 L40 40')
  })
})

describe('world size and empty chats', () => {
  it('has no lanes and no lines when nothing ran on a cloud service', () => {
    const layout = computeLayout({ steps: [], filter: 'all' })
    expect(layout).toMatchObject({ lanes: [], lines: [], steps: [], columns: [] })
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
