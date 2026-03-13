import { useState } from 'react'
import type { ActivityGraph, ActivityInstance, ActivityStatus } from './types'

const STATUS_FILL: Record<ActivityStatus | 'default', string> = {
  pending:       '#e0e6ed',
  running:       '#3498db',
  completed:     '#2ecc71',
  failed:        '#e74c3c',
  skipped:       '#bdc3c7',
  waiting_human: '#f39c12',
  default:       '#e0e6ed',
}
const STATUS_TEXT: Record<ActivityStatus | 'default', string> = {
  pending:       '#555',
  running:       'white',
  completed:     'white',
  failed:        'white',
  skipped:       '#666',
  waiting_human: 'white',
  default:       '#555',
}

const NODE_W = 160
const NODE_H = 56
const COL_GAP = 100
const ROW_GAP = 80

interface NodePos { x: number; y: number }
interface TooltipState { nodeId: string; x: number; y: number }

function layoutGraph(graph: ActivityGraph): Map<string, NodePos> {
  // BFS to assign layers (columns)
  const layers = new Map<string, number>()
  const queue: string[] = [graph.start_node]
  layers.set(graph.start_node, 0)
  const visited = new Set<string>([graph.start_node])

  while (queue.length > 0) {
    const id = queue.shift()!
    const node = graph.nodes[id]
    if (!node) continue
    const layer = layers.get(id)!
    for (const t of node.transitions) {
      if (t.next_node && !visited.has(t.next_node)) {
        visited.add(t.next_node)
        layers.set(t.next_node, layer + 1)
        queue.push(t.next_node)
      }
    }
  }

  // Group by layer, assign row within layer
  const byLayer = new Map<number, string[]>()
  for (const [id, layer] of layers) {
    const arr = byLayer.get(layer) ?? []
    arr.push(id)
    byLayer.set(layer, arr)
  }

  const positions = new Map<string, NodePos>()
  for (const [layer, ids] of byLayer) {
    const x = 60 + layer * (NODE_W + COL_GAP)
    ids.forEach((id, row) => {
      const totalH = ids.length * NODE_H + (ids.length - 1) * ROW_GAP
      const startY = 40 + row * (NODE_H + ROW_GAP) - totalH / 2 + totalH / 2
      positions.set(id, { x, y: startY })
    })
  }
  return positions
}

interface Props {
  graph: ActivityGraph
  activities: ActivityInstance[]
  currentNode: string
}

export default function WorkflowGraph({ graph, activities, currentNode }: Props) {
  const [tooltip, setTooltip] = useState<TooltipState | null>(null)

  const positions = layoutGraph(graph)

  // Map nodeID → latest activity instance
  const activityMap = new Map<string, ActivityInstance>()
  for (const a of activities) {
    activityMap.set(a.node_id, a)
  }

  // Map nodeID → status
  const statusMap = new Map<string, ActivityStatus>()
  for (const a of activities) {
    statusMap.set(a.node_id, a.status)
  }

  // Compute SVG dimensions
  let maxX = 0, maxY = 0
  for (const p of positions.values()) {
    maxX = Math.max(maxX, p.x + NODE_W)
    maxY = Math.max(maxY, p.y + NODE_H)
  }
  const svgW = maxX + 60
  const svgH = maxY + 60

  // Build edges
  const edges: Array<{ x1: number; y1: number; x2: number; y2: number; label: string }> = []
  for (const [id, node] of Object.entries(graph.nodes)) {
    const src = positions.get(id)
    if (!src) continue
    for (const t of node.transitions) {
      if (!t.next_node) continue
      const dst = positions.get(t.next_node)
      if (!dst) continue
      const x1 = src.x + NODE_W
      const y1 = src.y + NODE_H / 2
      const x2 = dst.x
      const y2 = dst.y + NODE_H / 2
      const conds = t.conditions ?? []
      const label = t.label ?? (conds.length > 0
        ? conds.map(c => `${c.key} ${c.operator} ${c.value}`).join(' & ')
        : '')
      edges.push({ x1, y1, x2, y2, label })
    }
  }

  const tooltipAct = tooltip ? activityMap.get(tooltip.nodeId) : null

  return (
    <div style={{ position: 'relative', background: '#f8f9fa', borderRadius: '8px', padding: '8px', overflow: 'auto' }}>
      <svg width={svgW} height={svgH} style={{ display: 'block' }}>
        <defs>
          <marker id="arrow" markerWidth="8" markerHeight="8" refX="6" refY="3" orient="auto">
            <path d="M0,0 L0,6 L8,3 z" fill="#aaa" />
          </marker>
        </defs>

        {/* START dot */}
        {positions.has(graph.start_node) && (() => {
          const p = positions.get(graph.start_node)!
          return (
            <>
              <circle cx={p.x - 30} cy={p.y + NODE_H / 2} r={8} fill="#667eea" />
              <line x1={p.x - 22} y1={p.y + NODE_H / 2} x2={p.x} y2={p.y + NODE_H / 2}
                stroke="#aaa" strokeWidth={1.5} markerEnd="url(#arrow)" />
            </>
          )
        })()}

        {/* Edges */}
        {edges.map((e, i) => {
          const mx = (e.x1 + e.x2) / 2
          return (
            <g key={i}>
              <path d={`M${e.x1},${e.y1} C${mx},${e.y1} ${mx},${e.y2} ${e.x2},${e.y2}`}
                fill="none" stroke="#aaa" strokeWidth={1.5} markerEnd="url(#arrow)" />
              {e.label && (
                <text x={mx} y={(e.y1 + e.y2) / 2 - 4} textAnchor="middle" fontSize={10} fill="#888">{e.label}</text>
              )}
            </g>
          )
        })}

        {/* Nodes */}
        {Array.from(positions.entries()).map(([id, pos]) => {
          const node = graph.nodes[id]
          const status = statusMap.get(id) ?? 'default'
          const fill = STATUS_FILL[status as ActivityStatus] ?? STATUS_FILL.default
          const textColor = STATUS_TEXT[status as ActivityStatus] ?? STATUS_TEXT.default
          const isCurrent = id === currentNode
          const isHuman = node?.is_human
          const hasData = activityMap.has(id)

          return (
            <g key={id} style={{ cursor: hasData ? 'pointer' : 'default' }}
              onMouseEnter={(e) => setTooltip({ nodeId: id, x: e.clientX, y: e.clientY })}
              onMouseMove={(e) => setTooltip(prev => prev ? { ...prev, x: e.clientX, y: e.clientY } : null)}
              onMouseLeave={() => setTooltip(null)}
            >
              {isCurrent && (
                <rect x={pos.x - 3} y={pos.y - 3} width={NODE_W + 6} height={NODE_H + 6}
                  rx={9} fill="none" stroke="#667eea" strokeWidth={2} strokeDasharray="4 2" />
              )}
              <rect x={pos.x} y={pos.y} width={NODE_W} height={NODE_H} rx={6}
                fill={fill} stroke={isCurrent ? '#667eea' : '#ccc'} strokeWidth={isCurrent ? 2 : 1} />
              {isHuman && (
                <text x={pos.x + 10} y={pos.y + 20} fontSize={14} fill={textColor}>👤</text>
              )}
              <text x={pos.x + NODE_W / 2} y={pos.y + 20} textAnchor="middle" fontSize={12} fontWeight={600} fill={textColor}>{id}</text>
              <text x={pos.x + NODE_W / 2} y={pos.y + 36} textAnchor="middle" fontSize={10} fill={textColor} opacity={0.8}>{node?.activity_name ?? ''}</text>
            </g>
          )
        })}
      </svg>

      {/* Hover tooltip */}
      {tooltip && tooltipAct && (
        <div style={{
          position: 'fixed',
          top: tooltip.y + 14,
          left: tooltip.x + 14,
          zIndex: 9999,
          background: '#1e1e1e',
          color: '#d4d4d4',
          borderRadius: '6px',
          padding: '10px 12px',
          fontSize: '11px',
          fontFamily: 'monospace',
          maxWidth: '320px',
          boxShadow: '0 4px 16px rgba(0,0,0,0.4)',
          pointerEvents: 'none',
          lineHeight: 1.5,
        }}>
          <div style={{ color: '#9cdcfe', fontWeight: 700, marginBottom: '6px' }}>{tooltipAct.node_id} ({tooltipAct.status})</div>
          <div style={{ color: '#4ec9b0', fontWeight: 600, marginBottom: '2px' }}>Input</div>
          <pre style={{ margin: '0 0 8px', whiteSpace: 'pre-wrap', wordBreak: 'break-all' }}>
            {Object.keys(tooltipAct.input ?? {}).length > 0
              ? JSON.stringify(tooltipAct.input, null, 2)
              : '—'}
          </pre>
          <div style={{ color: '#4ec9b0', fontWeight: 600, marginBottom: '2px' }}>Output</div>
          <pre style={{ margin: 0, whiteSpace: 'pre-wrap', wordBreak: 'break-all' }}>
            {tooltipAct.output && Object.keys(tooltipAct.output).length > 0
              ? JSON.stringify(tooltipAct.output, null, 2)
              : '—'}
          </pre>
        </div>
      )}
    </div>
  )
}
