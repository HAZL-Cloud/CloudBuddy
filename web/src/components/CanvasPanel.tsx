import {
  Background,
  BackgroundVariant,
  ReactFlow,
  ReactFlowProvider,
  useReactFlow,
  type NodeChange,
} from '@xyflow/react'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import {
  INSPECTOR_INSET,
  MAX_ZOOM,
  MIN_ZOOM,
  clamp,
  fitViewport,
  framingViewport,
  initialViewport,
} from '../camera'
import { prefersReducedMotion, useElementSize } from '../hooks'
import { RESULT_ID, computeLayout, type StepFilter } from '../layout'
import { plural } from '../format'
import type { ConversationDetail, Decision, Step } from '../types'
import { CanvasViewContext, type CanvasView } from './canvasContext'
import { EDGE_TYPES, EdgeMarkers, NODE_TYPES } from './canvasNodes'
import { ColumnRuler, LaneRail, TallyPills, ZoomControls } from './CanvasChrome'
import { LANE_PREFIX, toFlow, type CanvasNode, type Trace } from './flowGraph'
import { Icon } from './Icon'
import { Inspector } from './Inspector'
import { Scrubber } from './Scrubber'
import type { Deciding } from './StepParts'

export type FocusTarget = 'waiting' | { stepId: string }
/** Asks the canvas to show a step: from the chat, or from the sidebar's Approvals entry. */
export interface FocusRequest {
  convId: string
  target: FocusTarget
  nonce: number
}

export interface CanvasPanelProps {
  detail: ConversationDetail | undefined
  mobile: boolean
  focusRequest: FocusRequest | null
  onFocusHandled: (nonce: number) => void
  deciding: Deciding | null
  onDecide: (step: Step, decision: Decision) => void
  /** Folded to a thin strip so the chat gets the room. The cards still arrive; they show when it opens. */
  collapsed: boolean
  onCollapse: () => void
  onExpand: () => void
}

const NO_STEPS: Step[] = []
const REPLAY_MS = 1050
const DEFAULT_ZOOM = 0.86

export function CanvasPanel(props: CanvasPanelProps) {
  if (props.collapsed) return <CanvasRail steps={props.detail?.steps ?? NO_STEPS} onExpand={props.onExpand} />
  return (
    <ReactFlowProvider>
      <CanvasInner {...props} />
    </ReactFlowProvider>
  )
}

/** The folded canvas: one button down the right edge, with the step count and anything waiting for you. */
function CanvasRail({ steps, onExpand }: { steps: Step[]; onExpand: () => void }) {
  const cloud = steps.filter((s) => s.kind === 'cloud').length
  const waiting = steps.filter((s) => s.status === 'waiting').length
  const label = `Show the canvas: ${plural(cloud, 'step')}${waiting > 0 ? `, ${waiting} waiting for you` : ''}`
  return (
    <section className="canvas-panel canvas-rail" aria-label="Steps canvas">
      <button type="button" className="rail-btn" aria-label={label} title="Show the canvas" onClick={onExpand}>
        <Icon name="panelOpen" />
        <span className="rail-label">Steps</span>
        <span className="rail-count">{cloud}</span>
        {waiting > 0 && <span className="rail-wait">{waiting}</span>}
      </button>
    </section>
  )
}

function CanvasInner({ detail, mobile, focusRequest, onFocusHandled, deciding, onDecide, onCollapse }: CanvasPanelProps) {
  const rf = useReactFlow()
  const frameRef = useRef<HTMLDivElement>(null)
  const size = useElementSize(frameRef)

  const [filter, setFilter] = useState<StepFilter>('all')
  const [selectedId, setSelectedId] = useState<string | null>(null)
  const [keyboardOpen, setKeyboardOpen] = useState(false)
  const [viewIndex, setViewIndex] = useState<number | null>(null)
  const [playing, setPlaying] = useState(false)
  const [heights, setHeights] = useState<Record<string, number>>({})
  const [pendingShow, setPendingShow] = useState<string | null>(null)
  const [trace, setTrace] = useState<Trace | null>(null)

  const steps = detail?.steps ?? NO_STEPS
  const result = detail?.result
  const layout = useMemo(() => computeLayout({ steps, result, filter, heights }), [steps, result, filter, heights])
  const graph = useMemo(() => toFlow(layout, heights, result, trace), [layout, heights, result, trace])

  // Cards are measured by React Flow after they render. Until every card has a height, positions are estimates.
  const measured =
    layout.blocks.every((b) => heights[b.id] !== undefined) && (!layout.hasResult || heights[RESULT_ID] !== undefined)

  const onNodesChange = useCallback((changes: NodeChange<CanvasNode>[]) => {
    const seen: Record<string, number> = {}
    for (const c of changes) {
      if (c.type === 'dimensions' && c.dimensions && !c.id.startsWith(LANE_PREFIX)) seen[c.id] = c.dimensions.height
    }
    const ids = Object.keys(seen)
    if (ids.length === 0) return
    setHeights((prev) => {
      let next = prev
      for (const id of ids) {
        const h = seen[id] as number
        const old = prev[id]
        if (old === undefined || Math.abs(old - h) >= 0.5) {
          if (next === prev) next = { ...prev }
          next[id] = h
        }
      }
      return next
    })
  }, [])

  // ---- Camera --------------------------------------------------------------------------------------------------

  const moveTo = useCallback(
    (vp: { x: number; y: number; zoom: number }, smooth = true) => {
      void rf.setViewport(vp, { duration: smooth && !prefersReducedMotion() ? 480 : 0 })
    },
    [rf],
  )

  const focusOn = (
    id: string,
    opts: { select?: boolean; keepZoom?: boolean; xFrac?: number; inspectorOpen?: boolean } = {},
  ) => {
    const target = layout.place[id]
    if (!target) return
    const inspectorOpen = opts.inspectorOpen ?? (opts.select === true || selectedId !== null)
    const zoom = opts.keepZoom ? rf.getZoom() : clamp(Math.max(rf.getZoom(), 0.8), MIN_ZOOM, 1.4)
    moveTo(
      framingViewport({
        view: size,
        worldHeight: layout.height,
        target,
        zoom,
        xFrac: opts.xFrac ?? 0.5,
        insetX: inspectorOpen && !mobile ? INSPECTOR_INSET : 0,
        insetY: inspectorOpen && mobile ? Math.min(size.height * 0.64, 420) + 8 : 0,
      }),
    )
  }
  // Timers and effects call the newest version, never a stale copy.
  const focusRef = useRef(focusOn)
  focusRef.current = focusOn

  const fit = () => moveTo(fitViewport(size, layout))

  // The first view of a chat, and after a filter change: fit everything, or look at what needs you.
  const viewKey = useRef(0)
  const appliedKey = useRef(-1)
  const smoothNext = useRef(false)
  useEffect(() => {
    if (!detail || !measured || size.width === 0 || appliedKey.current === viewKey.current) return
    appliedKey.current = viewKey.current
    moveTo(initialViewport(layout, size, mobile), smoothNext.current)
    smoothNext.current = false
  }, [detail, measured, size, layout, mobile, moveTo])

  // While you watch live, a new card brings the camera along.
  const known = useRef<Set<string> | null>(null)
  useEffect(() => {
    if (!detail) return
    const ids = new Set(layout.steps.map((s) => s.id))
    if (layout.hasResult) ids.add(RESULT_ID)
    const before = known.current
    known.current = ids
    if (!before || playing || viewIndex !== null || appliedKey.current !== viewKey.current) return
    const added = [...ids].filter((id) => !before.has(id))
    const target = added.includes(RESULT_ID) ? RESULT_ID : added[added.length - 1]
    if (target) focusRef.current(target, { keepZoom: true, xFrac: mobile ? 0.5 : 0.6 })
  }, [detail, layout.steps, layout.hasResult])
  const isNew = useCallback((id: string) => known.current !== null && !known.current.has(id), [])

  // ---- Selection and the filter --------------------------------------------------------------------------------

  const select = useCallback((id: string, viaKeyboard = false) => {
    setSelectedId(id)
    setKeyboardOpen(viaKeyboard)
  }, [])

  const closeInspector = useCallback(() => {
    if (selectedId && keyboardOpen) {
      // Put keyboard focus back on the card the panel was opened from.
      const cards = frameRef.current?.querySelectorAll<HTMLElement>('[data-step-id]') ?? []
      Array.from(cards).find((el) => el.dataset.stepId === selectedId)?.focus()
    }
    setSelectedId(null)
  }, [selectedId, keyboardOpen])

  useEffect(() => {
    if (!selectedId) return
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') closeInspector()
    }
    document.addEventListener('keydown', onKey)
    return () => document.removeEventListener('keydown', onKey)
  }, [selectedId, closeInspector])

  const changeFilter = (next: StepFilter) => {
    if (next === filter) return
    setTrace(null)
    setPlaying(false)
    setViewIndex(null)
    known.current = null
    viewKey.current++
    smoothNext.current = true
    setFilter(next)
  }
  const changeFilterRef = useRef(changeFilter)
  changeFilterRef.current = changeFilter

  // "Show this step": wait until it is on the canvas (switching the filter if it is hidden), then select and frame it.
  useEffect(() => {
    if (!detail || !focusRequest || focusRequest.convId !== detail.id) return
    const target = focusRequest.target
    const id =
      target === 'waiting' ? steps.find((s) => s.kind === 'cloud' && s.status === 'waiting')?.id : target.stepId
    onFocusHandled(focusRequest.nonce)
    if (id) setPendingShow(id)
  }, [detail, focusRequest, steps, onFocusHandled])

  useEffect(() => {
    if (!pendingShow || !detail) return
    if (!layout.cloud.some((s) => s.id === pendingShow)) {
      setPendingShow(null) // a local step: it is not on the canvas
      return
    }
    if (!layout.place[pendingShow]) {
      changeFilterRef.current('all') // hidden by "Changes only"
      return
    }
    if (!measured || size.width === 0) return
    setPendingShow(null)
    setPlaying(false)
    setViewIndex(null)
    select(pendingShow)
    focusRef.current(pendingShow, { select: true })
  }, [pendingShow, detail, layout, measured, size, select])

  // ---- Replay --------------------------------------------------------------------------------------------------

  const count = layout.steps.length

  const goTo = (i: number) => {
    setPlaying(false)
    setViewIndex(i >= count - 1 ? null : i)
    const s = layout.steps[i]
    if (s) focusOn(s.id, { keepZoom: true })
  }

  const play = () => {
    if (playing) {
      setPlaying(false)
      return
    }
    const first = layout.steps[0]
    if (!first) return
    setSelectedId(null)
    setPlaying(true)
    setViewIndex(0)
    focusOn(first.id, { keepZoom: true, inspectorOpen: false })
  }

  const showLatest = () => {
    setPlaying(false)
    setViewIndex(null)
    const last = layout.steps[count - 1]
    const id = layout.hasResult ? RESULT_ID : last?.id
    if (id) focusOn(id, { keepZoom: true })
  }

  useEffect(() => {
    if (!playing) return
    const timer = setTimeout(() => {
      if (viewIndex === null || viewIndex >= count - 1) {
        setPlaying(false)
        setViewIndex(null)
        if (layout.hasResult) focusRef.current(RESULT_ID, { keepZoom: true })
        return
      }
      const next = viewIndex + 1
      setViewIndex(next)
      const s = layout.steps[next]
      if (s) focusRef.current(s.id, { keepZoom: true })
    }, REPLAY_MS)
    return () => clearTimeout(timer)
  }, [playing, viewIndex, count, layout])

  // ---- Chips ---------------------------------------------------------------------------------------------------

  const onTrace = useCallback((target: string, sources: string[] | null) => {
    setTrace(sources && sources.length > 0 ? { target, sources } : null)
  }, [])
  const onShow = useCallback((id: string) => setPendingShow(id), [])
  const traced = useMemo(() => new Set(trace?.sources ?? []), [trace])

  // ---- Render --------------------------------------------------------------------------------------------------

  const view = useMemo<CanvasView>(
    () => ({ numbers: layout.numbers, selectedId, viewIndex, deciding, traced, isNew, onSelect: select, onDecide, onTrace, onShow }),
    [layout.numbers, selectedId, viewIndex, deciding, traced, isNew, select, onDecide, onTrace, onShow],
  )
  const selectedStep = selectedId ? steps.find((s) => s.id === selectedId) : undefined
  const empty = layout.cloud.length === 0 && !result

  return (
    <section className="canvas-panel" aria-label="Steps canvas">
      <header className="canvas-head">
        <div className="canvas-title">
          <b>Steps</b>
          <span>{detail?.subtitle}</span>
        </div>
        <TallyPills steps={layout.cloud} />
        <div className="seg" role="group" aria-label="Filter steps">
          <button type="button" className={filter === 'all' ? 'on' : ''} aria-pressed={filter === 'all'} onClick={() => changeFilter('all')}>
            All steps
          </button>
          <button type="button" className={filter === 'changes' ? 'on' : ''} aria-pressed={filter === 'changes'} onClick={() => changeFilter('changes')}>
            Changes only
          </button>
        </div>
        <ZoomControls onFit={fit} />
        <button type="button" className="icon-btn collapse-btn" aria-label="Hide the canvas" title="Hide the canvas" onClick={onCollapse}>
          <Icon name="panelClose" />
        </button>
      </header>
      <div className="viewport" ref={frameRef} role="region" aria-label="Canvas of steps">
        <CanvasViewContext.Provider value={view}>
          <ReactFlow
            nodes={graph.nodes}
            edges={graph.edges}
            nodeTypes={NODE_TYPES}
            edgeTypes={EDGE_TYPES}
            onNodesChange={onNodesChange}
            defaultViewport={{ x: 0, y: 0, zoom: DEFAULT_ZOOM }}
            minZoom={MIN_ZOOM}
            maxZoom={MAX_ZOOM}
            nodesDraggable={false}
            nodesConnectable={false}
            elementsSelectable={false}
            nodesFocusable={false}
            edgesFocusable={false}
            disableKeyboardA11y
            panOnScroll
            zoomOnScroll={false}
            zoomOnDoubleClick={false}
            deleteKeyCode={null}
            selectionKeyCode={null}
            multiSelectionKeyCode={null}
            onPaneClick={closeInspector}
            attributionPosition="bottom-left"
          >
            <Background variant={BackgroundVariant.Dots} gap={22} size={2.2} color="var(--grid)" />
          </ReactFlow>
        </CanvasViewContext.Provider>
        <LaneRail lanes={layout.lanes} />
        <ColumnRuler columns={layout.columns} viewIndex={viewIndex} />
        {selectedStep && (
          <Inspector
            step={selectedStep}
            number={layout.numbers[selectedStep.id] ?? 0}
            numbers={layout.numbers}
            deciding={deciding}
            onDecide={onDecide}
            onClose={closeInspector}
            focusOnOpen={keyboardOpen}
          />
        )}
        {empty && (
          <div className="canvas-empty">
            <div className="empty">
              <b>No cloud steps yet</b>
              <p>Each step that touches a cloud service lands here as a card, left to right.</p>
            </div>
          </div>
        )}
        <div className="legend">
          <span className="lg">
            <i className="wire" />
            Made here, used later
          </span>
          <span className="lg">
            <span className="chip-from">from 2</span>
            Found earlier: hover to trace
          </span>
          <span className="lg-hint">Drag to move · ⌘ or Ctrl + scroll, or pinch, to zoom</span>
        </div>
      </div>
      <Scrubber
        steps={layout.steps}
        numbers={layout.numbers}
        total={layout.cloud.length}
        viewIndex={viewIndex}
        playing={playing}
        onGoTo={goTo}
        onPlay={play}
        onLatest={showLatest}
      />
      <EdgeMarkers />
    </section>
  )
}
