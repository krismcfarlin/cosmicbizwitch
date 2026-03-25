import { useState, useEffect, useCallback } from 'react'
import { useNavigate } from 'react-router-dom'

interface Setting {
  key: string
  value: string
  label: string
  description: string
  is_secret: boolean
}

interface RowState {
  value: string
  saving: boolean
  status: 'idle' | 'saved' | 'error'
  errorMsg: string
}

export default function Settings() {
  const [settings, setSettings] = useState<Setting[]>([])
  const [rowStates, setRowStates] = useState<Record<string, RowState>>({})
  const [loading, setLoading] = useState(true)
  const [loadError, setLoadError] = useState('')
  const navigate = useNavigate()

  const load = useCallback(async () => {
    setLoading(true)
    setLoadError('')
    try {
      const res = await fetch('/app/settings')
      const text = await res.text()
      if (!res.ok) {
        setLoadError(`API error ${res.status}: ${text}`)
        setLoading(false)
        return
      }
      const data: Setting[] = JSON.parse(text)
      setSettings(data)
      const initial: Record<string, RowState> = {}
      for (const s of data) {
        initial[s.key] = { value: '', saving: false, status: 'idle', errorMsg: '' }
      }
      setRowStates(initial)
    } catch (err) {
      setLoadError(String(err))
    }
    setLoading(false)
  }, [])

  useEffect(() => { load() }, [load])

  const patchRow = (key: string, patch: Partial<RowState>) => {
    setRowStates(prev => ({ ...prev, [key]: { ...prev[key], ...patch } }))
  }

  const save = async (key: string) => {
    const state = rowStates[key]
    if (!state) return
    patchRow(key, { saving: true, status: 'idle', errorMsg: '' })
    try {
      const res = await fetch(`/app/settings/${key}`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ value: state.value }),
      })
      if (res.ok) {
        patchRow(key, { saving: false, status: 'saved' })
        setTimeout(() => patchRow(key, { status: 'idle' }), 3000)
      } else {
        const text = await res.text()
        patchRow(key, { saving: false, status: 'error', errorMsg: text || `Error ${res.status}` })
      }
    } catch (err) {
      patchRow(key, { saving: false, status: 'error', errorMsg: String(err) })
    }
  }

  return (
    <div style={{ fontFamily: '-apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif', minHeight: '100vh', background: '#f5f7fa' }}>
      <header style={{ background: 'white', borderBottom: '1px solid #e0e6ed', padding: '14px 24px', display: 'flex', justifyContent: 'space-between', alignItems: 'center', boxShadow: '0 2px 4px rgba(0,0,0,0.05)' }}>
        <h1 style={{ fontSize: '20px', color: '#6b46c1', fontWeight: 600, margin: 0 }}>Settings</h1>
        <div style={{ display: 'flex', gap: '10px' }}>
          <a href="/" style={navBtn('#667eea')}>Home</a>
          <button onClick={() => navigate('/workflows')} style={navBtn('#667eea')}>Workflows</button>
          <button onClick={() => navigate('/triggers')} style={navBtn('#667eea')}>Triggers</button>
          <a href="/logs" style={navBtn('#667eea')}>Logs</a>
          <a href="/logout" style={navBtn('#e74c3c')}>Logout</a>
        </div>
      </header>

      <div style={{ padding: '20px 24px', maxWidth: '760px' }}>
        <div style={{ background: 'white', borderRadius: '8px', boxShadow: '0 2px 8px rgba(0,0,0,0.06)', overflow: 'hidden' }}>
          {loading ? (
            <div style={{ padding: '40px', textAlign: 'center', color: '#999' }}>Loading...</div>
          ) : loadError ? (
            <div style={{ padding: '40px', textAlign: 'center', color: '#e74c3c', fontFamily: 'monospace', fontSize: '13px' }}>{loadError}</div>
          ) : settings.length === 0 ? (
            <div style={{ padding: '40px', textAlign: 'center', color: '#999' }}>No settings found.</div>
          ) : (
            <div>
              {settings.map((s, idx) => {
                const state = rowStates[s.key] ?? { value: '', saving: false, status: 'idle', errorMsg: '' }
                return (
                  <div
                    key={s.key}
                    style={{
                      padding: '20px 24px',
                      borderBottom: idx < settings.length - 1 ? '1px solid #e0e6ed' : 'none',
                      display: 'flex',
                      alignItems: 'flex-start',
                      gap: '24px',
                    }}
                  >
                    <div style={{ flex: '0 0 220px' }}>
                      <div style={{ fontWeight: 600, fontSize: '14px', color: '#2c3e50', marginBottom: '4px' }}>{s.label}</div>
                      <div style={{ fontSize: '12px', color: '#888', lineHeight: '1.5' }}>{s.description}</div>
                    </div>
                    <div style={{ flex: 1, display: 'flex', flexDirection: 'column', gap: '8px' }}>
                      <div style={{ display: 'flex', gap: '8px', alignItems: 'center' }}>
                        <input
                          type={s.is_secret ? 'password' : 'text'}
                          value={state.value}
                          placeholder={s.is_secret ? '••••••••' : s.key}
                          onChange={e => patchRow(s.key, { value: e.target.value })}
                          style={inputStyle}
                        />
                        <button
                          onClick={() => save(s.key)}
                          disabled={state.saving}
                          style={saveBtn(state.saving)}
                        >
                          {state.saving ? 'Saving...' : 'Save'}
                        </button>
                      </div>
                      {state.status === 'saved' && (
                        <span style={{ fontSize: '12px', color: '#27ae60', fontWeight: 500 }}>Saved</span>
                      )}
                      {state.status === 'error' && (
                        <span style={{ fontSize: '12px', color: '#e74c3c', fontWeight: 500 }}>{state.errorMsg || 'Error saving'}</span>
                      )}
                    </div>
                  </div>
                )
              })}
            </div>
          )}
        </div>
      </div>
    </div>
  )
}

const inputStyle: React.CSSProperties = {
  flex: 1,
  padding: '8px 10px',
  border: '1px solid #e0e6ed',
  borderRadius: '4px',
  fontSize: '13px',
  boxSizing: 'border-box',
  fontFamily: 'inherit',
}

function saveBtn(disabled: boolean): React.CSSProperties {
  return {
    background: disabled ? '#a78bca' : '#6b46c1',
    color: 'white',
    border: 'none',
    borderRadius: '4px',
    padding: '8px 16px',
    cursor: disabled ? 'default' : 'pointer',
    fontSize: '13px',
    fontWeight: 500,
    whiteSpace: 'nowrap',
    opacity: disabled ? 0.7 : 1,
  }
}

function navBtn(bg: string): React.CSSProperties {
  return {
    background: bg,
    color: 'white',
    padding: '6px 14px',
    borderRadius: '4px',
    textDecoration: 'none',
    fontSize: '13px',
    fontWeight: 500,
    border: 'none',
    cursor: 'pointer',
    display: 'inline-block',
  }
}
