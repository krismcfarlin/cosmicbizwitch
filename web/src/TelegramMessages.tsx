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

export default function TelegramMessages() {
  const [messages, setMessages] = useState<TelegramMessage[]>([])
  const [loading, setLoading] = useState(true)
  const [autoRefresh, setAutoRefresh] = useState(true)
  const [clearing, setClearing] = useState(false)
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

  useEffect(() => {
    load()
  }, [load])

  useEffect(() => {
    if (autoRefresh) {
      intervalRef.current = setInterval(load, 5000)
    } else {
      if (intervalRef.current) clearInterval(intervalRef.current)
    }
    return () => {
      if (intervalRef.current) clearInterval(intervalRef.current)
    }
  }, [autoRefresh, load])

  const clearAll = async () => {
    if (!confirm('Clear all Telegram messages?')) return
    setClearing(true)
    await fetch('/api/telegram/messages', { method: 'DELETE' })
    await load()
    setClearing(false)
  }

  return (
    <div style={{ display: 'flex', height: '100vh', fontFamily: '-apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif' }}>
      <Nav />
      <div style={{ flex: 1, overflow: 'auto', background: '#f5f7fa' }}>
      <header style={{ background: 'white', borderBottom: '1px solid #e0e6ed', padding: '14px 24px', display: 'flex', justifyContent: 'space-between', alignItems: 'center', boxShadow: '0 2px 4px rgba(0,0,0,0.05)' }}>
        <h1 style={{ fontSize: '20px', color: '#667eea', fontWeight: 600, margin: 0 }}>Telegram Messages</h1>
        <div style={{ display: 'flex', gap: '10px', alignItems: 'center' }}>
          <label style={{ display: 'flex', alignItems: 'center', gap: '6px', fontSize: '13px', color: '#555', cursor: 'pointer', userSelect: 'none' }}>
            <input
              type="checkbox"
              checked={autoRefresh}
              onChange={e => setAutoRefresh(e.target.checked)}
              style={{ cursor: 'pointer' }}
            />
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
                  {['#', 'Received', 'JSON'].map(h => (
                    <th key={h} style={{ padding: '12px 16px', textAlign: 'left', fontSize: '12px', fontWeight: 600, color: '#666', background: '#f8f9fa', borderBottom: '2px solid #e0e6ed', textTransform: 'uppercase', letterSpacing: '0.5px', whiteSpace: 'nowrap' }}>{h}</th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {messages.map(msg => (
                  <tr key={msg.id} style={{ background: 'white', verticalAlign: 'top' }}>
                    <td style={{ padding: '12px 16px', borderBottom: '1px solid #e0e6ed', fontSize: '13px', color: '#888', whiteSpace: 'nowrap', fontFamily: 'monospace' }}>{msg.id}</td>
                    <td style={{ padding: '12px 16px', borderBottom: '1px solid #e0e6ed', fontSize: '13px', color: '#555', whiteSpace: 'nowrap' }}>{formatDate(msg.received_at)}</td>
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
                ))}
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
