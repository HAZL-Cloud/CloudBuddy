import { Handle, Position, type EdgeProps, type NodeProps } from '@xyflow/react'
import { useRef } from 'react'
import { ANCHOR_Y } from '../layout'
import { useCanvasView } from './canvasContext'
import type { FlowEdge as FlowEdgeType, LaneFlowNode, ResultFlowNode, StepFlowNode } from './flowGraph'
import { ResultCard } from './ResultCard'
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
  const { step, number, index } = data
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

export function FlowEdge({ data }: EdgeProps<FlowEdgeType>) {
  const { viewIndex } = useCanvasView()
  if (!data) return null
  const cls = ['cb-edge', data.kind === 'next' ? '' : data.kind, data.extra ? 'extra' : '', isFuture(viewIndex, data.toIndex) ? 'fut' : '']
    .filter(Boolean)
    .join(' ')
  const marker = data.kind === 'gate' ? 'cb-ahg' : data.kind === 'flow' ? 'cb-ahf' : 'cb-ah'
  return <path className={cls} d={data.path} markerEnd={`url(#${marker})`} />
}

export const NODE_TYPES = { step: StepNode, result: ResultNode, lane: LaneNode }
export const EDGE_TYPES = { flow: FlowEdge }

/** The three arrowheads the edges point to. Rendered once, outside React Flow's own svg. */
export function EdgeMarkers() {
  return (
    <svg className="marker-defs" aria-hidden="true" focusable="false">
      <defs>
        {(['cb-ah', 'cb-ahg', 'cb-ahf'] as const).map((id) => (
          <marker key={id} id={id} viewBox="0 0 10 10" refX="9" refY="5" markerUnits="userSpaceOnUse" markerWidth="10" markerHeight="10" orient="auto">
            <path className={id.slice(3)} d="M0 1 L10 5 L0 9 Z" />
          </marker>
        ))}
      </defs>
    </svg>
  )
}

