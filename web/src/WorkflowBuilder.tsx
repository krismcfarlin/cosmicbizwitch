import { useState, useEffect, useRef, useCallback } from 'react'
import { useNavigate } from 'react-router-dom'

// ── Types ────────────────────────────────────────────────────────────────────

interface FieldMeta { name: string; type: string; description: string }
interface ActivityMeta { description: string; input_fields: FieldMeta[]; output_fields: FieldMeta[] }
interface ActivityInfo { name: string; meta: ActivityMeta }

interface BNode {
  id: string
  activityName: string  // 'END' for finish node
  x: number
  y: number
  label: string
  maxRetries: number
  isHuman: boolean
  needsConversion: boolean  // flagged to be rewritten as a native Go activity
  staticInput: Record<string, string>  // pre-set input fields baked into the node
}

interface CondRow { key: string; operator: string; value: string }
interface BEdge {
  id: string
  fromNode: string
  toNode: string   // node id, or 'END' when pointing to finish
  conditions: CondRow[]
  label: string
}

// ── Constants ────────────────────────────────────────────────────────────────

const NODE_W = 160
const NODE_H = 52
const PORT_R = 6
const OPERATORS = ['eq', 'neq', 'gt', 'lt', 'gte', 'lte', 'contains', 'exists', 'not_exists']

let _nc = 0; const newNid = () => `node_${++_nc}`
let _ec = 0; const newEid = () => `edge_${++_ec}`

function parseVal(s: string): unknown {
  if (s === 'true') return true
  if (s === 'false') return false
  const n = Number(s)
  if (!isNaN(n) && s.trim() !== '') return n
  return s
}

// ── ActivityCard (sidebar item) ───────────────────────────────────────────────

function ActivityCard({ info, onDragStart }: { info: ActivityInfo; onDragStart: (name: string) => void }) {
  const [hover, setHover] = useState(false)

  return (
    <div
      draggable
      onDragStart={() => onDragStart(info.name)}
      onMouseEnter={() => setHover(true)}
      onMouseLeave={() => setHover(false)}
      style={{
        background: 'white', border: '1px solid #e0e6ed', borderRadius: '6px',
        padding: '8px 10px', cursor: 'grab', marginBottom: '6px',
        fontSize: '12px', fontFamily: 'monospace', color: '#2c3e50',
        userSelect: 'none', position: 'relative',
        boxShadow: hover ? '0 2px 8px rgba(0,0,0,0.12)' : 'none',
        transition: 'box-shadow 0.15s',
      }}
    >
      <div style={{ fontWeight: 600 }}>{info.name}</div>
      {info.meta.description && (
        <div style={{ fontSize: '11px', color: '#888', marginTop: '2px', fontFamily: 'sans-serif' }}>
          {info.meta.description}
        </div>
      )}

      {hover && (info.meta.input_fields?.length > 0 || info.meta.output_fields?.length > 0) && (
        <div style={{
          position: 'absolute', left: 'calc(100% + 8px)', top: 0, zIndex: 9999,
          background: '#1e1e1e', color: '#d4d4d4', borderRadius: '6px',
          padding: '10px 12px', fontSize: '11px', fontFamily: 'monospace',
          width: '240px', boxShadow: '0 4px 16px rgba(0,0,0,0.4)', lineHeight: 1.6,
          pointerEvents: 'none',
        }}>
          {info.meta.input_fields?.length > 0 && (
            <>
              <div style={{ color: '#4ec9b0', fontWeight: 700, marginBottom: '4px' }}>Inputs</div>
              {info.meta.input_fields.map(f => (
                <div key={f.name}><span style={{ color: '#9cdcfe' }}>{f.name}</span>
                  <span style={{ color: '#888' }}> {f.type}</span>
                  {f.description && <span style={{ color: '#666' }}> — {f.description}</span>}
                </div>
              ))}
            </>
          )}
          {info.meta.output_fields?.length > 0 && (
            <>
              <div style={{ color: '#ce9178', fontWeight: 700, marginTop: '8px', marginBottom: '4px' }}>Outputs</div>
              {info.meta.output_fields.map(f => (
                <div key={f.name}><span style={{ color: '#9cdcfe' }}>{f.name}</span>
                  <span style={{ color: '#888' }}> {f.type}</span>
                  {f.description && <span style={{ color: '#666' }}> — {f.description}</span>}
                </div>
              ))}
            </>
          )}
        </div>
      )}
    </div>
  )
}

// ── Edge conditions editor ────────────────────────────────────────────────────

function EdgeEditor({ edge, onChange, onDelete }: {
  edge: BEdge
  onChange: (e: BEdge) => void
  onDelete: () => void
}) {
  const set = (patch: Partial<BEdge>) => onChange({ ...edge, ...patch })
  const setCond = (i: number, patch: Partial<CondRow>) => {
    const conds = edge.conditions.map((c, j) => j === i ? { ...c, ...patch } : c)
    set({ conditions: conds })
  }
  const addCond = () => set({ conditions: [...edge.conditions, { key: '', operator: 'eq', value: '' }] })
  const delCond = (i: number) => set({ conditions: edge.conditions.filter((_, j) => j !== i) })

  return (
    <div>
      <div style={{ fontWeight: 600, marginBottom: '8px', color: '#2c3e50' }}>Edge Properties</div>
      <label style={labelStyle}>Label (optional)</label>
      <input value={edge.label} onChange={e => set({ label: e.target.value })}
        style={inputStyle} placeholder="e.g. high path" />

      <div style={{ marginTop: '12px', marginBottom: '6px', fontWeight: 600, fontSize: '12px', color: '#555' }}>
        Conditions <span style={{ fontWeight: 400, color: '#aaa' }}>(all must match; empty = always)</span>
      </div>

      {edge.conditions.map((c, i) => (
        <div key={i} style={{ display: 'flex', gap: '4px', marginBottom: '4px', alignItems: 'center' }}>
          <input value={c.key} onChange={e => setCond(i, { key: e.target.value })}
            placeholder="key" style={{ ...inputStyle, width: '80px', margin: 0 }} />
          <select value={c.operator} onChange={e => setCond(i, { operator: e.target.value })}
            style={{ ...inputStyle, margin: 0, flex: 1 }}>
            {OPERATORS.map(op => <option key={op} value={op}>{op}</option>)}
          </select>
          {!['exists', 'not_exists'].includes(c.operator) && (
            <input value={c.value} onChange={e => setCond(i, { value: e.target.value })}
              placeholder="value" style={{ ...inputStyle, width: '70px', margin: 0 }} />
          )}
          <button onClick={() => delCond(i)} style={iconBtn('#e74c3c')}>✕</button>
        </div>
      ))}
      <button onClick={addCond} style={{ ...iconBtn('#667eea'), fontSize: '11px', padding: '3px 8px', marginTop: '4px' }}>+ Add Condition</button>

      <button onClick={onDelete} style={{ display: 'block', marginTop: '16px', width: '100%', ...iconBtn('#e74c3c'), padding: '6px' }}>
        Delete Edge
      </button>
    </div>
  )
}

// ── Python code editor panel with inline Pyodide test ─────────────────────────

function PythonPanel({ code, onCodeChange, needsConversion, onNeedsConversionChange }: {
  code: string
  onCodeChange: (code: string) => void
  needsConversion: boolean
  onNeedsConversionChange: (v: boolean) => void
}) {
  const [testInput, setTestInput] = useState('{"value": 21}')
  const [phase, setPhase] = useState<'idle' | 'loading' | 'running' | 'done' | 'error'>('idle')
  const [stdout, setStdout] = useState('')
  const [resultStr, setResultStr] = useState('')
  const [errorMsg, setErrorMsg] = useState('')

  const runInBrowser = async () => {
    setPhase('loading'); setStdout(''); setResultStr(''); setErrorMsg('')
    try {
      let inputData: Record<string, unknown> = {}
      try { inputData = JSON.parse(testInput) } catch { throw new Error('Test input is not valid JSON') }

      if (!(window as unknown as Record<string, unknown>)['loadPyodide']) {
        await new Promise<void>((resolve, reject) => {
          if (document.querySelector('script[data-pyodide]')) { resolve(); return }
          const s = document.createElement('script')
          s.src = 'https://cdn.jsdelivr.net/pyodide/v0.27.3/full/pyodide.js'
          s.dataset.pyodide = '1'
          s.onload = () => resolve()
          s.onerror = () => reject(new Error('Failed to load Pyodide'))
          document.head.appendChild(s)
        })
      }

      setPhase('running')
      const loadPyodide = (window as unknown as Record<string, unknown>)['loadPyodide'] as (o: Record<string, unknown>) => Promise<unknown>
      const lines: string[] = []
      const pyodide = await loadPyodide({
        stdout: (line: string) => { lines.push(line); setStdout(lines.join('\n')) },
      }) as {
        globals: { set: (k: string, v: unknown) => void; get: (k: string) => unknown }
        toPy: (v: unknown) => unknown
        runPythonAsync: (code: string) => Promise<void>
      }

      pyodide.globals.set('input', pyodide.toPy(inputData))
      await pyodide.runPythonAsync(code)

      const raw = pyodide.globals.get('result')
      let result: Record<string, unknown> = {}
      if (raw instanceof Map) result = Object.fromEntries(raw)
      else if (raw && typeof (raw as { toJs?: () => unknown }).toJs === 'function') {
        const js = (raw as { toJs: () => unknown }).toJs()
        result = js instanceof Map ? Object.fromEntries(js) : (js as Record<string, unknown> ?? {})
      } else if (raw && typeof raw === 'object') result = raw as Record<string, unknown>

      setResultStr(JSON.stringify(result, null, 2))
      setPhase('done')
    } catch (e) {
      setErrorMsg(String(e))
      setPhase('error')
    }
  }

  return (
    <>
      <div style={{ marginTop: '14px', marginBottom: '6px', fontSize: '11px', fontWeight: 700, color: '#3572A5', borderTop: '1px solid #e0e6ed', paddingTop: '10px' }}>
        🐍 Python Code
      </div>
      <div style={{ fontSize: '10px', color: '#aaa', marginBottom: '8px', lineHeight: 1.5 }}>
        <code>input</code> = workflow context dict. Set <code>result</code> dict to pass values forward. <code>print()</code> is captured.
      </div>
      <textarea
        value={code}
        onChange={e => onCodeChange(e.target.value)}
        rows={10}
        spellCheck={false}
        style={{ width: '100%', boxSizing: 'border-box', fontFamily: 'monospace', fontSize: '11px', padding: '8px 10px', background: '#1e1e2e', color: '#cdd6f4', border: '1px solid #3572A5', borderRadius: '4px', resize: 'vertical', lineHeight: 1.5 }}
      />

      <div style={{ marginTop: '10px', fontSize: '11px', fontWeight: 600, color: '#555', marginBottom: '4px' }}>Test Input (JSON)</div>
      <textarea
        value={testInput}
        onChange={e => setTestInput(e.target.value)}
        rows={3}
        spellCheck={false}
        style={{ width: '100%', boxSizing: 'border-box', fontFamily: 'monospace', fontSize: '11px', padding: '6px 8px', border: '1px solid #ccd', borderRadius: '4px', resize: 'vertical', lineHeight: 1.5 }}
      />
      <button
        onClick={runInBrowser}
        disabled={phase === 'loading' || phase === 'running'}
        style={{ marginTop: '6px', width: '100%', padding: '6px', background: '#3572A5', color: 'white', border: 'none', borderRadius: '4px', cursor: 'pointer', fontWeight: 600, fontSize: '12px', opacity: (phase === 'loading' || phase === 'running') ? 0.6 : 1 }}
      >
        {phase === 'loading' ? '⏳ Loading Pyodide...' : phase === 'running' ? '▶ Running...' : '▶ Run in Browser'}
      </button>

      {(phase === 'done' || phase === 'error' || stdout) && (
        <div style={{ marginTop: '8px', background: '#1e1e2e', borderRadius: '4px', padding: '8px', fontSize: '11px', fontFamily: 'monospace' }}>
          {stdout ? <div style={{ color: '#a6e3a1', whiteSpace: 'pre-wrap', marginBottom: resultStr ? '6px' : 0 }}>{stdout}</div> : null}
          {phase === 'done' && resultStr && <div style={{ color: '#89b4fa', whiteSpace: 'pre-wrap' }}>result = {resultStr}</div>}
          {phase === 'error' && <div style={{ color: '#f38ba8', whiteSpace: 'pre-wrap' }}>{errorMsg}</div>}
        </div>
      )}

      <label style={{ ...labelStyle, display: 'flex', alignItems: 'center', gap: '6px', cursor: 'pointer', marginTop: '8px', padding: '6px 8px', background: needsConversion ? '#fffbea' : '#f8f9fa', borderRadius: '4px', border: `1px solid ${needsConversion ? '#f39c12' : '#e0e6ed'}` }}>
        <input type="checkbox" checked={needsConversion} onChange={e => onNeedsConversionChange(e.target.checked)} />
        <span>🔄 Mark for Go conversion</span>
      </label>
      {needsConversion && (
        <div style={{ fontSize: '10px', color: '#e67e22', marginTop: '4px', lineHeight: 1.5 }}>
          Flagged — rewrite this as a native Go activity for production use.
        </div>
      )}
    </>
  )
}

// ── Node properties editor ────────────────────────────────────────────────────

function NodeEditor({ node, isStart, activities, onSetStart, onChange, onDelete, onTestFromHere }: {
  node: BNode; isStart: boolean
  activities: ActivityInfo[]
  onSetStart: () => void
  onChange: (n: BNode) => void
  onDelete: () => void
  onTestFromHere: (nodeId: string) => void
}) {
  const set = (patch: Partial<BNode>) => onChange({ ...node, ...patch })
  const setInput = (key: string, val: string) =>
    set({ staticInput: { ...(node.staticInput ?? {}), [key]: val } })

  if (node.activityName === 'END') {
    return (
      <div>
        <div style={{ fontWeight: 600, marginBottom: '8px', color: '#2c3e50' }}>Finish Node</div>
        <div style={{ fontSize: '12px', color: '#888', marginBottom: '12px' }}>
          Any edge connecting to this node will end the workflow.
        </div>
        <button onClick={onDelete} style={{ display: 'block', width: '100%', ...iconBtn('#e74c3c'), padding: '6px' }}>
          Delete Node
        </button>
      </div>
    )
  }

  const actMeta = activities.find(a => a.name === node.activityName)
  // Input fields that aren't wildcard (*) and aren't purely pass-through
  const inputFields = (actMeta?.meta.input_fields ?? []).filter(f => f.name !== '*')
  const isPython = node.activityName === 'python_eval'

  return (
    <div>
      <div style={{ fontWeight: 600, marginBottom: '8px', color: '#2c3e50' }}>Node: {node.id}</div>
      <div style={{ fontSize: '11px', fontFamily: 'monospace', color: isPython ? '#3572A5' : node.isHuman ? '#f39c12' : '#667eea', marginBottom: '10px' }}>
        {isPython ? '🐍 ' : node.isHuman ? '👤 ' : ''}{node.activityName}
      </div>

      <label style={labelStyle}>Label (optional)</label>
      <input value={node.label} onChange={e => set({ label: e.target.value })}
        style={inputStyle} placeholder="Display label" />

      <label style={labelStyle}>Max Retries</label>
      <input type="number" min={0} value={node.maxRetries} onChange={e => set({ maxRetries: Number(e.target.value) })}
        style={inputStyle} />

      <label style={{ ...labelStyle, display: 'flex', alignItems: 'center', gap: '6px', cursor: 'pointer' }}>
        <input type="checkbox" checked={node.isHuman} onChange={e => set({ isHuman: e.target.checked })} />
        Human-in-the-loop (pause for approval)
      </label>

      {isPython && (
        <PythonPanel
          code={(node.staticInput ?? {}).code ?? ''}
          onCodeChange={code => setInput('code', code)}
          needsConversion={node.needsConversion ?? false}
          onNeedsConversionChange={v => set({ needsConversion: v })}
        />
      )}

      {!isPython && inputFields.length > 0 && (
        <>
          <div style={{ marginTop: '14px', marginBottom: '6px', fontSize: '11px', fontWeight: 700, color: '#444', borderTop: '1px solid #e0e6ed', paddingTop: '10px' }}>
            Input Fields
          </div>
          <div style={{ fontSize: '10px', color: '#aaa', marginBottom: '8px' }}>
            Pre-set values baked into this node. Use <code>{'{{key}}'}</code> to reference the workflow context.
          </div>
          {inputFields.map(f => (
            <div key={f.name}>
              <label style={labelStyle}>
                {f.name}
                {f.type && <span style={{ color: '#aaa', fontWeight: 400 }}> ({f.type})</span>}
              </label>
              {f.description && (
                <div style={{ fontSize: '10px', color: '#aaa', marginBottom: '2px' }}>{f.description}</div>
              )}
              <input
                value={(node.staticInput ?? {})[f.name] ?? ''}
                onChange={e => setInput(f.name, e.target.value)}
                style={inputStyle}
                placeholder={f.type === 'object' ? 'JSON or {{key}}' : `{{${f.name}}} or literal`}
              />
            </div>
          ))}
        </>
      )}

      {!isStart && (
        <button onClick={onSetStart}
          style={{ marginTop: '10px', display: 'block', width: '100%', ...iconBtn('#27ae60'), padding: '6px' }}>
          Set as Start Node
        </button>
      )}
      {isStart && (
        <div style={{ marginTop: '8px', fontSize: '12px', color: '#27ae60', fontWeight: 600 }}>✓ Start Node</div>
      )}

      <button onClick={() => onTestFromHere(node.id)}
        style={{ display: 'block', marginTop: '10px', width: '100%', ...iconBtn('#e67e22'), padding: '6px' }}>
        ▶ Test from this node
      </button>

      <button onClick={onDelete} style={{ display: 'block', marginTop: '8px', width: '100%', ...iconBtn('#e74c3c'), padding: '6px' }}>
        Delete Node
      </button>
    </div>
  )
}

// ── Test-from-node modal ──────────────────────────────────────────────────────

function TestModal({ nodeId, graphName, onClose, onRun }: {
  nodeId: string
  graphName: string
  onClose: () => void
  onRun: (nodeId: string, context: string) => Promise<void>
}) {
  const [context, setContext] = useState('{}')
  const [running, setRunning] = useState(false)
  const [error, setError] = useState('')

  const run = async () => {
    setRunning(true); setError('')
    try {
      await onRun(nodeId, context)
    } catch (e) {
      setError(String(e))
      setRunning(false)
    }
  }

  return (
    <div style={{ position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.5)', display: 'flex', alignItems: 'center', justifyContent: 'center', zIndex: 1000 }}>
      <div style={{ background: 'white', borderRadius: '8px', padding: '24px', width: '440px', boxShadow: '0 20px 60px rgba(0,0,0,0.3)' }}>
        <h3 style={{ margin: '0 0 4px', color: '#2c3e50' }}>Test from node</h3>
        <div style={{ fontSize: '12px', color: '#888', marginBottom: '16px' }}>
          Graph: <code style={{ color: '#667eea' }}>{graphName}</code> · Starting at: <code style={{ color: '#e67e22' }}>{nodeId}</code>
        </div>
        <div style={{ fontSize: '12px', color: '#555', marginBottom: '4px', fontWeight: 600 }}>
          Initial Context (JSON)
        </div>
        <div style={{ fontSize: '11px', color: '#aaa', marginBottom: '6px' }}>
          These values will be merged into the workflow context before running. Use to provide the inputs your node expects.
        </div>
        <textarea value={context} onChange={e => setContext(e.target.value)} rows={8}
          style={{ width: '100%', fontFamily: 'monospace', fontSize: '12px', padding: '10px', border: '1px solid #e0e6ed', borderRadius: '4px', boxSizing: 'border-box', resize: 'vertical' }} />
        {error && <div style={{ color: '#e74c3c', fontSize: '12px', marginTop: '6px' }}>{error}</div>}
        <div style={{ display: 'flex', gap: '10px', justifyContent: 'flex-end', marginTop: '14px' }}>
          <button onClick={onClose} style={{ padding: '8px 16px', background: '#f0f0f0', border: 'none', borderRadius: '4px', cursor: 'pointer', fontWeight: 500 }}>Cancel</button>
          <button onClick={run} disabled={running}
            style={{ padding: '8px 16px', background: running ? '#aaa' : '#e67e22', color: 'white', border: 'none', borderRadius: '4px', cursor: running ? 'default' : 'pointer', fontWeight: 500 }}>
            {running ? 'Starting…' : '▶ Run'}
          </button>
        </div>
      </div>
    </div>
  )
}

// ── Main WorkflowBuilder ──────────────────────────────────────────────────────

export default function WorkflowBuilder() {
  const navigate = useNavigate()
  const svgRef = useRef<SVGSVGElement>(null)

  const [activities, setActivities] = useState<ActivityInfo[]>([])
  const [nodes, setNodes] = useState<BNode[]>([])
  const [edges, setEdges] = useState<BEdge[]>([])
  const [startNode, setStartNode] = useState<string | null>(null)
  const [selectedNode, setSelectedNode] = useState<string | null>(null)
  const [selectedEdge, setSelectedEdge] = useState<string | null>(null)
  const [connecting, setConnecting] = useState<string | null>(null)  // fromNodeId
  const [draggingNode, setDraggingNode] = useState<string | null>(null)
  const [dragOffset, setDragOffset] = useState({ x: 0, y: 0 })
  const [graphName, setGraphName] = useState('my_workflow')
  const [saveStatus, setSaveStatus] = useState<string>('')
  const [testModal, setTestModal] = useState<string | null>(null)  // nodeId being tested
  const [graphNotFound, setGraphNotFound] = useState<string>('')

  // Drag from sidebar
  const dragActivity = useRef<string | null>(null)

  useEffect(() => {
    fetch('/api/workflows/activities')
      .then(r => r.json())
      .then(d => setActivities(d.activities ?? []))
  }, [])

  // Load existing graph if ?graph=name is in the URL
  useEffect(() => {
    const params = new URLSearchParams(window.location.search)
    const editGraph = params.get('graph')
    if (!editGraph) return
    fetch('/api/workflows/graphs')
      .then(r => r.json())
      .then(data => {
        const g = data.graphs?.[editGraph]
        if (!g) {
          setGraphName(editGraph)
          setGraphNotFound(editGraph)
          return
        }
        setGraphNotFound('')
        setGraphName(g.name)
        // Lay out nodes in a grid since we don't store positions
        const nodeEntries = Object.values(g.nodes) as Array<{
          id: string; activity_name: string; max_retries: number; is_human: boolean
          needs_conversion?: boolean; input?: Record<string, string>; transitions: Array<{ next_node: string; conditions: unknown[]; label?: string }>
        }>
        let col = 0, row = 0
        const loadedNodes: BNode[] = nodeEntries.map(n => {
          const x = 80 + col * 220; const y = 100 + row * 120
          col++; if (col > 3) { col = 0; row++ }
          return {
            id: n.id, activityName: n.activity_name,
            x, y, label: '', maxRetries: n.max_retries ?? 0,
            isHuman: n.is_human ?? false, needsConversion: n.needs_conversion ?? false,
            staticInput: (n.input ?? {}) as Record<string, string>,
          }
        })
        const loadedEdges: BEdge[] = []
        for (const n of nodeEntries) {
          for (const t of n.transitions ?? []) {
            if (!t.next_node) continue
            loadedEdges.push({
              id: newEid(), fromNode: n.id, toNode: t.next_node,
              conditions: [], label: t.label ?? '',
            })
          }
        }
        setNodes(loadedNodes)
        setEdges(loadedEdges)
        setStartNode(g.start_node)
      })
  }, [])

  // ── SVG coordinate helper ─────────────────────────────────────────────────

  const svgCoords = useCallback((clientX: number, clientY: number) => {
    if (!svgRef.current) return { x: clientX, y: clientY }
    const rect = svgRef.current.getBoundingClientRect()
    return { x: clientX - rect.left, y: clientY - rect.top }
  }, [])

  // ── Canvas drop (from sidebar) ────────────────────────────────────────────

  const onCanvasDrop = (e: React.DragEvent) => {
    e.preventDefault()
    if (!dragActivity.current) return
    const { x, y } = svgCoords(e.clientX, e.clientY)
    const name = dragActivity.current
    dragActivity.current = null

    const id = newNid()
    const isHumanDrop = name === '__human__'
    const isPythonDrop = name === '__python__'
    const activityName = isHumanDrop ? 'human_input' : isPythonDrop ? 'python_eval' : name
    const defaultStaticInput: Record<string, string> =
      isPythonDrop ? { code: 'print("input was:", input)\nresult = {"doubled": input.get("value", 0) * 2, "msg": "hello from pyodide"}' }
      : activityName === 'loop' ? { list_key: 'records', item_key: 'item' }
      : {}
    const node: BNode = {
      id, activityName, x: x - NODE_W / 2, y: y - NODE_H / 2,
      label: '', maxRetries: 0, isHuman: isHumanDrop || isPythonDrop, needsConversion: false, staticInput: defaultStaticInput,
    }
    setNodes(prev => {
      const next = [...prev, node]
      // First non-END node becomes start
      if (activityName !== 'END' && !startNode) setStartNode(id)
      return next
    })
    setSelectedNode(id)
    setSelectedEdge(null)
  }

  // ── Node dragging (move on canvas) ────────────────────────────────────────

  const onNodeMouseDown = (e: React.MouseEvent, nodeId: string) => {
    if (connecting) return // don't drag while connecting
    e.stopPropagation()
    const node = nodes.find(n => n.id === nodeId)!
    const { x, y } = svgCoords(e.clientX, e.clientY)
    setDraggingNode(nodeId)
    setDragOffset({ x: x - node.x, y: y - node.y })
    setSelectedNode(nodeId)
    setSelectedEdge(null)
  }

  const onSvgMouseMove = (e: React.MouseEvent) => {
    if (!draggingNode) return
    const { x, y } = svgCoords(e.clientX, e.clientY)
    setNodes(prev => prev.map(n =>
      n.id === draggingNode ? { ...n, x: x - dragOffset.x, y: y - dragOffset.y } : n
    ))
  }

  const onSvgMouseUp = () => {
    setDraggingNode(null)
  }

  // ── Port click (connect nodes) ────────────────────────────────────────────

  const onOutputPortClick = (e: React.MouseEvent, nodeId: string) => {
    e.stopPropagation()
    if (nodeId === connecting) { setConnecting(null); return }
    if (connecting) {
      // Complete edge: connecting → nodeId
      createEdge(connecting, nodeId)
      setConnecting(null)
    } else {
      setConnecting(nodeId)
      setSelectedNode(null)
      setSelectedEdge(null)
    }
  }

  const onInputPortClick = (e: React.MouseEvent, nodeId: string) => {
    e.stopPropagation()
    if (!connecting) return
    if (connecting === nodeId) { setConnecting(null); return }
    createEdge(connecting, nodeId)
    setConnecting(null)
  }

  const createEdge = (fromNode: string, toNode: string) => {
    const id = newEid()
    const edge: BEdge = { id, fromNode, toNode, conditions: [], label: '' }
    setEdges(prev => [...prev, edge])
    setSelectedEdge(id)
    setSelectedNode(null)
  }

  // ── Canvas click (deselect) ───────────────────────────────────────────────

  const onSvgClick = () => {
    if (connecting) { setConnecting(null); return }
    setSelectedNode(null)
    setSelectedEdge(null)
  }

  // ── Save graph ────────────────────────────────────────────────────────────

  const saveGraph = async () => {
    if (!graphName.trim()) { setSaveStatus('Graph name is required.'); return }
    if (nodes.filter(n => n.activityName !== 'END').length === 0) {
      setSaveStatus('Add at least one activity node.'); return
    }
    if (!startNode) { setSaveStatus('Set a start node.'); return }

    const graphNodes: Record<string, object> = {}
    for (const n of nodes) {
      if (n.activityName === 'END') continue
      const outEdges = edges.filter(e => e.fromNode === n.id)
      const transitions = outEdges.map(e => ({
        conditions: e.conditions
          .filter(c => c.key || ['exists', 'not_exists'].includes(c.operator))
          .map(c => ({ key: c.key, operator: c.operator, value: parseVal(c.value) })),
        next_node: e.toNode === 'END' || nodes.find(x => x.id === e.toNode)?.activityName === 'END'
          ? ''
          : e.toNode,
        label: e.label || undefined,
      }))
      const si = n.staticInput ?? {}
      const inputPayload = n.activityName === 'python_eval'
        ? { code: si.code ?? '' }
        : Object.keys(si).length > 0 ? si : undefined
      graphNodes[n.id] = {
        id: n.id,
        activity_name: n.activityName,
        max_retries: n.maxRetries,
        transitions,
        is_human: n.isHuman,
        ...(n.needsConversion ? { needs_conversion: true } : {}),
        ...(inputPayload !== undefined ? { input: inputPayload } : {}),
      }
    }

    const graph = { name: graphName, start_node: startNode, nodes: graphNodes }
    const res = await fetch('/api/workflows/graphs', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ name: graphName, graph }),
    })
    if (res.ok) {
      setSaveStatus('Saved!')
      setTimeout(() => setSaveStatus(''), 2000)
      return true
    } else {
      const txt = await res.text()
      setSaveStatus(`Error: ${txt}`)
      return false
    }
  }

  // ── Test from node ────────────────────────────────────────────────────────

  const runTestFromNode = async (nodeId: string, contextJson: string) => {
    // Save the graph first
    const ok = await saveGraph()
    if (!ok) throw new Error(saveStatus || 'Save failed — check graph name and nodes')

    let ctx: Record<string, unknown> = {}
    try { ctx = JSON.parse(contextJson) } catch { throw new Error('Invalid JSON in context') }

    const res = await fetch('/api/workflows', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ graph_name: graphName, start_node: nodeId, context: ctx }),
    })
    if (!res.ok) {
      const txt = await res.text()
      throw new Error(txt)
    }
    const data = await res.json()
    navigate(`/workflows/${data.workflow.id}`)
  }

  // ── Render helpers ────────────────────────────────────────────────────────

  const selNode = selectedNode ? nodes.find(n => n.id === selectedNode) : null
  const selEdge = selectedEdge ? edges.find(e => e.id === selectedEdge) : null

  const svgW = Math.max(800, ...nodes.map(n => n.x + NODE_W + 100))
  const svgH = Math.max(600, ...nodes.map(n => n.y + NODE_H + 100))

  // Build edge paths
  function edgePath(fromNode: string, toNode: string) {
    const src = nodes.find(n => n.id === fromNode)
    const dst = nodes.find(n => n.id === toNode)
    if (!src || !dst) return null
    const x1 = src.x + NODE_W + PORT_R
    const y1 = src.y + NODE_H / 2
    const x2 = dst.x - PORT_R
    const y2 = dst.y + NODE_H / 2
    const mx = (x1 + x2) / 2
    return { x1, y1, x2, y2, mx }
  }

  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100vh', fontFamily: '-apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif', background: '#f5f7fa' }}>

      {/* Header */}
      <header style={{ background: 'white', borderBottom: '1px solid #e0e6ed', padding: '10px 16px', display: 'flex', alignItems: 'center', gap: '12px', boxShadow: '0 2px 4px rgba(0,0,0,0.05)' }}>
        <button onClick={() => navigate('/workflows')} style={hBtn('#667eea')}>← Workflows</button>
        <h1 style={{ margin: 0, fontSize: '18px', color: '#667eea', fontWeight: 600 }}>Workflow Builder</h1>
        <div style={{ flex: 1 }} />
        <label style={{ fontSize: '12px', color: '#666', fontWeight: 600 }}>Graph Name:</label>
        <input value={graphName} onChange={e => setGraphName(e.target.value)}
          style={{ fontFamily: 'monospace', padding: '5px 8px', border: '1px solid #e0e6ed', borderRadius: '4px', fontSize: '13px', width: '180px' }} />
        <button onClick={saveGraph} style={hBtn('#27ae60')}>Save Graph</button>
        {saveStatus && <span style={{ fontSize: '12px', color: saveStatus.startsWith('Error') ? '#e74c3c' : '#27ae60', fontWeight: 600 }}>{saveStatus}</span>}
      </header>

      <div style={{ display: 'flex', flex: 1, overflow: 'hidden' }}>

        {/* Left sidebar — activity library */}
        <div style={{ width: '220px', background: 'white', borderRight: '1px solid #e0e6ed', padding: '12px', overflowY: 'auto', flexShrink: 0 }}>
          <div style={{ fontSize: '11px', fontWeight: 700, color: '#888', textTransform: 'uppercase', letterSpacing: '0.5px', marginBottom: '8px' }}>
            Activities
          </div>
          <div style={{ fontSize: '10px', color: '#aaa', marginBottom: '10px' }}>Drag onto canvas</div>

          {/* END / Finish node */}
          <div
            draggable
            onDragStart={() => { dragActivity.current = 'END' }}
            style={{
              background: '#fff0f0', border: '1px solid #f9c0c0', borderRadius: '6px',
              padding: '8px 10px', cursor: 'grab', marginBottom: '6px',
              fontSize: '12px', fontFamily: 'monospace', color: '#c0392b',
              fontWeight: 700, userSelect: 'none',
            }}
          >
            ⏹ END (Finish)
          </div>

          {/* Human Input node */}
          <div
            draggable
            onDragStart={() => { dragActivity.current = '__human__' }}
            style={{
              background: '#fffbf0', border: '1px solid #f9e0a0', borderRadius: '6px',
              padding: '8px 10px', cursor: 'grab', marginBottom: '6px',
              fontSize: '12px', fontFamily: 'monospace', color: '#b7770d',
              fontWeight: 700, userSelect: 'none',
            }}
          >
            👤 Human Input
            <div style={{ fontSize: '10px', fontFamily: 'sans-serif', fontWeight: 400, color: '#a08030', marginTop: '2px' }}>
              Pauses for human approval
            </div>
          </div>

          {/* Python Script node */}
          <div
            draggable
            onDragStart={() => { dragActivity.current = '__python__' }}
            style={{
              background: '#f0f0ff', border: '1px solid #3572A5', borderRadius: '6px',
              padding: '8px 10px', cursor: 'grab', marginBottom: '6px',
              fontSize: '12px', fontFamily: 'monospace', color: '#3572A5',
              fontWeight: 700, userSelect: 'none',
            }}
          >
            🐍 Python Script
            <div style={{ fontSize: '10px', fontFamily: 'sans-serif', fontWeight: 400, color: '#5080a0', marginTop: '2px' }}>
              Run Python code in-browser via Pyodide
            </div>
          </div>

          {/* Muxer / Condenser */}
          <div
            draggable
            onDragStart={() => { dragActivity.current = 'muxer' }}
            style={{
              background: '#f0f8ff', border: '1px solid #a0c8f0', borderRadius: '6px',
              padding: '8px 10px', cursor: 'grab', marginBottom: '6px',
              fontSize: '12px', fontFamily: 'monospace', color: '#1a6aa0',
              fontWeight: 700, userSelect: 'none',
            }}
          >
            ⇉ Muxer
            <div style={{ fontSize: '10px', fontFamily: 'sans-serif', fontWeight: 400, color: '#4080a0', marginTop: '2px' }}>
              Fan-out: runs all next nodes in parallel
            </div>
          </div>
          <div
            draggable
            onDragStart={() => { dragActivity.current = 'condenser' }}
            style={{
              background: '#f0fff4', border: '1px solid #90d0a0', borderRadius: '6px',
              padding: '8px 10px', cursor: 'grab', marginBottom: '6px',
              fontSize: '12px', fontFamily: 'monospace', color: '#1a7a40',
              fontWeight: 700, userSelect: 'none',
            }}
          >
            ⇇ Condenser
            <div style={{ fontSize: '10px', fontFamily: 'sans-serif', fontWeight: 400, color: '#408050', marginTop: '2px' }}>
              Fan-in: merges parallel branches into one
            </div>
          </div>

          {/* Loop */}
          <div
            draggable
            onDragStart={() => { dragActivity.current = 'loop' }}
            style={{
              background: '#f0fffe', border: '1px solid #1abc9c', borderRadius: '6px',
              padding: '8px 10px', cursor: 'grab', marginBottom: '6px',
              fontSize: '12px', fontFamily: 'monospace', color: '#0e8c72',
              fontWeight: 700, userSelect: 'none',
            }}
          >
            ↻ Loop
            <div style={{ fontSize: '10px', fontFamily: 'sans-serif', fontWeight: 400, color: '#1a9a80', marginTop: '2px' }}>
              Iterate over a list. Label edges "body" and "done"
            </div>
          </div>
          <div
            draggable
            onDragStart={() => { dragActivity.current = 'loop_next' }}
            style={{
              background: '#f5fff5', border: '1px solid #82c996', borderRadius: '6px',
              padding: '8px 10px', cursor: 'grab', marginBottom: '10px',
              fontSize: '12px', fontFamily: 'monospace', color: '#1a6a30',
              fontWeight: 700, userSelect: 'none',
            }}
          >
            ↺ Loop Next
            <div style={{ fontSize: '10px', fontFamily: 'sans-serif', fontWeight: 400, color: '#2a804a', marginTop: '2px' }}>
              End of one iteration — place after loop body
            </div>
          </div>

          {activities.map(a => (
            <ActivityCard key={a.name} info={a}
              onDragStart={name => { dragActivity.current = name }} />
          ))}

          {activities.length === 0 && (
            <div style={{ fontSize: '12px', color: '#aaa' }}>Loading activities…</div>
          )}
        </div>

        {/* Canvas */}
        <div style={{ flex: 1, overflow: 'auto', position: 'relative' }}>
          {graphNotFound && (
            <div style={{ background: '#fff3cd', border: '1px solid #ffc107', borderRadius: '6px', padding: '12px 16px', margin: '12px', fontSize: '13px', color: '#856404' }}>
              Graph "{graphNotFound}" was not found — it may have been created before the last server restart. The graph name has been pre-filled below. Rebuild it and click Save Graph.
            </div>
          )}
          {connecting && (
            <div style={{ position: 'absolute', top: '8px', left: '50%', transform: 'translateX(-50%)', background: '#f39c12', color: 'white', padding: '4px 14px', borderRadius: '20px', fontSize: '12px', fontWeight: 600, zIndex: 10, pointerEvents: 'none' }}>
              Click another node's left port (●) to connect, or click canvas to cancel
            </div>
          )}
          <svg
            ref={svgRef}
            width={svgW} height={svgH}
            style={{ display: 'block', cursor: connecting ? 'crosshair' : draggingNode ? 'grabbing' : 'default' }}
            onDragOver={e => e.preventDefault()}
            onDrop={onCanvasDrop}
            onMouseMove={onSvgMouseMove}
            onMouseUp={onSvgMouseUp}
            onClick={onSvgClick}
          >
            <defs>
              <marker id="arrowB" markerWidth="8" markerHeight="8" refX="6" refY="3" orient="auto">
                <path d="M0,0 L0,6 L8,3 z" fill="#aaa" />
              </marker>
            </defs>

            {/* Grid */}
            <defs>
              <pattern id="grid" width="24" height="24" patternUnits="userSpaceOnUse">
                <path d="M 24 0 L 0 0 0 24" fill="none" stroke="#e8ecf0" strokeWidth="0.5" />
              </pattern>
            </defs>
            <rect width={svgW} height={svgH} fill="url(#grid)" />

            {/* Edges */}
            {edges.map(e => {
              const p = edgePath(e.fromNode, e.toNode)
              if (!p) return null
              const isSelected = e.id === selectedEdge
              return (
                <g key={e.id} style={{ cursor: 'pointer' }}
                  onClick={(ev) => { ev.stopPropagation(); setSelectedEdge(e.id); setSelectedNode(null) }}>
                  {/* Wider invisible hit area */}
                  <path d={`M${p.x1},${p.y1} C${p.mx},${p.y1} ${p.mx},${p.y2} ${p.x2},${p.y2}`}
                    fill="none" stroke="transparent" strokeWidth={12} />
                  <path d={`M${p.x1},${p.y1} C${p.mx},${p.y1} ${p.mx},${p.y2} ${p.x2},${p.y2}`}
                    fill="none" stroke={isSelected ? '#667eea' : '#aaa'} strokeWidth={isSelected ? 2 : 1.5}
                    markerEnd="url(#arrowB)" />
                  {(e.label || e.conditions.length > 0) && (
                    <text x={(p.x1 + p.x2) / 2} y={(p.y1 + p.y2) / 2 - 6}
                      textAnchor="middle" fontSize={10} fill={isSelected ? '#667eea' : '#888'}>
                      {e.label || e.conditions.map(c => `${c.key} ${c.operator} ${c.value}`).join(' & ')}
                    </text>
                  )}
                </g>
              )
            })}

            {/* Nodes */}
            {nodes.map(n => {
              const isEnd = n.activityName === 'END'
              const isMuxer = n.activityName === 'muxer'
              const isCondenser = n.activityName === 'condenser'
              const isPython = n.activityName === 'python_eval'
              const isLoop = n.activityName === 'loop'
              const isLoopNext = n.activityName === 'loop_next'
              const isStart = n.id === startNode
              const isSelected = n.id === selectedNode
              const fill = isEnd ? '#fff0f0' : isMuxer ? '#eef6ff' : isCondenser ? '#efffef' : isPython ? '#f0f0ff' : isLoop ? '#f0fffe' : isLoopNext ? '#f5fff5' : n.isHuman ? '#fffbf0' : isStart ? '#f0f4ff' : 'white'
              const borderColor = isSelected ? '#667eea' : isMuxer ? '#a0c8f0' : isCondenser ? '#90d0a0' : isPython ? '#3572A5' : isLoop ? '#1abc9c' : isLoopNext ? '#82c996' : isStart ? '#667eea' : isEnd ? '#f9c0c0' : n.isHuman ? '#f9e0a0' : '#ccc'

              return (
                <g key={n.id}
                  style={{ cursor: draggingNode === n.id ? 'grabbing' : 'grab' }}
                  onMouseDown={e => onNodeMouseDown(e, n.id)}
                  onClick={e => { e.stopPropagation(); setSelectedNode(n.id); setSelectedEdge(null) }}
                >
                  {isSelected && (
                    <rect x={n.x - 3} y={n.y - 3} width={NODE_W + 6} height={NODE_H + 6}
                      rx={9} fill="none" stroke="#667eea" strokeWidth={2} strokeDasharray="4 2" />
                  )}
                  <rect x={n.x} y={n.y} width={NODE_W} height={NODE_H} rx={6}
                    fill={fill} stroke={borderColor} strokeWidth={isSelected ? 2 : 1} />

                  {isEnd ? (
                    <>
                      <text x={n.x + NODE_W / 2} y={n.y + NODE_H / 2 + 5}
                        textAnchor="middle" fontSize={13} fontWeight={700} fill="#c0392b">
                        ⏹ END
                      </text>
                      {/* Input port only */}
                      <circle cx={n.x - PORT_R} cy={n.y + NODE_H / 2} r={PORT_R}
                        fill={connecting ? '#27ae60' : '#e0e6ed'} stroke="#aaa" strokeWidth={1}
                        style={{ cursor: connecting ? 'pointer' : 'default' }}
                        onClick={e => onInputPortClick(e, n.id)} />
                    </>
                  ) : isMuxer ? (
                    <>
                      <text x={n.x + NODE_W / 2} y={n.y + 20}
                        textAnchor="middle" fontSize={12} fontWeight={700} fill="#1a6aa0">
                        ⇉ {n.label || n.id}
                      </text>
                      <text x={n.x + NODE_W / 2} y={n.y + 36}
                        textAnchor="middle" fontSize={10} fill="#4080a0" fontFamily="monospace">muxer</text>
                      <circle cx={n.x - PORT_R} cy={n.y + NODE_H / 2} r={PORT_R}
                        fill={connecting ? '#27ae60' : '#e0e6ed'} stroke="#aaa" strokeWidth={1}
                        style={{ cursor: connecting ? 'pointer' : 'default' }}
                        onClick={e => onInputPortClick(e, n.id)} />
                      <circle cx={n.x + NODE_W + PORT_R} cy={n.y + NODE_H / 2} r={PORT_R}
                        fill={connecting === n.id ? '#667eea' : '#e0e6ed'} stroke="#aaa" strokeWidth={1}
                        style={{ cursor: 'pointer' }}
                        onClick={e => onOutputPortClick(e, n.id)} />
                    </>
                  ) : isCondenser ? (
                    <>
                      <text x={n.x + NODE_W / 2} y={n.y + 20}
                        textAnchor="middle" fontSize={12} fontWeight={700} fill="#1a7a40">
                        ⇇ {n.label || n.id}
                      </text>
                      <text x={n.x + NODE_W / 2} y={n.y + 36}
                        textAnchor="middle" fontSize={10} fill="#408050" fontFamily="monospace">condenser</text>
                      <circle cx={n.x - PORT_R} cy={n.y + NODE_H / 2} r={PORT_R}
                        fill={connecting ? '#27ae60' : '#e0e6ed'} stroke="#aaa" strokeWidth={1}
                        style={{ cursor: connecting ? 'pointer' : 'default' }}
                        onClick={e => onInputPortClick(e, n.id)} />
                      <circle cx={n.x + NODE_W + PORT_R} cy={n.y + NODE_H / 2} r={PORT_R}
                        fill={connecting === n.id ? '#667eea' : '#e0e6ed'} stroke="#aaa" strokeWidth={1}
                        style={{ cursor: 'pointer' }}
                        onClick={e => onOutputPortClick(e, n.id)} />
                    </>
                  ) : isLoop ? (
                    <>
                      <text x={n.x + NODE_W / 2} y={n.y + 20}
                        textAnchor="middle" fontSize={12} fontWeight={700} fill="#0e8c72">
                        ↻ {n.label || n.id}
                      </text>
                      <text x={n.x + NODE_W / 2} y={n.y + 36}
                        textAnchor="middle" fontSize={10} fill="#1abc9c" fontFamily="monospace">loop</text>
                      <circle cx={n.x - PORT_R} cy={n.y + NODE_H / 2} r={PORT_R}
                        fill={connecting ? '#27ae60' : '#e0e6ed'} stroke="#aaa" strokeWidth={1}
                        style={{ cursor: connecting ? 'pointer' : 'default' }}
                        onClick={e => onInputPortClick(e, n.id)} />
                      <circle cx={n.x + NODE_W + PORT_R} cy={n.y + NODE_H / 2} r={PORT_R}
                        fill={connecting === n.id ? '#667eea' : '#e0e6ed'} stroke="#aaa" strokeWidth={1}
                        style={{ cursor: 'pointer' }}
                        onClick={e => onOutputPortClick(e, n.id)} />
                    </>
                  ) : isLoopNext ? (
                    <>
                      <text x={n.x + NODE_W / 2} y={n.y + 20}
                        textAnchor="middle" fontSize={12} fontWeight={700} fill="#1a6a30">
                        ↺ {n.label || n.id}
                      </text>
                      <text x={n.x + NODE_W / 2} y={n.y + 36}
                        textAnchor="middle" fontSize={10} fill="#82c996" fontFamily="monospace">loop_next</text>
                      <circle cx={n.x - PORT_R} cy={n.y + NODE_H / 2} r={PORT_R}
                        fill={connecting ? '#27ae60' : '#e0e6ed'} stroke="#aaa" strokeWidth={1}
                        style={{ cursor: connecting ? 'pointer' : 'default' }}
                        onClick={e => onInputPortClick(e, n.id)} />
                    </>
                  ) : isPython ? (
                    <>
                      <text x={n.x + 10} y={n.y + 18} fontSize={12} fill="#3572A5">🐍</text>
                      <text x={n.x + NODE_W / 2} y={n.y + (n.label ? 18 : 22)}
                        textAnchor="middle" fontSize={11} fontWeight={600} fill="#3572A5">
                        🐍 {n.label || n.id}
                      </text>
                      <text x={n.x + NODE_W / 2} y={n.y + 36}
                        textAnchor="middle" fontSize={10} fill="#3572A5" fontFamily="monospace">python_eval</text>
                      {isStart && (
                        <text x={n.x + NODE_W / 2} y={n.y + NODE_H - 4}
                          textAnchor="middle" fontSize={9} fill="#27ae60">START</text>
                      )}
                      {n.needsConversion && (
                        <text x={n.x + NODE_W - 6} y={n.y + 14} textAnchor="end" fontSize={11}>🔄</text>
                      )}
                      <circle cx={n.x - PORT_R} cy={n.y + NODE_H / 2} r={PORT_R}
                        fill={connecting ? '#27ae60' : '#e0e6ed'} stroke="#aaa" strokeWidth={1}
                        style={{ cursor: connecting ? 'pointer' : 'default' }}
                        onClick={e => onInputPortClick(e, n.id)} />
                      <circle cx={n.x + NODE_W + PORT_R} cy={n.y + NODE_H / 2} r={PORT_R}
                        fill={connecting === n.id ? '#667eea' : '#e0e6ed'} stroke="#aaa" strokeWidth={1}
                        style={{ cursor: 'pointer' }}
                        onClick={e => onOutputPortClick(e, n.id)} />
                    </>
                  ) : (
                    <>
                      {n.isHuman && (
                        <text x={n.x + 10} y={n.y + 18} fontSize={12} fill="#555">👤</text>
                      )}
                      <text x={n.x + NODE_W / 2} y={n.y + (n.label ? 18 : 22)}
                        textAnchor="middle" fontSize={11} fontWeight={600} fill="#2c3e50">
                        {n.label || n.id}
                      </text>
                      <text x={n.x + NODE_W / 2} y={n.y + 36}
                        textAnchor="middle" fontSize={10} fill="#667eea" fontFamily="monospace">
                        {n.activityName}
                      </text>
                      {isStart && (
                        <text x={n.x + NODE_W / 2} y={n.y + NODE_H - 4}
                          textAnchor="middle" fontSize={9} fill="#27ae60">START</text>
                      )}

                      {/* Input port (left) */}
                      <circle cx={n.x - PORT_R} cy={n.y + NODE_H / 2} r={PORT_R}
                        fill={connecting ? '#27ae60' : '#e0e6ed'} stroke="#aaa" strokeWidth={1}
                        style={{ cursor: connecting ? 'pointer' : 'default' }}
                        onClick={e => onInputPortClick(e, n.id)} />
                      {/* Output port (right) */}
                      <circle cx={n.x + NODE_W + PORT_R} cy={n.y + NODE_H / 2} r={PORT_R}
                        fill={connecting === n.id ? '#667eea' : '#e0e6ed'} stroke="#aaa" strokeWidth={1}
                        style={{ cursor: 'pointer' }}
                        onClick={e => onOutputPortClick(e, n.id)} />
                    </>
                  )}
                </g>
              )
            })}
          </svg>
        </div>

        {/* Right panel — properties */}
        <div style={{ width: '240px', background: 'white', borderLeft: '1px solid #e0e6ed', padding: '14px', overflowY: 'auto', flexShrink: 0 }}>
          {selNode ? (
            <NodeEditor
              node={selNode}
              isStart={selNode.id === startNode}
              activities={activities}
              onSetStart={() => setStartNode(selNode.id)}
              onChange={updated => setNodes(prev => prev.map(n => n.id === updated.id ? updated : n))}
              onDelete={() => {
                setNodes(prev => prev.filter(n => n.id !== selNode.id))
                setEdges(prev => prev.filter(e => e.fromNode !== selNode.id && e.toNode !== selNode.id))
                if (startNode === selNode.id) setStartNode(null)
                setSelectedNode(null)
              }}
              onTestFromHere={nodeId => setTestModal(nodeId)}
            />
          ) : selEdge ? (
            <EdgeEditor
              edge={selEdge}
              onChange={updated => setEdges(prev => prev.map(e => e.id === updated.id ? updated : e))}
              onDelete={() => { setEdges(prev => prev.filter(e => e.id !== selEdge.id)); setSelectedEdge(null) }}
            />
          ) : (
            <div>
              <div style={{ fontWeight: 700, fontSize: '13px', color: '#555', marginBottom: '12px' }}>Canvas</div>
              <div style={{ fontSize: '12px', color: '#aaa', lineHeight: 1.7 }}>
                <div>• Drag activities from the left panel onto the canvas</div>
                <div>• Click the right ● port of a node to start connecting</div>
                <div>• Click the left ● port of another node to complete the edge</div>
                <div>• Click a node or edge to edit its properties</div>
                <div>• Drag a node to reposition it</div>
              </div>
              <div style={{ marginTop: '16px', fontSize: '12px', color: '#888' }}>
                {nodes.length} nodes · {edges.length} edges
              </div>
            </div>
          )}
        </div>
      </div>

      {testModal && (
        <TestModal
          nodeId={testModal}
          graphName={graphName}
          onClose={() => setTestModal(null)}
          onRun={runTestFromNode}
        />
      )}
    </div>
  )
}

// ── Style helpers ─────────────────────────────────────────────────────────────

const labelStyle: React.CSSProperties = {
  fontSize: '11px', fontWeight: 600, color: '#666', display: 'block',
  marginBottom: '3px', marginTop: '8px',
}
const inputStyle: React.CSSProperties = {
  width: '100%', padding: '5px 7px', border: '1px solid #e0e6ed', borderRadius: '4px',
  fontSize: '12px', boxSizing: 'border-box', marginBottom: '2px',
}
function iconBtn(bg: string): React.CSSProperties {
  return { background: bg, color: 'white', border: 'none', padding: '4px 8px', borderRadius: '4px', cursor: 'pointer', fontSize: '12px' }
}
function hBtn(bg: string): React.CSSProperties {
  return { background: bg, color: 'white', border: 'none', padding: '6px 12px', borderRadius: '4px', cursor: 'pointer', fontSize: '13px', fontWeight: 500 }
}
