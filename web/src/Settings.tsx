import { useState, useEffect, useCallback } from 'react'
import { useNavigate } from 'react-router-dom'

interface Setting {
  key: string
  value: string
  label: string
  description: string
  is_secret: boolean
  is_set: boolean
}

interface RowState {
  value: string
  saving: boolean
  status: 'idle' | 'saved' | 'error'
  errorMsg: string
}

interface OpenRouterKey {
  name: string
  key: string
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
  const [slackConnected, setSlackConnected] = useState<boolean | null>(null)
  const [slackWorkspace, setSlackWorkspace] = useState('')
  const [slackConnecting, setSlackConnecting] = useState(false)
  const [telegramBotTokenSet, setTelegramBotTokenSet] = useState<boolean | null>(null)
  const [telegramConnected, setTelegramConnected] = useState<boolean | null>(null)
  const [telegramWebhookRegistering, setTelegramWebhookRegistering] = useState(false)
  const [telegramWebhookResult, setTelegramWebhookResult] = useState<'idle' | 'success' | 'error'>('idle')
  const [openRouterKeys, setOpenRouterKeys] = useState<OpenRouterKey[]>([])
  const [openRouterSaving, setOpenRouterSaving] = useState(false)
  const [openRouterStatus, setOpenRouterStatus] = useState<'idle' | 'saved' | 'error'>('idle')
  const [openRouterErrorMsg, setOpenRouterErrorMsg] = useState('')
  const [openRouterValidationMsg, setOpenRouterValidationMsg] = useState('')
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

  // Check Slack connection status on mount.
  useEffect(() => {
    fetch('/api/slack/auth/status')
      .then(r => r.json())
      .then(d => {
        setSlackConnected(d.connected)
        if (d.workspace) setSlackWorkspace(d.workspace)
      })
      .catch(() => setSlackConnected(false))
  }, [])

  // Check Telegram connection status on mount.
  useEffect(() => {
    fetch('/api/telegram/status')
      .then(r => r.json())
      .then(d => {
        setTelegramConnected(d.connected)
        setTelegramBotTokenSet(d.bot_token_set)
      })
      .catch(() => {
        setTelegramConnected(false)
        setTelegramBotTokenSet(false)
      })
  }, [])

  // Load OpenRouter keys from settings on mount.
  useEffect(() => {
    fetch('/app/settings')
      .then(r => r.json())
      .then((data: Setting[]) => {
        const entry = data.find(s => s.key === 'OPENROUTER_KEYS')
        if (entry && entry.value) {
          try {
            const parsed = JSON.parse(entry.value)
            if (Array.isArray(parsed)) {
              setOpenRouterKeys(parsed)
            }
          } catch { /* ignore malformed */ }
        }
      })
      .catch(() => {})
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

  const connectSlack = () => {
    setSlackConnecting(true)
    window.open('/api/slack/auth/start', '_blank', 'width=520,height=640')
    // Poll until the OAuth callback saves the token.
    const interval = setInterval(async () => {
      try {
        const r = await fetch('/api/slack/auth/status')
        const d = await r.json()
        if (d.connected) {
          setSlackConnected(true)
          if (d.workspace) setSlackWorkspace(d.workspace)
          setSlackConnecting(false)
          clearInterval(interval)
        }
      } catch { /* ignore */ }
    }, 2000)
    // Stop polling after 60 seconds.
    setTimeout(() => { clearInterval(interval); setSlackConnecting(false) }, 60000)
  }

  const registerTelegramWebhook = async () => {
    setTelegramWebhookRegistering(true)
    setTelegramWebhookResult('idle')
    try {
      const res = await fetch('/api/telegram/set-webhook', { method: 'POST' })
      if (res.ok) {
        setTelegramWebhookResult('success')
        setTelegramConnected(true)
        setTelegramBotTokenSet(true)
      } else {
        setTelegramWebhookResult('error')
      }
    } catch {
      setTelegramWebhookResult('error')
    }
    setTelegramWebhookRegistering(false)
  }

  const saveOpenRouterKeys = async () => {
    setOpenRouterValidationMsg('')
    // Check for partial rows (name without key, or key without name).
    const partial = openRouterKeys.filter(k => (k.name && !k.key) || (!k.name && k.key))
    if (partial.length > 0) {
      setOpenRouterValidationMsg('Each row must have both a name and a key, or leave both empty.')
      return
    }
    // Filter out completely empty rows before saving.
    const toSave = openRouterKeys.filter(k => k.name || k.key)
    setOpenRouterSaving(true)
    setOpenRouterStatus('idle')
    setOpenRouterErrorMsg('')
    try {
      const res = await fetch('/app/settings/OPENROUTER_KEYS', {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ value: JSON.stringify(toSave) }),
      })
      if (res.ok) {
        setOpenRouterKeys(toSave)
        setOpenRouterStatus('saved')
        setTimeout(() => setOpenRouterStatus('idle'), 3000)
      } else {
        const text = await res.text()
        setOpenRouterStatus('error')
        setOpenRouterErrorMsg(text || `Error ${res.status}`)
      }
    } catch (err) {
      setOpenRouterStatus('error')
      setOpenRouterErrorMsg(String(err))
    }
    setOpenRouterSaving(false)
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
        // Update is_set badge immediately based on whether new value is non-empty
        setSettings(prev => prev.map(s => s.key === key ? { ...s, is_set: state.value !== '' } : s))
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
                    <div style={{ flex: '0 0 64px', display: 'flex', justifyContent: 'flex-end', alignItems: 'center', paddingTop: '6px' }}>
                      {s.is_set
                        ? <span style={{ fontSize: '18px', color: '#27ae60', lineHeight: 1 }} title="Set">✓</span>
                        : <span style={{ fontSize: '11px', color: '#bbb', fontStyle: 'italic', whiteSpace: 'nowrap' }}>not set</span>
                      }
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

        {/* Slack connection */}
        {(() => {
          const clientIdSetting = settings.find(s => s.key === 'SLACK_CLIENT_ID')
          const clientSecretSetting = settings.find(s => s.key === 'SLACK_CLIENT_SECRET')
          const credsMissing = !clientIdSetting?.is_set || !clientSecretSetting?.is_set
          return (
            <div style={{ background: 'white', borderRadius: '8px', boxShadow: '0 2px 8px rgba(0,0,0,0.06)', padding: '20px 24px' }}>
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start' }}>
                <div>
                  <div style={{ fontWeight: 600, fontSize: '14px', color: '#2c3e50', marginBottom: '4px' }}>Slack</div>
                  <div style={{ fontSize: '12px', color: '#888', lineHeight: '1.5' }}>
                    OAuth2 authorization for Slack workflow nodes.
                  </div>
                </div>
                <div style={{ display: 'flex', alignItems: 'center', gap: '14px', flexShrink: 0, marginLeft: '24px' }}>
                  {slackConnected === null ? (
                    <span style={{ fontSize: '12px', color: '#aaa' }}>Checking...</span>
                  ) : slackConnected ? (
                    <span style={{ fontSize: '12px', color: '#27ae60', fontWeight: 600 }}>● Connected{slackWorkspace ? `: ${slackWorkspace}` : ''}</span>
                  ) : (
                    <span style={{ fontSize: '12px', color: '#e74c3c', fontWeight: 500 }}>○ Not connected</span>
                  )}
                  <div style={{ position: 'relative', display: 'inline-block' }}>
                    <button
                      onClick={credsMissing ? undefined : connectSlack}
                      disabled={slackConnecting || credsMissing}
                      title={credsMissing ? 'Add Client ID and Secret first' : undefined}
                      style={saveBtn(slackConnecting || credsMissing)}
                    >
                      {slackConnecting ? 'Waiting for auth...' : slackConnected ? 'Reconnect' : 'Connect Slack'}
                    </button>
                  </div>
                </div>
              </div>
            </div>
          )
        })()}

        {/* Telegram connection */}
        {(() => {
          const tokenSetting = settings.find(s => s.key === 'TELEGRAM_BOT_TOKEN')
          const tokenMissing = !tokenSetting?.is_set && !telegramBotTokenSet
          return (
            <div style={{ background: 'white', borderRadius: '8px', boxShadow: '0 2px 8px rgba(0,0,0,0.06)', padding: '20px 24px' }}>
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start' }}>
                <div>
                  <div style={{ fontWeight: 600, fontSize: '14px', color: '#2c3e50', marginBottom: '4px' }}>Telegram</div>
                  <div style={{ fontSize: '12px', color: '#888', lineHeight: '1.5' }}>
                    Token-based bot authorization for Telegram workflow nodes.
                  </div>
                </div>
                <div style={{ display: 'flex', alignItems: 'center', gap: '14px', flexShrink: 0, marginLeft: '24px' }}>
                  {telegramConnected === null ? (
                    <span style={{ fontSize: '12px', color: '#aaa' }}>Checking...</span>
                  ) : telegramBotTokenSet ? (
                    <span style={{ fontSize: '12px', color: '#27ae60', fontWeight: 600 }}>● Bot connected</span>
                  ) : (
                    <span style={{ fontSize: '12px', color: '#999', fontWeight: 500 }}>○ Not connected</span>
                  )}
                  <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'flex-end', gap: '6px' }}>
                    <button
                      onClick={tokenMissing || telegramWebhookRegistering ? undefined : registerTelegramWebhook}
                      disabled={telegramWebhookRegistering || tokenMissing}
                      title={tokenMissing ? 'Add TELEGRAM_BOT_TOKEN first' : undefined}
                      style={saveBtn(telegramWebhookRegistering || tokenMissing || false)}
                    >
                      {telegramWebhookRegistering ? 'Registering...' : 'Register Webhook'}
                    </button>
                    {telegramWebhookResult === 'success' && (
                      <span style={{ fontSize: '12px', color: '#27ae60', fontWeight: 500 }}>Webhook registered ✓</span>
                    )}
                    {telegramWebhookResult === 'error' && (
                      <span style={{ fontSize: '12px', color: '#e74c3c', fontWeight: 500 }}>Registration failed</span>
                    )}
                  </div>
                </div>
              </div>
            </div>
          )
        })()}

        {/* OpenRouter API Keys */}
        <div style={{ background: 'white', borderRadius: '8px', boxShadow: '0 2px 8px rgba(0,0,0,0.06)', padding: '20px 24px' }}>
          <div style={{ marginBottom: '16px' }}>
            <div style={{ fontWeight: 600, fontSize: '14px', color: '#2c3e50', marginBottom: '4px' }}>OpenRouter API Keys</div>
            <div style={{ fontSize: '12px', color: '#888', lineHeight: '1.5' }}>
              Named keys used by AI workflow nodes. Each key can be selected by name in llm_prompt and image_generate nodes.
            </div>
          </div>

          {/* Key rows */}
          <div style={{ display: 'flex', flexDirection: 'column', gap: '8px', marginBottom: '12px' }}>
            {openRouterKeys.map((row, idx) => (
              <div key={idx} style={{ display: 'flex', gap: '8px', alignItems: 'center' }}>
                <input
                  type="text"
                  value={row.name}
                  placeholder="name (e.g. text)"
                  onChange={e => {
                    const updated = openRouterKeys.map((k, i) => i === idx ? { ...k, name: e.target.value } : k)
                    setOpenRouterKeys(updated)
                    setOpenRouterValidationMsg('')
                  }}
                  style={{ ...inputStyle, flex: '0 0 140px' }}
                />
                <input
                  type="password"
                  value={row.key}
                  placeholder="sk-or-..."
                  onChange={e => {
                    const updated = openRouterKeys.map((k, i) => i === idx ? { ...k, key: e.target.value } : k)
                    setOpenRouterKeys(updated)
                    setOpenRouterValidationMsg('')
                  }}
                  style={{ ...inputStyle, flex: 1 }}
                />
                <button
                  onClick={() => {
                    setOpenRouterKeys(openRouterKeys.filter((_, i) => i !== idx))
                    setOpenRouterValidationMsg('')
                  }}
                  title="Remove this key"
                  style={{
                    background: 'none',
                    border: '1px solid #e0e6ed',
                    borderRadius: '4px',
                    color: '#999',
                    cursor: 'pointer',
                    fontSize: '14px',
                    lineHeight: 1,
                    padding: '7px 10px',
                    flexShrink: 0,
                  }}
                >
                  ✕
                </button>
              </div>
            ))}
          </div>

          {/* Validation warning */}
          {openRouterValidationMsg && (
            <div style={{ fontSize: '12px', color: '#e67e22', fontWeight: 500, marginBottom: '10px' }}>
              {openRouterValidationMsg}
            </div>
          )}

          {/* Action row */}
          <div style={{ display: 'flex', gap: '8px', alignItems: 'center' }}>
            <button
              onClick={() => setOpenRouterKeys([...openRouterKeys, { name: '', key: '' }])}
              style={{
                background: 'none',
                border: '1px solid #6b46c1',
                borderRadius: '4px',
                color: '#6b46c1',
                cursor: 'pointer',
                fontSize: '13px',
                fontWeight: 500,
                padding: '8px 14px',
              }}
            >
              + Add Key
            </button>
            <button
              onClick={saveOpenRouterKeys}
              disabled={openRouterSaving}
              style={saveBtn(openRouterSaving)}
            >
              {openRouterSaving ? 'Saving...' : 'Save All'}
            </button>
            {openRouterStatus === 'saved' && (
              <span style={{ fontSize: '12px', color: '#27ae60', fontWeight: 500 }}>Saved</span>
            )}
            {openRouterStatus === 'error' && (
              <span style={{ fontSize: '12px', color: '#e74c3c', fontWeight: 500 }}>{openRouterErrorMsg || 'Error saving'}</span>
            )}
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
