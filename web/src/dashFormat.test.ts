import { describe, expect, it } from 'vitest'
import { categoryLabel, compact, delta, errorLine, formatValue, niceTicks, placeLabels, refreshLabel, tickLabel } from './dashFormat'

describe('dashboard numbers', () => {
  it('writes numbers in full under 10,000 and compact above', () => {
    expect(compact(1284)).toBe('1,284')
    expect(compact(12_900)).toBe('12.9K')
    expect(compact(4_200_000)).toBe('4.2M')
    expect(compact(150_000)).toBe('150K')
    expect(compact(3.14159)).toBe('3.1')
    expect(compact(0.25)).toBe('0.25')
    expect(compact(7, 2)).toBe('7.00')
  })

  it('formats currency, percent, bytes and durations', () => {
    expect(formatValue(1234.5, { format: 'currency', currency: 'USD' })).toBe('$1,234.50')
    expect(formatValue(4_200_000, { format: 'currency', currency: 'USD' })).toBe('$4.2M')
    expect(formatValue(45, { format: 'percent' })).toBe('45%')
    expect(formatValue(1536, { format: 'bytes' })).toBe('1.5 KB')
    expect(formatValue(200, { format: 'duration' })).toBe('3m 20s')
    expect(formatValue(7200, { format: 'duration' })).toBe('2 h')
  })

  it('signs a change and colours it by which way is good', () => {
    expect(delta(1204, 1100, { good: 'up' })).toMatchObject({ text: '+104', pct: '+9.5%', tone: 'good', direction: 'up' })
    expect(delta(90, 100, { good: 'up' })).toMatchObject({ text: '−10', tone: 'bad', direction: 'down' })
    expect(delta(90, 100, { good: 'down' }).tone).toBe('good')
    expect(delta(5, 5)).toMatchObject({ text: 'No change', tone: 'flat' })
    expect(delta(40, 35, { format: 'percent' }).text).toBe('+5 pts')
  })

  it('picks round axis ticks', () => {
    expect(niceTicks(0, 1000, 4)).toEqual([0, 250, 500, 750, 1000])
    expect(niceTicks(1162, 1204, 4)).toEqual([1160, 1180, 1200, 1220])
    const flat = niceTicks(5, 5)
    expect(flat.length).toBeGreaterThan(1)
    expect(flat[0]).toBeLessThan(5)
    expect(flat[flat.length - 1]).toBeGreaterThan(5)
    expect(tickLabel(20_000, 40_000)).toBe('20K')
    expect(tickLabel(2000, 4000)).toBe('2,000')
  })

  it('says how often a dashboard refreshes', () => {
    expect(refreshLabel('15m')).toBe('every 15 min')
    expect(refreshLabel('1h')).toBe('every hour')
    expect(refreshLabel('6h')).toBe('every 6 hours')
    expect(refreshLabel('1d')).toBe('every day')
  })

  it('writes plain dates on a chart axis as month and day', () => {
    expect(categoryLabel('2026-10-16')).toBe('Oct 16')
    expect(categoryLabel('2026-01-02', true)).toBe('Jan 2, 2026')
    expect(categoryLabel('free')).toBe('free')
    expect(categoryLabel('2026-13-01')).toBe('2026-13-01')
  })

  it('places x-axis labels so none touch: first, last and evenly between', () => {
    // 30 days across 400 px, as on a two-column tile
    const xs = Array.from({ length: 30 }, (_, i) => 52 + (i * 400) / 29)
    const widths = xs.map(() => 66) // "2026-10-16" at 11 px
    const placed = placeLabels(xs, widths, 52, 452)
    expect(placed[0]?.i).toBe(0)
    expect(placed[placed.length - 1]?.i).toBe(29)
    expect(placed.length).toBeGreaterThan(2)
    for (let j = 1; j < placed.length; j++) {
      const prev = placed[j - 1]!
      expect(placed[j]!.x - (prev.x + 66)).toBeGreaterThanOrEqual(12)
    }
    // the first and last labels stay inside the plot
    expect(placed[0]!.x).toBeGreaterThanOrEqual(52)
    expect(placed[placed.length - 1]!.x + 66).toBeLessThanOrEqual(452)
    // too narrow for two: just the first
    expect(placeLabels([0, 10], [66, 66], 0, 80)).toEqual([{ i: 0, x: 0 }])
  })

  it('picks the line of an error that says what went wrong', () => {
    const node = [
      'node:internal/errors:983',
      '  const err = new Error(message);',
      '              ^',
      '',
      'Error: Command failed: npx wrangler whoami',
      '    at genericNodeError (node:internal/errors:983:15)',
      'Node.js v24.2.0',
    ].join('\n')
    expect(errorLine(node)).toBe('Error: Command failed: npx wrangler whoami')
    expect(errorLine('✘ [ERROR] Not logged in.\n')).toBe('✘ [ERROR] Not logged in.')
    expect(errorLine('psql: error: connection to server at "db" failed: timeout')).toBe('psql: error: connection to server at "db" failed: timeout')
    expect(errorLine('Traceback (most recent call last):\n  File "s.py", line 3\nFileNotFoundError: [Errno 2] No such file')).toBe(
      'FileNotFoundError: [Errno 2] No such file',
    )
    expect(errorLine('something odd\nlast words')).toBe('last words')
    expect(errorLine('')).toBe('')
  })
})
