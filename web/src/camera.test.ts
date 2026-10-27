import { describe, expect, it } from 'vitest'
import { INSPECTOR_INSET, fitViewport, framingViewport, initialTarget, initialViewport, rawFitZoom } from './camera'
import { CARD_W, RESULT_ID, computeLayout } from './layout'
import { deployConv, newsletterConv } from './mock/data'

const view = { width: 900, height: 600 }

describe('fitViewport', () => {
  it('scales the world to fit and centres it', () => {
    const world = { width: 1800, height: 400 }
    const vp = fitViewport(view, world)
    expect(vp.zoom).toBeCloseTo((900 - 48) / 1800)
    expect(vp.x).toBeCloseTo((900 - 1800 * vp.zoom) / 2)
    expect(vp.y).toBeCloseTo((600 - 400 * vp.zoom) / 2)
  })

  it('never zooms in past 100%', () => {
    expect(fitViewport(view, { width: 100, height: 100 }).zoom).toBe(1)
  })

  it('never zooms out past 25%', () => {
    expect(fitViewport(view, { width: 100000, height: 100 }).zoom).toBe(0.25)
  })
})

describe('framingViewport', () => {
  const target = { x: 1000, y: 300, h: 200 }

  it('puts the card centre at the chosen share of the width', () => {
    const vp = framingViewport({ view, worldHeight: 2000, target, zoom: 1, xFrac: 0.5 })
    expect(vp.x + (target.x + CARD_W / 2) * vp.zoom).toBeCloseTo(450)
  })

  it('leaves room for the details panel', () => {
    const vp = framingViewport({ view, worldHeight: 2000, target, zoom: 1, xFrac: 0.5, insetX: INSPECTOR_INSET })
    expect(vp.x + (target.x + CARD_W / 2) * vp.zoom).toBeCloseTo((900 - INSPECTOR_INSET) / 2)
  })

  it('centres a short world vertically instead of following the card', () => {
    const vp = framingViewport({ view, worldHeight: 300, target, zoom: 1, xFrac: 0.5 })
    expect(vp.y).toBeCloseTo((600 - 300) / 2)
  })

  it('keeps a tall world on the card but never scrolls past its edges', () => {
    const vp = framingViewport({ view, worldHeight: 3000, target: { x: 0, y: 0, h: 100 }, zoom: 1, xFrac: 0.5 })
    expect(vp.y).toBe(16)
  })
})

describe('initialViewport', () => {
  it('fits everything when a small chat is open', () => {
    const layout = computeLayout({ steps: deployConv().steps.slice(0, 3), filter: 'all' })
    const vp = initialViewport(layout, view, false)
    expect(vp).toEqual(fitViewport(view, layout))
  })

  it('looks at the step waiting for you when the world is too big to fit', () => {
    const news = newsletterConv()
    const layout = computeLayout({ steps: news.steps, filter: 'all' })
    expect(rawFitZoom(view, layout)).toBeLessThan(0.62)
    expect(initialTarget(layout)).toBe('n9')
    const vp = initialViewport(layout, view, false)
    expect(vp.zoom).toBe(0.86)
    const p = layout.place.n9!
    expect(vp.x + (p.x + CARD_W / 2) * vp.zoom).toBeCloseTo(900 * 0.66)
  })

  it('uses a smaller zoom and a centred card on a phone', () => {
    const layout = computeLayout({ steps: newsletterConv().steps, filter: 'all' })
    expect(initialViewport(layout, { width: 390, height: 600 }, true).zoom).toBe(0.74)
  })

  it('prefers the result card, then the last step, when nothing waits', () => {
    const deploy = deployConv()
    const done = computeLayout({ steps: deploy.steps, result: deploy.result, filter: 'all' })
    expect(initialTarget(done)).toBe(RESULT_ID)
    const noResult = computeLayout({ steps: deploy.steps, filter: 'all' })
    expect(initialTarget(noResult)).toBe('d9')
  })
})
