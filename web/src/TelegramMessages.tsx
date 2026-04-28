import { useState, useEffect, useCallback, useRef } from 'react'
import { useNavigate } from 'react-router-dom'
import Nav from './Nav'

interface TelegramMessage {
  id: string
  received_at: string
  raw_json: string
}

function formatDate(s: string): string {
  const d = new Date(s)
  return d.toLocaleString(undefined, {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  })
}

function prettyJson(raw: string): string {
  try {
    return JSON.stringify(JSON.parse(raw), null, 2)
  } catch {
    return raw
  }
}

const LAUREN_ID = 6326470139

function extractSender(rawJson: string): { firstName: string; lastName: string; text: string; fromId: number } | null {
  try {
    const p = JSON.parse(rawJson)
    const event = p.message || p.edited_message || p.channel_post
    if (!event) return null

    const msgText = (event.text || event.caption || '').trim()

    type Candidate = { firstName: string; lastName: string; fromId: number }
    const candidates: Candidate[] = []

    // 1. Real reply-to sender (not a forum topic anchor)
    const reply = event.reply_to_message
    if (reply?.from?.first_name && !reply.forum_topic_created) {
      candidates.push({
        firstName: (reply.from.first_name || '').trim(),
        lastName: (reply.from.last_name || '').trim(),
        fromId: reply.from.id || 0,
      })
    }

    // 2. Forwarded-from sender
    const fwd = event.forward_from || event.forward_origin?.sender_user
    if (fwd?.first_name) {
      candidates.push({
        firstName: (fwd.first_name || '').trim(),
        lastName: (fwd.last_name || '').trim(),
        fromId: fwd.id || 0,
      })
    }

    // 3. Direct sender
    const from = event.from || {}
    if (from.first_name) {
      candidates.push({
        firstName: (from.first_name || '').trim(),
        lastName: (from.last_name || '').trim(),
        fromId: from.id || 0,
      })
    }

    // Pick first non-Lauren candidate, fall back to Lauren only if no one else
    const pick = candidates.find(c => c.fromId !== LAUREN_ID) ?? candidates[0]
    if (!pick?.firstName) return null
    return { ...pick, text: msgText }
  } catch {
    return null
  }
}

type WorkflowState = { status: 'idle' } | { status: 'running' } | { status: 'done'; result: unknown } | { status: 'error'; message: string }

export default function TelegramMessages() {
  const [messages, setMessages] = useState<TelegramMessage[]>([])
  const [loading, setLoading] = useState(true)
  const [autoRefresh, setAutoRefresh] = useState(true)
  const [clearing, setClearing] = useState(false)
  const [wfStates, setWfStates] = useState<Record<string, WorkflowState>>({})
  const [copied, setCopied] = useState<Record<string, boolean>>({})
  const intervalRef = useRef<ReturnType<typeof setInterval> | null>(null)
  const navigate = useNavigate()

  const load = useCallback(async () => {
    const res = await fetch('/api/telegram/messages')
    if (res.ok) {
      const data = await res.json()
      setMessages(data ?? [])
    }
    setLoading(false)
  }, [])

  useEffect(() => { load() }, [load])

  useEffect(() => {
    if (autoRefresh) {
      intervalRef.current = setInterval(load, 5000)
    } else {
      if (intervalRef.current) clearInterval(intervalRef.current)
    }
    return () => { if (intervalRef.current) clearInterval(intervalRef.current) }
  }, [autoRefresh, load])

  const clearAll = async () => {
    if (!confirm('Clear all Telegram messages?')) return
    setClearing(true)
    await fetch('/api/telegram/messages', { method: 'DELETE' })
    await load()
    setClearing(false)
  }

  const runWorkflow = async (msgId: string, rawJson: string) => {
    const sender = extractSender(rawJson)
    if (!sender) return
    setWfStates(s => ({ ...s, [msgId]: { status: 'running' } }))
    try {
      const res = await fetch('/api/workflows/run-sync', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          graph_name: 'MCP_challenge_context',
          context: {
            first_name: sender.firstName,
            last_name: sender.lastName,
            from_id: sender.fromId,
            query: sender.text.slice(0, 500),
          },
        }),
      })
      const data = await res.json()
      if (!res.ok || data.status === 'failed') {
        setWfStates(s => ({ ...s, [msgId]: { status: 'error', message: data.error || 'workflow failed' } }))
      } else {
        setWfStates(s => ({ ...s, [msgId]: { status: 'done', result: data.context } }))
      }
    } catch (e: unknown) {
      setWfStates(s => ({ ...s, [msgId]: { status: 'error', message: String(e) } }))
    }
  }

  const copyResult = async (msgId: string) => {
    const state = wfStates[msgId]
    if (!state || state.status !== 'done') return
    const text = JSON.stringify(state.result, null, 2)
    await navigator.clipboard.writeText(text)
    setCopied(c => ({ ...c, [msgId]: true }))
    setTimeout(() => setCopied(c => ({ ...c, [msgId]: false })), 2000)
  }

  return (
    <div style={{ display: 'flex', height: '100vh', fontFamily: '-apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif' }}>
      <Nav />
      <div style={{ flex: 1, overflow: 'auto', background: '#f5f7fa' }}>
        <header style={{ background: 'white', borderBottom: '1px solid #e0e6ed', padding: '14px 24px', display: 'flex', justifyContent: 'space-between', alignItems: 'center', boxShadow: '0 2px 4px rgba(0,0,0,0.05)' }}>
          <h1 style={{ fontSize: '20px', color: '#667eea', fontWeight: 600, margin: 0 }}>Telegram Messages</h1>
          <div style={{ display: 'flex', gap: '10px', alignItems: 'center' }}>
            <label style={{ display: 'flex', alignItems: 'center', gap: '6px', fontSize: '13px', color: '#555', cursor: 'pointer', userSelect: 'none' }}>
              <input type="checkbox" checked={autoRefresh} onChange={e => setAutoRefresh(e.target.checked)} style={{ cursor: 'pointer' }} />
              Auto-refresh
            </label>
            <button onClick={load} style={navBtn('#667eea')}>Refresh</button>
            <button onClick={clearAll} disabled={clearing} style={{ ...navBtn('#e74c3c'), opacity: clearing ? 0.7 : 1, cursor: clearing ? 'default' : 'pointer' }}>
              {clearing ? 'Clearing...' : 'Clear All'}
            </button>
          </div>
        </header>

        <div style={{ padding: '20px 24px' }}>
          <div style={{ background: 'white', borderRadius: '8px', boxShadow: '0 2px 8px rgba(0,0,0,0.06)', overflow: 'hidden' }}>
            {loading ? (
              <div style={{ padding: '40px', textAlign: 'center', color: '#999' }}>Loading...</div>
            ) : messages.length === 0 ? (
              <div style={{ padding: '60px 40px', textAlign: 'center', color: '#999' }}>
                <div style={{ fontSize: '16px', marginBottom: '8px' }}>No messages received yet.</div>
                <div style={{ fontSize: '13px' }}>Send a message to your Telegram bot.</div>
              </div>
            ) : (
              <table style={{ width: '100%', borderCollapse: 'collapse' }}>
                <thead>
                  <tr>
                    {['#', 'Received', 'Actions', 'JSON'].map(h => (
                      <th key={h} style={{ padding: '12px 16px', textAlign: 'left', fontSize: '12px', fontWeight: 600, color: '#666', background: '#f8f9fa', borderBottom: '2px solid #e0e6ed', textTransform: 'uppercase', letterSpacing: '0.5px', whiteSpace: 'nowrap' }}>{h}</th>
                    ))}
                  </tr>
                </thead>
                <tbody>
                  {messages.map(msg => {
                    const sender = extractSender(msg.raw_json)
                    const wfState = wfStates[msg.id] ?? { status: 'idle' }
                    const isCopied = copied[msg.id] ?? false
                    return (
                      <tr key={msg.id} style={{ background: 'white', verticalAlign: 'top' }}>
                        <td style={{ padding: '12px 16px', borderBottom: '1px solid #e0e6ed', fontSize: '13px', color: '#888', whiteSpace: 'nowrap', fontFamily: 'monospace' }}>{msg.id}</td>
                        <td style={{ padding: '12px 16px', borderBottom: '1px solid #e0e6ed', fontSize: '13px', color: '#555', whiteSpace: 'nowrap' }}>{formatDate(msg.received_at)}</td>
                        <td style={{ padding: '12px 16px', borderBottom: '1px solid #e0e6ed', whiteSpace: 'nowrap', verticalAlign: 'middle' }}>
                          {sender ? (
                            <div style={{ display: 'flex', flexDirection: 'column', gap: '6px' }}>
                              <div style={{ fontSize: '12px', color: '#888', marginBottom: '2px' }}>
                                {sender.firstName} {sender.lastName}
                              </div>
                              <button
                                onClick={() => runWorkflow(msg.id, msg.raw_json)}
                                disabled={wfState.status === 'running'}
                                style={{
                                  ...actionBtn('#667eea'),
                                  opacity: wfState.status === 'running' ? 0.7 : 1,
                                  cursor: wfState.status === 'running' ? 'default' : 'pointer',
                                }}
                              >
                                {wfState.status === 'running' ? '⏳ Running…' : '🔍 Lookup'}
                              </button>
                              <button
                                onClick={() => copyResult(msg.id)}
                                disabled={wfState.status !== 'done'}
                                style={{
                                  ...actionBtn(isCopied ? '#27ae60' : '#555'),
                                  opacity: wfState.status === 'done' ? 1 : 0.4,
                                  cursor: wfState.status === 'done' ? 'pointer' : 'default',
                                }}
                              >
                                {isCopied ? '✅ Copied!' : '📋 Copy Result'}
                              </button>
                              {wfState.status === 'error' && (
                                <div style={{ fontSize: '11px', color: '#e74c3c', maxWidth: '180px', wordBreak: 'break-word' }}>
                                  {wfState.message}
                                </div>
                              )}
                              {wfState.status === 'done' && (
                                <div style={{ fontSize: '11px', color: '#27ae60' }}>Done ✓</div>
                              )}
                            </div>
                          ) : (
                            <span style={{ fontSize: '12px', color: '#bbb' }}>—</span>
                          )}
                        </td>
                        <td style={{ padding: '12px 16px', borderBottom: '1px solid #e0e6ed', width: '100%' }}>
                          <pre style={{
                            margin: 0,
                            padding: '12px 14px',
                            background: '#1e1e1e',
                            color: '#d4d4d4',
                            fontFamily: '"SF Mono", "Fira Code", "Cascadia Code", Consolas, monospace',
                            fontSize: '12px',
                            borderRadius: '6px',
                            overflowX: 'auto',
                            lineHeight: 1.6,
                            whiteSpace: 'pre-wrap',
                            wordBreak: 'break-all',
                            maxHeight: '320px',
                            overflowY: 'auto',
                          }}>
                            {prettyJson(msg.raw_json)}
                          </pre>
                        </td>
                      </tr>
                    )
                  })}
                </tbody>
              </table>
            )}
          </div>
        </div>
      </div>
    </div>
  )
}

function navBtn(bg: string): React.CSSProperties {
  return { background: bg, color: 'white', padding: '6px 14px', borderRadius: '4px', textDecoration: 'none', fontSize: '13px', fontWeight: 500, border: 'none', cursor: 'pointer' }
}

function actionBtn(bg: string): React.CSSProperties {
  return { background: bg, color: 'white', padding: '5px 10px', borderRadius: '4px', fontSize: '12px', fontWeight: 500, border: 'none', width: '120px', textAlign: 'center' }
}
