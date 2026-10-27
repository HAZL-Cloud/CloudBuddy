import { EdgeLabelRenderer, Handle, Position, type EdgeProps, type NodeProps } from '@xyflow/react'
import { useRef } from 'react'
import { ANCHOR_Y, HUES } from '../layout'
import { useCanvasView } from './canvasContext'
import type { FlowEdge as FlowEdgeType, LaneFlowNode, ResultFlowNode, StackFlowNode, StepFlowNode } from './flowGraph'
import { ResultCard } from './ResultCard'
import { StackCard } from './StackCard'
import { StepCard } from './StepCard'

// React Flow needs a handle on each node to draw an edge. Ours are invisible: the edge paths come from the layout.
function Handles() {
  const style = { top: ANCHOR_Y }
  return (
    <>
      <Handle type="target" position={Position.Left} className="cb-handle" style={style} isConnectable={false} />
      <Handle type="source" position={Position.Right} className="cb-handle" style={style} isConnectable={false} />
    </>
  )
}

const isFuture = (viewIndex: number | null, index: number) => viewIndex !== null && index > viewIndex

export function StepNode({ data }: NodeProps<StepFlowNode>) {
  const view = useCanvasView()
  const { step, number, index, chips, carried } = data
  // Decided once, when the card first appears, so the entrance animation is not cut short by later renders.
  const enter = useRef(view.isNew(step.id)).current
  return (
    <>
      <Handles />
      <StepCard
        step={step}
        number={number}
        numbers={view.numbers}
        selected={view.selectedId === step.id}
        current={view.viewIndex === index}
        future={isFuture(view.viewIndex, index)}
        enter={enter}
        deciding={view.deciding}
        onSelect={view.onSelect}
        onDecide={view.onDecide}
        chips={chips}
        carried={carried}
        traced={view.traced.has(step.id)}
        onTrace={view.onTrace}
        onShow={view.onShow}
      />
    </>
  )
}

export function StackNode({ data }: NodeProps<StackFlowNode>) {
  const view = useCanvasView()
  const { steps, indices } = data
  const enter = useRef(view.isNew(steps[0]?.id ?? '')).current
  return (
    <>
      <Handles />
      <StackCard
        steps={steps}
        indices={indices}
        numbers={view.numbers}
        selectedId={view.selectedId}
        viewIndex={view.viewIndex}
        traced={steps.some((s) => view.traced.has(s.id))}
        enter={enter}
        onSelect={view.onSelect}
      />
    </>
  )
}

export function ResultNode({ data }: NodeProps<ResultFlowNode>) {
  const view = useCanvasView()
  const enter = useRef(view.isNew('result')).current
  return (
    <>
      <Handles />
      <ResultCard
        result={data.result}
        steps={data.steps}
        current={view.viewIndex === data.index}
        future={isFuture(view.viewIndex, data.index)}
        enter={enter}
      />
    </>
  )
}

export function LaneNode({ data }: NodeProps<LaneFlowNode>) {
  return <div className={data.alternate ? 'lane a' : 'lane'} />
}

/** A line from something a step created to a step that builds on it, or the dashed trace of a hovered chip. */
export function WireEdge({ data }: EdgeProps<FlowEdgeType>) {
  const { viewIndex } = useCanvasView()
  if (!data) return null
  if (data.kind === 'trace') return <path className="cb-wire trace" d={data.path} markerEnd="url(#cb-aht)" />
  const hue = `h${data.hue}`
  const fut = (i: number) => (isFuture(viewIndex, i) ? ' fut' : '')
  return (
    <>
      <path className={`cb-wire ${hue}${fut(data.toIndex)}`} d={data.path} markerEnd={`url(#cb-ah${data.hue})`} />
      {data.start && <circle className={`cb-dot ${hue}${fut(data.fromIndex)}`} cx={data.start.x} cy={data.start.y} r={4} />}
      {data.label && (
        <EdgeLabelRenderer>
          <div
            className={`wire-label ${hue}${fut(data.fromIndex)}`}
            style={{ transform: `translate(${data.label.x}px, ${data.label.y}px) translateY(-50%)` }}
          >
            {data.label.text}
          </div>
        </EdgeLabelRenderer>
      )}
    </>
  )
}

export const NODE_TYPES = { step: StepNode, stack: StackNode, result: ResultNode, lane: LaneNode }
export const EDGE_TYPES = { wire: WireEdge }

const MARKERS = [...Array.from({ length: HUES }, (_, i) => ({ id: `cb-ah${i}`, cls: `h${i}` })), { id: 'cb-aht', cls: 'trace' }]

/** The arrowheads the lines point to, one per colour. Rendered once, outside React Flow's own svg. */
export function EdgeMarkers() {
  return (
    <svg className="marker-defs" aria-hidden="true" focusable="false">
      <defs>
        {MARKERS.map((m) => (
          <marker key={m.id} id={m.id} viewBox="0 0 10 10" refX="9" refY="5" markerUnits="userSpaceOnUse" markerWidth="10" markerHeight="10" orient="auto">
            <path className={`ah ${m.cls}`} d="M0 1 L10 5 L0 9 Z" />
          </marker>
        ))}
      </defs>
    </svg>
  )
}
