import type { QueryClient } from '@tanstack/react-query'
import { keys } from './queryKeys'
import type { StreamEvent } from './types'

export function parseStreamEvent(raw: string): StreamEvent | null {
  let data: unknown
  try {
    data = JSON.parse(raw)
  } catch {
    return null
  }
  if (typeof data !== 'object' || data === null) return null
  const ev = data as Record<string, unknown>
  if (ev.type === 'conversation' && typeof ev.conversationId === 'string') {
    return { type: 'conversation', conversationId: ev.conversationId }
  }
  if (ev.type === 'conversations') return { type: 'conversations' }
  if (ev.type === 'connections') return { type: 'connections' }
  return null
}

// Every event only means "refetch this" (api.md). The server's data stays the single source of truth.
export function applyStreamEvent(client: QueryClient, ev: StreamEvent): void {
  switch (ev.type) {
    case 'conversation':
      void client.invalidateQueries({ queryKey: keys.conversation(ev.conversationId) })
      void client.invalidateQueries({ queryKey: keys.conversations })
      break
    case 'conversations':
      void client.invalidateQueries({ queryKey: keys.conversations })
      break
    case 'connections':
      void client.invalidateQueries({ queryKey: keys.connections })
      break
  }
}

export function connectStream(client: QueryClient): () => void {
  if (import.meta.env.VITE_MOCK === '1') {
    let closed = false
    let off = () => {}
    void import('./mock/mockApi').then((mock) => {
      if (!closed) off = mock.subscribeMock((ev) => applyStreamEvent(client, ev))
    })
    return () => {
      closed = true
      off()
    }
  }
  const source = new EventSource('/stream')
  let dropped = false
  source.onmessage = (msg: MessageEvent<string>) => {
    const ev = parseStreamEvent(msg.data)
    if (ev) applyStreamEvent(client, ev)
  }
  source.onerror = () => {
    dropped = true
  }
  // After a reconnect we may have missed events, so refetch everything once.
  source.onopen = () => {
    if (dropped) {
      dropped = false
      void client.invalidateQueries()
    }
  }
  return () => source.close()
}
