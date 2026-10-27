import { CARD_W, RESULT_ID, type Layout } from './layout'

export interface Viewport {
  x: number
  y: number
  zoom: number
}

export const MIN_ZOOM = 0.25
export const MAX_ZOOM = 2
/** Width taken by the details panel (340px plus its margins) when it is open on a wide screen. */
export const INSPECTOR_INSET = 352

export const clamp = (v: number, lo: number, hi: number) => Math.max(lo, Math.min(hi, v))

export interface Size {
  width: number
  height: number
}

/** The zoom that would fit the whole world, before the 1.0 cap. */
export function rawFitZoom(view: Size, world: Size): number {
  return Math.min((view.width - 48) / world.width, (view.height - 48) / world.height)
}

export function fitViewport(view: Size, world: Size): Viewport {
  const zoom = clamp(rawFitZoom(view, world), MIN_ZOOM, 1)
  return { zoom, x: (view.width - world.width * zoom) / 2, y: (view.height - world.height * zoom) / 2 }
}

export interface FramingOptions {
  view: Size
  worldHeight: number
  target: { x: number; y: number; h: number }
  zoom: number
  /** Where across the free width the card's centre should land (0.5 is the middle). */
  xFrac: number
  /** Space covered by the details panel. */
  insetX?: number
  insetY?: number
}

export function framingViewport(o: FramingOptions): Viewport {
  const { view, worldHeight, target, zoom } = o
  const insetX = o.insetX ?? 0
  const insetY = o.insetY ?? 0
  const x = (view.width - insetX) * o.xFrac - (target.x + CARD_W / 2) * zoom
  const free = view.height - insetY
  const y =
    worldHeight * zoom <= free - 32
      ? (free - worldHeight * zoom) / 2
      : clamp(free / 2 - (target.y + target.h / 2) * zoom, free - worldHeight * zoom - 16, 16)
  return { x, y, zoom }
}

/** Which card to look at when a chat opens: the step waiting for you, else the result, else the last step. */
export function initialTarget(layout: Layout): string | null {
  const waiting = layout.steps.find((s) => s.status === 'waiting')
  if (waiting) return waiting.id
  if (layout.hasResult) return RESULT_ID
  const last = layout.steps[layout.steps.length - 1]
  return last ? last.id : null
}

export function initialViewport(layout: Layout, view: Size, mobile: boolean): Viewport {
  const world = { width: layout.width, height: layout.height }
  const targetId = initialTarget(layout)
  const target = targetId ? layout.place[targetId] : undefined
  if (!target || rawFitZoom(view, world) >= 0.62) return fitViewport(view, world)
  return framingViewport({
    view,
    worldHeight: layout.height,
    target,
    zoom: mobile ? 0.74 : 0.86,
    xFrac: mobile ? 0.5 : 0.66,
  })
}
