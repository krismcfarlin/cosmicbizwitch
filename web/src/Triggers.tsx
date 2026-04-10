import { useState, useEffect, useCallback } from 'react'
import { useNavigate } from 'react-router-dom'

interface Trigger {
  id: string
  name: string
  type: 'webhook' | 'record_hook' | 'cron' | 'telegram' | 'telegram_callback'
  graph_name: string
  config: Record<string, string>
  token: string
  enabled: boolean
  last_fired?: string
  created_at: string
  updated_at: string
}

const TYPE_COLORS: Record<string, { bg: string; color: string }> = {
  webhook:     { bg: '#e8f4fd', color: '#2980b9' },
  record_hook: { bg: '#fef9e7', color: '#e67e22' },
  cron:        { bg: '#eafaf1', color: '#1e8449' },
  telegram:          { bg: '#e8f1fd', color: '#2563eb' },
  telegram_callback: { bg: '#ede9fe', color: '#7c3aed' },
}

function TypeBadge({ type }: { type: string }) {
  const c = TYPE_COLORS[type] ?? { bg: '#f0f0f0', color: '#555' }
  return (
    <span style={{ background: c.bg, color: c.color, padding: '2px 10px', borderRadius: '12px', fontSize: '12px', fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.5px' }}>
      {type.replace('_', ' ')}
    </span>
  )
}

function formatDate(s?: string) {
  if (!s) return '—'
  return new Date(s).toLocaleString()
}

const EMPTY_TRIGGER: Omit<Trigger, 'id' | 'token' | 'created_at' | 'updated_at'> = {
  name: '',
  type: 'webhook',
  graph_name: '',
  config: {},
  enabled: true,
}

export default function Triggers() {
  const [triggers, setTriggers] = useState<Trigger[]>([])
  const [graphs, setGraphs] = useState<string[]>([])
  const [loading, setLoading] = useState(true)
  const [modal, setModal] = useState<{ mode: 'create' | 'edit'; trigger: Partial<Trigger> } | null>(null)
  const [saving, setSaving] = useState(false)
  const [copiedId, setCopiedId] = useState<string | null>(null)
  const navigate = useNavigate()

  const load = useCallback(async () => {
    const [tRes, gRes] = await Promise.all([
      fetch('/api/triggers'),
      fetch('/api/workflows/graphs'),
    ])
    if (tRes.ok) {
      const d = await tRes.json()
      setTriggers(d.triggers ?? [])
    }
    if (gRes.ok) {
      const d = await gRes.json()
      setGraphs(d.names ?? [])
    }
    setLoading(false)
  }, [])

  useEffect(() => { load() }, [load])

  const openCreate = () => {
    setModal({ mode: 'create', trigger: { ...EMPTY_TRIGGER, graph_name: graphs[0] ?? '', config: {} } })
  }

  const openEdit = (t: Trigger) => {
    setModal({ mode: 'edit', trigger: { ...t } })
  }

  const save = async () => {
    if (!modal) return
    setSaving(true)
    const { trigger, mode } = modal
    const body = {
      name: trigger.name,
      type: trigger.type,
      graph_name: trigger.graph_name,
      config: trigger.config ?? {},
      token: trigger.token ?? '',
      enabled: trigger.enabled ?? true,
    }
    const res = mode === 'create'
      ? await fetch('/api/triggers', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) })
      : await fetch(`/api/triggers/${trigger.id}`, { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) })

    if (res.ok) {
      setModal(null)
      await load()
    }
    setSaving(false)
  }

  const deleteTrigger = async (id: string) => {
    if (!confirm('Delete this trigger?')) return
    await fetch(`/api/triggers/${id}`, { method: 'DELETE' })
    await load()
  }

  const toggleEnabled = async (t: Trigger) => {
    await fetch(`/api/triggers/${t.id}`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ ...t, enabled: !t.enabled }),
    })
    await load()
  }

  const copyWebhookUrl = (token: string, id: string) => {
    const url = `${window.location.origin}/webhooks/${token}`
    navigator.clipboard.writeText(url)
    setCopiedId(id + '_token')
    setTimeout(() => setCopiedId(null), 2000)
  }

  const copyTriggerUrl = (id: string) => {
    const url = `${window.location.origin}/trigger/${id}`
    navigator.clipboard.writeText(url)
    setCopiedId(id + '_trigger')
    setTimeout(() => setCopiedId(null), 2000)
  }

  const patch = (key: string, value: unknown) =>
    setModal(m => m ? { ...m, trigger: { ...m.trigger, [key]: value } } : m)

  const patchConfig = (key: string, value: string) =>
    setModal(m => m ? { ...m, trigger: { ...m.trigger, config: { ...(m.trigger.config ?? {}), [key]: value } } } : m)

  const getInitialContextStr = () => {
    const ic = modal?.trigger.config?.initial_context
    if (!ic) return '{}'
    try { return JSON.stringify(ic, null, 2) } catch { return '{}' }
  }

  const setInitialContext = (raw: string) => {
    try {
      const parsed = JSON.parse(raw)
      setModal(m => m ? { ...m, trigger: { ...m.trigger, config: { ...(m.trigger.config ?? {}), initial_context: parsed } } } : m)
    } catch { /* ignore invalid JSON while typing */ }
  }

  return (
    <div style={{ fontFamily: '-apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif', minHeight: '100vh', background: '#f5f7fa' }}>
      <header style={{ background: 'white', borderBottom: '1px solid #e0e6ed', padding: '14px 24px', display: 'flex', justifyContent: 'space-between', alignItems: 'center', boxShadow: '0 2px 4px rgba(0,0,0,0.05)' }}>
        <h1 style={{ fontSize: '20px', color: '#667eea', fontWeight: 600, margin: 0 }}>Triggers</h1>
        <div style={{ display: 'flex', gap: '10px' }}>
          <button onClick={openCreate} style={navBtn('#27ae60')}>+ New Trigger</button>
          <button onClick={() => navigate('/workflows')} style={navBtn('#667eea')}>Workflows</button>
          <button onClick={() => navigate('/telegram/messages')} style={navBtn('#2563eb')}>Telegram Debug</button>
          <button onClick={() => navigate('/settings')} style={navBtn('#6b46c1')}>Settings</button>
          <a href="/logs" style={navBtn('#667eea')}>Logs</a>
          <a href="/logout" style={navBtn('#e74c3c')}>Logout</a>
        </div>
      </header>

      <div style={{ padding: '20px 24px' }}>
        <div style={{ background: 'white', borderRadius: '8px', boxShadow: '0 2px 8px rgba(0,0,0,0.06)', overflow: 'hidden' }}>
          {loading ? (
            <div style={{ padding: '40px', textAlign: 'center', color: '#999' }}>Loading...</div>
          ) : triggers.length === 0 ? (
            <div style={{ padding: '40px', textAlign: 'center', color: '#999' }}>
              No triggers yet. Create one to automatically start workflows.
            </div>
          ) : (
            <table style={{ width: '100%', borderCollapse: 'collapse' }}>
              <thead>
                <tr>
                  {['Name', 'Type', 'Graph', 'Config', 'Last Fired', 'Enabled', 'Actions'].map(h => (
                    <th key={h} style={{ padding: '12px 16px', textAlign: 'left', fontSize: '12px', fontWeight: 600, color: '#666', background: '#f8f9fa', borderBottom: '2px solid #e0e6ed', textTransform: 'uppercase', letterSpacing: '0.5px' }}>{h}</th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {triggers.map(t => (
                  <tr key={t.id} style={{ background: 'white' }}>
                    <td style={{ padding: '12px 16px', borderBottom: '1px solid #e0e6ed', fontWeight: 500 }}>{t.name}</td>
                    <td style={{ padding: '12px 16px', borderBottom: '1px solid #e0e6ed' }}><TypeBadge type={t.type} /></td>
                    <td style={{ padding: '12px 16px', borderBottom: '1px solid #e0e6ed', fontSize: '13px', color: '#667eea', fontFamily: 'monospace' }}>{t.graph_name}</td>
                    <td style={{ padding: '12px 16px', borderBottom: '1px solid #e0e6ed', fontSize: '12px', color: '#555' }}>
                      <div style={{ display: 'flex', flexDirection: 'column', gap: '4px' }}>
                        {/* Trigger ID URL — works for all types */}
                        <div style={{ display: 'flex', alignItems: 'center', gap: '6px' }}>
                          <code style={{ fontSize: '11px', color: '#475569', background: '#f1f5f9', padding: '2px 6px', borderRadius: '3px', whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis', maxWidth: '180px' }}>
                            /trigger/{t.id}
                          </code>
                          <button onClick={() => copyTriggerUrl(t.id)}
                            style={{ background: copiedId === t.id + '_trigger' ? '#27ae60' : '#e8f4fd', color: copiedId === t.id + '_trigger' ? 'white' : '#2980b9', border: 'none', borderRadius: '4px', padding: '2px 8px', cursor: 'pointer', fontSize: '11px', fontWeight: 600, whiteSpace: 'nowrap' }}>
                            {copiedId === t.id + '_trigger' ? 'Copied!' : 'Copy'}
                          </button>
                        </div>
                        {/* Type-specific config info */}
                        {t.type === 'webhook' && t.token && (
                          <button onClick={() => copyWebhookUrl(t.token, t.id)}
                            style={{ alignSelf: 'flex-start', background: copiedId === t.id + '_token' ? '#27ae60' : '#f0f0f0', color: copiedId === t.id + '_token' ? 'white' : '#333', border: 'none', borderRadius: '4px', padding: '2px 8px', cursor: 'pointer', fontSize: '11px' }}>
                            {copiedId === t.id + '_token' ? 'Copied!' : 'Copy token URL'}
                          </button>
                        )}
                        {t.type === 'record_hook' && (
                          <span style={{ fontFamily: 'monospace', fontSize: '11px', color: '#888' }}>{t.config.collection} / {t.config.event}</span>
                        )}
                        {t.type === 'cron' && (
                          <span style={{ fontFamily: 'monospace', fontSize: '11px', color: '#888' }}>every {t.config.interval}</span>
                        )}
                        {t.type === 'telegram' && (
                          <span style={{ fontFamily: 'monospace', fontSize: '11px', color: '#888' }}>
                            {[
                              t.config.chat_type && t.config.chat_type,
                              t.config.chat_id && `id:${t.config.chat_id}`,
                              t.config.username && `@${t.config.username}`,
                              t.config.text_contains && `contains:"${t.config.text_contains}"`,
                              t.config.text_starts_with && `starts:"${t.config.text_starts_with}"`,
                            ].filter(Boolean).join(', ') || 'any message'}
                          </span>
                        )}
                      </div>
                    </td>
                    <td style={{ padding: '12px 16px', borderBottom: '1px solid #e0e6ed', fontSize: '12px', color: '#888' }}>{formatDate(t.last_fired)}</td>
                    <td style={{ padding: '12px 16px', borderBottom: '1px solid #e0e6ed' }}>
                      <button onClick={() => toggleEnabled(t)}
                        style={{ background: t.enabled ? '#eafaf1' : '#f8f9fa', color: t.enabled ? '#1e8449' : '#999', border: `1px solid ${t.enabled ? '#a9dfbf' : '#ddd'}`, borderRadius: '4px', padding: '3px 10px', cursor: 'pointer', fontSize: '12px', fontWeight: 600 }}>
                        {t.enabled ? 'ON' : 'OFF'}
                      </button>
                    </td>
                    <td style={{ padding: '12px 16px', borderBottom: '1px solid #e0e6ed' }}>
                      <div style={{ display: 'flex', gap: '6px' }}>
                        <button onClick={() => openEdit(t)} style={actionBtn('#667eea')}>Edit</button>
                        <button onClick={() => deleteTrigger(t.id)} style={actionBtn('#e74c3c')}>Delete</button>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </div>
      </div>

      {modal && (
        <div style={{ position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.5)', display: 'flex', alignItems: 'center', justifyContent: 'center', zIndex: 1000 }}>
          <div style={{ background: 'white', borderRadius: '8px', padding: '24px', width: '520px', boxShadow: '0 20px 60px rgba(0,0,0,0.3)' }}>
            <h3 style={{ margin: '0 0 16px', color: '#2c3e50' }}>
              {modal.mode === 'create' ? 'New Trigger' : 'Edit Trigger'}
            </h3>
            <div style={{ display: 'flex', flexDirection: 'column', gap: '12px' }}>
              <Field label="Name">
                <input value={modal.trigger.name ?? ''} onChange={e => patch('name', e.target.value)}
                  placeholder="My Trigger" style={inputStyle} />
              </Field>

              <Field label="Type">
                <select value={modal.trigger.type ?? 'webhook'} onChange={e => patch('type', e.target.value)} style={inputStyle}>
                  <option value="webhook">Webhook — POST to a secret URL</option>
                  <option value="record_hook">Record Hook — PocketBase collection event</option>
                  <option value="cron">Cron — interval timer</option>
                  <option value="telegram">Telegram — incoming message</option>
                  <option value="telegram_callback">Telegram — button callback</option>
                </select>
              </Field>

              <Field label="Workflow Graph">
                <select value={modal.trigger.graph_name ?? ''} onChange={e => patch('graph_name', e.target.value)} style={inputStyle}>
                  {graphs.map(g => <option key={g} value={g}>{g}</option>)}
                </select>
              </Field>

              {modal.trigger.type === 'webhook' && (
                <div style={{ padding: '10px 12px', background: '#f8f9fa', borderRadius: '6px', fontSize: '12px', color: '#666' }}>
                  A secret URL will be generated. POST JSON to it to start the workflow. The body becomes the workflow context.
                </div>
              )}

              {modal.trigger.type === 'record_hook' && (
                <>
                  <Field label="Collection name">
                    <input value={modal.trigger.config?.collection ?? ''} onChange={e => patchConfig('collection', e.target.value)}
                      placeholder="e.g. contacts" style={inputStyle} />
                  </Field>
                  <Field label="Event">
                    <select value={modal.trigger.config?.event ?? 'create'} onChange={e => patchConfig('event', e.target.value)} style={inputStyle}>
                      <option value="create">create</option>
                      <option value="update">update</option>
                      <option value="delete">delete</option>
                      <option value="create_or_update">create or update</option>
                    </select>
                  </Field>
                </>
              )}

              {modal.trigger.type === 'cron' && (
                <Field label="Interval (Go duration)">
                  <input value={modal.trigger.config?.interval ?? ''} onChange={e => patchConfig('interval', e.target.value)}
                    placeholder="e.g. 1h, 30m, 5m" style={inputStyle} />
                  <div style={{ fontSize: '11px', color: '#999', marginTop: '4px' }}>
                    Valid units: h, m, s. Example: 1h30m, 15m, 24h
                  </div>
                </Field>
              )}

              {modal.trigger.type === 'telegram_callback' && (
                <>
                  <div style={{ padding: '8px 12px', background: '#f5f3ff', border: '1px solid #ddd6fe', borderRadius: '6px', fontSize: '12px', color: '#5b21b6' }}>
                    Fires when a user clicks an inline button sent by <strong>telegram_send_button</strong>. Filter by callback_data to route different buttons to different workflows.
                  </div>
                  <Field label="Callback data equals">
                    <input value={modal.trigger.config?.callback_data ?? ''} onChange={e => patchConfig('callback_data', e.target.value)}
                      placeholder='e.g. "yes" or "approve_order"' style={inputStyle} />
                  </Field>
                  <Field label="Callback data starts with">
                    <input value={modal.trigger.config?.callback_data_prefix ?? ''} onChange={e => patchConfig('callback_data_prefix', e.target.value)}
                      placeholder='e.g. "action_"' style={inputStyle} />
                  </Field>
                  <Field label="Chat ID">
                    <input type="number" value={modal.trigger.config?.chat_id ?? ''} onChange={e => patchConfig('chat_id', e.target.value)}
                      placeholder="e.g. -100123456" style={inputStyle} />
                  </Field>
                </>
              )}

              {modal.trigger.type === 'telegram' && (
                <>
                  <div style={{ padding: '8px 12px', background: '#eff6ff', border: '1px solid #bfdbfe', borderRadius: '6px', fontSize: '12px', color: '#1e40af' }}>
                    All filled fields must match (AND logic). Leave blank to match any message.
                  </div>
                  <Field label="Text contains">
                    <input value={modal.trigger.config?.text_contains ?? ''} onChange={e => patchConfig('text_contains', e.target.value)}
                      placeholder='e.g. "order"' style={inputStyle} />
                  </Field>
                  <Field label="Text starts with">
                    <input value={modal.trigger.config?.text_starts_with ?? ''} onChange={e => patchConfig('text_starts_with', e.target.value)}
                      placeholder='e.g. "/status"' style={inputStyle} />
                  </Field>
                  <Field label="Chat ID">
                    <input type="number" value={modal.trigger.config?.chat_id ?? ''} onChange={e => patchConfig('chat_id', e.target.value)}
                      placeholder="e.g. -100123456" style={inputStyle} />
                  </Field>
                  <Field label="Chat type">
                    <select value={modal.trigger.config?.chat_type ?? ''} onChange={e => patchConfig('chat_type', e.target.value)} style={inputStyle}>
                      <option value="">any</option>
                      <option value="private">private</option>
                      <option value="group">group</option>
                      <option value="supergroup">supergroup</option>
                      <option value="channel">channel</option>
                    </select>
                  </Field>
                  <Field label="From username">
                    <input value={modal.trigger.config?.username ?? ''} onChange={e => patchConfig('username', e.target.value)}
                      placeholder="e.g. john_doe (no @)" style={inputStyle} />
                  </Field>
                </>
              )}

              <Field label="Initial Context (JSON)">
                <textarea
                  defaultValue={getInitialContextStr()}
                  onChange={e => setInitialContext(e.target.value)}
                  rows={4}
                  style={{ ...inputStyle, fontFamily: 'monospace', fontSize: '12px', resize: 'vertical' }}
                />
                <div style={{ fontSize: '11px', color: '#999', marginTop: '4px' }}>
                  Static key/value pairs added to every workflow started by this trigger. Event data is merged on top.
                </div>
              </Field>

              <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
                <input type="checkbox" id="enabled-chk" checked={modal.trigger.enabled ?? true} onChange={e => patch('enabled', e.target.checked)} />
                <label htmlFor="enabled-chk" style={{ fontSize: '13px', color: '#444', cursor: 'pointer' }}>Enabled</label>
              </div>
            </div>

            <div style={{ display: 'flex', gap: '10px', justifyContent: 'flex-end', marginTop: '20px' }}>
              <button onClick={() => setModal(null)} style={{ padding: '8px 16px', background: '#f0f0f0', border: 'none', borderRadius: '4px', cursor: 'pointer', fontWeight: 500 }}>Cancel</button>
              <button onClick={save} disabled={saving}
                style={{ padding: '8px 16px', background: '#667eea', color: 'white', border: 'none', borderRadius: '4px', cursor: saving ? 'default' : 'pointer', fontWeight: 500, opacity: saving ? 0.7 : 1 }}>
                {saving ? 'Saving...' : modal.mode === 'create' ? 'Create' : 'Save'}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div>
      <label style={{ fontSize: '12px', color: '#666', fontWeight: 600, display: 'block', marginBottom: '4px' }}>{label}</label>
      {children}
    </div>
  )
}

const inputStyle: React.CSSProperties = {
  width: '100%', padding: '8px', border: '1px solid #e0e6ed', borderRadius: '4px',
  fontSize: '13px', boxSizing: 'border-box',
}

function navBtn(bg: string): React.CSSProperties {
  return { background: bg, color: 'white', padding: '6px 14px', borderRadius: '4px', textDecoration: 'none', fontSize: '13px', fontWeight: 500, border: 'none', cursor: 'pointer' }
}

function actionBtn(bg: string): React.CSSProperties {
  return { background: bg, color: 'white', border: 'none', padding: '4px 10px', borderRadius: '4px', cursor: 'pointer', fontSize: '12px', fontWeight: 500 }
}
