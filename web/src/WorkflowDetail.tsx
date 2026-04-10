import { useState, useEffect, useRef, useCallback } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import type { Workflow, ActivityInstance, ActivityGraph, WorkflowStatus, ActivityStatus } from './types'
import WorkflowGraph from './WorkflowGraph'

const STATUS_COLORS: Record<WorkflowStatus | ActivityStatus, { bg: string; color: string }> = {
  pending:       { bg: '#e8f4fd', color: '#2980b9' },
  scheduled:     { bg: '#eaf5ea', color: '#27ae60' },
  running:       { bg: '#fef9e7', color: '#f39c12' },
  paused:        { bg: '#fdf2e9', color: '#e67e22' },
  completed:     { bg: '#eafaf1', color: '#1e8449' },
  cancelled:     { bg: '#f2f3f4', color: '#717d7e' },
  failed:        { bg: '#fdedec', color: '#c0392b' },
  skipped:       { bg: '#f2f3f4', color: '#95a5a6' },
  waiting_human: { bg: '#fef3e2', color: '#d68910' },
}

function Badge({ status }: { status: string }) {
  const c = STATUS_COLORS[status as WorkflowStatus] ?? { bg: '#f0f0f0', color: '#555' }
  return (
    <span style={{ background: c.bg, color: c.color, padding: '2px 10px', borderRadius: '12px', fontSize: '11px', fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.5px' }}>
      {status}
    </span>
  )
}

function JsonDisplay({ data, excludeKeys }: { data: Record<string, unknown> | undefined; excludeKeys?: string[] }) {
  const filtered = data && excludeKeys && excludeKeys.length > 0
    ? Object.fromEntries(Object.entries(data).filter(([k]) => !excludeKeys.includes(k)))
    : data
  if (!filtered || Object.keys(filtered).length === 0) {
    return <span style={{ color: '#aaa', fontSize: '12px' }}>—</span>
  }
  return (
    <pre style={{ background: '#1e1e1e', color: '#d4d4d4', padding: '10px 12px', borderRadius: '6px', fontSize: '11px', overflowX: 'auto', margin: 0, lineHeight: 1.6 }}>
      {JSON.stringify(filtered, null, 2)}
    </pre>
  )
}

function CurlModal({ curlCommand, onClose }: { curlCommand: string; onClose: () => void }) {
  const [copied, setCopied] = useState(false)

  const handleCopy = useCallback(() => {
    navigator.clipboard.writeText(curlCommand).then(() => {
      setCopied(true)
      setTimeout(() => setCopied(false), 2000)
    })
  }, [curlCommand])

  // Close on backdrop click or Escape key
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => { if (e.key === 'Escape') onClose() }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [onClose])

  return (
    <div
      onClick={onClose}
      style={{ position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.6)', display: 'flex', alignItems: 'center', justifyContent: 'center', zIndex: 2000 }}
    >
      <div
        onClick={e => e.stopPropagation()}
        style={{ background: '#1e1e2e', borderRadius: '10px', padding: '20px', width: 'min(720px, 90vw)', boxShadow: '0 24px 80px rgba(0,0,0,0.5)', display: 'flex', flexDirection: 'column', gap: '12px' }}
      >
        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
          <span style={{ fontFamily: 'monospace', fontSize: '12px', fontWeight: 700, color: '#89b4fa', textTransform: 'uppercase', letterSpacing: '0.5px' }}>curl command</span>
          <div style={{ display: 'flex', gap: '8px' }}>
            <button
              onClick={handleCopy}
              style={{ background: copied ? '#a6e3a1' : '#313244', color: copied ? '#1e1e2e' : '#cdd6f4', border: 'none', borderRadius: '6px', padding: '5px 14px', fontSize: '12px', fontWeight: 600, cursor: 'pointer', transition: 'background 0.15s, color 0.15s' }}
            >
              {copied ? 'Copied!' : 'Copy'}
            </button>
            <button
              onClick={onClose}
              style={{ background: '#313244', color: '#cdd6f4', border: 'none', borderRadius: '6px', padding: '5px 10px', fontSize: '14px', cursor: 'pointer' }}
            >
              ✕
            </button>
          </div>
        </div>
        <pre style={{ background: '#181825', color: '#cdd6f4', padding: '14px 16px', borderRadius: '6px', fontSize: '12px', fontFamily: 'monospace', overflowX: 'auto', margin: 0, lineHeight: 1.7, whiteSpace: 'pre-wrap', wordBreak: 'break-all' }}>
          {curlCommand}
        </pre>
      </div>
    </div>
  )
}

function formatDate(s?: string) {
  if (!s) return '—'
  return new Date(s).toLocaleString()
}

function duration(a?: string, b?: string) {
  if (!a || !b) return null
  const ms = new Date(b).getTime() - new Date(a).getTime()
  if (ms < 1000) return `${ms}ms`
  if (ms < 60000) return `${(ms / 1000).toFixed(1)}s`
  return `${Math.floor(ms / 60000)}m ${Math.floor((ms % 60000) / 1000)}s`
}

// ── PyodideRunner ──────────────────────────────────────────────────────────────

interface PyodideRunnerProps {
  workflowId: string
  nodeId: string
  code: string
  input: Record<string, unknown>
  onDone: () => void
}

function PyodideRunner({ workflowId, nodeId, code, input, onDone }: PyodideRunnerProps) {
  const [phase, setPhase] = useState<'loading' | 'running' | 'done' | 'error'>('loading')
  const [stdout, setStdout] = useState('')
  const [errorMsg, setErrorMsg] = useState('')
  const ran = useRef(false)

  useEffect(() => {
    if (ran.current) return
    ran.current = true

    const run = async () => {
      try {
        // Load Pyodide script if not already present
        if (!(window as unknown as Record<string, unknown>)['loadPyodide']) {
          await new Promise<void>((resolve, reject) => {
            const existing = document.querySelector('script[data-pyodide]')
            if (existing) { resolve(); return }
            const s = document.createElement('script')
            s.src = 'https://cdn.jsdelivr.net/pyodide/v0.27.3/full/pyodide.js'
            s.dataset.pyodide = '1'
            s.onload = () => resolve()
            s.onerror = () => reject(new Error('Failed to load Pyodide script'))
            document.head.appendChild(s)
          })
        }

        setPhase('running')

        const loadPyodide = (window as unknown as Record<string, unknown>)['loadPyodide'] as (opts: Record<string, unknown>) => Promise<unknown>
        const capturedLines: string[] = []
        const pyodide = await loadPyodide({
          stdout: (line: string) => { capturedLines.push(line); setStdout(capturedLines.join('\n')) },
        }) as {
          globals: { set: (k: string, v: unknown) => void; get: (k: string) => unknown }
          toPy: (v: unknown) => unknown
          runPythonAsync: (code: string) => Promise<void>
        }

        pyodide.globals.set('input', pyodide.toPy(input))
        await pyodide.runPythonAsync(code)

        const rawResult = pyodide.globals.get('result')
        let result: Record<string, unknown> = {}
        if (rawResult instanceof Map) {
          result = Object.fromEntries(rawResult)
        } else if (rawResult && typeof (rawResult as { toJs?: () => unknown }).toJs === 'function') {
          const js = (rawResult as { toJs: () => unknown }).toJs()
          result = js instanceof Map ? Object.fromEntries(js) : (js as Record<string, unknown> ?? {})
        } else if (rawResult && typeof rawResult === 'object') {
          result = rawResult as Record<string, unknown>
        }

        await fetch(`/api/workflows/${workflowId}/trigger`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ input: result }),
        })

        setPhase('done')
        onDone()
      } catch (err) {
        setErrorMsg(String(err))
        setPhase('error')
      }
    }

    run()
  }, []) // intentionally run once on mount; props captured via closure

  const dark: React.CSSProperties = {
    background: '#1e1e2e', color: '#cdd6f4', borderRadius: '8px',
    padding: '16px', fontFamily: 'monospace', fontSize: '12px',
  }

  return (
    <div style={{ background: 'white', borderRadius: '8px', padding: '16px', boxShadow: '0 2px 8px rgba(0,0,0,0.06)', border: '2px solid #3572A5' }}>
      <h3 style={{ fontSize: '13px', color: '#3572A5', margin: '0 0 12px', textTransform: 'uppercase', letterSpacing: '0.5px' }}>
        🐍 Python Eval — node: <code style={{ fontSize: '12px' }}>{nodeId}</code>
      </h3>

      {phase === 'loading' && (
        <div style={{ color: '#f39c12', fontFamily: 'monospace', fontSize: '13px', marginBottom: '8px' }}>
          ⏳ Loading Pyodide...
        </div>
      )}
      {phase === 'running' && (
        <div style={{ color: '#27ae60', fontFamily: 'monospace', fontSize: '13px', marginBottom: '8px' }}>
          ▶ Running Python...
        </div>
      )}
      {phase === 'done' && (
        <div style={{ color: '#27ae60', fontFamily: 'monospace', fontSize: '13px', marginBottom: '8px' }}>
          ✓ Done — result posted to workflow
        </div>
      )}
      {phase === 'error' && (
        <div style={{ color: '#e74c3c', fontFamily: 'monospace', fontSize: '12px', marginBottom: '8px' }}>
          ✗ Error: {errorMsg}
        </div>
      )}

      <div style={{ marginBottom: '8px', fontSize: '11px', color: '#888' }}>Code</div>
      <pre style={{ ...dark, marginBottom: '12px', whiteSpace: 'pre-wrap', wordBreak: 'break-word' }}>{code}</pre>

      {stdout && (
        <>
          <div style={{ marginBottom: '8px', fontSize: '11px', color: '#888' }}>stdout</div>
          <pre style={{ ...dark, color: '#a6e3a1' }}>{stdout}</pre>
        </>
      )}
    </div>
  )
}

// ── WorkflowDetail ─────────────────────────────────────────────────────────────

export default function WorkflowDetail() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const [workflow, setWorkflow] = useState<Workflow | null>(null)
  const [activities, setActivities] = useState<ActivityInstance[]>([])
  const [graph, setGraph] = useState<ActivityGraph | null>(null)
  const [expanded, setExpanded] = useState<Set<string>>(new Set())
  const [triggerModal, setTriggerModal] = useState(false)
  const [triggerInput, setTriggerInput] = useState('{}')
  const [loading, setLoading] = useState(true)
  const [restarting, setRestarting] = useState(false)
  const [pyodidePanel, setPyodidePanel] = useState<{ code: string; input: Record<string, unknown>; nodeId: string } | null>(null)
  const [curlModal, setCurlModal] = useState<string | null>(null)

  const load = async () => {
    const res = await fetch(`/api/workflows/${id}`)
    if (!res.ok) return
    const data = await res.json()
    setWorkflow(data.workflow)
    setActivities(data.activities ?? [])
    setGraph(data.graph ?? null)
    setLoading(false)
  }

  useEffect(() => { load() }, [id])

  // SSE for live updates
  useEffect(() => {
    const es = new EventSource('/api/workflows/stream')
    es.onmessage = (e) => {
      const ev = JSON.parse(e.data)
      if (ev.type === 'workflow_update' && ev.workflow?.id === id) {
        setWorkflow(ev.workflow)
      }
      if (ev.type === 'activity_update' && ev.activity?.workflow_id === id) {
        setActivities(prev => {
          const idx = prev.findIndex(a => a.id === ev.activity.id)
          if (idx >= 0) {
            const next = [...prev]; next[idx] = ev.activity; return next
          }
          return [...prev, ev.activity]
        })
      }
    }
    return () => es.close()
  }, [id])

  // Fallback polling — catches updates missed by SSE (race on fast workflows)
  useEffect(() => {
    const terminal = new Set(['completed', 'failed', 'cancelled'])
    if (!workflow || terminal.has(workflow.status)) return
    const t = setInterval(load, 3000)
    return () => clearInterval(t)
  }, [workflow?.status])

  // Detect when workflow pauses at a python_eval node
  useEffect(() => {
    if (workflow?.status !== 'paused') { setPyodidePanel(null); return }
    const pausedAct = activities.find(a =>
      a.activity_name === 'python_eval' && (a.status === 'pending' || a.status === 'running' || a.status === 'waiting_human')
    )
    if (!pausedAct) return
    const rawNode = graph?.nodes?.[pausedAct.node_id] as unknown as { input?: { code?: string } } | undefined
    const code = rawNode?.input?.code
      ?? (pausedAct.input as { code?: string } | undefined)?.code
      ?? '# no code\nresult = input'
    // Strip 'code' from the input dict — it's the script itself, not data
    const rawInput = { ...(pausedAct.input ?? {}) }
    delete (rawInput as Record<string, unknown>).code
    setPyodidePanel({ code, input: rawInput, nodeId: pausedAct.node_id })
  }, [workflow?.status, activities, graph])

  const cancel = async () => {
    await fetch(`/api/workflows/${id}/cancel`, { method: 'POST' })
  }

  const restart = async () => {
    setRestarting(true)
    await fetch(`/api/workflows/${id}/restart`, { method: 'POST' })
    await load()
    setRestarting(false)
  }

  const trigger = async () => {
    let input: Record<string, unknown> = {}
    try { input = JSON.parse(triggerInput) } catch { /* keep empty */ }
    await fetch(`/api/workflows/${id}/trigger`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ input }),
    })
    setTriggerModal(false)
  }

  const toggleExpand = (actId: string) => {
    setExpanded(prev => {
      const next = new Set(prev)
      next.has(actId) ? next.delete(actId) : next.add(actId)
      return next
    })
  }

  if (loading) return <div style={{ padding: '40px', textAlign: 'center', color: '#999' }}>Loading...</div>
  if (!workflow) return <div style={{ padding: '40px', textAlign: 'center', color: '#e74c3c' }}>Workflow not found.</div>

  return (
    <div style={{ fontFamily: '-apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif', minHeight: '100vh', background: '#f5f7fa' }}>
      <header style={{ background: 'white', borderBottom: '1px solid #e0e6ed', padding: '14px 24px', display: 'flex', justifyContent: 'space-between', alignItems: 'center', boxShadow: '0 2px 4px rgba(0,0,0,0.05)' }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: '12px' }}>
          <button onClick={() => navigate('/workflows')} style={{ background: 'none', border: 'none', cursor: 'pointer', color: '#667eea', fontSize: '18px', padding: 0 }}>←</button>
          <div>
            <h1 style={{ fontSize: '18px', color: '#2c3e50', fontWeight: 600, margin: 0 }}>{workflow.name || workflow.id}</h1>
            <span style={{ fontSize: '12px', color: '#888', fontFamily: 'monospace' }}>{workflow.graph_name}</span>
          </div>
          <Badge status={workflow.status} />
        </div>
        <div style={{ display: 'flex', gap: '8px', alignItems: 'center' }}>
          {workflow.status === 'paused' && (
            <button onClick={() => setTriggerModal(true)} style={btn('#f39c12')}>Trigger Human</button>
          )}
          {['running', 'paused', 'pending', 'scheduled'].includes(workflow.status) && (
            <button onClick={cancel} style={btn('#e74c3c')}>Cancel</button>
          )}
          {['completed', 'failed', 'cancelled'].includes(workflow.status) && (
            <button onClick={restart} disabled={restarting} style={{ ...btn('#667eea'), opacity: restarting ? 0.6 : 1 }}>
              {restarting ? 'Restarting...' : 'Restart'}
            </button>
          )}
          <a href="/logout" style={{ ...btn('#95a5a6'), textDecoration: 'none' }}>Logout</a>
        </div>
      </header>

      <div style={{ padding: '20px 24px', display: 'flex', gap: '20px' }}>
        {/* Left: graph + workflow meta */}
        <div style={{ flex: '0 0 420px', display: 'flex', flexDirection: 'column', gap: '16px' }}>
          {/* Failure banner when no activities ran */}
          {workflow.status === 'failed' && activities.length === 0 && (
            <div style={{ background: '#fdedec', border: '1px solid #f5c6cb', borderRadius: '6px', padding: '12px 16px', fontSize: '13px', color: '#c0392b' }}>
              ⚠ This workflow failed before executing any activities. The graph "{workflow.graph_name}" may not be registered on this server — it could have been created in a previous server session before graph persistence was added. To fix: rebuild the graph in the Workflow Builder and save it, then restart this workflow.
              <button onClick={() => navigate('/workflows/builder?graph=' + encodeURIComponent(workflow.graph_name))} style={{ marginTop: '8px', display: 'block', background: '#e74c3c', color: 'white', border: 'none', padding: '6px 12px', borderRadius: '4px', cursor: 'pointer', fontSize: '12px', fontWeight: 600 }}>Open Builder →</button>
            </div>
          )}
          {/* Meta card */}
          <div style={{ background: 'white', borderRadius: '8px', padding: '16px', boxShadow: '0 2px 8px rgba(0,0,0,0.06)' }}>
            <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '10px 20px', fontSize: '13px' }}>
              {[
                ['Current Node', workflow.current_node || '—'],
                ['Started', formatDate(workflow.started_at)],
                ['Finished', formatDate(workflow.finished_at)],
                ['Duration', duration(workflow.started_at, workflow.finished_at) ?? '—'],
              ].map(([label, value]) => (
                <div key={label}>
                  <div style={{ color: '#999', fontSize: '11px', textTransform: 'uppercase', marginBottom: '2px' }}>{label}</div>
                  <div style={{ color: '#2c3e50', fontWeight: 500, fontFamily: label === 'Current Node' ? 'monospace' : undefined }}>{value}</div>
                </div>
              ))}
            </div>
          </div>

          {/* Graph visualization */}
          {graph && (
            <div style={{ background: 'white', borderRadius: '8px', padding: '16px', boxShadow: '0 2px 8px rgba(0,0,0,0.06)' }}>
              <h3 style={{ fontSize: '13px', color: '#667eea', margin: '0 0 12px', textTransform: 'uppercase', letterSpacing: '0.5px' }}>Flow</h3>
              <WorkflowGraph graph={graph} activities={activities} currentNode={workflow.current_node} />
            </div>
          )}

          {/* Pyodide runner */}
          {pyodidePanel && (
            <PyodideRunner
              workflowId={id!}
              nodeId={pyodidePanel.nodeId}
              code={pyodidePanel.code}
              input={pyodidePanel.input}
              onDone={() => { setPyodidePanel(null); load() }}
            />
          )}

          {/* Context */}
          <div style={{ background: 'white', borderRadius: '8px', padding: '16px', boxShadow: '0 2px 8px rgba(0,0,0,0.06)' }}>
            <h3 style={{ fontSize: '13px', color: '#667eea', margin: '0 0 4px', textTransform: 'uppercase', letterSpacing: '0.5px' }}>Accumulated Context</h3>
            <div style={{ fontSize: '11px', color: '#aaa', marginBottom: '8px' }}>State accumulated across all runs of this workflow</div>
            <JsonDisplay data={workflow.context} />
          </div>
        </div>

        {/* Right: activity list */}
        <div style={{ flex: 1, display: 'flex', flexDirection: 'column', gap: '10px' }}>
          <h2 style={{ fontSize: '15px', color: '#2c3e50', fontWeight: 600, margin: 0 }}>
            Activities ({activities.length})
          </h2>
          {activities.length === 0 && (
            <div style={{ background: 'white', borderRadius: '8px', padding: '30px', textAlign: 'center', color: '#999', boxShadow: '0 2px 8px rgba(0,0,0,0.06)' }}>
              No activities yet.
            </div>
          )}
          {activities.map(act => {
            const isExp = expanded.has(act.id)
            return (
              <div key={act.id} style={{ background: 'white', borderRadius: '8px', boxShadow: '0 2px 8px rgba(0,0,0,0.06)', overflow: 'hidden' }}>
                <div onClick={() => toggleExpand(act.id)} style={{ padding: '12px 16px', display: 'flex', alignItems: 'center', gap: '12px', cursor: 'pointer', userSelect: 'none' }}>
                  <span style={{ fontSize: '12px', color: '#888', transform: isExp ? 'rotate(90deg)' : 'none', display: 'inline-block', transition: 'transform 0.15s' }}>▶</span>
                  <span style={{ fontFamily: 'monospace', fontWeight: 600, fontSize: '14px', color: '#2c3e50', flex: 1 }}>{act.node_id}</span>
                  <span style={{ fontSize: '12px', color: '#888' }}>{act.activity_name}</span>
                  <Badge status={act.status} />
                  {act.error_count > 0 && (
                    <span style={{ fontSize: '11px', color: '#e74c3c', fontWeight: 600 }}>⚠ {act.error_count} err</span>
                  )}
                  <span style={{ fontSize: '11px', color: '#aaa' }}>
                    {duration(act.started_at, act.finished_at) ?? (act.started_at ? 'running…' : '')}
                  </span>
                </div>
                {isExp && (
                  <div style={{ borderTop: '1px solid #e0e6ed', padding: '16px' }}>
                    <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '16px' }}>
                      <div>
                        <div style={{ fontSize: '11px', color: '#667eea', fontWeight: 600, textTransform: 'uppercase', marginBottom: '8px' }}>Input</div>
                        <JsonDisplay data={act.input} />
                      </div>
                      <div>
                        <div style={{ fontSize: '11px', color: '#27ae60', fontWeight: 600, textTransform: 'uppercase', marginBottom: '8px', display: 'flex', alignItems: 'center', gap: '8px' }}>
                          Output
                          {act.output && typeof act.output['curl'] === 'string' && (
                            <button
                              onClick={e => { e.stopPropagation(); setCurlModal(act.output!['curl'] as string) }}
                              style={{ background: '#313244', color: '#89b4fa', border: 'none', borderRadius: '10px', padding: '2px 10px', fontSize: '11px', fontFamily: 'monospace', fontWeight: 700, cursor: 'pointer', letterSpacing: '0.3px' }}
                            >
                              curl
                            </button>
                          )}
                        </div>
                        <JsonDisplay data={act.output} excludeKeys={['curl']} />
                      </div>
                    </div>
                    {act.error_msg && (
                      <div style={{ marginTop: '12px', background: '#fdedec', border: '1px solid #f5c6cb', borderRadius: '4px', padding: '10px', fontSize: '12px', color: '#c0392b', fontFamily: 'monospace' }}>
                        {act.error_msg}
                      </div>
                    )}
                    <div style={{ marginTop: '12px', display: 'flex', gap: '16px', fontSize: '12px', color: '#888' }}>
                      <span>Started: {formatDate(act.started_at)}</span>
                      <span>Finished: {formatDate(act.finished_at)}</span>
                      <span>Retries: {act.error_count}/{act.max_retries}</span>
                    </div>
                  </div>
                )}
              </div>
            )
          })}
        </div>
      </div>

      {/* Curl modal */}
      {curlModal !== null && (
        <CurlModal curlCommand={curlModal} onClose={() => setCurlModal(null)} />
      )}

      {/* Trigger modal */}
      {triggerModal && (
        <div style={{ position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.5)', display: 'flex', alignItems: 'center', justifyContent: 'center', zIndex: 1000 }}>
          <div style={{ background: 'white', borderRadius: '8px', padding: '24px', width: '440px', boxShadow: '0 20px 60px rgba(0,0,0,0.3)' }}>
            <h3 style={{ margin: '0 0 16px', color: '#2c3e50' }}>Trigger Human Step</h3>
            <p style={{ fontSize: '13px', color: '#666', margin: '0 0 12px' }}>Provide JSON input to resume the workflow. This will be merged into the workflow context.</p>
            <textarea
              value={triggerInput}
              onChange={e => setTriggerInput(e.target.value)}
              rows={6}
              style={{ width: '100%', fontFamily: 'monospace', fontSize: '12px', padding: '10px', border: '1px solid #e0e6ed', borderRadius: '4px', boxSizing: 'border-box' }}
            />
            <div style={{ display: 'flex', gap: '10px', justifyContent: 'flex-end', marginTop: '16px' }}>
              <button onClick={() => setTriggerModal(false)} style={{ padding: '8px 16px', background: '#f0f0f0', border: 'none', borderRadius: '4px', cursor: 'pointer', fontWeight: 500 }}>Cancel</button>
              <button onClick={trigger} style={{ ...btn('#f39c12'), border: 'none', cursor: 'pointer' }}>Trigger</button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}

function btn(bg: string): React.CSSProperties {
  return { background: bg, color: 'white', padding: '7px 16px', borderRadius: '4px', fontSize: '13px', fontWeight: 500, border: 'none', cursor: 'pointer' }
}
