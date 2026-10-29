import '@testing-library/jest-dom/vitest'
import { cleanup } from '@testing-library/react'
import { afterEach, beforeEach } from 'vitest'
import { configureMock, resetMock } from '../mock/mockApi'

afterEach(() => cleanup())

// Every test starts from the seeded mock server with no waiting delays.
beforeEach(() => {
  window.localStorage.clear()
  delete document.documentElement.dataset.theme
  resetMock()
  configureMock({ speed: 0 })
})

// jsdom has no layout engine. These stand-ins give React Flow and our size hooks something to measure.
class ResizeObserverStub {
  constructor(private readonly callback: ResizeObserverCallback) {}
  observe(target: Element) {
    const contentRect = { x: 0, y: 0, top: 0, left: 0, width: 900, height: 600, right: 900, bottom: 600 }
    this.callback([{ target, contentRect } as unknown as ResizeObserverEntry], this as unknown as ResizeObserver)
  }
  unobserve() {}
  disconnect() {}
}
globalThis.ResizeObserver = ResizeObserverStub as unknown as typeof ResizeObserver

class DOMMatrixReadOnlyStub {
  m22 = 1
}
globalThis.DOMMatrixReadOnly = DOMMatrixReadOnlyStub as unknown as typeof DOMMatrixReadOnly

for (const [prop, value] of [
  ['clientWidth', 900],
  ['clientHeight', 600],
  ['offsetWidth', 244],
  ['offsetHeight', 180],
] as const) {
  Object.defineProperty(HTMLElement.prototype, prop, { configurable: true, get: () => value })
}

if (!window.matchMedia) {
  window.matchMedia = ((query: string) => ({
    matches: false,
    media: query,
    onchange: null,
    addEventListener: () => {},
    removeEventListener: () => {},
    addListener: () => {},
    removeListener: () => {},
    dispatchEvent: () => false,
  })) as unknown as typeof window.matchMedia
}
