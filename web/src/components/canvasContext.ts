import { createContext, useContext } from 'react'
import type { Decision, Step, StepApp } from '../types'
import type { Deciding } from './StepParts'

/** What the cards and edges inside React Flow need from the panel, without putting it in every node object. */
export interface CanvasView {
  numbers: Record<string, number>
  selectedId: string | null
  /** null means "live": nothing is dimmed. */
  viewIndex: number | null
  deciding: Deciding | null
  /** Steps a hovered chip traces back to. */
  traced: ReadonlySet<string>
  /** True for cards that appeared after the chat was opened. */
  isNew: (id: string) => boolean
  onSelect: (id: string, viaKeyboard?: boolean) => void
  onDecide: (step: Step, decision: Decision) => void
  onTrace: (stepId: string, sources: string[] | null) => void
  /** Select a step and bring it into view, switching the filter if it is hidden. */
  onShow: (stepId: string) => void
  /** Open what a step made, such as a dashboard (D-067). */
  onOpenApp: (app: StepApp) => void
}

const empty: CanvasView = {
  numbers: {},
  selectedId: null,
  viewIndex: null,
  deciding: null,
  traced: new Set(),
  isNew: () => false,
  onSelect: () => {},
  onDecide: () => {},
  onTrace: () => {},
  onShow: () => {},
  onOpenApp: () => {},
}

export const CanvasViewContext = createContext<CanvasView>(empty)
export const useCanvasView = () => useContext(CanvasViewContext)
