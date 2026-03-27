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
  const [googleConnected, setGoogleConnected] = useState<boolean | null>(null)
  const [googleConnecting, setGoogleConnecting] = useState(false)
  const [googleRefreshToken, setGoogleRefreshToken] = useState('')
  const [testTemplateDocId, setTestTemplateDocId] = useState('')
  const [testDestFolderId, setTestDestFolderId] = useState('')
  const [copiedText, setCopiedText] = useState('')
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

  // Check Google Drive connection status on mount.
  useEffect(() => {
    fetch('/api/google/auth/status')
      .then(r => r.json())
      .then(d => setGoogleConnected(d.connected))
      .catch(() => setGoogleConnected(false))
  }, [])

  // Fetch refresh token when Google is connected
  useEffect(() => {
    if (googleConnected) {
      fetch('/app/settings')
        .then(r => r.json())
        .then((data: Setting[]) => {
          const tokenSetting = data.find(s => s.key === 'GOOGLE_REFRESH_TOKEN')
          if (tokenSetting && tokenSetting.value) {
            setGoogleRefreshToken(tokenSetting.value)
          }
        })
        .catch(() => {})
    }
  }, [googleConnected])

  const copyToClipboard = (text: string, label: string) => {
    navigator.clipboard.writeText(text)
    setCopiedText(label)
    setTimeout(() => setCopiedText(''), 2000)
  }

  const connectGoogle = () => {
    setGoogleConnecting(true)
    window.open('/api/google/auth/start', '_blank', 'width=520,height=640')
    // Poll until the OAuth callback saves the refresh token.
    const interval = setInterval(async () => {
      try {
        const r = await fetch('/api/google/auth/status')
        const d = await r.json()
        if (d.connected) {
          setGoogleConnected(true)
          setGoogleConnecting(false)
          clearInterval(interval)
        }
      } catch { /* ignore */ }
    }, 2000)
    // Stop polling after 3 minutes.
    setTimeout(() => { clearInterval(interval); setGoogleConnecting(false) }, 180000)
  }

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

      <div style={{ padding: '20px 24px', maxWidth: '760px', display: 'flex', flexDirection: 'column', gap: '16px' }}>
        {/* API credentials */}
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

        {/* Google Drive connection */}
        <div style={{ background: 'white', borderRadius: '8px', boxShadow: '0 2px 8px rgba(0,0,0,0.06)', padding: '20px 24px' }}>
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start' }}>
            <div>
              <div style={{ fontWeight: 600, fontSize: '14px', color: '#2c3e50', marginBottom: '4px' }}>Google Drive</div>
              <div style={{ fontSize: '12px', color: '#888', lineHeight: '1.5' }}>
                OAuth2 authorization for Google Workspace workflow nodes.
              </div>
            </div>
            <div style={{ display: 'flex', alignItems: 'center', gap: '14px', flexShrink: 0, marginLeft: '24px' }}>
              {googleConnected === null ? (
                <span style={{ fontSize: '12px', color: '#aaa' }}>Checking...</span>
              ) : googleConnected ? (
                <span style={{ fontSize: '12px', color: '#27ae60', fontWeight: 600 }}>● Connected</span>
              ) : (
                <span style={{ fontSize: '12px', color: '#e74c3c', fontWeight: 500 }}>○ Not connected</span>
              )}
              <button
                onClick={connectGoogle}
                disabled={googleConnecting}
                style={saveBtn(googleConnecting)}
              >
                {googleConnecting ? 'Waiting for auth...' : googleConnected ? 'Reconnect' : 'Connect Google Drive'}
              </button>
            </div>
          </div>
        </div>

        {/* Integration Test Setup */}
        <div style={{ background: '#1e293b', borderRadius: '8px', boxShadow: '0 2px 8px rgba(0,0,0,0.06)', overflow: 'hidden' }}>
          <div style={{ background: '#0f172a', padding: '16px 24px', borderBottom: '1px solid #334155' }}>
            <div style={{ fontWeight: 700, fontSize: '16px', color: '#fff' }}>🧪 Integration Test Setup</div>
            <div style={{ fontSize: '12px', color: '#cbd5e1', marginTop: '4px' }}>Get credentials to run gdrive_fill_template test</div>
          </div>

          <div style={{ padding: '24px' }}>
            {/* Token */}
            <div style={{ marginBottom: '24px' }}>
              <div style={{ fontWeight: 600, fontSize: '13px', color: '#e2e8f0', marginBottom: '8px' }}>1. GOOGLE_REFRESH_TOKEN</div>
              <div style={{ display: 'flex', gap: '8px' }}>
                <input
                  type="password"
                  value={googleRefreshToken}
                  readOnly
                  placeholder="Token will appear here after connecting Google Drive..."
                  style={{ flex: 1, padding: '10px', background: '#0f172a', border: '1px solid #334155', borderRadius: '4px', color: '#e2e8f0', fontFamily: 'monospace', fontSize: '12px' }}
                />
                <button
                  onClick={() => {
                    navigator.clipboard.writeText(googleRefreshToken)
                    setCopiedText('TOKEN')
                    setTimeout(() => setCopiedText(''), 2000)
                  }}
                  style={{ padding: '10px 16px', background: '#0ea5e9', color: 'white', border: 'none', borderRadius: '4px', cursor: 'pointer', fontWeight: 600, fontSize: '12px', whiteSpace: 'nowrap' }}
                >
                  {copiedText === 'TOKEN' ? '✓ Copied' : 'Copy'}
                </button>
              </div>
            </div>

            {/* Doc ID */}
            <div style={{ marginBottom: '24px' }}>
              <div style={{ fontWeight: 600, fontSize: '13px', color: '#e2e8f0', marginBottom: '8px' }}>2. TEST_TEMPLATE_DOC_ID</div>
              <div style={{ display: 'flex', gap: '8px' }}>
                <input
                  type="text"
                  value={testTemplateDocId}
                  onChange={e => setTestTemplateDocId(e.target.value)}
                  placeholder="Paste your template Google Doc ID here"
                  style={{ flex: 1, padding: '10px', background: '#0f172a', border: '1px solid #334155', borderRadius: '4px', color: '#e2e8f0', fontFamily: 'monospace', fontSize: '12px' }}
                />
                <button
                  onClick={() => {
                    navigator.clipboard.writeText(testTemplateDocId)
                    setCopiedText('DOC')
                    setTimeout(() => setCopiedText(''), 2000)
                  }}
                  disabled={!testTemplateDocId}
                  style={{ padding: '10px 16px', background: testTemplateDocId ? '#0ea5e9' : '#64748b', color: 'white', border: 'none', borderRadius: '4px', cursor: testTemplateDocId ? 'pointer' : 'default', fontWeight: 600, fontSize: '12px', whiteSpace: 'nowrap' }}
                >
                  {copiedText === 'DOC' ? '✓ Copied' : 'Copy'}
                </button>
              </div>
              <div style={{ fontSize: '11px', color: '#94a3b8', marginTop: '6px' }}>Get from Workflows: Browse gdrive_fill_template template_id → check server logs for [TEST_TEMPLATE_DOC_ID]</div>
            </div>

            {/* Folder ID */}
            <div style={{ marginBottom: '24px' }}>
              <div style={{ fontWeight: 600, fontSize: '13px', color: '#e2e8f0', marginBottom: '8px' }}>3. TEST_DEST_FOLDER_ID</div>
              <div style={{ display: 'flex', gap: '8px' }}>
                <input
                  type="text"
                  value={testDestFolderId}
                  onChange={e => setTestDestFolderId(e.target.value)}
                  placeholder="Paste your destination folder ID here"
                  style={{ flex: 1, padding: '10px', background: '#0f172a', border: '1px solid #334155', borderRadius: '4px', color: '#e2e8f0', fontFamily: 'monospace', fontSize: '12px' }}
                />
                <button
                  onClick={() => {
                    navigator.clipboard.writeText(testDestFolderId)
                    setCopiedText('FOLDER')
                    setTimeout(() => setCopiedText(''), 2000)
                  }}
                  disabled={!testDestFolderId}
                  style={{ padding: '10px 16px', background: testDestFolderId ? '#0ea5e9' : '#64748b', color: 'white', border: 'none', borderRadius: '4px', cursor: testDestFolderId ? 'pointer' : 'default', fontWeight: 600, fontSize: '12px', whiteSpace: 'nowrap' }}
                >
                  {copiedText === 'FOLDER' ? '✓ Copied' : 'Copy'}
                </button>
              </div>
              <div style={{ fontSize: '11px', color: '#94a3b8', marginTop: '6px' }}>Get from Workflows: Browse gdrive_fill_template destination_folder_id → check server logs for [TEST_DEST_FOLDER_ID]</div>
            </div>

            {/* Test Command */}
            <div style={{ background: '#0f172a', border: '1px solid #334155', borderRadius: '4px', padding: '12px', marginTop: '24px' }}>
              <div style={{ fontWeight: 600, fontSize: '12px', color: '#e2e8f0', marginBottom: '8px' }}>Run Test Command:</div>
              <div style={{ fontFamily: 'monospace', fontSize: '11px', color: '#cbd5e1', lineHeight: '1.6', background: '#000000', padding: '12px', borderRadius: '3px', overflow: 'auto' }}>
                {googleRefreshToken && testTemplateDocId && testDestFolderId ? (
                  <>
                    export GOOGLE_REFRESH_TOKEN="{googleRefreshToken}"<br/>
                    export TEST_TEMPLATE_DOC_ID="{testTemplateDocId}"<br/>
                    export TEST_DEST_FOLDER_ID="{testDestFolderId}"<br/>
                    go test -v ./internal/app/workflows -run TestGdriveFillTemplateWorkflow
                  </>
                ) : (
                  'Fill in all three values above to see the command'
                )}
              </div>
              {googleRefreshToken && testTemplateDocId && testDestFolderId && (
                <button
                  onClick={() => {
                    const cmd = `export GOOGLE_REFRESH_TOKEN="${googleRefreshToken}"\nexport TEST_TEMPLATE_DOC_ID="${testTemplateDocId}"\nexport TEST_DEST_FOLDER_ID="${testDestFolderId}"\ngo test -v ./internal/app/workflows -run TestGdriveFillTemplateWorkflow`
                    navigator.clipboard.writeText(cmd)
                    setCopiedText('CMD')
                    setTimeout(() => setCopiedText(''), 2000)
                  }}
                  style={{ marginTop: '12px', padding: '10px 16px', background: '#10b981', color: 'white', border: 'none', borderRadius: '4px', cursor: 'pointer', fontWeight: 600, fontSize: '12px', width: '100%' }}
                >
                  {copiedText === 'CMD' ? '✓ Copied Full Command' : 'Copy Full Command'}
                </button>
              )}
            </div>
          </div>
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
