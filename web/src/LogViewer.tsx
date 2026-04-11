import { useState, useEffect, useRef, useCallback } from 'react'
import Nav from './Nav'

interface LogLine {
  Time: string
  Text: string
}

type Level = 'error' | 'warn' | 'success' | 'info' | 'default'

function lineLevel(text: string): Level {
  const lower = text.toLowerCase()
  if (lower.includes('error') || lower.includes('failed') || lower.includes('fatal')) return 'error'
  if (lower.includes('warn')) return 'warn'
  if (lower.includes('success') || lower.includes('complete')) return 'success'
  if (lower.includes('start')) return 'info'
  return 'default'
}

const COLORS: Record<Level, string> = {
  error:   '#f48771',
  warn:    '#dcdcaa',
  success: '#4ec9b0',
  info:    '#9cdcfe',
  default: '#d4d4d4',
}

const MAX_LINES = 2000

export default function LogViewer() {
  const [lines, setLines] = useState<LogLine[]>([])
  const [filter, setFilter] = useState('')
  const [levelFilter, setLevelFilter] = useState<Level | 'all'>('all')
  const [connected, setConnected] = useState(false)
  const [status, setStatus] = useState('Connecting...')
  const esRef = useRef<EventSource | null>(null)
  const reconnectTimer = useRef<ReturnType<typeof setTimeout> | null>(null)

  const connect = useCallback(() => {
    if (esRef.current) {
      esRef.current.close()
      esRef.current = null
    }

    const es = new EventSource('/logs/stream')
    esRef.current = es

    es.onopen = () => {
      setConnected(true)
      setStatus('Live')
    }

    es.onmessage = (e: MessageEvent) => {
      const line: LogLine = JSON.parse(e.data)
      setLines(prev => {
        const next = [line, ...prev]
        return next.length > MAX_LINES ? next.slice(0, MAX_LINES) : next
      })
    }

    es.onerror = () => {
      setConnected(false)
      setStatus('Reconnecting in 3s...')
      es.close()
      esRef.current = null
      reconnectTimer.current = setTimeout(connect, 3000)
    }
  }, [])

  useEffect(() => {
    connect()
    return () => {
      esRef.current?.close()
      if (reconnectTimer.current) clearTimeout(reconnectTimer.current)
    }
  }, [connect])

  const filtered = lines.filter(l => {
    if (levelFilter !== 'all' && lineLevel(l.Text) !== levelFilter) return false
    if (filter && !l.Text.toLowerCase().includes(filter.toLowerCase())) return false
    return true
  })

  return (
    <div style={{ display: 'flex', height: '100vh', fontFamily: '-apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif' }}>
      <Nav />
      <div style={{ flex: 1, overflow: 'hidden', background: '#f5f7fa', display: 'flex', flexDirection: 'column' }}>

      {/* Header */}
      <header style={{ background: 'white', borderBottom: '1px solid #e0e6ed', padding: '14px 24px', display: 'flex', justifyContent: 'space-between', alignItems: 'center', boxShadow: '0 2px 4px rgba(0,0,0,0.05)', flexShrink: 0 }}>
        <h1 style={{ fontSize: '20px', color: '#667eea', fontWeight: 600 }}>Server Logs</h1>
        <div style={{ display: 'flex', gap: '12px', alignItems: 'center' }}>
          <span style={{ fontSize: '12px', fontWeight: 600, color: connected ? '#4ec9b0' : '#f48771' }}>
            {connected ? '● ' : '○ '}{status}
          </span>
        </div>
      </header>

      <div style={{ padding: '16px 24px', display: 'flex', flexDirection: 'column', gap: '12px', flex: 1, overflow: 'hidden' }}>

        {/* Toolbar */}
        <div style={{ background: 'white', borderRadius: '8px', padding: '12px 16px', boxShadow: '0 2px 8px rgba(0,0,0,0.06)', display: 'flex', gap: '10px', alignItems: 'center', flexWrap: 'wrap', flexShrink: 0 }}>
          <input
            type="text"
            placeholder="Filter logs..."
            value={filter}
            onChange={e => setFilter(e.target.value)}
            style={{ flex: 1, minWidth: '180px', padding: '7px 11px', border: '1px solid #e0e6ed', borderRadius: '4px', fontSize: '13px', outline: 'none' }}
          />
          <select
            value={levelFilter}
            onChange={e => setLevelFilter(e.target.value as Level | 'all')}
            style={{ padding: '7px 11px', border: '1px solid #e0e6ed', borderRadius: '4px', fontSize: '13px', background: 'white', cursor: 'pointer' }}
          >
            <option value="all">All levels</option>
            <option value="error">Errors</option>
            <option value="warn">Warnings</option>
            <option value="success">Success</option>
            <option value="info">Info</option>
          </select>
          <span style={{ fontSize: '12px', color: '#999', whiteSpace: 'nowrap' }}>
            {filtered.length.toLocaleString()} / {lines.length.toLocaleString()} lines
          </span>
          <button
            onClick={() => setLines([])}
            style={{ padding: '7px 14px', background: '#6c757d', color: 'white', border: 'none', borderRadius: '4px', cursor: 'pointer', fontSize: '13px', fontWeight: 500 }}
          >
            Clear
          </button>
        </div>

        {/* Log output */}
        <div style={{ fontFamily: '"SF Mono", "Fira Code", "Cascadia Code", Consolas, monospace', fontSize: '12px', background: '#1e1e1e', color: '#d4d4d4', padding: '16px', borderRadius: '8px', overflowY: 'auto', flex: 1, lineHeight: 1.6 }}>
          {filtered.length === 0 ? (
            <div style={{ color: '#555', textAlign: 'center', padding: '60px 0' }}>
              {lines.length === 0 ? 'Waiting for log entries...' : 'No lines match the current filter.'}
            </div>
          ) : (
            filtered.map((line, i) => {
              const level = lineLevel(line.Text)
              return (
                <div key={i} style={{ display: 'flex', gap: '12px', padding: '1px 0' }}>
                  <span style={{ color: '#555', userSelect: 'none', flexShrink: 0, minWidth: '152px' }}>{line.Time}</span>
                  <span style={{ color: COLORS[level], wordBreak: 'break-all' }}>{line.Text}</span>
                </div>
              )
            })
          )}
        </div>
      </div>
      </div>
    </div>
  )
}
