import { useState, useEffect, useCallback } from 'react'
import { useNavigate } from 'react-router-dom'
import type { Workflow, WorkflowStatus } from './types'
import Nav from './Nav'

const STATUS_COLORS: Record<WorkflowStatus, { bg: string; color: string }> = {
  pending:   { bg: '#e8f4fd', color: '#2980b9' },
  scheduled: { bg: '#eaf5ea', color: '#27ae60' },
  running:   { bg: '#fef9e7', color: '#f39c12' },
  paused:    { bg: '#fdf2e9', color: '#e67e22' },
  completed: { bg: '#eafaf1', color: '#1e8449' },
  cancelled: { bg: '#f2f3f4', color: '#717d7e' },
  failed:    { bg: '#fdedec', color: '#c0392b' },
}

const ACTIVE_STATUSES: WorkflowStatus[] = ['pending', 'scheduled', 'running', 'paused']

function StatusBadge({ status }: { status: WorkflowStatus }) {
  const c = STATUS_COLORS[status] ?? { bg: '#f0f0f0', color: '#555' }
  return (
    <span style={{ background: c.bg, color: c.color, padding: '2px 10px', borderRadius: '12px', fontSize: '12px', fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.5px' }}>
      {status}
    </span>
  )
}

function formatDate(s?: string) {
  if (!s) return '—'
  return new Date(s).toLocaleString()
}

function sortByStartedAt(list: Workflow[]): Workflow[] {
  return [...list].sort((a, b) => {
    const aTime = a.started_at ? new Date(a.started_at).getTime() : 0
    const bTime = b.started_at ? new Date(b.started_at).getTime() : 0
    return bTime - aTime
  })
}

const TABLE_HEADERS = ['Name', 'Graph', 'Status', 'Current Node', 'Started', 'Finished', 'Actions']

function WorkflowTableRows({ workflows, navigate, cancel, restart, deleteWorkflow }: {
  workflows: Workflow[]
  navigate: (path: string) => void
  cancel: (id: string, e: React.MouseEvent) => void
  restart: (id: string, e: React.MouseEvent) => void
  deleteWorkflow: (id: string, name: string, e: React.MouseEvent) => void
}) {
  return (
    <>
      {workflows.map(wf => (
        <tr key={wf.id} onClick={() => navigate(`/workflows/${wf.id}`)}
          style={{ cursor: 'pointer', transition: 'background 0.1s' }}
          onMouseEnter={e => (e.currentTarget.style.background = '#f8f9fa')}
          onMouseLeave={e => (e.currentTarget.style.background = 'white')}>
          <td style={{ padding: '12px 16px', borderBottom: '1px solid #e0e6ed', fontWeight: 500 }}>
            {wf.name || <span style={{ color: '#aaa' }}>{wf.id.slice(0, 8)}</span>}
          </td>
          <td style={{ padding: '12px 16px', borderBottom: '1px solid #e0e6ed', fontSize: '13px', color: '#667eea', fontFamily: 'monospace' }}>{wf.graph_name}</td>
          <td style={{ padding: '12px 16px', borderBottom: '1px solid #e0e6ed' }}><StatusBadge status={wf.status} /></td>
          <td style={{ padding: '12px 16px', borderBottom: '1px solid #e0e6ed', fontSize: '13px', fontFamily: 'monospace', color: '#555' }}>{wf.current_node || '—'}</td>
          <td style={{ padding: '12px 16px', borderBottom: '1px solid #e0e6ed', fontSize: '12px', color: '#888' }}>{formatDate(wf.started_at)}</td>
          <td style={{ padding: '12px 16px', borderBottom: '1px solid #e0e6ed', fontSize: '12px', color: '#888' }}>{formatDate(wf.finished_at)}</td>
          <td style={{ padding: '12px 16px', borderBottom: '1px solid #e0e6ed' }} onClick={e => e.stopPropagation()}>
            <div style={{ display: 'flex', gap: '6px' }}>
              <button onClick={e => { e.stopPropagation(); navigate(`/workflows/builder?graph=${encodeURIComponent(wf.graph_name)}`) }} style={actionBtn('#9b59b6')}>Edit</button>
              {['running', 'paused', 'pending', 'scheduled'].includes(wf.status) && (
                <button onClick={e => cancel(wf.id, e)} style={actionBtn('#e74c3c')}>Cancel</button>
              )}
              {['completed', 'failed', 'cancelled'].includes(wf.status) && (
                <button onClick={e => restart(wf.id, e)} style={actionBtn('#667eea')}>Restart</button>
              )}
              <button onClick={e => deleteWorkflow(wf.id, wf.name, e)} style={actionBtn('#c0392b')}>Delete</button>
            </div>
          </td>
        </tr>
      ))}
    </>
  )
}

export default function WorkflowList() {
  const [workflows, setWorkflows] = useState<Workflow[]>([])
  const [loading, setLoading] = useState(true)
  const [createModal, setCreateModal] = useState(false)
  const [graphs, setGraphs] = useState<string[]>([])
  const [newName, setNewName] = useState('')
  const [newGraph, setNewGraph] = useState('')
  const [newContext, setNewContext] = useState('{}')
  const [graphsExpanded, setGraphsExpanded] = useState(true)
  const navigate = useNavigate()

  const fetchWorkflows = useCallback(async () => {
    const res = await fetch('/api/workflows')
    if (res.ok) {
      const data = await res.json()
      setWorkflows(data.workflows ?? [])
    }
    setLoading(false)
  }, [])

  const fetchGraphs = useCallback(async () => {
    const res = await fetch('/api/workflows/graphs')
    if (res.ok) {
      const data = await res.json()
      setGraphs(data.names ?? [])
    }
  }, [])

  useEffect(() => { fetchGraphs() }, [fetchGraphs])

  useEffect(() => {
    fetchWorkflows()
  }, [fetchWorkflows])

  // SSE for live updates
  useEffect(() => {
    const es = new EventSource('/api/workflows/stream')
    es.onmessage = (e) => {
      const ev = JSON.parse(e.data)
      if (ev.type === 'workflow_update' && ev.workflow) {
        setWorkflows(prev => {
          const idx = prev.findIndex(w => w.id === ev.workflow.id)
          if (idx >= 0) {
            const next = [...prev]
            next[idx] = ev.workflow
            return next
          }
          return [ev.workflow, ...prev]
        })
      }
    }
    return () => es.close()
  }, [])

  const openCreateModal = async () => {
    setNewGraph(sortedGraphs[0] ?? '')
    setNewName('')
    setNewContext('{}')
    setCreateModal(true)
  }

  const createWorkflow = async () => {
    let ctx: Record<string, unknown> = {}
    try { ctx = JSON.parse(newContext) } catch { /* keep empty */ }
    const res = await fetch('/api/workflows', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ name: newName, graph_name: newGraph, context: ctx }),
    })
    if (res.ok) {
      const data = await res.json()
      setCreateModal(false)
      navigate(`/workflows/${data.workflow.id}`)
    }
  }

  const cancel = async (id: string, e: React.MouseEvent) => {
    e.stopPropagation()
    await fetch(`/api/workflows/${id}/cancel`, { method: 'POST' })
    fetchWorkflows()
  }

  const deleteWorkflow = async (id: string, name: string, e: React.MouseEvent) => {
    e.stopPropagation()
    if (!confirm(`Delete workflow "${name || id.slice(0, 8)}"? This cannot be undone.`)) return
    await fetch(`/api/workflows/${id}`, { method: 'DELETE' })
    setWorkflows(prev => prev.filter(w => w.id !== id))
  }

  const restart = async (id: string, e: React.MouseEvent) => {
    e.stopPropagation()
    await fetch(`/api/workflows/${id}/restart`, { method: 'POST' })
    fetchWorkflows()
  }

  const duplicateGraph = async (g: string) => {
    const res = await fetch(`/api/workflows/graphs/${encodeURIComponent(g)}/duplicate`, { method: 'POST' })
    if (res.ok) {
      await fetchGraphs()
    } else {
      alert(`Failed to duplicate graph "${g}"`)
    }
  }

  const deleteGraph = async (g: string) => {
    if (!confirm(`Delete graph "${g}"? This cannot be undone.`)) return
    const res = await fetch(`/api/workflows/graphs/${encodeURIComponent(g)}`, { method: 'DELETE' })
    if (res.ok) {
      setGraphs(prev => prev.filter(name => name !== g))
    } else {
      alert(`Failed to delete graph "${g}"`)
    }
  }

  const sortedGraphs = [...graphs].sort((a, b) => a.localeCompare(b))
  const activeWorkflows = sortByStartedAt(workflows.filter(w => (ACTIVE_STATUSES as string[]).includes(w.status)))

  return (
    <div style={{ display: 'flex', height: '100vh', fontFamily: '-apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif' }}>
      <Nav />
      <div style={{ flex: 1, overflow: 'auto', background: '#f5f7fa' }}>

      <header style={{ background: 'white', borderBottom: '1px solid #e0e6ed', padding: '14px 24px', display: 'flex', justifyContent: 'space-between', alignItems: 'center', boxShadow: '0 2px 4px rgba(0,0,0,0.05)' }}>
        <h1 style={{ fontSize: '20px', color: '#667eea', fontWeight: 600, margin: 0 }}>Workflows</h1>
        <div style={{ display: 'flex', gap: '10px' }}>
          <button onClick={() => navigate('/workflows/builder')} style={navBtn('#9b59b6')}>+ New Workflow</button>
          <button onClick={openCreateModal} style={navBtn('#27ae60')}>▶ Run</button>
        </div>
      </header>

      <div style={{ padding: '20px 24px' }}>
        {/* Graphs panel */}
        <div style={{ background: 'white', borderRadius: '8px', boxShadow: '0 2px 8px rgba(0,0,0,0.06)', marginBottom: '16px', overflow: 'hidden' }}>
          <div onClick={() => setGraphsExpanded(e => !e)}
            style={{ padding: '12px 16px', display: 'flex', justifyContent: 'space-between', alignItems: 'center', cursor: 'pointer', borderBottom: graphsExpanded ? '1px solid #e0e6ed' : 'none' }}>
            <span style={{ fontWeight: 600, fontSize: '13px', color: '#444', textTransform: 'uppercase', letterSpacing: '0.5px' }}>Graphs ({graphs.length})</span>
            <span style={{ color: '#999', fontSize: '12px' }}>{graphsExpanded ? '▲ collapse' : '▼ expand'}</span>
          </div>
          {graphsExpanded && (
            sortedGraphs.length === 0
              ? <div style={{ padding: '16px', color: '#999', fontSize: '13px' }}>No graphs registered.</div>
              : <div>
                  {sortedGraphs.map((g, i) => (
                    <div key={g} style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', padding: '10px 16px', borderBottom: i < sortedGraphs.length - 1 ? '1px solid #f0f2f5' : 'none' }}>
                      <span style={{ fontFamily: 'monospace', fontSize: '13px', color: '#2c3e50' }}>{g}</span>
                      <div style={{ display: 'flex', gap: '6px' }}>
                        <button onClick={() => navigate(`/workflows/builder?graph=${encodeURIComponent(g)}`)}
                          style={{ background: '#9b59b6', color: 'white', border: 'none', borderRadius: '4px', padding: '4px 10px', cursor: 'pointer', fontSize: '12px', fontWeight: 500 }}>Edit</button>
                        <button onClick={() => duplicateGraph(g)}
                          style={{ background: '#2980b9', color: 'white', border: 'none', borderRadius: '4px', padding: '4px 10px', cursor: 'pointer', fontSize: '12px', fontWeight: 500 }}>Duplicate</button>
                        <button onClick={() => navigate(`/history?graph=${encodeURIComponent(g)}`)}
                          style={{ background: '#e67e22', color: 'white', border: 'none', borderRadius: '4px', padding: '4px 10px', cursor: 'pointer', fontSize: '12px', fontWeight: 500 }}>History</button>
                        <button onClick={() => deleteGraph(g)}
                          style={{ background: '#c0392b', color: 'white', border: 'none', borderRadius: '4px', padding: '4px 10px', cursor: 'pointer', fontSize: '12px', fontWeight: 500 }}>Delete</button>
                        <button onClick={() => { setNewGraph(g); setNewName(''); setNewContext('{}'); setCreateModal(true) }}
                          style={{ background: '#27ae60', color: 'white', border: 'none', borderRadius: '4px', padding: '4px 10px', cursor: 'pointer', fontSize: '12px', fontWeight: 500 }}>▶ Run</button>
                      </div>
                    </div>
                  ))}
                </div>
          )}
        </div>

        {/* Active workflows */}
        <div style={{ background: 'white', borderRadius: '8px', boxShadow: '0 2px 8px rgba(0,0,0,0.06)', overflow: 'hidden', marginBottom: '16px' }}>
          {loading ? (
            <div style={{ padding: '40px', textAlign: 'center', color: '#999' }}>Loading...</div>
          ) : activeWorkflows.length === 0 ? (
            <div style={{ padding: '40px', textAlign: 'center', color: '#999' }}>No active workflows.</div>
          ) : (
            <table style={{ width: '100%', borderCollapse: 'collapse' }}>
              <thead>
                <tr>
                  {TABLE_HEADERS.map(h => (
                    <th key={h} style={{ padding: '12px 16px', textAlign: 'left', fontSize: '12px', fontWeight: 600, color: '#666', background: '#f8f9fa', borderBottom: '2px solid #e0e6ed', textTransform: 'uppercase', letterSpacing: '0.5px' }}>{h}</th>
                  ))}
                </tr>
              </thead>
              <tbody>
                <WorkflowTableRows
                  workflows={activeWorkflows}
                  navigate={navigate}
                  cancel={cancel}
                  restart={restart}
                  deleteWorkflow={deleteWorkflow}
                />
              </tbody>
            </table>
          )}
        </div>

      </div>

      {/* Create workflow modal */}
      {createModal && (
        <div style={{ position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.5)', display: 'flex', alignItems: 'center', justifyContent: 'center', zIndex: 1000 }}>
          <div style={{ background: 'white', borderRadius: '8px', padding: '24px', width: '480px', boxShadow: '0 20px 60px rgba(0,0,0,0.3)' }}>
            <h3 style={{ margin: '0 0 16px', color: '#2c3e50' }}>Run Workflow</h3>
            <div style={{ display: 'flex', flexDirection: 'column', gap: '12px' }}>
              <div>
                <label style={{ fontSize: '12px', color: '#666', fontWeight: 600, display: 'block', marginBottom: '4px' }}>Workflow</label>
                <select value={newGraph} onChange={e => setNewGraph(e.target.value)}
                  style={{ width: '100%', padding: '8px', border: '1px solid #e0e6ed', borderRadius: '4px', fontSize: '13px', fontFamily: 'monospace' }}>
                  {sortedGraphs.map(g => <option key={g} value={g}>{g}</option>)}
                </select>
              </div>
              <div>
                <label style={{ fontSize: '12px', color: '#666', fontWeight: 600, display: 'block', marginBottom: '4px' }}>Name <span style={{ fontWeight: 400, color: '#aaa' }}>(optional)</span></label>
                <input value={newName} onChange={e => setNewName(e.target.value)}
                  placeholder="Leave blank to auto-generate"
                  style={{ width: '100%', padding: '8px', border: '1px solid #e0e6ed', borderRadius: '4px', fontSize: '13px', boxSizing: 'border-box' }} />
              </div>
              <div>
                <label style={{ fontSize: '12px', color: '#666', fontWeight: 600, display: 'block', marginBottom: '4px' }}>Initial Context (JSON)</label>
                <textarea value={newContext} onChange={e => setNewContext(e.target.value)} rows={5}
                  style={{ width: '100%', fontFamily: 'monospace', fontSize: '12px', padding: '10px', border: '1px solid #e0e6ed', borderRadius: '4px', boxSizing: 'border-box', resize: 'vertical' }} />
              </div>
            </div>
            <div style={{ display: 'flex', gap: '10px', justifyContent: 'flex-end', marginTop: '16px' }}>
              <button onClick={() => setCreateModal(false)} style={{ padding: '8px 16px', background: '#f0f0f0', border: 'none', borderRadius: '4px', cursor: 'pointer', fontWeight: 500 }}>Cancel</button>
              <button onClick={createWorkflow} style={{ padding: '8px 16px', background: '#27ae60', color: 'white', border: 'none', borderRadius: '4px', cursor: 'pointer', fontWeight: 500 }}>Create</button>
            </div>
          </div>
        </div>
      )}
      </div>
    </div>
  )
}

function navBtn(bg: string): React.CSSProperties {
  return { background: bg, color: 'white', padding: '6px 14px', borderRadius: '4px', textDecoration: 'none', fontSize: '13px', fontWeight: 500, border: 'none', cursor: 'pointer' }
}

function actionBtn(bg: string): React.CSSProperties {
  return { background: bg, color: 'white', border: 'none', padding: '4px 10px', borderRadius: '4px', cursor: 'pointer', fontSize: '12px', fontWeight: 500 }
}
