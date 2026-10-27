import { createContext, useContext } from 'react'
import type { Decision, Step } from '../types'
import type { Deciding } from './StepParts'

/** What the cards and edges inside React Flow need from the panel, without putting it in every node object. */
export interface CanvasView {
  numbers: Record<string, number>
  selectedId: string | null
  /** null means "live": nothing is dimmed. */
  viewIndex: number | null
  deciding: Deciding | null
  /** True for cards that appeared after the chat was opened. */
  isNew: (id: string) => boolean
  onSelect: (id: string, viaKeyboard?: boolean) => void
  onDecide: (step: Step, decision: Decision) => void
}

const empty: CanvasView = {
  numbers: {},
  selectedId: null,
  viewIndex: null,
  deciding: null,
  isNew: () => false,
  onSelect: () => {},
  onDecide: () => {},
}

export const CanvasViewContext = createContext<CanvasView>(empty)
export const useCanvasView = () => useContext(CanvasViewContext)
