import { QueryClient } from '@tanstack/react-query'
import { describe, expect, it, vi } from 'vitest'
import { keys } from './queryKeys'
import { applyStreamEvent, parseStreamEvent } from './stream'

describe('parseStreamEvent', () => {
  it('reads the three event types from api.md', () => {
    expect(parseStreamEvent('{"type":"conversation","conversationId":"c1"}')).toEqual({ type: 'conversation', conversationId: 'c1' })
    expect(parseStreamEvent('{"type":"conversations"}')).toEqual({ type: 'conversations' })
    expect(parseStreamEvent('{"type":"connections"}')).toEqual({ type: 'connections' })
  })
  it('ignores anything else', () => {
    expect(parseStreamEvent('nope')).toBeNull()
    expect(parseStreamEvent('null')).toBeNull()
    expect(parseStreamEvent('{"type":"conversation"}')).toBeNull()
    expect(parseStreamEvent('{"type":"other"}')).toBeNull()
  })
})

describe('applyStreamEvent', () => {
  const spy = () => {
    const client = new QueryClient()
    const invalidate = vi.spyOn(client, 'invalidateQueries').mockResolvedValue()
    return { client, keysOf: () => invalidate.mock.calls.map((c) => c[0]?.queryKey) }
  }
  it('refetches that conversation and the list', () => {
    const { client, keysOf } = spy()
    applyStreamEvent(client, { type: 'conversation', conversationId: 'c1' })
    expect(keysOf()).toEqual([keys.conversation('c1'), keys.conversations])
  })
  it('refetches only the list, or only the connections', () => {
    const a = spy()
    applyStreamEvent(a.client, { type: 'conversations' })
    expect(a.keysOf()).toEqual([keys.conversations])
    const b = spy()
    applyStreamEvent(b.client, { type: 'connections' })
    expect(b.keysOf()).toEqual([keys.connections])
  })
})
