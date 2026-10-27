import type { Action, Step } from './types'

export type Tally = Record<Action | 'denied', number>

/** How many steps of each action ran. A step you said no to counts as stopped, not as its action. */
export function countActions(steps: Step[]): Tally {
  const n: Tally = { read: 0, create: 0, change: 0, send: 0, delete: 0, unknown: 0, denied: 0 }
  for (const s of steps) {
    if (s.status === 'denied') n.denied++
    else n[s.action]++
  }
  return n
}
