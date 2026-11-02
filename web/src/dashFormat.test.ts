import { describe, expect, it } from 'vitest'
import { compact, delta, formatValue, niceTicks, refreshLabel, tickLabel } from './dashFormat'

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
})
