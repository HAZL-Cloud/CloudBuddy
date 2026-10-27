import { useSyncExternalStore } from 'react'

// Any 401 from the server means the cookie is missing or expired. The whole app then shows one calm message.
let signedOut = false
const listeners = new Set<() => void>()

export function setSignedOut(value: boolean): void {
  if (signedOut === value) return
  signedOut = value
  listeners.forEach((fn) => fn())
}

function subscribe(fn: () => void): () => void {
  listeners.add(fn)
  return () => listeners.delete(fn)
}

export function useSignedOut(): boolean {
  return useSyncExternalStore(subscribe, () => signedOut)
}
