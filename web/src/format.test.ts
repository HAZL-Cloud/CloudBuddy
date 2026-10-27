import { describe, expect, it } from 'vitest'
import { clock, clockShort, duration, plural, relativeWhen, splitCode } from './format'

const local = (y: number, mo: number, d: number, h = 0, mi = 0, s = 0) => new Date(y, mo - 1, d, h, mi, s)

describe('clock', () => {
  it('shows local 24-hour time with seconds', () => {
    expect(clock(local(2026, 10, 26, 20, 38, 4).toISOString())).toBe('20:38:04')
    expect(clockShort(local(2026, 10, 26, 9, 5, 59).toISOString())).toBe('09:05')
  })
  it('is empty for a missing or broken time', () => {
    expect(clock(undefined)).toBe('')
    expect(clock('not a date')).toBe('')
  })
})

describe('duration', () => {
  it('formats seconds with one decimal, then minutes', () => {
    expect(duration(400)).toBe('0.4s')
    expect(duration(21_400)).toBe('21.4s')
    expect(duration(65_000)).toBe('1m 05s')
    expect(duration(undefined)).toBe('')
  })
})

describe('relativeWhen', () => {
  const now = local(2026, 10, 26, 22, 0, 0)
  const ago = (ms: number) => new Date(now.getTime() - ms).toISOString()
  it('counts minutes, then hours, for today', () => {
    expect(relativeWhen(ago(10_000), now)).toBe('Just now')
    expect(relativeWhen(ago(5 * 60_000), now)).toBe('5 min ago')
    expect(relativeWhen(ago(2 * 3_600_000), now)).toBe('2 h ago')
  })
  it('says Yesterday, then a date', () => {
    expect(relativeWhen(local(2026, 10, 25, 9, 31).toISOString(), now)).toBe('Yesterday')
    expect(relativeWhen(local(2026, 10, 22, 14, 10).toISOString(), now)).toBe('Oct 22')
    expect(relativeWhen(local(2025, 3, 2, 14, 10).toISOString(), now)).toBe('Mar 2, 2025')
  })
  it('does not show a future time as negative', () => {
    expect(relativeWhen(new Date(now.getTime() + 60_000).toISOString(), now)).toBe('Just now')
  })
})

describe('plural', () => {
  it('uses the singular for one', () => {
    expect(plural(1, 'step')).toBe('1 step')
    expect(plural(6, 'step')).toBe('6 steps')
  })
})

describe('splitCode', () => {
  it('splits text on backtick spans', () => {
    expect(splitCode('Add them in `shipyard.dev` now')).toEqual([
      { text: 'Add them in ', code: false },
      { text: 'shipyard.dev', code: true },
      { text: ' now', code: false },
    ])
  })
  it('leaves an unmatched backtick alone and never produces markup', () => {
    expect(splitCode('a ` b')).toEqual([{ text: 'a ` b', code: false }])
    expect(splitCode('<b>`x`</b>')).toEqual([
      { text: '<b>', code: false },
      { text: 'x', code: true },
      { text: '</b>', code: false },
    ])
  })
})
