import { useState, useEffect, useCallback } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
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

const TERMINAL_STATUSES: WorkflowStatus[] = ['completed', 'failed', 'cancelled']
const PAGE_SIZE = 50

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

const TABLE_HEADERS = ['Name', 'Graph', 'Status', 'Started', 'Finished', 'Actions']

export default function WorkflowHistory() {
  const [searchParams] = useSearchParams()
  const [workflows, setWorkflows] = useState<Workflow[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(0)
  const [statusFilter, setStatusFilter] = useState<string>('terminal')
  const [graphFilter, setGraphFilter] = useState(() => searchParams.get('graph') ?? '')
  const [loading, setLoading] = useState(true)
  const navigate = useNavigate()

  const fetchWorkflows = useCallback(async (p = page, status = statusFilter, graph = graphFilter) => {
    setLoading(true)
    const params = new URLSearchParams({ limit: String(PAGE_SIZE), offset: String(p * PAGE_SIZE) })
    if (status === 'terminal') {
      TERMINAL_STATUSES.forEach(s => params.append('status', s))
    } else if (status) {
      params.set('status', status)
    }
    if (graph) params.set('graph', graph)
    const res = await fetch(`/api/workflows?${params}`)
    if (res.ok) {
      const data = await res.json()
      setWorkflows(data.workflows ?? [])
      setTotal(data.count ?? 0)
    }
    setLoading(false)
  }, [page, statusFilter, graphFilter])

  useEffect(() => { fetchWorkflows(page, statusFilter, graphFilter) }, [page, statusFilter, graphFilter])

  // Live updates via SSE — only update current page view
  useEffect(() => {
    const es = new EventSource('/api/workflows/stream')
    es.onmessage = (e) => {
      const ev = JSON.parse(e.data)
      if (ev.type === 'workflow_update' && ev.workflow) {
        const wf: Workflow = ev.workflow
        setWorkflows(prev => {
          const idx = prev.findIndex(w => w.id === wf.id)
          if (idx >= 0) {
            const next = [...prev]; next[idx] = wf; return next
          }
          return prev
        })
      }
    }
    return () => es.close()
  }, [])

  const restart = async (id: string, e: React.MouseEvent) => {
    e.stopPropagation()
    await fetch(`/api/workflows/${id}/restart`, { method: 'POST' })
    fetchWorkflows(page, statusFilter, graphFilter)
  }

  const restartAllFailed = async () => {
    const failed = workflows.filter(w => w.status === 'failed')
    if (failed.length === 0) return
    await Promise.all(failed.map(w => fetch(`/api/workflows/${w.id}/restart`, { method: 'POST' })))
    fetchWorkflows(page, statusFilter, graphFilter)
  }

  const deleteWorkflow = async (id: string, name: string, e: React.MouseEvent) => {
    e.stopPropagation()
    if (!confirm(`Delete workflow "${name || id.slice(0, 8)}"? This cannot be undone.`)) return
    await fetch(`/api/workflows/${id}`, { method: 'DELETE' })
    setWorkflows(prev => prev.filter(w => w.id !== id))
    setTotal(t => t - 1)
  }

  const totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE))
  const from = total === 0 ? 0 : page * PAGE_SIZE + 1
  const to = Math.min((page + 1) * PAGE_SIZE, total)

  return (
    <div style={{ display: 'flex', height: '100vh', fontFamily: '-apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif' }}>
      <Nav />
      <div style={{ flex: 1, overflow: 'auto', background: '#f5f7fa' }}>
        <header style={{ background: 'white', borderBottom: '1px solid #e0e6ed', padding: '14px 24px', display: 'flex', justifyContent: 'space-between', alignItems: 'center', boxShadow: '0 2px 4px rgba(0,0,0,0.05)' }}>
          <h1 style={{ fontSize: '20px', color: '#667eea', fontWeight: 600, margin: 0 }}>History</h1>
          <div style={{ display: 'flex', gap: '10px', alignItems: 'center' }}>
            <select
              value={statusFilter}
              onChange={e => { setStatusFilter(e.target.value); setPage(0) }}
              style={{ padding: '6px 10px', borderRadius: '4px', border: '1px solid #d0d7de', fontSize: '13px', cursor: 'pointer' }}
            >
              <option value="terminal">Completed / Failed / Cancelled</option>
              <option value="completed">Completed</option>
              <option value="failed">Failed</option>
              <option value="cancelled">Cancelled</option>
              <option value="">All statuses</option>
            </select>
            <input
              placeholder="Filter by graph…"
              value={graphFilter}
              onChange={e => { setGraphFilter(e.target.value); setPage(0) }}
              style={{ padding: '6px 10px', borderRadius: '4px', border: '1px solid #d0d7de', fontSize: '13px', width: '160px' }}
            />
            {workflows.some(w => w.status === 'failed') && (
              <button onClick={restartAllFailed} style={{ background: '#e67e22', color: 'white', padding: '6px 14px', borderRadius: '4px', fontSize: '13px', fontWeight: 500, border: 'none', cursor: 'pointer' }}>
                Restart All Failed ({workflows.filter(w => w.status === 'failed').length})
              </button>
            )}
            <button onClick={() => fetchWorkflows(page, statusFilter, graphFilter)} style={{ background: '#667eea', color: 'white', padding: '6px 14px', borderRadius: '4px', fontSize: '13px', fontWeight: 500, border: 'none', cursor: 'pointer' }}>Refresh</button>
          </div>
        </header>

        <div style={{ padding: '20px 24px' }}>
          <div style={{ background: 'white', borderRadius: '8px', boxShadow: '0 2px 8px rgba(0,0,0,0.06)', overflow: 'hidden' }}>
            {loading ? (
              <div style={{ padding: '40px', textAlign: 'center', color: '#999' }}>Loading...</div>
            ) : workflows.length === 0 ? (
              <div style={{ padding: '40px', textAlign: 'center', color: '#999' }}>No history yet.</div>
            ) : (
              <>
                <table style={{ width: '100%', borderCollapse: 'collapse' }}>
                  <thead>
                    <tr>
                      {TABLE_HEADERS.map(h => (
                        <th key={h} style={{ padding: '12px 16px', textAlign: 'left', fontSize: '12px', fontWeight: 600, color: '#666', background: '#f8f9fa', borderBottom: '2px solid #e0e6ed', textTransform: 'uppercase', letterSpacing: '0.5px' }}>{h}</th>
                      ))}
                    </tr>
                  </thead>
                  <tbody>
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
                        <td style={{ padding: '12px 16px', borderBottom: '1px solid #e0e6ed', fontSize: '12px', color: '#888' }}>{formatDate(wf.started_at)}</td>
                        <td style={{ padding: '12px 16px', borderBottom: '1px solid #e0e6ed', fontSize: '12px', color: '#888' }}>{formatDate(wf.finished_at)}</td>
                        <td style={{ padding: '12px 16px', borderBottom: '1px solid #e0e6ed' }} onClick={e => e.stopPropagation()}>
                          <div style={{ display: 'flex', gap: '6px' }}>
                            <button onClick={e => restart(wf.id, e)} style={actionBtn('#667eea')}>Restart</button>
                            <button onClick={e => deleteWorkflow(wf.id, wf.name, e)} style={actionBtn('#c0392b')}>Delete</button>
                          </div>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
                <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', padding: '12px 16px', borderTop: '1px solid #e0e6ed', background: '#f8f9fa' }}>
                  <span style={{ fontSize: '13px', color: '#666' }}>
                    {total === 0 ? '0 results' : `${from}–${to} of ${total}`}
                  </span>
                  <div style={{ display: 'flex', gap: '6px', alignItems: 'center' }}>
                    <button
                      disabled={page === 0}
                      onClick={() => setPage(p => Math.max(0, p - 1))}
                      style={{ ...pageBtn, opacity: page === 0 ? 0.4 : 1 }}
                    >← Prev</button>
                    <span style={{ fontSize: '13px', color: '#666', padding: '0 6px' }}>
                      Page {page + 1} / {totalPages}
                    </span>
                    <button
                      disabled={page >= totalPages - 1}
                      onClick={() => setPage(p => Math.min(totalPages - 1, p + 1))}
                      style={{ ...pageBtn, opacity: page >= totalPages - 1 ? 0.4 : 1 }}
                    >Next →</button>
                  </div>
                </div>
              </>
            )}
          </div>
        </div>
      </div>
    </div>
  )
}

function actionBtn(bg: string): React.CSSProperties {
  return { background: bg, color: 'white', border: 'none', padding: '4px 10px', borderRadius: '4px', cursor: 'pointer', fontSize: '12px', fontWeight: 500 }
}

const pageBtn: React.CSSProperties = {
  background: 'white', border: '1px solid #d0d7de', color: '#444', padding: '4px 12px',
  borderRadius: '4px', cursor: 'pointer', fontSize: '13px', fontWeight: 500,
}
