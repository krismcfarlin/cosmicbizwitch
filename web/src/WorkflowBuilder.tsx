import React, { useState, useEffect, useRef, useCallback, useMemo } from 'react'
import { useNavigate } from 'react-router-dom'
import Nav from './Nav'

// ── Types ────────────────────────────────────────────────────────────────────

interface FieldMeta { name: string; type: string; description: string; required?: boolean; options?: string[] }
interface ActivityMeta { description: string; category?: string; input_fields: FieldMeta[]; output_fields: FieldMeta[] }
interface ActivityInfo { name: string; meta: ActivityMeta }
interface DriveItem { id: string; name: string; type: 'folder' | 'file'; mime_type?: string; is_folder?: boolean }

interface BNode {
  id: string
  activityName: string  // 'END' for finish node
  x: number
  y: number
  label: string
  maxRetries: number
  isHuman: boolean
  needsConversion: boolean  // flagged to be rewritten as a native Go activity
  staticInput: Record<string, string>  // pre-set input fields baked into the node
}

interface CondRow { key: string; operator: string; value: string }
interface BEdge {
  id: string
  fromNode: string
  toNode: string   // node id, or 'END' when pointing to finish
  conditions: CondRow[]
  label: string
}

// ── Constants ────────────────────────────────────────────────────────────────

const NODE_W = 160
const NODE_H = 52
const PORT_R = 6
const OPERATORS = ['eq', 'neq', 'gt', 'lt', 'gte', 'lte', 'contains', 'exists', 'not_exists']

const newNid = () => `n_${Math.random().toString(36).slice(2, 9)}`
const newEid = () => `e_${Math.random().toString(36).slice(2, 9)}`

function parseVal(s: string): unknown {
  if (s === 'true') return true
  if (s === 'false') return false
  const n = Number(s)
  if (!isNaN(n) && s.trim() !== '') return n
  return s
}

// ── LLM model list ────────────────────────────────────────────────────────────
const LLM_MODELS = [
  { group: 'Anthropic (Direct)', models: [
    { value: 'anthropic/claude-opus-4-6', label: 'Claude Opus 4.6' },
    { value: 'anthropic/claude-sonnet-4-6', label: 'Claude Sonnet 4.6' },
    { value: 'anthropic/claude-haiku-4-5', label: 'Claude Haiku 4.5' },
  ]},
  { group: 'OpenRouter — Anthropic', models: [
    { value: 'openrouter/anthropic/claude-opus-4', label: 'Claude Opus 4 (OR)' },
    { value: 'openrouter/anthropic/claude-sonnet-4-5', label: 'Claude Sonnet 4.5 (OR)' },
    { value: 'openrouter/anthropic/claude-3-5-haiku', label: 'Claude 3.5 Haiku (OR)' },
  ]},
  { group: 'OpenRouter — OpenAI', models: [
    { value: 'openrouter/openai/gpt-4o', label: 'GPT-4o (OR)' },
    { value: 'openrouter/openai/gpt-4o-mini', label: 'GPT-4o Mini (OR)' },
  ]},
  { group: 'OpenRouter — Google', models: [
    { value: 'openrouter/google/gemini-2.0-flash-001', label: 'Gemini 2.0 Flash (OR)' },
    { value: 'openrouter/google/gemini-2.5-pro-preview-06-05', label: 'Gemini 2.5 Pro (OR)' },
  ]},
  { group: 'OpenRouter — Other', models: [
    { value: 'openrouter/meta-llama/llama-3.3-70b-instruct', label: 'Llama 3.3 70B (OR)' },
    { value: 'openrouter/deepseek/deepseek-r1', label: 'DeepSeek R1 (OR)' },
    { value: 'openrouter/mistralai/mistral-large', label: 'Mistral Large (OR)' },
  ]},
]

// ── ActivityCard (sidebar item) ───────────────────────────────────────────────

function ActivityCard({ info, onDragStart }: { info: ActivityInfo; onDragStart: (name: string) => void }) {
  const [hover, setHover] = useState(false)

  return (
    <div
      draggable
      onDragStart={() => onDragStart(info.name)}
      onMouseEnter={() => setHover(true)}
      onMouseLeave={() => setHover(false)}
      style={{
        background: 'white', border: '1px solid #e0e6ed', borderRadius: '6px',
        padding: '8px 10px', cursor: 'grab', marginBottom: '6px',
        fontSize: '12px', fontFamily: 'monospace', color: '#2c3e50',
        userSelect: 'none', position: 'relative',
        boxShadow: hover ? '0 2px 8px rgba(0,0,0,0.12)' : 'none',
        transition: 'box-shadow 0.15s',
      }}
    >
      <div style={{ fontWeight: 600 }}>{info.name}</div>
      {info.meta.description && (
        <div style={{ fontSize: '11px', color: '#888', marginTop: '2px', fontFamily: 'sans-serif' }}>
          {info.meta.description}
        </div>
      )}

      {hover && (info.meta.input_fields?.length > 0 || info.meta.output_fields?.length > 0) && (
        <div style={{
          position: 'absolute', left: 'calc(100% + 8px)', top: 0, zIndex: 9999,
          background: '#1e1e1e', color: '#d4d4d4', borderRadius: '6px',
          padding: '10px 12px', fontSize: '11px', fontFamily: 'monospace',
          width: '240px', boxShadow: '0 4px 16px rgba(0,0,0,0.4)', lineHeight: 1.6,
          pointerEvents: 'none',
        }}>
          {info.meta.input_fields?.length > 0 && (
            <>
              <div style={{ color: '#4ec9b0', fontWeight: 700, marginBottom: '4px' }}>Inputs</div>
              {info.meta.input_fields.map(f => (
                <div key={f.name}><span style={{ color: '#9cdcfe' }}>{f.name}</span>
                  <span style={{ color: '#888' }}> {f.type}</span>
                  {f.description && <span style={{ color: '#666' }}> — {f.description}</span>}
                </div>
              ))}
            </>
          )}
          {info.meta.output_fields?.length > 0 && (
            <>
              <div style={{ color: '#ce9178', fontWeight: 700, marginTop: '8px', marginBottom: '4px' }}>Outputs</div>
              {info.meta.output_fields.map(f => (
                <div key={f.name}><span style={{ color: '#9cdcfe' }}>{f.name}</span>
                  <span style={{ color: '#888' }}> {f.type}</span>
                  {f.description && <span style={{ color: '#666' }}> — {f.description}</span>}
                </div>
              ))}
            </>
          )}
        </div>
      )}
    </div>
  )
}

// ── CategoryGroup (sidebar) ───────────────────────────────────────────────────

function CategoryGroup({ category, activities, onDragStart, open, onOpen }: {
  category: string
  activities: ActivityInfo[]
  onDragStart: (name: string) => void
  open: boolean
  onOpen: () => void
}) {
  return (
    <div style={{ marginBottom: '8px' }}>
      <button
        onClick={onOpen}
        style={{
          width: '100%', textAlign: 'left', background: '#f0f2f5', border: 'none',
          borderRadius: '4px', padding: '4px 8px', cursor: 'pointer',
          fontSize: '10px', fontWeight: 700, color: '#555', textTransform: 'uppercase',
          letterSpacing: '0.4px', display: 'flex', justifyContent: 'space-between',
          alignItems: 'center', marginBottom: open ? '6px' : '0',
        }}
      >
        <span>{category}</span>
        <span style={{ fontSize: '10px', color: '#999' }}>{open ? '▾' : '▸'}</span>
      </button>
      {open && activities.map(a => (
        <ActivityCard key={a.name} info={a} onDragStart={onDragStart} />
      ))}
    </div>
  )
}

// ── Edge conditions editor ────────────────────────────────────────────────────

function EdgeEditor({ edge, onChange, onDelete }: {
  edge: BEdge
  onChange: (e: BEdge) => void
  onDelete: () => void
}) {
  const set = (patch: Partial<BEdge>) => onChange({ ...edge, ...patch })
  const setCond = (i: number, patch: Partial<CondRow>) => {
    const conds = edge.conditions.map((c, j) => j === i ? { ...c, ...patch } : c)
    set({ conditions: conds })
  }
  const addCond = () => set({ conditions: [...edge.conditions, { key: '', operator: 'eq', value: '' }] })
  const delCond = (i: number) => set({ conditions: edge.conditions.filter((_, j) => j !== i) })

  return (
    <div>
      <div style={{ fontWeight: 600, marginBottom: '8px', color: '#2c3e50' }}>Edge Properties</div>
      <label style={labelStyle}>Label (optional)</label>
      <input value={edge.label} onChange={e => set({ label: e.target.value })}
        style={inputStyle} placeholder="e.g. high path" />

      <div style={{ marginTop: '12px', marginBottom: '6px', fontWeight: 600, fontSize: '12px', color: '#555' }}>
        Conditions <span style={{ fontWeight: 400, color: '#aaa' }}>(all must match; empty = always)</span>
      </div>

      {edge.conditions.map((c, i) => (
        <div key={i} style={{ display: 'flex', gap: '4px', marginBottom: '4px', alignItems: 'center' }}>
          <input value={c.key} onChange={e => setCond(i, { key: e.target.value })}
            placeholder="key" style={{ ...inputStyle, width: '80px', margin: 0 }} />
          <select value={c.operator} onChange={e => setCond(i, { operator: e.target.value })}
            style={{ ...inputStyle, margin: 0, flex: 1 }}>
            {OPERATORS.map(op => <option key={op} value={op}>{op}</option>)}
          </select>
          {!['exists', 'not_exists'].includes(c.operator) && (
            <input value={c.value} onChange={e => setCond(i, { value: e.target.value })}
              placeholder="value" style={{ ...inputStyle, width: '70px', margin: 0 }} />
          )}
          <button onClick={() => delCond(i)} style={iconBtn('#e74c3c')}>✕</button>
        </div>
      ))}
      <button onClick={addCond} style={{ ...iconBtn('#667eea'), fontSize: '11px', padding: '3px 8px', marginTop: '4px' }}>+ Add Condition</button>

      <button onClick={onDelete} style={{ display: 'block', marginTop: '16px', width: '100%', ...iconBtn('#e74c3c'), padding: '6px' }}>
        Delete Edge
      </button>
    </div>
  )
}

// ── Python code editor panel with inline Pyodide test ─────────────────────────

function PythonPanel({ code, onCodeChange, needsConversion, onNeedsConversionChange }: {
  code: string
  onCodeChange: (code: string) => void
  needsConversion: boolean
  onNeedsConversionChange: (v: boolean) => void
}) {
  const [testInput, setTestInput] = useState('{"value": 21}')
  const [phase, setPhase] = useState<'idle' | 'loading' | 'running' | 'done' | 'error'>('idle')
  const [stdout, setStdout] = useState('')
  const [resultStr, setResultStr] = useState('')
  const [errorMsg, setErrorMsg] = useState('')

  const runInBrowser = async () => {
    setPhase('loading'); setStdout(''); setResultStr(''); setErrorMsg('')
    try {
      let inputData: Record<string, unknown> = {}
      try { inputData = JSON.parse(testInput) } catch { throw new Error('Test input is not valid JSON') }

      if (!(window as unknown as Record<string, unknown>)['loadPyodide']) {
        await new Promise<void>((resolve, reject) => {
          if (document.querySelector('script[data-pyodide]')) { resolve(); return }
          const s = document.createElement('script')
          s.src = 'https://cdn.jsdelivr.net/pyodide/v0.27.3/full/pyodide.js'
          s.dataset.pyodide = '1'
          s.onload = () => resolve()
          s.onerror = () => reject(new Error('Failed to load Pyodide'))
          document.head.appendChild(s)
        })
      }

      setPhase('running')
      const loadPyodide = (window as unknown as Record<string, unknown>)['loadPyodide'] as (o: Record<string, unknown>) => Promise<unknown>
      const lines: string[] = []
      const pyodide = await loadPyodide({
        stdout: (line: string) => { lines.push(line); setStdout(lines.join('\n')) },
      }) as {
        globals: { set: (k: string, v: unknown) => void; get: (k: string) => unknown }
        toPy: (v: unknown) => unknown
        runPythonAsync: (code: string) => Promise<void>
      }

      pyodide.globals.set('input', pyodide.toPy(inputData))
      await pyodide.runPythonAsync(code)

      const raw = pyodide.globals.get('result')
      let result: Record<string, unknown> = {}
      if (raw instanceof Map) result = Object.fromEntries(raw)
      else if (raw && typeof (raw as { toJs?: () => unknown }).toJs === 'function') {
        const js = (raw as { toJs: () => unknown }).toJs()
        result = js instanceof Map ? Object.fromEntries(js) : (js as Record<string, unknown> ?? {})
      } else if (raw && typeof raw === 'object') result = raw as Record<string, unknown>

      setResultStr(JSON.stringify(result, null, 2))
      setPhase('done')
    } catch (e) {
      setErrorMsg(String(e))
      setPhase('error')
    }
  }

  return (
    <>
      <div style={{ marginTop: '14px', marginBottom: '6px', fontSize: '11px', fontWeight: 700, color: '#3572A5', borderTop: '1px solid #e0e6ed', paddingTop: '10px' }}>
        🐍 Python Code
      </div>
      <div style={{ fontSize: '10px', color: '#aaa', marginBottom: '8px', lineHeight: 1.5 }}>
        <code>input</code> = workflow context dict. Set <code>result</code> dict to pass values forward. <code>print()</code> is captured.
      </div>
      <textarea
        value={code}
        onChange={e => onCodeChange(e.target.value)}
        rows={10}
        spellCheck={false}
        style={{ width: '100%', boxSizing: 'border-box', fontFamily: 'monospace', fontSize: '11px', padding: '8px 10px', background: '#1e1e2e', color: '#cdd6f4', border: '1px solid #3572A5', borderRadius: '4px', resize: 'vertical', lineHeight: 1.5 }}
      />

      <div style={{ marginTop: '10px', fontSize: '11px', fontWeight: 600, color: '#555', marginBottom: '4px' }}>Test Input (JSON)</div>
      <textarea
        value={testInput}
        onChange={e => setTestInput(e.target.value)}
        rows={3}
        spellCheck={false}
        style={{ width: '100%', boxSizing: 'border-box', fontFamily: 'monospace', fontSize: '11px', padding: '6px 8px', border: '1px solid #ccd', borderRadius: '4px', resize: 'vertical', lineHeight: 1.5 }}
      />
      <button
        onClick={runInBrowser}
        disabled={phase === 'loading' || phase === 'running'}
        style={{ marginTop: '6px', width: '100%', padding: '6px', background: '#3572A5', color: 'white', border: 'none', borderRadius: '4px', cursor: 'pointer', fontWeight: 600, fontSize: '12px', opacity: (phase === 'loading' || phase === 'running') ? 0.6 : 1 }}
      >
        {phase === 'loading' ? '⏳ Loading Pyodide...' : phase === 'running' ? '▶ Running...' : '▶ Run in Browser'}
      </button>

      {(phase === 'done' || phase === 'error' || stdout) && (
        <div style={{ marginTop: '8px', background: '#1e1e2e', borderRadius: '4px', padding: '8px', fontSize: '11px', fontFamily: 'monospace' }}>
          {stdout ? <div style={{ color: '#a6e3a1', whiteSpace: 'pre-wrap', marginBottom: resultStr ? '6px' : 0 }}>{stdout}</div> : null}
          {phase === 'done' && resultStr && <div style={{ color: '#89b4fa', whiteSpace: 'pre-wrap' }}>result = {resultStr}</div>}
          {phase === 'error' && <div style={{ color: '#f38ba8', whiteSpace: 'pre-wrap' }}>{errorMsg}</div>}
        </div>
      )}

      <label style={{ ...labelStyle, display: 'flex', alignItems: 'center', gap: '6px', cursor: 'pointer', marginTop: '8px', padding: '6px 8px', background: needsConversion ? '#fffbea' : '#f8f9fa', borderRadius: '4px', border: `1px solid ${needsConversion ? '#f39c12' : '#e0e6ed'}` }}>
        <input type="checkbox" checked={needsConversion} onChange={e => onNeedsConversionChange(e.target.checked)} />
        <span>🔄 Mark for Go conversion</span>
      </label>
      {needsConversion && (
        <div style={{ fontSize: '10px', color: '#e67e22', marginTop: '4px', lineHeight: 1.5 }}>
          Flagged — rewrite this as a native Go activity for production use.
        </div>
      )}
    </>
  )
}

// ── Fuzzy field matching helpers ─────────────────────────────────────────────

function levenshtein(a: string, b: string): number {
  const m = a.length, n = b.length
  const dp: number[] = Array.from({ length: n + 1 }, (_, i) => i)
  for (let i = 1; i <= m; i++) {
    let prev = i
    for (let j = 1; j <= n; j++) {
      const cur = a[i - 1] === b[j - 1] ? dp[j - 1] : 1 + Math.min(dp[j - 1], dp[j], prev)
      dp[j - 1] = prev
      prev = cur
    }
    dp[n] = prev
  }
  return dp[n]
}

// Given a field name and a list of source paths, return the best-matching path
// or null if no reasonable match exists. Scoring priority:
//  1. Exact match on full path
//  2. Last dot-segment exactly equals field name
//  3. Lowest Levenshtein distance on last segment (only if distance <= half field length)
function bestMatch(field: string, paths: string[]): string | null {
  if (!paths.length) return null
  const norm = (s: string) => s.toLowerCase().replace(/[_\s]/g, '')
  const nf = norm(field)

  // 1. Exact full path
  const exact = paths.find(p => norm(p) === nf)
  if (exact) return exact

  // 2. Last segment exact
  const segExact = paths.find(p => norm(p.split('.').pop()!) === nf)
  if (segExact) return segExact

  // 3. Levenshtein on last segment
  let best: string | null = null
  let bestDist = Infinity
  for (const p of paths) {
    const seg = p.split('.').pop()!
    const dist = levenshtein(nf, norm(seg))
    if (dist < bestDist) { bestDist = dist; best = p }
  }
  // Only accept if distance is at most half the field name length
  if (best && bestDist <= Math.max(2, Math.floor(nf.length / 2))) return best
  return null
}

// ── PbQueryBuilderModal ───────────────────────────────────────────────────────

interface QueryFilter { id: string; field: string; operator: string; value: string }
interface QueryConfig {
  tableName: string
  resultKey: string
  filters: QueryFilter[]
  filterMode: 'and' | 'or'
  sortField: string
  sortDir: 'asc' | 'desc'
  limit: number
}

const PB_OPERATORS: Record<string, Array<{ value: string; label: string }>> = {
  text:   [{ value: 'eq', label: '= equals' }, { value: 'neq', label: '≠ not equals' }, { value: 'contains', label: '~ contains' }, { value: 'not_contains', label: '!~ doesn\'t contain' }],
  number: [{ value: 'eq', label: '= equals' }, { value: 'neq', label: '≠ not equals' }, { value: 'gt', label: '> greater' }, { value: 'gte', label: '≥ ≥ or equal' }, { value: 'lt', label: '< less' }, { value: 'lte', label: '≤ ≤ or equal' }],
  bool:   [{ value: 'eq', label: '= equals' }, { value: 'neq', label: '≠ not equals' }],
  date:   [{ value: 'eq', label: '= equals' }, { value: 'neq', label: '≠ not equals' }, { value: 'gt', label: '> after' }, { value: 'gte', label: '≥ on or after' }, { value: 'lt', label: '< before' }, { value: 'lte', label: '≤ on or before' }],
}
const ALL_OPERATORS = [
  { value: 'eq', label: '= equals' }, { value: 'neq', label: '≠ not equals' },
  { value: 'gt', label: '> greater' }, { value: 'gte', label: '≥ or equal' },
  { value: 'lt', label: '< less' }, { value: 'lte', label: '≤ or equal' },
  { value: 'contains', label: '~ contains' }, { value: 'not_contains', label: '!~ doesn\'t contain' },
]
function getOperatorsForType(type: string) {
  if (type === 'number' || type === 'int' || type === 'float') return PB_OPERATORS.number
  if (type === 'bool') return PB_OPERATORS.bool
  if (type === 'date' || type === 'datetime' || type === 'autodate') return PB_OPERATORS.date
  if (type === 'text' || type === 'email' || type === 'url' || type === 'editor' || type === 'select') return PB_OPERATORS.text
  return ALL_OPERATORS
}

const LIMIT_OPTIONS = [1, 5, 10, 25, 50, 100, 250, 500]
const qid = () => Math.random().toString(36).slice(2, 8)

function PbQueryBuilderModal({ existing, defaultSourceJson, onSave, onClose }: {
  existing: QueryConfig
  defaultSourceJson: string
  onSave: (cfg: QueryConfig) => void
  onClose: () => void
}) {
  const dragRef = useRef<string>('')
  const [tableName, setTableName] = useState(existing.tableName)
  const [resultKey, setResultKey] = useState(existing.resultKey)
  const [filters, setFilters] = useState<QueryFilter[]>(existing.filters.length ? existing.filters : [{ id: qid(), field: '', operator: 'eq', value: '' }])
  const [filterMode, setFilterMode] = useState<'and' | 'or'>(existing.filterMode)
  const [sortField, setSortField] = useState(existing.sortField)
  const [sortDir, setSortDir] = useState<'asc' | 'desc'>(existing.sortDir)
  const [limit, setLimit] = useState(existing.limit || 50)
  const [schema, setSchema] = useState<Array<{ name: string; type: string }>>([])
  const [schemaLoading, setSchemaLoading] = useState(false)
  const [sourceText, setSourceText] = useState(defaultSourceJson)
  const [sourceParseError, setSourceParseError] = useState('')
  const [sourcePaths, setSourcePaths] = useState<string[]>([])
  const [sourceValues, setSourceValues] = useState<Record<string, unknown>>({})
  const [sourceFilter, setSourceFilter] = useState(() => {
    try {
      const p = JSON.parse(defaultSourceJson)
      if (p && typeof p === 'object' && 'item' in p) return 'item.'
    } catch { /* ignore */ }
    return ''
  })
  const [hiddenSourceKeys, setHiddenSourceKeys] = useState<Set<string>>(new Set())
  const [dragOverIdx, setDragOverIdx] = useState<number | null>(null)

  // Auto-parse context keys on open
  useEffect(() => {
    if (!defaultSourceJson.trim()) return
    try {
      const parsed = JSON.parse(defaultSourceJson)
      const out: string[] = []
      collectLeafPaths(parsed, '', 0, out)
      setSourcePaths(out.filter(p => p !== ''))
      setSourceValues(buildValueMap(parsed))
    } catch { /* ignore */ }
  }, [defaultSourceJson])

  // Fetch schema when tableName changes
  useEffect(() => {
    if (!tableName.trim()) return
    setSchemaLoading(true)
    fetch(`/api/pb/collections/${encodeURIComponent(tableName)}/fields`)
      .then(r => r.json())
      .then((data: { fields?: Array<{ name: string; type: string }> }) => {
        if (data.fields) setSchema(data.fields)
      })
      .catch(() => {})
      .finally(() => setSchemaLoading(false))
  }, [tableName])

  const parseSource = () => {
    setSourceParseError('')
    setSourcePaths([])
    let parsed: unknown
    try { parsed = JSON.parse(sourceText) } catch { setSourceParseError('Invalid JSON.'); return }
    const out: string[] = []
    collectLeafPaths(parsed, '', 0, out)
    setSourcePaths(out.filter(p => p !== ''))
    setSourceValues(buildValueMap(parsed))
  }

  const addFilter = () => setFilters(prev => [...prev, { id: qid(), field: '', operator: 'eq', value: '' }])
  const removeFilter = (id: string) => setFilters(prev => prev.filter(f => f.id !== id))
  const updateFilter = (id: string, patch: Partial<QueryFilter>) =>
    setFilters(prev => prev.map(f => f.id === id ? { ...f, ...patch } : f))

  const schemaTypeFor = (fieldName: string) => schema.find(s => s.name === fieldName)?.type ?? ''

  const handleSave = () => {
    onSave({ tableName, resultKey, filters: filters.filter(f => f.field), filterMode, sortField, sortDir, limit })
    onClose()
  }

  // Styles
  const selStyle: React.CSSProperties = { padding: '4px 6px', border: '1px solid #ddd', borderRadius: '4px', fontSize: '12px', background: 'white', cursor: 'pointer' }
  const valInputStyle: React.CSSProperties = { flex: 1, padding: '4px 8px', border: '1px solid #ddd', borderRadius: '4px', fontSize: '12px', fontFamily: 'monospace', minWidth: 0 }

  return (
    <div style={{ position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.65)', display: 'flex', alignItems: 'center', justifyContent: 'center', zIndex: 2100 }}>
      <div style={{ background: 'white', borderRadius: '10px', width: '900px', maxHeight: '88vh', display: 'flex', flexDirection: 'column', boxShadow: '0 24px 64px rgba(0,0,0,0.45)', overflow: 'hidden' }}>

        {/* Header */}
        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', padding: '14px 20px', background: '#1e293b', color: 'white' }}>
          <div>
            <div style={{ fontWeight: 700, fontSize: '15px' }}>
              PocketBase Query Builder
              {tableName && <span style={{ marginLeft: '8px', fontSize: '12px', color: '#7c3aed', background: '#ede9fe', borderRadius: '4px', padding: '1px 6px' }}>{tableName}</span>}
              {schemaLoading && <span style={{ marginLeft: '8px', fontSize: '11px', color: '#94a3b8' }}>loading schema…</span>}
            </div>
            <div style={{ fontSize: '11px', color: '#94a3b8', marginTop: '2px' }}>Build a filter query visually — drag context keys into value fields</div>
          </div>
          <button onClick={onClose} style={{ background: 'none', border: 'none', cursor: 'pointer', fontSize: '18px', color: '#94a3b8', lineHeight: 1 }}>✕</button>
        </div>

        {/* Body */}
        <div style={{ flex: 1, overflow: 'auto', display: 'flex', gap: 0, minHeight: 0 }}>

          {/* LEFT — Context Keys */}
          <div style={{ flex: '0 0 260px', borderRight: '1px solid #e0e6ed', padding: '16px', display: 'flex', flexDirection: 'column', gap: '8px', overflow: 'auto' }}>
            <div style={{ fontSize: '11px', fontWeight: 700, color: '#555', textTransform: 'uppercase', letterSpacing: '0.5px' }}>
              Context Keys <span style={{ fontWeight: 400, color: '#aaa', textTransform: 'none' }}>(drag into values)</span>
            </div>
            <textarea
              value={sourceText}
              onChange={e => setSourceText(e.target.value)}
              rows={6}
              spellCheck={false}
              placeholder={'{\n  "email": "user@example.com"\n}'}
              style={{ width: '100%', boxSizing: 'border-box', fontFamily: 'monospace', fontSize: '11px', padding: '6px 8px', border: '1px solid #ddd', borderRadius: '4px', resize: 'vertical' }}
            />
            {sourceParseError && <div style={{ color: '#e74c3c', fontSize: '11px' }}>{sourceParseError}</div>}
            <button onClick={parseSource} style={{ padding: '5px', background: '#3b82f6', color: 'white', border: 'none', borderRadius: '4px', cursor: 'pointer', fontWeight: 600, fontSize: '11px' }}>
              Parse Keys
            </button>
            {sourcePaths.length > 0 && (
              <>
                <div style={{ display: 'flex', alignItems: 'center', gap: '6px', marginTop: '4px' }}>
                  <div style={{ fontSize: '10px', fontWeight: 700, color: '#555', whiteSpace: 'nowrap' }}>SOURCE KEYS:</div>
                  <input type="text" placeholder="filter..." value={sourceFilter} onChange={e => setSourceFilter(e.target.value)}
                    style={{ flex: 1, fontSize: '10px', padding: '3px 6px', border: '1px solid #ddd', borderRadius: '4px', fontFamily: 'monospace', minWidth: 0 }} />
                  {sourceFilter && <button onClick={() => setSourceFilter('')} style={{ background: 'none', border: 'none', cursor: 'pointer', color: '#aaa', fontSize: '12px', padding: 0, lineHeight: 1 }}>✕</button>}
                </div>
                <div style={{ display: 'flex', flexWrap: 'wrap', gap: '5px' }}>
                  {sourcePaths.filter(p => !hiddenSourceKeys.has(p) && (!sourceFilter || p.toLowerCase().includes(sourceFilter.toLowerCase()))).map(p => (
                    <div key={p} draggable onDragStart={() => { dragRef.current = p }} onDragEnd={() => { dragRef.current = '' }}
                      title={p in sourceValues ? fmtVal(sourceValues[p]) : undefined}
                      style={{ display: 'inline-flex', alignItems: 'center', gap: '3px', background: '#dbeafe', border: '1px solid #93c5fd', borderRadius: '4px', padding: '3px 7px', fontFamily: 'monospace', fontSize: '10px', color: '#1e40af', cursor: 'grab', userSelect: 'none' }}>
                      <span style={{ pointerEvents: 'none' }}>{p}</span>
                      <span
                        draggable={false}
                        onMouseDown={e => { e.stopPropagation(); e.preventDefault() }}
                        onClick={e => { e.stopPropagation(); setHiddenSourceKeys(prev => new Set([...prev, p])) }}
                        style={{ cursor: 'pointer', color: '#93c5fd', fontSize: '9px', lineHeight: 1, paddingLeft: '2px', pointerEvents: 'all' }}
                        title="Hide this key"
                      >✕</span>
                    </div>
                  ))}
                  {hiddenSourceKeys.size > 0 && (
                    <button onClick={() => setHiddenSourceKeys(new Set())} style={{ fontSize: '10px', color: '#6b7280', background: 'none', border: 'none', cursor: 'pointer', padding: '2px 4px', textDecoration: 'underline', alignSelf: 'center' }}>
                      show {hiddenSourceKeys.size} hidden
                    </button>
                  )}
                </div>
              </>
            )}
          </div>

          {/* RIGHT — Query config */}
          <div style={{ flex: 1, padding: '16px', display: 'flex', flexDirection: 'column', gap: '14px', overflow: 'auto' }}>

            {/* Row 1: Table + Result Key + Limit */}
            <div style={{ display: 'flex', gap: '12px', alignItems: 'flex-end' }}>
              <div style={{ flex: 1 }}>
                <label style={{ fontSize: '11px', fontWeight: 600, color: '#555', display: 'block', marginBottom: '4px' }}>Table</label>
                <input value={tableName} onChange={e => setTableName(e.target.value)} style={{ ...selStyle, width: '100%', boxSizing: 'border-box' }} placeholder="collection name" />
              </div>
              <div style={{ flex: 1 }}>
                <label style={{ fontSize: '11px', fontWeight: 600, color: '#555', display: 'block', marginBottom: '4px' }}>
                  Result Key
                  <span title="Stores all output under this context key, e.g. &quot;contacts&quot; → {{contacts.records}}. Required when using multiple pb_query nodes." style={{ marginLeft: '4px', color: '#0ea5e9', cursor: 'help' }}>ℹ</span>
                </label>
                <input value={resultKey} onChange={e => setResultKey(e.target.value)} style={{ ...selStyle, width: '100%', boxSizing: 'border-box' }} placeholder="e.g. contacts" />
              </div>
              <div>
                <label style={{ fontSize: '11px', fontWeight: 600, color: '#555', display: 'block', marginBottom: '4px' }}>Limit</label>
                <select value={limit} onChange={e => setLimit(Number(e.target.value))} style={selStyle}>
                  {LIMIT_OPTIONS.map(l => <option key={l} value={l}>{l}</option>)}
                </select>
              </div>
            </div>

            {/* Row 2: Sort */}
            <div style={{ display: 'flex', gap: '12px', alignItems: 'flex-end' }}>
              <div style={{ flex: 1 }}>
                <label style={{ fontSize: '11px', fontWeight: 600, color: '#555', display: 'block', marginBottom: '4px' }}>Sort by</label>
                {schema.length > 0
                  ? <select value={sortField} onChange={e => setSortField(e.target.value)} style={{ ...selStyle, width: '100%' }}>
                      <option value="">— none —</option>
                      {schema.map(f => <option key={f.name} value={f.name}>{f.name}</option>)}
                    </select>
                  : <input value={sortField} onChange={e => setSortField(e.target.value)} style={{ ...selStyle, width: '100%', boxSizing: 'border-box' }} placeholder="field name" />
                }
              </div>
              <div>
                <label style={{ fontSize: '11px', fontWeight: 600, color: '#555', display: 'block', marginBottom: '4px' }}>Direction</label>
                <select value={sortDir} onChange={e => setSortDir(e.target.value as 'asc' | 'desc')} style={selStyle}>
                  <option value="asc">ASC ↑</option>
                  <option value="desc">DESC ↓</option>
                </select>
              </div>
            </div>

            {/* Filters section */}
            <div style={{ borderTop: '1px solid #e0e6ed', paddingTop: '14px' }}>
              <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: '10px' }}>
                <div style={{ display: 'flex', alignItems: 'center', gap: '10px' }}>
                  <span style={{ fontSize: '12px', fontWeight: 700, color: '#334155' }}>Filters</span>
                  <div style={{ display: 'flex', gap: '2px', border: '1px solid #ddd', borderRadius: '4px', overflow: 'hidden' }}>
                    {(['and', 'or'] as const).map(m => (
                      <button key={m} onClick={() => setFilterMode(m)}
                        style={{ padding: '3px 10px', fontSize: '11px', fontWeight: 600, border: 'none', cursor: 'pointer', background: filterMode === m ? '#334155' : 'white', color: filterMode === m ? 'white' : '#555' }}>
                        {m.toUpperCase()}
                      </button>
                    ))}
                  </div>
                  <span style={{ fontSize: '11px', color: '#94a3b8' }}>
                    {filterMode === 'and' ? 'All conditions must match' : 'Any condition must match'}
                  </span>
                </div>
                <button onClick={addFilter}
                  style={{ padding: '4px 12px', background: '#3b82f6', color: 'white', border: 'none', borderRadius: '4px', cursor: 'pointer', fontWeight: 600, fontSize: '11px' }}>
                  + Add Filter
                </button>
              </div>

              {filters.length === 0 && (
                <div style={{ textAlign: 'center', padding: '20px', color: '#94a3b8', fontSize: '12px', border: '1px dashed #ddd', borderRadius: '6px' }}>
                  No filters — returns all records (up to limit)
                </div>
              )}

              <div style={{ display: 'flex', flexDirection: 'column', gap: '6px' }}>
                {filters.map((f, idx) => {
                  const fieldType = schemaTypeFor(f.field)
                  const ops = getOperatorsForType(fieldType)
                  const isOver = dragOverIdx === idx
                  return (
                    <div key={f.id}
                      style={{ display: 'flex', gap: '6px', alignItems: 'center', padding: '8px', background: '#f8fafc', borderRadius: '6px', border: '1px solid #e0e6ed' }}>
                      {/* Field */}
                      {schema.length > 0
                        ? <select value={f.field} onChange={e => updateFilter(f.id, { field: e.target.value, operator: 'eq' })} style={{ ...selStyle, minWidth: '130px' }}>
                            <option value="">— field —</option>
                            {schema.map(s => <option key={s.name} value={s.name}>{s.name}</option>)}
                          </select>
                        : <input value={f.field} onChange={e => updateFilter(f.id, { field: e.target.value })}
                            style={{ ...selStyle, width: '120px' }} placeholder="field" />
                      }
                      {/* Operator */}
                      <select value={f.operator} onChange={e => updateFilter(f.id, { operator: e.target.value })} style={{ ...selStyle, minWidth: '150px' }}>
                        {ops.map(op => <option key={op.value} value={op.value}>{op.label}</option>)}
                      </select>
                      {/* Value — droppable */}
                      <div style={{ flex: 1, position: 'relative' }}
                        onDragOver={e => { e.preventDefault(); setDragOverIdx(idx) }}
                        onDragLeave={() => setDragOverIdx(null)}
                        onDrop={() => {
                          if (dragRef.current) updateFilter(f.id, { value: `{{${dragRef.current}}}` })
                          setDragOverIdx(null)
                        }}>
                        <input
                          value={f.value}
                          onChange={e => updateFilter(f.id, { value: e.target.value })}
                          style={{ ...valInputStyle, border: isOver ? '2px solid #22c55e' : '1px solid #ddd', background: isOver ? '#f0fdf4' : 'white' }}
                          placeholder="value or {{context.key}}"
                        />
                      </div>
                      {/* Remove */}
                      <button onClick={() => removeFilter(f.id)}
                        style={{ padding: '4px 8px', background: 'none', border: '1px solid #fca5a5', borderRadius: '4px', cursor: 'pointer', color: '#ef4444', fontSize: '13px', lineHeight: 1, flexShrink: 0 }}>
                        ✕
                      </button>
                    </div>
                  )
                })}
              </div>
            </div>
          </div>
        </div>

        {/* Footer */}
        <div style={{ display: 'flex', justifyContent: 'flex-end', gap: '10px', padding: '12px 20px', borderTop: '1px solid #e0e6ed', background: '#f8f9fa' }}>
          <button onClick={onClose} style={{ padding: '8px 16px', background: '#f0f0f0', border: 'none', borderRadius: '4px', cursor: 'pointer', fontWeight: 500, fontSize: '13px' }}>
            Cancel
          </button>
          <button onClick={handleSave} style={{ padding: '8px 20px', background: '#0ea5e9', color: 'white', border: 'none', borderRadius: '4px', cursor: 'pointer', fontWeight: 600, fontSize: '13px' }}>
            Apply Query
          </button>
        </div>
      </div>
    </div>
  )
}

// ── PbDataMapperModal (pb_update / pb_create) ────────────────────────────────

function PbDataMapperModal({ existingData, existingId, existingResultKey, defaultSourceJson, tableName, onSave, onSaveId, onSaveResultKey, onClose, mode }: {
  existingData: string
  existingId: string
  existingResultKey: string
  defaultSourceJson: string
  tableName: string
  onSave: (data: string) => void
  onSaveId: (id: string) => void
  onSaveResultKey: (key: string) => void
  onClose: () => void
  mode: 'create' | 'update' | 'upsert' | 'delete'
}) {
  const needsId = mode === 'update' || mode === 'upsert' || mode === 'delete'
  const hasDataFields = mode !== 'delete'
  const dragRef = useRef<string>('')
  const [sourceText, setSourceText] = useState(defaultSourceJson)
  const [sourceParseError, setSourceParseError] = useState('')
  const [sourcePaths, setSourcePaths] = useState<string[]>([])
  const [sourceValues, setSourceValues] = useState<Record<string, unknown>>({})
  const [sourceFilter, setSourceFilter] = useState(() => {
    try {
      const p = JSON.parse(defaultSourceJson)
      if (p && typeof p === 'object' && 'item' in p) return 'item.'
    } catch { /* ignore */ }
    return ''
  })
  const [hiddenSourceKeys, setHiddenSourceKeys] = useState<Set<string>>(new Set())
  const [schemaLoading, setSchemaLoading] = useState(false)
  const [rows, setRows] = useState<Array<{ field: string; value: string }>>(() => {
    try {
      const parsed = JSON.parse(existingData)
      if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) {
        return Object.entries(parsed as Record<string, string>).map(([field, value]) => ({ field, value: String(value) }))
      }
    } catch { /* fall through */ }
    return [{ field: '', value: '' }]
  })
  const [dragOverIdx, setDragOverIdx] = useState<number | null>(null)
  const [idValue, setIdValue] = useState(existingId)
  const [dragOverId, setDragOverId] = useState(false)
  const [resultKey, setResultKey] = useState(existingResultKey)

  useEffect(() => {
    if (defaultSourceJson.trim() === '') return
    try {
      const parsed = JSON.parse(defaultSourceJson)
      const out: string[] = []
      collectLeafPaths(parsed, '', 0, out)
      setSourcePaths(out.filter(p => p !== ''))
      setSourceValues(buildValueMap(parsed))
    } catch { /* ignore */ }
  }, [defaultSourceJson])

  // Fetch schema fields when tableName is provided
  useEffect(() => {
    if (!tableName) return
    setSchemaLoading(true)
    fetch(`/api/pb/collections/${encodeURIComponent(tableName)}/fields`)
      .then(r => r.json())
      .then((data: { fields?: Array<{ name: string; type: string }> }) => {
        if (!data.fields) return
        setRows(prev => {
          const existingFields = new Set(prev.filter(r => r.field).map(r => r.field))
          const newRows = data.fields!
            .filter(f => !existingFields.has(f.name))
            .map(f => ({ field: f.name, value: '' }))
          const base = prev.filter(r => r.field) // drop blank placeholder rows
          return [...base, ...newRows]
        })
      })
      .catch(() => { /* schema fetch failed — silently ignore */ })
      .finally(() => setSchemaLoading(false))
  }, [tableName])

  const parseSource = () => {
    setSourceParseError('')
    setSourcePaths([])
    let parsed: unknown
    try { parsed = JSON.parse(sourceText) } catch { setSourceParseError('Invalid JSON.'); return }
    const out: string[] = []
    collectLeafPaths(parsed, '', 0, out)
    setSourcePaths(out.filter(p => p !== ''))
    setSourceValues(buildValueMap(parsed))
  }

  const handleDrop = (i: number) => {
    const from = dragRef.current
    if (!from) return
    setRows(prev => prev.map((r, idx) => idx === i ? { ...r, value: `{{${from}}}` } : r))
    setDragOverIdx(null)
  }

  const visibleSourcePaths = sourcePaths.filter(p =>
    !hiddenSourceKeys.has(p) && (!sourceFilter || p.toLowerCase().includes(sourceFilter.toLowerCase()))
  )

  const handleBestGuess = () => {
    setRows(prev => prev.map(row => {
      if (!row.field || row.value) return row // skip already-mapped or blank field rows
      const match = bestMatch(row.field, visibleSourcePaths)
      return match ? { ...row, value: `{{${match}}}` } : row
    }))
  }

  const handleSave = () => {
    if (needsId) onSaveId(idValue)
    onSaveResultKey(resultKey)
    if (hasDataFields) {
      const obj: Record<string, string> = {}
      for (const r of rows) {
        if (r.field.trim() !== '') obj[r.field.trim()] = r.value
      }
      onSave(JSON.stringify(obj))
    }
    onClose()
  }

  const titles: Record<string, string> = {
    update: 'PocketBase Update Mapper',
    create: 'PocketBase Create Mapper',
    upsert: 'PocketBase Upsert Mapper',
    delete: 'PocketBase Delete',
  }
  const subtitles: Record<string, string> = {
    update: 'Merge update — only specified fields change; omitted fields are untouched.',
    create: 'Create — all specified fields are set on the new record.',
    upsert: 'Upsert — updates the record if id is supplied, creates a new one if not.',
    delete: 'Deletes the record with the given id.',
  }

  return (
    <div style={{ position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.65)', display: 'flex', alignItems: 'center', justifyContent: 'center', zIndex: 2100 }}>
      <div style={{ background: 'white', borderRadius: '10px', width: '860px', maxHeight: '85vh', display: 'flex', flexDirection: 'column', boxShadow: '0 24px 64px rgba(0,0,0,0.45)', overflow: 'hidden' }}>

        {/* Header */}
        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', padding: '14px 20px', background: '#1e293b', color: 'white' }}>
          <div>
            <div style={{ fontWeight: 700, fontSize: '15px' }}>
              {titles[mode]}
              {tableName && <span style={{ marginLeft: '8px', fontSize: '12px', color: '#7c3aed', background: '#ede9fe', borderRadius: '4px', padding: '1px 6px' }}>{tableName}</span>}
              {schemaLoading && <span style={{ marginLeft: '8px', fontSize: '11px', color: '#94a3b8' }}>loading schema…</span>}
            </div>
            <div style={{ fontSize: '11px', color: '#94a3b8', marginTop: '2px' }}>{subtitles[mode]}</div>
          </div>
          <button onClick={onClose} style={{ background: 'none', border: 'none', cursor: 'pointer', fontSize: '18px', color: '#94a3b8', lineHeight: 1 }}>✕</button>
        </div>

        {/* Result Key bar */}
        <div style={{ display: 'flex', alignItems: 'center', gap: '10px', padding: '8px 20px', background: '#f8fafc', borderBottom: '1px solid #e0e6ed' }}>
          <label style={{ fontSize: '11px', fontWeight: 600, color: '#555', whiteSpace: 'nowrap' }}>
            Result Key
            <span title="Store output under this context key (e.g. &quot;contact&quot; → {{contact.id}}, {{contact.record}}). Useful when multiple PB nodes share similar output keys." style={{ marginLeft: '4px', color: '#6b46c1', cursor: 'help' }}>ℹ</span>
          </label>
          <input
            value={resultKey}
            onChange={e => setResultKey(e.target.value)}
            placeholder="optional — e.g. contact"
            style={{ width: '200px', padding: '4px 8px', border: '1px solid #ddd', borderRadius: '4px', fontSize: '12px', fontFamily: 'monospace' }}
          />
          {resultKey && <span style={{ fontSize: '11px', color: '#6b46c1', fontFamily: 'monospace' }}>→ {'{{'}{ resultKey }.id{'}}'}, {'{{'}{ resultKey }.record{'}}'}</span>}
        </div>

        {/* Two-column body */}
        <div style={{ flex: 1, overflow: 'auto', display: 'flex', gap: 0, minHeight: 0 }}>

          {/* LEFT — Source Keys */}
          <div style={{ flex: '0 0 280px', borderRight: '1px solid #e0e6ed', padding: '16px', display: 'flex', flexDirection: 'column', gap: '8px', overflow: 'auto' }}>
            <div style={{ fontSize: '11px', fontWeight: 700, color: '#555', textTransform: 'uppercase', letterSpacing: '0.5px' }}>
              Context Keys <span style={{ fontWeight: 400, color: '#aaa', textTransform: 'none' }}>(drag from here)</span>
            </div>
            <div style={{ fontSize: '10px', color: '#aaa', lineHeight: 1.5 }}>
              Paste sample JSON from the workflow context, then click Parse Keys.
            </div>
            <textarea
              value={sourceText}
              onChange={e => setSourceText(e.target.value)}
              rows={6}
              placeholder={'{\n  "email": "user@example.com",\n  "name": "Jane"\n}'}
              spellCheck={false}
              style={{ width: '100%', boxSizing: 'border-box', fontFamily: 'monospace', fontSize: '11px', padding: '6px 8px', border: '1px solid #ddd', borderRadius: '4px', resize: 'vertical' }}
            />
            {sourceParseError && <div style={{ color: '#e74c3c', fontSize: '11px' }}>{sourceParseError}</div>}
            <button
              onClick={parseSource}
              style={{ padding: '5px', background: '#3b82f6', color: 'white', border: 'none', borderRadius: '4px', cursor: 'pointer', fontWeight: 600, fontSize: '11px' }}
            >
              Parse Keys
            </button>

            {sourcePaths.length > 0 && (
              <>
                <div style={{ display: 'flex', alignItems: 'center', gap: '6px', marginTop: '4px' }}>
                  <div style={{ fontSize: '10px', fontWeight: 700, color: '#555', whiteSpace: 'nowrap' }}>SOURCE KEYS:</div>
                  <input
                    type="text"
                    placeholder="filter..."
                    value={sourceFilter}
                    onChange={e => setSourceFilter(e.target.value)}
                    style={{ flex: 1, fontSize: '10px', padding: '3px 6px', border: '1px solid #ddd', borderRadius: '4px', fontFamily: 'monospace', minWidth: 0 }}
                  />
                  {sourceFilter && (
                    <button onClick={() => setSourceFilter('')} style={{ background: 'none', border: 'none', cursor: 'pointer', color: '#aaa', fontSize: '12px', padding: 0, lineHeight: 1 }}>✕</button>
                  )}
                </div>
                <div style={{ display: 'flex', flexWrap: 'wrap', gap: '5px' }}>
                  {sourcePaths.filter(p => !hiddenSourceKeys.has(p) && (!sourceFilter || p.toLowerCase().includes(sourceFilter.toLowerCase()))).map(p => (
                    <div
                      key={p}
                      draggable
                      onDragStart={() => { dragRef.current = p }}
                      onDragEnd={() => { dragRef.current = '' }}
                      title={p in sourceValues ? fmtVal(sourceValues[p]) : undefined}
                      style={{
                        display: 'inline-flex', alignItems: 'center', gap: '3px',
                        background: '#dbeafe', border: '1px solid #93c5fd',
                        borderRadius: '4px', padding: '3px 7px',
                        fontFamily: 'monospace', fontSize: '10px',
                        color: '#1e40af', cursor: 'grab', userSelect: 'none',
                      }}
                    >
                      <span style={{ pointerEvents: 'none' }}>{p}</span>
                      <span
                        draggable={false}
                        onMouseDown={e => { e.stopPropagation(); e.preventDefault() }}
                        onClick={e => { e.stopPropagation(); setHiddenSourceKeys(prev => new Set([...prev, p])) }}
                        style={{ cursor: 'pointer', color: '#93c5fd', fontSize: '9px', lineHeight: 1, paddingLeft: '2px', pointerEvents: 'all' }}
                        title="Hide this key"
                      >✕</span>
                    </div>
                  ))}
                </div>
                {hiddenSourceKeys.size > 0 && (
                  <button onClick={() => setHiddenSourceKeys(new Set())} style={{ alignSelf: 'flex-start', fontSize: '10px', color: '#6b7280', background: 'none', border: 'none', cursor: 'pointer', padding: '2px 0', textDecoration: 'underline' }}>
                    show {hiddenSourceKeys.size} hidden
                  </button>
                )}
              </>
            )}
          </div>

          {/* RIGHT — Field rows */}
          <div style={{ flex: 1, padding: '16px', display: 'flex', flexDirection: 'column', gap: '8px', overflow: 'auto' }}>

            {/* Pinned Record ID row */}
            {needsId && (
              <div style={{ background: '#faf5ff', border: '1px solid #e9d5ff', borderRadius: '6px', padding: '10px 12px', marginBottom: '4px' }}>
                <div style={{ fontSize: '10px', fontWeight: 700, color: '#7c3aed', textTransform: 'uppercase', letterSpacing: '0.5px', marginBottom: '6px' }}>
                  Record ID <span style={{ fontWeight: 400, color: '#a78bfa', textTransform: 'none' }}>(required)</span>
                </div>
                <div
                  onDragOver={e => { e.preventDefault(); setDragOverId(true) }}
                  onDragLeave={() => setDragOverId(false)}
                  onDrop={() => { if (dragRef.current) { setIdValue(`{{${dragRef.current}}}`); dragRef.current = ''; setDragOverId(false) } }}
                  style={{
                    display: 'flex', alignItems: 'center', minHeight: '32px',
                    background: dragOverId ? '#ede9fe' : 'white',
                    border: `1px dashed ${dragOverId ? '#7c3aed' : '#c4b5fd'}`,
                    borderRadius: '4px', transition: 'background 0.1s, border-color 0.1s', overflow: 'hidden',
                  }}
                >
                  <input
                    value={idValue}
                    onChange={e => setIdValue(e.target.value)}
                    placeholder="{{id}} or literal id value"
                    title={resolveTitle(idValue, sourceValues)}
                    style={{ flex: 1, fontFamily: 'monospace', fontSize: '11px', padding: '5px 8px', border: 'none', background: 'transparent', outline: 'none', color: idValue.startsWith('{{') ? '#6d28d9' : '#333' }}
                  />
                  {idValue && <button onClick={() => setIdValue('')} style={{ background: 'none', border: 'none', cursor: 'pointer', color: '#c4b5fd', fontSize: '12px', padding: '0 8px', lineHeight: 1 }}>✕</button>}
                </div>
              </div>
            )}

            {hasDataFields && <>
            <div style={{ fontSize: '11px', fontWeight: 700, color: '#555', textTransform: 'uppercase', letterSpacing: '0.5px' }}>
              Field Mappings <span style={{ fontWeight: 400, color: '#aaa', textTransform: 'none' }}>(drop context keys onto value cells)</span>
            </div>
            {rows.map((row, i) => (
              <div key={i} style={{ display: 'flex', gap: '6px', alignItems: 'center' }}>
                <input
                  value={row.field}
                  onChange={e => setRows(prev => prev.map((r, idx) => idx === i ? { ...r, field: e.target.value } : r))}
                  placeholder="field name"
                  style={{ flex: '0 0 140px', fontFamily: 'monospace', fontSize: '11px', padding: '5px 8px', border: '1px solid #ddd', borderRadius: '4px', boxSizing: 'border-box' }}
                />
                <div
                  onDragOver={e => { e.preventDefault(); setDragOverIdx(i) }}
                  onDragLeave={() => setDragOverIdx(null)}
                  onDrop={() => handleDrop(i)}
                  style={{
                    flex: 1, minHeight: '30px', display: 'flex', alignItems: 'center',
                    background: dragOverIdx === i ? '#dcfce7' : '#f8fafc',
                    border: `1px dashed ${dragOverIdx === i ? '#22c55e' : '#cbd5e1'}`,
                    borderRadius: '4px', transition: 'background 0.1s, border-color 0.1s',
                    overflow: 'hidden',
                  }}
                >
                  <input
                    value={row.value}
                    onChange={e => setRows(prev => prev.map((r, idx) => idx === i ? { ...r, value: e.target.value } : r))}
                    placeholder="value or {{key}}"
                    title={resolveTitle(row.value, sourceValues)}
                    style={{ flex: 1, fontFamily: 'monospace', fontSize: '11px', padding: '5px 8px', border: 'none', background: 'transparent', outline: 'none', boxSizing: 'border-box' }}
                  />
                </div>
                <button
                  onClick={() => setRows(prev => prev.filter((_, idx) => idx !== i))}
                  style={{ flex: '0 0 auto', background: 'none', border: 'none', cursor: 'pointer', color: '#e74c3c', fontSize: '14px', padding: '2px 4px', lineHeight: 1 }}
                >
                  ✕
                </button>
              </div>
            ))}
            <button
              onClick={() => setRows(prev => [...prev, { field: '', value: '' }])}
              style={{ alignSelf: 'flex-start', padding: '5px 10px', background: '#f0f9ff', color: '#0369a1', border: '1px dashed #7dd3fc', borderRadius: '4px', cursor: 'pointer', fontSize: '11px', fontWeight: 600, marginTop: '4px' }}
            >
              + Add Field
            </button>
            </>}
          </div>
        </div>

        {/* Footer */}
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', gap: '10px', padding: '12px 20px', borderTop: '1px solid #e0e6ed', background: '#f8f9fa' }}>
          <button
            onClick={handleBestGuess}
            disabled={visibleSourcePaths.length === 0}
            title={visibleSourcePaths.length === 0 ? 'Parse context keys first' : `Auto-map unmapped fields using fuzzy matching against ${visibleSourcePaths.length} visible keys`}
            style={{ padding: '8px 14px', background: visibleSourcePaths.length === 0 ? '#f0f0f0' : '#ede9fe', border: `1px solid ${visibleSourcePaths.length === 0 ? '#ddd' : '#c4b5fd'}`, borderRadius: '4px', cursor: visibleSourcePaths.length === 0 ? 'not-allowed' : 'pointer', fontWeight: 600, fontSize: '12px', color: visibleSourcePaths.length === 0 ? '#aaa' : '#6b46c1' }}
          >
            ✨ Best Guess
          </button>
          <div style={{ display: 'flex', gap: '10px' }}>
            <button onClick={onClose} style={{ padding: '8px 16px', background: '#f0f0f0', border: 'none', borderRadius: '4px', cursor: 'pointer', fontWeight: 500, fontSize: '13px' }}>
              Cancel
            </button>
            <button
              onClick={handleSave}
              style={{ padding: '8px 16px', background: '#6b46c1', color: 'white', border: 'none', borderRadius: '4px', cursor: 'pointer', fontWeight: 600, fontSize: '13px' }}
            >
              Save Mappings
            </button>
          </div>
        </div>
      </div>
    </div>
  )
}

// ── TelegramButtonMapperModal (telegram_send_button) ─────────────────────────

type PayloadRow = { key: string; value: string }
type TelegramButton = { label: string; payload: PayloadRow[] }
type TelegramKeyboard = TelegramButton[][]

function emptyBtn(): TelegramButton { return { label: '', payload: [{ key: '', value: '' }] } }

// Parse a callback_data string back into PayloadRow[].
// If it's a JSON object, expand into rows. Otherwise treat as a single "data" value.
function parseCallbackData(raw: string): PayloadRow[] {
  if (!raw) return [{ key: '', value: '' }]
  try {
    const obj = JSON.parse(raw)
    if (obj && typeof obj === 'object' && !Array.isArray(obj)) {
      const rows = Object.entries(obj as Record<string, string>).map(([k, v]) => ({ key: k, value: String(v) }))
      return rows.length > 0 ? rows : [{ key: '', value: '' }]
    }
  } catch { /* not JSON — treat as plain string */ }
  return [{ key: 'data', value: raw }]
}

// Serialize PayloadRow[] to a JSON object string (callback_data).
function serializePayload(rows: PayloadRow[]): string {
  const obj: Record<string, string> = {}
  for (const r of rows) { if (r.key.trim()) obj[r.key.trim()] = r.value }
  return JSON.stringify(obj)
}

function TelegramButtonMapperModal({ existingButtons, defaultSourceJson, onSave, onClose }: {
  existingButtons: string
  defaultSourceJson: string
  onSave: (buttons: string) => void
  onClose: () => void
}) {
  const dragRef = useRef<string>('')
  const [sourceText, setSourceText] = useState(defaultSourceJson)
  const [sourceParseError, setSourceParseError] = useState('')
  const [sourcePaths, setSourcePaths] = useState<string[]>([])
  const [sourceValues, setSourceValues] = useState<Record<string, unknown>>({})
  const [sourceFilter, setSourceFilter] = useState('')
  const [hiddenSourceKeys, setHiddenSourceKeys] = useState<Set<string>>(new Set())

  const [keyboard, setKeyboard] = useState<TelegramKeyboard>(() => {
    try {
      const parsed = JSON.parse(existingButtons)
      if (Array.isArray(parsed)) {
        return (parsed as Array<Array<{ text: string; callback_data: string }>>).map(row =>
          row.map(btn => ({ label: btn.text ?? '', payload: parseCallbackData(btn.callback_data ?? '') }))
        )
      }
    } catch { /* fall through */ }
    return [[emptyBtn()]]
  })

  // drag-over tracking: { row, btn, payloadIdx } for payload value cells
  const [dragOver, setDragOver] = useState<{ row: number; btn: number; pIdx: number } | null>(null)
  const [dragOverLabel, setDragOverLabel] = useState<{ row: number; btn: number } | null>(null)

  useEffect(() => {
    if (defaultSourceJson.trim() === '') return
    try {
      const parsed = JSON.parse(defaultSourceJson)
      const out: string[] = []
      collectLeafPaths(parsed, '', 0, out)
      setSourcePaths(out.filter(p => p !== ''))
      setSourceValues(buildValueMap(parsed))
    } catch { /* ignore */ }
  }, [defaultSourceJson])

  const parseSource = () => {
    setSourceParseError('')
    setSourcePaths([])
    let parsed: unknown
    try { parsed = JSON.parse(sourceText) } catch { setSourceParseError('Invalid JSON.'); return }
    const out: string[] = []
    collectLeafPaths(parsed, '', 0, out)
    setSourcePaths(out.filter(p => p !== ''))
    setSourceValues(buildValueMap(parsed))
  }

  const setLabel = (rowIdx: number, btnIdx: number, value: string) => {
    setKeyboard(prev => prev.map((row, ri) =>
      ri !== rowIdx ? row : row.map((btn, bi) => bi !== btnIdx ? btn : { ...btn, label: value })
    ))
  }

  const setPayloadRow = (rowIdx: number, btnIdx: number, pIdx: number, field: 'key' | 'value', value: string) => {
    setKeyboard(prev => prev.map((row, ri) =>
      ri !== rowIdx ? row : row.map((btn, bi) =>
        bi !== btnIdx ? btn : {
          ...btn,
          payload: btn.payload.map((p, pi) => pi !== pIdx ? p : { ...p, [field]: value })
        }
      )
    ))
  }

  const addPayloadRow = (rowIdx: number, btnIdx: number) => {
    setKeyboard(prev => prev.map((row, ri) =>
      ri !== rowIdx ? row : row.map((btn, bi) =>
        bi !== btnIdx ? btn : { ...btn, payload: [...btn.payload, { key: '', value: '' }] }
      )
    ))
  }

  const removePayloadRow = (rowIdx: number, btnIdx: number, pIdx: number) => {
    setKeyboard(prev => prev.map((row, ri) =>
      ri !== rowIdx ? row : row.map((btn, bi) =>
        bi !== btnIdx ? btn : { ...btn, payload: btn.payload.filter((_, pi) => pi !== pIdx) }
      )
    ))
  }

  const addButton = (rowIdx: number) => {
    setKeyboard(prev => prev.map((row, ri) => ri !== rowIdx ? row : [...row, emptyBtn()]))
  }

  const removeButton = (rowIdx: number, btnIdx: number) => {
    setKeyboard(prev => prev.map((row, ri) =>
      ri !== rowIdx ? row : row.filter((_, bi) => bi !== btnIdx)
    ).filter(row => row.length > 0))
  }

  const addRow = () => setKeyboard(prev => [...prev, [emptyBtn()]])
  const removeRow = (rowIdx: number) => setKeyboard(prev => prev.filter((_, ri) => ri !== rowIdx))

  const handleSave = () => {
    const serialized = JSON.stringify(
      keyboard.map(row => row.map(btn => ({ text: btn.label, callback_data: serializePayload(btn.payload) })))
    )
    onSave(serialized)
    onClose()
  }

  const visibleSourcePaths = sourcePaths.filter(p =>
    !hiddenSourceKeys.has(p) && (!sourceFilter || p.toLowerCase().includes(sourceFilter.toLowerCase()))
  )

  return (
    <div style={{ position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.65)', display: 'flex', alignItems: 'center', justifyContent: 'center', zIndex: 2100 }}>
      <div style={{ background: 'white', borderRadius: '10px', width: '860px', maxHeight: '85vh', display: 'flex', flexDirection: 'column', boxShadow: '0 24px 64px rgba(0,0,0,0.45)', overflow: 'hidden' }}>

        {/* Header */}
        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', padding: '14px 20px', background: '#1e293b', color: 'white' }}>
          <div>
            <div style={{ fontWeight: 700, fontSize: '15px' }}>
              Telegram Button Keyboard Builder
            </div>
            <div style={{ fontSize: '11px', color: '#94a3b8', marginTop: '2px' }}>
              Build inline keyboard rows. Each row appears on a separate line in the message.
            </div>
          </div>
          <button onClick={onClose} style={{ background: 'none', border: 'none', cursor: 'pointer', fontSize: '18px', color: '#94a3b8', lineHeight: 1 }}>✕</button>
        </div>

        {/* Two-column body */}
        <div style={{ flex: 1, overflow: 'auto', display: 'flex', gap: 0, minHeight: 0 }}>

          {/* LEFT — Source Keys */}
          <div style={{ flex: '0 0 280px', borderRight: '1px solid #e0e6ed', padding: '16px', display: 'flex', flexDirection: 'column', gap: '8px', overflow: 'auto' }}>
            <div style={{ fontSize: '11px', fontWeight: 700, color: '#555', textTransform: 'uppercase', letterSpacing: '0.5px' }}>
              Context Keys <span style={{ fontWeight: 400, color: '#aaa', textTransform: 'none' }}>(drag from here)</span>
            </div>
            <div style={{ fontSize: '10px', color: '#aaa', lineHeight: 1.5 }}>
              Paste sample JSON from the workflow context, then click Parse Keys.
            </div>
            <textarea
              value={sourceText}
              onChange={e => setSourceText(e.target.value)}
              rows={6}
              placeholder={'{\n  "chat_id": "123456",\n  "item": { "label": "Yes" }\n}'}
              spellCheck={false}
              style={{ width: '100%', boxSizing: 'border-box', fontFamily: 'monospace', fontSize: '11px', padding: '6px 8px', border: '1px solid #ddd', borderRadius: '4px', resize: 'vertical' }}
            />
            {sourceParseError && <div style={{ color: '#e74c3c', fontSize: '11px' }}>{sourceParseError}</div>}
            <button
              onClick={parseSource}
              style={{ padding: '5px', background: '#3b82f6', color: 'white', border: 'none', borderRadius: '4px', cursor: 'pointer', fontWeight: 600, fontSize: '11px' }}
            >
              Parse Keys
            </button>

            {sourcePaths.length > 0 && (
              <>
                <div style={{ display: 'flex', alignItems: 'center', gap: '6px', marginTop: '4px' }}>
                  <div style={{ fontSize: '10px', fontWeight: 700, color: '#555', whiteSpace: 'nowrap' }}>SOURCE KEYS:</div>
                  <input
                    type="text"
                    placeholder="filter..."
                    value={sourceFilter}
                    onChange={e => setSourceFilter(e.target.value)}
                    style={{ flex: 1, fontSize: '10px', padding: '3px 6px', border: '1px solid #ddd', borderRadius: '4px', fontFamily: 'monospace', minWidth: 0 }}
                  />
                  {sourceFilter && (
                    <button onClick={() => setSourceFilter('')} style={{ background: 'none', border: 'none', cursor: 'pointer', color: '#aaa', fontSize: '12px', padding: 0, lineHeight: 1 }}>✕</button>
                  )}
                </div>
                <div style={{ display: 'flex', flexWrap: 'wrap', gap: '5px' }}>
                  {visibleSourcePaths.map(p => (
                    <div
                      key={p}
                      draggable
                      onDragStart={() => { dragRef.current = p }}
                      onDragEnd={() => { dragRef.current = '' }}
                      title={p in sourceValues ? fmtVal(sourceValues[p]) : undefined}
                      style={{
                        display: 'inline-flex', alignItems: 'center', gap: '3px',
                        background: '#dbeafe', border: '1px solid #93c5fd',
                        borderRadius: '4px', padding: '3px 7px',
                        fontFamily: 'monospace', fontSize: '10px',
                        color: '#1e40af', cursor: 'grab', userSelect: 'none',
                      }}
                    >
                      <span style={{ pointerEvents: 'none' }}>{p}</span>
                      <span
                        draggable={false}
                        onMouseDown={e => { e.stopPropagation(); e.preventDefault() }}
                        onClick={e => { e.stopPropagation(); setHiddenSourceKeys(prev => new Set([...prev, p])) }}
                        style={{ cursor: 'pointer', color: '#93c5fd', fontSize: '9px', lineHeight: 1, paddingLeft: '2px', pointerEvents: 'all' }}
                        title="Hide this key"
                      >✕</span>
                    </div>
                  ))}
                </div>
                {hiddenSourceKeys.size > 0 && (
                  <button onClick={() => setHiddenSourceKeys(new Set())} style={{ alignSelf: 'flex-start', fontSize: '10px', color: '#6b7280', background: 'none', border: 'none', cursor: 'pointer', padding: '2px 0', textDecoration: 'underline' }}>
                    show {hiddenSourceKeys.size} hidden
                  </button>
                )}
              </>
            )}
          </div>

          {/* RIGHT — Button keyboard builder */}
          <div style={{ flex: 1, padding: '16px', display: 'flex', flexDirection: 'column', gap: '12px', overflow: 'auto' }}>
            <div style={{ fontSize: '11px', fontWeight: 700, color: '#555', textTransform: 'uppercase', letterSpacing: '0.5px' }}>
              Keyboard Rows <span style={{ fontWeight: 400, color: '#aaa', textTransform: 'none' }}>(each row = one line of buttons)</span>
            </div>

            {keyboard.map((row, rowIdx) => (
              <div key={rowIdx} style={{ background: '#f0f9ff', border: '1px solid #bae6fd', borderRadius: '6px', padding: '10px 12px' }}>
                <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: '8px' }}>
                  <div style={{ fontSize: '10px', fontWeight: 700, color: '#0369a1', textTransform: 'uppercase', letterSpacing: '0.5px' }}>
                    Row {rowIdx + 1}
                  </div>
                  <button
                    onClick={() => removeRow(rowIdx)}
                    style={{ background: 'none', border: 'none', cursor: 'pointer', color: '#e74c3c', fontSize: '13px', padding: '1px 4px', lineHeight: 1 }}
                    title="Remove this row"
                  >✕</button>
                </div>

                <div style={{ display: 'flex', flexDirection: 'column', gap: '10px' }}>
                  {row.map((btn, btnIdx) => (
                    <div key={btnIdx} style={{ background: 'white', border: '1px solid #bae6fd', borderRadius: '6px', padding: '8px 10px' }}>
                      <div style={{ display: 'flex', alignItems: 'center', gap: '6px', marginBottom: '6px' }}>
                        <div style={{ fontSize: '10px', fontWeight: 700, color: '#0369a1', flex: '0 0 auto' }}>Button {btnIdx + 1}</div>
                        {/* Label */}
                        <div
                          onDragOver={e => { e.preventDefault(); setDragOverLabel({ row: rowIdx, btn: btnIdx }) }}
                          onDragLeave={() => setDragOverLabel(null)}
                          onDrop={() => { if (dragRef.current) { setLabel(rowIdx, btnIdx, `{{${dragRef.current}}}`); setDragOverLabel(null) } }}
                          style={{
                            flex: 1, minHeight: '28px', display: 'flex', alignItems: 'center',
                            background: dragOverLabel?.row === rowIdx && dragOverLabel?.btn === btnIdx ? '#e0f2fe' : '#f8fafc',
                            border: `1px dashed ${dragOverLabel?.row === rowIdx && dragOverLabel?.btn === btnIdx ? '#0ea5e9' : '#bae6fd'}`,
                            borderRadius: '4px', overflow: 'hidden',
                          }}
                        >
                          <input
                            value={btn.label}
                            onChange={e => setLabel(rowIdx, btnIdx, e.target.value)}
                            placeholder="Button label (text user sees)"
                            title={resolveTitle(btn.label, sourceValues)}
                            style={{ flex: 1, fontFamily: 'monospace', fontSize: '11px', padding: '4px 8px', border: 'none', background: 'transparent', outline: 'none' }}
                          />
                        </div>
                        <button onClick={() => removeButton(rowIdx, btnIdx)} style={{ background: 'none', border: 'none', cursor: 'pointer', color: '#e74c3c', fontSize: '13px', padding: '2px 4px', lineHeight: 1 }} title="Remove button">✕</button>
                      </div>

                      {/* Payload mapper */}
                      <div style={{ fontSize: '10px', fontWeight: 600, color: '#555', marginBottom: '4px', textTransform: 'uppercase', letterSpacing: '0.4px' }}>
                        Payload <span style={{ fontWeight: 400, color: '#aaa', textTransform: 'none' }}>(sent as callback_data JSON)</span>
                      </div>
                      {btn.payload.map((p, pIdx) => (
                        <div key={pIdx} style={{ display: 'flex', gap: '4px', alignItems: 'center', marginBottom: '4px' }}>
                          <input
                            value={p.key}
                            onChange={e => setPayloadRow(rowIdx, btnIdx, pIdx, 'key', e.target.value)}
                            placeholder="key"
                            style={{ flex: '0 0 110px', fontFamily: 'monospace', fontSize: '11px', padding: '4px 6px', border: '1px solid #ddd', borderRadius: '4px', boxSizing: 'border-box' }}
                          />
                          <div
                            onDragOver={e => { e.preventDefault(); setDragOver({ row: rowIdx, btn: btnIdx, pIdx }) }}
                            onDragLeave={() => setDragOver(null)}
                            onDrop={() => { if (dragRef.current) { setPayloadRow(rowIdx, btnIdx, pIdx, 'value', `{{${dragRef.current}}}`); setDragOver(null) } }}
                            style={{
                              flex: 1, minHeight: '28px', display: 'flex', alignItems: 'center',
                              background: dragOver?.row === rowIdx && dragOver?.btn === btnIdx && dragOver?.pIdx === pIdx ? '#dcfce7' : '#f8fafc',
                              border: `1px dashed ${dragOver?.row === rowIdx && dragOver?.btn === btnIdx && dragOver?.pIdx === pIdx ? '#22c55e' : '#cbd5e1'}`,
                              borderRadius: '4px', overflow: 'hidden',
                            }}
                          >
                            <input
                              value={p.value}
                              onChange={e => setPayloadRow(rowIdx, btnIdx, pIdx, 'value', e.target.value)}
                              placeholder="value or {{key}}"
                              title={resolveTitle(p.value, sourceValues)}
                              style={{ flex: 1, fontFamily: 'monospace', fontSize: '11px', padding: '4px 8px', border: 'none', background: 'transparent', outline: 'none' }}
                            />
                          </div>
                          <button onClick={() => removePayloadRow(rowIdx, btnIdx, pIdx)} style={{ background: 'none', border: 'none', cursor: 'pointer', color: '#e74c3c', fontSize: '12px', padding: '2px 4px', lineHeight: 1 }}>✕</button>
                        </div>
                      ))}
                      <button
                        onClick={() => addPayloadRow(rowIdx, btnIdx)}
                        style={{ padding: '3px 8px', background: 'transparent', color: '#6b7280', border: '1px dashed #d1d5db', borderRadius: '4px', cursor: 'pointer', fontSize: '10px', fontWeight: 600 }}
                      >+ Add Field</button>
                    </div>
                  ))}
                </div>

                <button
                  onClick={() => addButton(rowIdx)}
                  style={{ marginTop: '8px', padding: '4px 10px', background: 'transparent', color: '#0369a1', border: '1px dashed #7dd3fc', borderRadius: '4px', cursor: 'pointer', fontSize: '11px', fontWeight: 600 }}
                >
                  + Add Button
                </button>
              </div>
            ))}

            <button
              onClick={addRow}
              style={{ alignSelf: 'flex-start', padding: '6px 14px', background: '#f0f9ff', color: '#0369a1', border: '1px dashed #7dd3fc', borderRadius: '4px', cursor: 'pointer', fontSize: '11px', fontWeight: 600 }}
            >
              + Add Row
            </button>
          </div>
        </div>

        {/* Footer */}
        <div style={{ display: 'flex', justifyContent: 'flex-end', alignItems: 'center', gap: '10px', padding: '12px 20px', borderTop: '1px solid #e0e6ed', background: '#f8f9fa' }}>
          <button onClick={onClose} style={{ padding: '8px 16px', background: '#f0f0f0', border: 'none', borderRadius: '4px', cursor: 'pointer', fontWeight: 500, fontSize: '13px' }}>
            Cancel
          </button>
          <button
            onClick={handleSave}
            style={{ padding: '8px 16px', background: '#0ea5e9', color: 'white', border: 'none', borderRadius: '4px', cursor: 'pointer', fontWeight: 600, fontSize: '13px' }}
          >
            Save Keyboard
          </button>
        </div>
      </div>
    </div>
  )
}

// ── CfUpsertMapperModal (cf_upsert) ──────────────────────────────────────────

const CF_CONTACT_FIELDS = [
  'email_address', 'first_name', 'last_name', 'phone_number',
  'time_zone', 'fb_url', 'twitter_url', 'instagram_url', 'linkedin_url', 'website_url',
]

function CfUpsertMapperModal({ existingData, defaultSourceJson, onSave, onClose }: {
  existingData: string
  defaultSourceJson: string
  onSave: (data: string) => void
  onClose: () => void
}) {
  const dragRef = useRef<string>('')
  const [sourceText, setSourceText] = useState(defaultSourceJson)
  const [sourceParseError, setSourceParseError] = useState('')
  const [sourcePaths, setSourcePaths] = useState<string[]>([])
  const [sourceValues, setSourceValues] = useState<Record<string, unknown>>({})
  const [sourceFilter, setSourceFilter] = useState('')
  const [hiddenSourceKeys, setHiddenSourceKeys] = useState<Set<string>>(new Set())
  const [rows, setRows] = useState<Array<{ field: string; value: string }>>(() => {
    try {
      const parsed = JSON.parse(existingData)
      if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) {
        const existing = Object.entries(parsed as Record<string, string>).map(([field, value]) => ({ field, value: String(value) }))
        // merge with default CF fields so all fields are always visible
        const existingFields = new Set(existing.map(r => r.field))
        const defaults = CF_CONTACT_FIELDS.filter(f => !existingFields.has(f)).map(f => ({ field: f, value: '' }))
        return [...existing, ...defaults]
      }
    } catch { /* fall through */ }
    return CF_CONTACT_FIELDS.map(f => ({ field: f, value: '' }))
  })
  const [dragOverIdx, setDragOverIdx] = useState<number | null>(null)
  const [customAttrs, setCustomAttrs] = useState<Array<{ key: string; value: string }>>(() => {
    try {
      const parsed = JSON.parse(existingData)
      if (parsed?.custom_attributes) {
        const ca = typeof parsed.custom_attributes === 'string'
          ? JSON.parse(parsed.custom_attributes)
          : parsed.custom_attributes
        if (ca && typeof ca === 'object') {
          return Object.entries(ca as Record<string, string>).map(([k, v]) => ({ key: k, value: String(v) }))
        }
      }
    } catch { /* ignore */ }
    return []
  })
  const [dragOverAttrIdx, setDragOverAttrIdx] = useState<number | null>(null)

  useEffect(() => {
    if (defaultSourceJson.trim() === '') return
    try {
      const parsed = JSON.parse(defaultSourceJson)
      const out: string[] = []
      collectLeafPaths(parsed, '', 0, out)
      setSourcePaths(out.filter(p => p !== ''))
      setSourceValues(buildValueMap(parsed))
    } catch { /* ignore */ }
  }, [defaultSourceJson])

  const parseSource = () => {
    setSourceParseError('')
    setSourcePaths([])
    let parsed: unknown
    try { parsed = JSON.parse(sourceText) } catch { setSourceParseError('Invalid JSON.'); return }
    const out: string[] = []
    collectLeafPaths(parsed, '', 0, out)
    setSourcePaths(out.filter(p => p !== ''))
    setSourceValues(buildValueMap(parsed))
  }

  const handleDrop = (i: number) => {
    const from = dragRef.current
    if (!from) return
    setRows(prev => prev.map((r, idx) => idx === i ? { ...r, value: `{{${from}}}` } : r))
    setDragOverIdx(null)
  }

  const handleDropAttr = (i: number) => {
    const from = dragRef.current
    if (!from) return
    setCustomAttrs(prev => prev.map((a, idx) => idx === i ? { ...a, value: `{{${from}}}` } : a))
    setDragOverAttrIdx(null)
  }

  const handleSave = () => {
    const obj: Record<string, string> = {}
    for (const r of rows) {
      if (r.field.trim() !== '' && r.value.trim() !== '') obj[r.field.trim()] = r.value
    }
    const validAttrs = customAttrs.filter(a => a.key.trim() !== '')
    if (validAttrs.length > 0) {
      const attrsObj: Record<string, string> = {}
      for (const a of validAttrs) attrsObj[a.key.trim()] = a.value
      obj.custom_attributes = JSON.stringify(attrsObj)
    }
    onSave(JSON.stringify(obj))
    onClose()
  }

  const visibleSourcePaths = sourcePaths.filter(p =>
    !hiddenSourceKeys.has(p) && (!sourceFilter || p.toLowerCase().includes(sourceFilter.toLowerCase()))
  )

  return (
    <div style={{ position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.65)', display: 'flex', alignItems: 'center', justifyContent: 'center', zIndex: 2100 }}>
      <div style={{ background: 'white', borderRadius: '10px', width: '860px', maxHeight: '85vh', display: 'flex', flexDirection: 'column', boxShadow: '0 24px 64px rgba(0,0,0,0.45)', overflow: 'hidden' }}>

        {/* Header */}
        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', padding: '14px 20px', background: '#14532d', color: 'white' }}>
          <div>
            <div style={{ fontWeight: 700, fontSize: '15px' }}>ClickFunnels Contact Mapper</div>
            <div style={{ fontSize: '11px', color: '#86efac', marginTop: '2px' }}>Create or update a CF contact. Fields with no value are omitted from the API call.</div>
          </div>
          <button onClick={onClose} style={{ background: 'none', border: 'none', cursor: 'pointer', fontSize: '18px', color: '#86efac', lineHeight: 1 }}>✕</button>
        </div>

        {/* Two-column body */}
        <div style={{ flex: 1, overflow: 'auto', display: 'flex', gap: 0, minHeight: 0 }}>

          {/* LEFT — Source Keys */}
          <div style={{ flex: '0 0 280px', borderRight: '1px solid #e0e6ed', padding: '16px', display: 'flex', flexDirection: 'column', gap: '8px', overflow: 'auto' }}>
            <div style={{ fontSize: '11px', fontWeight: 700, color: '#555', textTransform: 'uppercase', letterSpacing: '0.5px' }}>
              Context Keys <span style={{ fontWeight: 400, color: '#aaa', textTransform: 'none' }}>(drag from here)</span>
            </div>
            <div style={{ fontSize: '10px', color: '#aaa', lineHeight: 1.5 }}>
              Paste sample JSON from the workflow context, then click Parse Keys.
            </div>
            <textarea
              value={sourceText}
              onChange={e => setSourceText(e.target.value)}
              rows={6}
              placeholder={'{\n  "email": "user@example.com",\n  "name": "Jane"\n}'}
              spellCheck={false}
              style={{ width: '100%', boxSizing: 'border-box', fontFamily: 'monospace', fontSize: '11px', padding: '6px 8px', border: '1px solid #ddd', borderRadius: '4px', resize: 'vertical' }}
            />
            {sourceParseError && <div style={{ color: '#e74c3c', fontSize: '11px' }}>{sourceParseError}</div>}
            <button
              onClick={parseSource}
              style={{ padding: '5px', background: '#16a34a', color: 'white', border: 'none', borderRadius: '4px', cursor: 'pointer', fontWeight: 600, fontSize: '11px' }}
            >
              Parse Keys
            </button>

            {sourcePaths.length > 0 && (
              <>
                <div style={{ display: 'flex', alignItems: 'center', gap: '6px', marginTop: '4px' }}>
                  <div style={{ fontSize: '10px', fontWeight: 700, color: '#555', whiteSpace: 'nowrap' }}>SOURCE KEYS:</div>
                  <input
                    type="text"
                    placeholder="filter..."
                    value={sourceFilter}
                    onChange={e => setSourceFilter(e.target.value)}
                    style={{ flex: 1, fontSize: '10px', padding: '3px 6px', border: '1px solid #ddd', borderRadius: '4px', fontFamily: 'monospace', minWidth: 0 }}
                  />
                  {sourceFilter && (
                    <button onClick={() => setSourceFilter('')} style={{ background: 'none', border: 'none', cursor: 'pointer', color: '#aaa', fontSize: '12px', padding: 0, lineHeight: 1 }}>✕</button>
                  )}
                </div>
                <div style={{ display: 'flex', flexWrap: 'wrap', gap: '5px' }}>
                  {visibleSourcePaths.map(p => (
                    <div
                      key={p}
                      draggable
                      onDragStart={() => { dragRef.current = p }}
                      onDragEnd={() => { dragRef.current = '' }}
                      title={p in sourceValues ? fmtVal(sourceValues[p]) : undefined}
                      style={{
                        display: 'inline-flex', alignItems: 'center', gap: '3px',
                        background: '#dcfce7', border: '1px solid #86efac',
                        borderRadius: '4px', padding: '3px 7px',
                        fontFamily: 'monospace', fontSize: '10px',
                        color: '#14532d', cursor: 'grab', userSelect: 'none',
                      }}
                    >
                      <span style={{ pointerEvents: 'none' }}>{p}</span>
                      <span
                        draggable={false}
                        onMouseDown={e => { e.stopPropagation(); e.preventDefault() }}
                        onClick={e => { e.stopPropagation(); setHiddenSourceKeys(prev => new Set([...prev, p])) }}
                        style={{ cursor: 'pointer', color: '#86efac', fontSize: '9px', lineHeight: 1, paddingLeft: '2px', pointerEvents: 'all' }}
                        title="Hide this key"
                      >✕</span>
                    </div>
                  ))}
                </div>
                {hiddenSourceKeys.size > 0 && (
                  <button onClick={() => setHiddenSourceKeys(new Set())} style={{ alignSelf: 'flex-start', fontSize: '10px', color: '#6b7280', background: 'none', border: 'none', cursor: 'pointer', padding: '2px 0', textDecoration: 'underline' }}>
                    show {hiddenSourceKeys.size} hidden
                  </button>
                )}
              </>
            )}
          </div>

          {/* RIGHT — CF field rows */}
          <div style={{ flex: 1, padding: '16px', display: 'flex', flexDirection: 'column', gap: '6px', overflow: 'auto' }}>
            <div style={{ fontSize: '11px', fontWeight: 700, color: '#555', textTransform: 'uppercase', letterSpacing: '0.5px', marginBottom: '4px' }}>
              Contact Fields <span style={{ fontWeight: 400, color: '#aaa', textTransform: 'none' }}>(edit field name · drop value from left)</span>
            </div>

            {rows.map((row, i) => (
              <div key={i} style={{ display: 'flex', alignItems: 'center', gap: '6px' }}>
                {/* Editable field name */}
                <input
                  value={row.field}
                  onChange={e => setRows(prev => prev.map((r, idx) => idx === i ? { ...r, field: e.target.value } : r))}
                  placeholder="field_name"
                  style={{ flex: '0 0 140px', fontFamily: 'monospace', fontSize: '11px', padding: '4px 8px', border: '1px solid #e2e8f0', borderRadius: '4px', color: '#374151', fontWeight: 600, outline: 'none', background: '#fafafa' }}
                />
                {/* Drop zone for value */}
                <div
                  onDragOver={e => { e.preventDefault(); setDragOverIdx(i) }}
                  onDragLeave={() => setDragOverIdx(null)}
                  onDrop={() => handleDrop(i)}
                  style={{
                    flex: 1, minHeight: '30px', display: 'flex', alignItems: 'center',
                    background: dragOverIdx === i ? '#dcfce7' : '#f8fafc',
                    border: `1px dashed ${dragOverIdx === i ? '#16a34a' : '#cbd5e1'}`,
                    borderRadius: '4px', overflow: 'hidden', transition: 'background 0.1s, border-color 0.1s',
                  }}
                >
                  <input
                    value={row.value}
                    onChange={e => setRows(prev => prev.map((r, idx) => idx === i ? { ...r, value: e.target.value } : r))}
                    placeholder="drop or type value"
                    title={resolveTitle(row.value, sourceValues)}
                    style={{ flex: 1, fontFamily: 'monospace', fontSize: '11px', padding: '4px 8px', border: 'none', background: 'transparent', outline: 'none', color: row.value.startsWith('{{') ? '#15803d' : '#333' }}
                  />
                  {row.value && (
                    <button
                      onClick={() => setRows(prev => prev.map((r, idx) => idx === i ? { ...r, value: '' } : r))}
                      style={{ background: 'none', border: 'none', cursor: 'pointer', color: '#cbd5e1', fontSize: '12px', padding: '0 8px', lineHeight: 1 }}
                    >✕</button>
                  )}
                </div>
                {/* Delete row */}
                <button
                  onClick={() => setRows(prev => prev.filter((_, idx) => idx !== i))}
                  title="Remove row"
                  style={{ background: 'none', border: 'none', cursor: 'pointer', color: '#fca5a5', fontSize: '14px', padding: '0 2px', lineHeight: 1, flexShrink: 0 }}
                >✕</button>
              </div>
            ))}

            <button
              onClick={() => setRows(prev => [...prev, { field: '', value: '' }])}
              style={{ alignSelf: 'flex-start', marginTop: '4px', padding: '5px 12px', background: '#f0fdf4', color: '#15803d', border: '1px dashed #86efac', borderRadius: '4px', cursor: 'pointer', fontWeight: 600, fontSize: '11px' }}
            >
              + Add Row
            </button>

            {/* custom_attributes sub-editor */}
            <div style={{ marginTop: '14px', borderTop: '1px solid #e0e6ed', paddingTop: '12px' }}>
              <div style={{ fontSize: '11px', fontWeight: 700, color: '#555', textTransform: 'uppercase', letterSpacing: '0.5px', marginBottom: '6px' }}>
                custom_attributes <span style={{ fontWeight: 400, color: '#aaa', textTransform: 'none' }}>(dictionary — key + value pairs)</span>
              </div>
              {customAttrs.map((attr, i) => (
                <div key={i} style={{ display: 'flex', alignItems: 'center', gap: '6px', marginBottom: '5px' }}>
                  <input
                    value={attr.key}
                    onChange={e => setCustomAttrs(prev => prev.map((a, idx) => idx === i ? { ...a, key: e.target.value } : a))}
                    placeholder="attribute_key"
                    style={{ flex: '0 0 140px', fontFamily: 'monospace', fontSize: '11px', padding: '4px 8px', border: '1px solid #e2e8f0', borderRadius: '4px', color: '#374151', fontWeight: 600, outline: 'none', background: '#fafafa' }}
                  />
                  <div
                    onDragOver={e => { e.preventDefault(); setDragOverAttrIdx(i) }}
                    onDragLeave={() => setDragOverAttrIdx(null)}
                    onDrop={() => handleDropAttr(i)}
                    style={{
                      flex: 1, minHeight: '30px', display: 'flex', alignItems: 'center',
                      background: dragOverAttrIdx === i ? '#dcfce7' : '#f8fafc',
                      border: `1px dashed ${dragOverAttrIdx === i ? '#16a34a' : '#cbd5e1'}`,
                      borderRadius: '4px', overflow: 'hidden', transition: 'background 0.1s, border-color 0.1s',
                    }}
                  >
                    <input
                      value={attr.value}
                      onChange={e => setCustomAttrs(prev => prev.map((a, idx) => idx === i ? { ...a, value: e.target.value } : a))}
                      placeholder="drop or type value"
                      title={resolveTitle(attr.value, sourceValues)}
                      style={{ flex: 1, fontFamily: 'monospace', fontSize: '11px', padding: '4px 8px', border: 'none', background: 'transparent', outline: 'none', color: attr.value.startsWith('{{') ? '#15803d' : '#333' }}
                    />
                    {attr.value && (
                      <button
                        onClick={() => setCustomAttrs(prev => prev.map((a, idx) => idx === i ? { ...a, value: '' } : a))}
                        style={{ background: 'none', border: 'none', cursor: 'pointer', color: '#cbd5e1', fontSize: '12px', padding: '0 8px', lineHeight: 1 }}
                      >✕</button>
                    )}
                  </div>
                  <button
                    onClick={() => setCustomAttrs(prev => prev.filter((_, idx) => idx !== i))}
                    title="Remove attribute"
                    style={{ background: 'none', border: 'none', cursor: 'pointer', color: '#fca5a5', fontSize: '14px', padding: '0 2px', lineHeight: 1, flexShrink: 0 }}
                  >✕</button>
                </div>
              ))}
              <button
                onClick={() => setCustomAttrs(prev => [...prev, { key: '', value: '' }])}
                style={{ alignSelf: 'flex-start', marginTop: '2px', padding: '5px 12px', background: '#f0fdf4', color: '#15803d', border: '1px dashed #86efac', borderRadius: '4px', cursor: 'pointer', fontWeight: 600, fontSize: '11px' }}
              >
                + Add Attribute
              </button>
            </div>
          </div>
        </div>

        {/* Footer */}
        <div style={{ display: 'flex', justifyContent: 'flex-end', alignItems: 'center', gap: '10px', padding: '12px 20px', borderTop: '1px solid #e0e6ed', background: '#f8f9fa' }}>
          <button onClick={onClose} style={{ padding: '8px 16px', background: '#f0f0f0', border: 'none', borderRadius: '4px', cursor: 'pointer', fontWeight: 500, fontSize: '13px' }}>
            Cancel
          </button>
          <button
            onClick={handleSave}
            style={{ padding: '8px 20px', background: '#16a34a', color: 'white', border: 'none', borderRadius: '4px', cursor: 'pointer', fontWeight: 600, fontSize: '13px' }}
          >
            Save CF Fields
          </button>
        </div>
      </div>
    </div>
  )
}

// ── CfContactIdMapperModal (cf_get_contact / cf_upsert / cf_add_tag / cf_remove_tag) ──

function CfContactIdMapperModal({ existingValue, defaultSourceJson, onSave, onClose }: {
  existingValue: string
  defaultSourceJson: string
  onSave: (value: string) => void
  onClose: () => void
}) {
  const dragRef = useRef<string>('')
  const [sourceText, setSourceText] = useState(defaultSourceJson)
  const [sourceParseError, setSourceParseError] = useState('')
  const [sourcePaths, setSourcePaths] = useState<string[]>([])
  const [sourceValues, setSourceValues] = useState<Record<string, unknown>>({})
  const [sourceFilter, setSourceFilter] = useState('')
  const [hiddenSourceKeys, setHiddenSourceKeys] = useState<Set<string>>(new Set())
  const [value, setValue] = useState(existingValue)
  const [dragOver, setDragOver] = useState(false)

  useEffect(() => {
    if (defaultSourceJson.trim() === '') return
    try {
      const parsed = JSON.parse(defaultSourceJson)
      const out: string[] = []
      collectLeafPaths(parsed, '', 0, out)
      setSourcePaths(out.filter(p => p !== ''))
      setSourceValues(buildValueMap(parsed))
    } catch { /* ignore */ }
  }, [defaultSourceJson])

  const parseSource = () => {
    setSourceParseError('')
    setSourcePaths([])
    let parsed: unknown
    try { parsed = JSON.parse(sourceText) } catch { setSourceParseError('Invalid JSON.'); return }
    const out: string[] = []
    collectLeafPaths(parsed, '', 0, out)
    setSourcePaths(out.filter(p => p !== ''))
    setSourceValues(buildValueMap(parsed))
  }

  const visibleSourcePaths = sourcePaths.filter(p =>
    !hiddenSourceKeys.has(p) && (!sourceFilter || p.toLowerCase().includes(sourceFilter.toLowerCase()))
  )

  return (
    <div style={{ position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.65)', display: 'flex', alignItems: 'center', justifyContent: 'center', zIndex: 2100 }}>
      <div style={{ background: 'white', borderRadius: '10px', width: '660px', maxHeight: '80vh', display: 'flex', flexDirection: 'column', boxShadow: '0 24px 64px rgba(0,0,0,0.45)', overflow: 'hidden' }}>

        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', padding: '14px 20px', background: '#7c3aed', color: 'white' }}>
          <div>
            <div style={{ fontWeight: 700, fontSize: '15px' }}>Contact ID Mapper</div>
            <div style={{ fontSize: '11px', color: '#ddd6fe', marginTop: '2px' }}>Drag a context key onto the contact_id field, or type a value directly.</div>
          </div>
          <button onClick={onClose} style={{ background: 'none', border: 'none', cursor: 'pointer', fontSize: '18px', color: '#ddd6fe', lineHeight: 1 }}>✕</button>
        </div>

        <div style={{ flex: 1, overflow: 'auto', display: 'flex', gap: 0, minHeight: 0 }}>

          {/* LEFT — context keys */}
          <div style={{ flex: '0 0 280px', borderRight: '1px solid #e0e6ed', padding: '16px', display: 'flex', flexDirection: 'column', gap: '8px', overflow: 'auto' }}>
            <div style={{ fontSize: '11px', fontWeight: 700, color: '#555', textTransform: 'uppercase', letterSpacing: '0.5px' }}>
              Context Keys <span style={{ fontWeight: 400, color: '#aaa', textTransform: 'none' }}>(drag from here)</span>
            </div>
            <textarea
              value={sourceText}
              onChange={e => setSourceText(e.target.value)}
              rows={6}
              placeholder={'{\n  "contact": {\n    "cf_id": 123\n  }\n}'}
              spellCheck={false}
              style={{ width: '100%', boxSizing: 'border-box', fontFamily: 'monospace', fontSize: '11px', padding: '6px 8px', border: '1px solid #ddd', borderRadius: '4px', resize: 'vertical' }}
            />
            {sourceParseError && <div style={{ color: '#e74c3c', fontSize: '11px' }}>{sourceParseError}</div>}
            <button onClick={parseSource} style={{ padding: '5px', background: '#7c3aed', color: 'white', border: 'none', borderRadius: '4px', cursor: 'pointer', fontWeight: 600, fontSize: '11px' }}>
              Parse Keys
            </button>
            {sourcePaths.length > 0 && (
              <>
                <div style={{ display: 'flex', alignItems: 'center', gap: '6px', marginTop: '4px' }}>
                  <div style={{ fontSize: '10px', fontWeight: 700, color: '#555', whiteSpace: 'nowrap' }}>SOURCE KEYS:</div>
                  <input
                    type="text" placeholder="filter..." value={sourceFilter} onChange={e => setSourceFilter(e.target.value)}
                    style={{ flex: 1, fontSize: '10px', padding: '3px 6px', border: '1px solid #ddd', borderRadius: '4px', fontFamily: 'monospace', minWidth: 0 }}
                  />
                  {sourceFilter && <button onClick={() => setSourceFilter('')} style={{ background: 'none', border: 'none', cursor: 'pointer', color: '#aaa', fontSize: '12px', padding: 0 }}>✕</button>}
                </div>
                <div style={{ display: 'flex', flexWrap: 'wrap', gap: '5px' }}>
                  {visibleSourcePaths.map(p => (
                    <div
                      key={p}
                      draggable
                      onDragStart={() => { dragRef.current = p }}
                      onDragEnd={() => { dragRef.current = '' }}
                      title={p in sourceValues ? fmtVal(sourceValues[p]) : undefined}
                      style={{ display: 'inline-flex', alignItems: 'center', gap: '3px', background: '#ede9fe', border: '1px solid #c4b5fd', borderRadius: '4px', padding: '3px 7px', fontFamily: 'monospace', fontSize: '10px', color: '#6d28d9', cursor: 'grab', userSelect: 'none' }}
                    >
                      <span style={{ pointerEvents: 'none' }}>{p}</span>
                      <span
                        draggable={false}
                        onMouseDown={e => { e.stopPropagation(); e.preventDefault() }}
                        onClick={e => { e.stopPropagation(); setHiddenSourceKeys(prev => new Set([...prev, p])) }}
                        style={{ cursor: 'pointer', color: '#c4b5fd', fontSize: '9px', lineHeight: 1, paddingLeft: '2px', pointerEvents: 'all' }}
                        title="Hide this key"
                      >✕</span>
                    </div>
                  ))}
                </div>
                {hiddenSourceKeys.size > 0 && (
                  <button onClick={() => setHiddenSourceKeys(new Set())} style={{ alignSelf: 'flex-start', fontSize: '10px', color: '#6b7280', background: 'none', border: 'none', cursor: 'pointer', padding: '2px 0', textDecoration: 'underline' }}>
                    show {hiddenSourceKeys.size} hidden
                  </button>
                )}
              </>
            )}
          </div>

          {/* RIGHT — contact_id drop zone */}
          <div style={{ flex: 1, padding: '20px', display: 'flex', flexDirection: 'column', gap: '12px', justifyContent: 'center' }}>
            <div style={{ fontSize: '11px', fontWeight: 700, color: '#555', textTransform: 'uppercase', letterSpacing: '0.5px' }}>
              contact_id <span style={{ color: '#e74c3c', fontWeight: 400 }}>(required)</span>
            </div>
            <div
              onDragOver={e => { e.preventDefault(); setDragOver(true) }}
              onDragLeave={() => setDragOver(false)}
              onDrop={() => { if (dragRef.current) { setValue(`{{${dragRef.current}}}`); dragRef.current = ''; setDragOver(false) } }}
              style={{
                display: 'flex', alignItems: 'center', minHeight: '44px',
                background: dragOver ? '#ede9fe' : 'white',
                border: `2px dashed ${dragOver ? '#7c3aed' : value ? '#c4b5fd' : '#ddd'}`,
                borderRadius: '6px', transition: 'background 0.1s, border-color 0.1s', overflow: 'hidden',
              }}
            >
              <input
                value={value}
                onChange={e => setValue(e.target.value)}
                placeholder="{{contact.cf_id}} or numeric ID"
                title={resolveTitle(value, sourceValues)}
                style={{ flex: 1, fontFamily: 'monospace', fontSize: '13px', padding: '8px 12px', border: 'none', background: 'transparent', outline: 'none', color: value.startsWith('{{') ? '#6d28d9' : '#333' }}
              />
              {value && <button onClick={() => setValue('')} style={{ background: 'none', border: 'none', cursor: 'pointer', color: '#c4b5fd', fontSize: '14px', padding: '0 12px', lineHeight: 1 }}>✕</button>}
            </div>
            {value && value.startsWith('{{') && (() => {
              const key = value.slice(2, -2)
              const resolved = key in sourceValues ? fmtVal(sourceValues[key]) : null
              return resolved ? (
                <div style={{ fontSize: '11px', color: '#6d28d9', fontFamily: 'monospace', background: '#f5f3ff', padding: '6px 10px', borderRadius: '4px' }}>
                  → {resolved}
                </div>
              ) : null
            })()}
            <div style={{ fontSize: '11px', color: '#aaa', lineHeight: 1.6 }}>
              Drop a context key from the left panel, or type a literal ID or <code>{'{{key}}'}</code> template.
            </div>
          </div>
        </div>

        <div style={{ display: 'flex', justifyContent: 'flex-end', gap: '10px', padding: '12px 20px', borderTop: '1px solid #e0e6ed', background: '#f8f9fa' }}>
          <button onClick={onClose} style={{ padding: '8px 16px', background: '#f0f0f0', border: 'none', borderRadius: '4px', cursor: 'pointer', fontWeight: 500, fontSize: '13px' }}>Cancel</button>
          <button
            onClick={() => { onSave(value); onClose() }}
            style={{ padding: '8px 20px', background: '#7c3aed', color: 'white', border: 'none', borderRadius: '4px', cursor: 'pointer', fontWeight: 600, fontSize: '13px' }}
          >
            Save Contact ID
          </button>
        </div>
      </div>
    </div>
  )
}

// ── CfTagPickerModal (cf_add_tag / cf_remove_tag) ─────────────────────────────

type CfTag = { id: number; name: string; color: string }

function CfTagPickerModal({ existingTagIds, mode, onSave, onClose }: {
  existingTagIds: string
  mode: 'add' | 'remove'
  onSave: (tagIds: string) => void
  onClose: () => void
}) {
  const [results, setResults] = useState<CfTag[]>([])
  const [loading, setLoading] = useState(false)
  const [loadError, setLoadError] = useState('')
  const [selected, setSelected] = useState<CfTag[]>(() => {
    try {
      const ids: number[] = JSON.parse(existingTagIds)
      return ids.map(id => ({ id, name: `#${id}`, color: '' }))
    } catch { return [] }
  })
  const [dropdownOpen, setDropdownOpen] = useState(false)
  const [filter, setFilter] = useState('')

  const fetchTags = useCallback((name: string) => {
    setLoading(true)
    setLoadError('')
    const url = name.trim() ? `/api/cf/tags?name=${encodeURIComponent(name.trim())}` : '/api/cf/tags'
    fetch(url, { credentials: 'include' })
      .then(r => r.json())
      .then((data: { tags?: CfTag[]; error?: string }) => {
        if (data.error) { setLoadError(data.error); return }
        const tags = data.tags ?? []
        setResults(tags)
        // backfill names for already-selected IDs from fresh results
        setSelected(prev => prev.map(s => tags.find(t => t.id === s.id) ?? s))
      })
      .catch(() => setLoadError('Failed to load tags'))
      .finally(() => setLoading(false))
  }, [])

  // initial load
  useEffect(() => { fetchTags('') }, [fetchTags])

  // debounced re-fetch when filter changes
  useEffect(() => {
    if (filter === '') { fetchTags(''); return }
    const t = setTimeout(() => fetchTags(filter), 300)
    return () => clearTimeout(t)
  }, [filter, fetchTags])

  const selectedIds = new Set(selected.map(t => t.id))
  const available = results.filter(t => !selectedIds.has(t.id))

  const addTag = (tag: CfTag) => { setSelected(prev => [...prev, tag]); setDropdownOpen(false); setFilter('') }
  const removeTag = (id: number) => setSelected(prev => prev.filter(t => t.id !== id))
  const handleSave = () => { onSave(JSON.stringify(selected.map(t => t.id))); onClose() }

  const accent = mode === 'add' ? '#16a34a' : '#dc2626'
  const accentLight = mode === 'add' ? '#dcfce7' : '#fee2e2'
  const title = mode === 'add' ? 'Add Tags to Contact' : 'Remove Tags from Contact'
  const subtitle = mode === 'add'
    ? 'Select tags to apply. All selected tags will be added in one step.'
    : 'Select tags to remove. All selected tags will be removed in one step.'

  return (
    <div style={{ position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.65)', display: 'flex', alignItems: 'center', justifyContent: 'center', zIndex: 2100 }}>
      <div style={{ background: 'white', borderRadius: '10px', width: '700px', maxHeight: '90vh', display: 'flex', flexDirection: 'column', boxShadow: '0 24px 64px rgba(0,0,0,0.45)', overflow: 'hidden' }}>
        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', padding: '14px 20px', background: accent, color: 'white' }}>
          <div>
            <div style={{ fontWeight: 700, fontSize: '15px' }}>{title}</div>
            <div style={{ fontSize: '11px', color: 'rgba(255,255,255,0.75)', marginTop: '2px' }}>{subtitle}</div>
          </div>
          <button onClick={onClose} style={{ background: 'none', border: 'none', cursor: 'pointer', fontSize: '18px', color: 'rgba(255,255,255,0.8)', lineHeight: 1 }}>✕</button>
        </div>

        <div style={{ flex: 1, overflow: 'auto', padding: '16px 20px', display: 'flex', flexDirection: 'column', gap: '14px' }}>
          {/* Selected chips */}
          <div>
            <div style={{ fontSize: '11px', fontWeight: 700, color: '#555', textTransform: 'uppercase', letterSpacing: '0.5px', marginBottom: '8px' }}>
              Selected Tags {selected.length > 0 && <span style={{ color: accent }}>({selected.length})</span>}
            </div>
            {selected.length === 0 ? (
              <div style={{ fontSize: '12px', color: '#aaa', fontStyle: 'italic' }}>No tags selected — use the picker below.</div>
            ) : (
              <div style={{ display: 'flex', flexWrap: 'wrap', gap: '6px' }}>
                {selected.map(tag => (
                  <div key={tag.id} style={{ display: 'inline-flex', alignItems: 'center', gap: '5px', background: accentLight, border: `1px solid ${accent}`, borderRadius: '20px', padding: '3px 10px 3px 8px', fontSize: '12px', fontWeight: 600, color: accent }}>
                    {tag.color && <span style={{ width: '8px', height: '8px', borderRadius: '50%', background: tag.color, display: 'inline-block', flexShrink: 0 }} />}
                    {tag.name}
                    <button onClick={() => removeTag(tag.id)} style={{ background: 'none', border: 'none', cursor: 'pointer', color: accent, fontSize: '13px', padding: 0, lineHeight: 1, opacity: 0.7 }}>✕</button>
                  </div>
                ))}
              </div>
            )}
          </div>

          {/* Dropdown picker */}
          <div>
            <div style={{ fontSize: '11px', fontWeight: 700, color: '#555', textTransform: 'uppercase', letterSpacing: '0.5px', marginBottom: '8px' }}>Add Tag</div>
            {loading ? (
              <div style={{ fontSize: '12px', color: '#aaa' }}>Loading tags…</div>
            ) : loadError ? (
              <div style={{ fontSize: '12px', color: '#e74c3c' }}>{loadError}</div>
            ) : (
              <div style={{ position: 'relative' }}>
                <div
                  onClick={() => setDropdownOpen(v => !v)}
                  style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', padding: '8px 12px', border: `1px solid ${dropdownOpen ? accent : '#ddd'}`, borderRadius: '6px', cursor: 'pointer', background: 'white', fontSize: '13px', color: '#555', userSelect: 'none' }}
                >
                  <span style={{ color: available.length > 0 ? '#555' : '#aaa' }}>{available.length > 0 ? 'Choose a tag…' : 'All tags selected'}</span>
                  <span style={{ color: '#aaa', fontSize: '10px' }}>{dropdownOpen ? '▲' : '▼'}</span>
                </div>
                {dropdownOpen && (
                  <div style={{ position: 'absolute', top: 'calc(100% + 4px)', left: 0, right: 0, background: 'white', border: '1px solid #e0e0e0', borderRadius: '6px', boxShadow: '0 8px 24px rgba(0,0,0,0.12)', zIndex: 10, overflow: 'hidden' }}>
                    <div style={{ padding: '8px' }}>
                      <input autoFocus value={filter} onChange={e => setFilter(e.target.value)} placeholder="Filter tags…" style={{ width: '100%', boxSizing: 'border-box', padding: '6px 8px', border: '1px solid #eee', borderRadius: '4px', fontSize: '12px', outline: 'none' }} />
                    </div>
                    <div style={{ maxHeight: '400px', overflow: 'auto' }}>
                      {available.length === 0 ? (
                        <div style={{ padding: '10px 14px', fontSize: '12px', color: '#aaa', fontStyle: 'italic' }}>No matching tags</div>
                      ) : available.map(tag => (
                        <div
                          key={tag.id}
                          onClick={() => addTag(tag)}
                          style={{ display: 'flex', alignItems: 'center', gap: '8px', padding: '8px 14px', cursor: 'pointer', fontSize: '13px' }}
                          onMouseEnter={e => (e.currentTarget.style.background = accentLight)}
                          onMouseLeave={e => (e.currentTarget.style.background = 'white')}
                        >
                          {tag.color && <span style={{ width: '10px', height: '10px', borderRadius: '50%', background: tag.color, display: 'inline-block', flexShrink: 0 }} />}
                          <span style={{ fontWeight: 500 }}>{tag.name}</span>
                          <span style={{ marginLeft: 'auto', fontSize: '10px', color: '#aaa' }}>id: {tag.id}</span>
                        </div>
                      ))}
                    </div>
                  </div>
                )}
              </div>
            )}
          </div>
        </div>

        <div style={{ display: 'flex', justifyContent: 'flex-end', gap: '10px', padding: '12px 20px', borderTop: '1px solid #e0e6ed', background: '#f8f9fa' }}>
          <button onClick={onClose} style={{ padding: '8px 16px', background: '#f0f0f0', border: 'none', borderRadius: '4px', cursor: 'pointer', fontWeight: 500, fontSize: '13px' }}>Cancel</button>
          <button
            onClick={handleSave}
            disabled={selected.length === 0}
            style={{ padding: '8px 20px', background: selected.length === 0 ? '#ccc' : accent, color: 'white', border: 'none', borderRadius: '4px', cursor: selected.length === 0 ? 'not-allowed' : 'pointer', fontWeight: 600, fontSize: '13px' }}
          >
            Save {selected.length > 0 ? `(${selected.length} tag${selected.length > 1 ? 's' : ''})` : ''}
          </button>
        </div>
      </div>
    </div>
  )
}

// ── HttpResponseMapperPanel (http_request) ────────────────────────────────────

function collectLeafPaths(obj: unknown, prefix: string, depth: number, out: string[]): void {
  if (depth > 5 || typeof obj !== 'object' || obj === null) {
    out.push(prefix)
    return
  }
  if (Array.isArray(obj)) {
    (obj as unknown[]).forEach((item, i) => {
      const childPath = prefix ? `${prefix}.${i}` : String(i)
      if (typeof item === 'object' && item !== null && depth < 5) {
        collectLeafPaths(item, childPath, depth + 1, out)
      } else {
        out.push(childPath)
      }
    })
    return
  }
  const keys = Object.keys(obj as Record<string, unknown>)
  if (keys.length === 0) { out.push(prefix); return }
  for (const k of keys) {
    const child = (obj as Record<string, unknown>)[k]
    const childPath = prefix ? `${prefix}.${k}` : k
    if (typeof child === 'object' && child !== null && depth < 5) {
      collectLeafPaths(child, childPath, depth + 1, out)
    } else {
      out.push(childPath)
    }
  }
}

function HttpResponseMapperPanel({ node, onInsertMapperNode }: {
  node: BNode
  onInsertMapperNode: (sourceNodeId: string, mappings: Array<{ from: string; to: string }>) => void
}) {
  const [open, setOpen] = useState(false)
  const [sampleJson, setSampleJson] = useState('')
  const [paths, setPaths] = useState<string[]>([])
  const [selected, setSelected] = useState<Set<string>>(new Set())
  const [toNames, setToNames] = useState<Record<string, string>>({})
  const [parseError, setParseError] = useState('')

  const parsePaths = () => {
    setParseError('')
    setPaths([])
    setSelected(new Set())
    setToNames({})
    let parsed: unknown
    try {
      parsed = JSON.parse(sampleJson)
    } catch {
      setParseError('Invalid JSON.')
      return
    }
    const out: string[] = []
    collectLeafPaths(parsed, '', 0, out)
    const filteredPaths = out.filter(p => p !== '')
    const defaultSelected = new Set(filteredPaths)
    const defaultToNames: Record<string, string> = {}
    for (const p of filteredPaths) {
      const segs = p.split('.')
      defaultToNames[p] = segs[segs.length - 1]
    }
    setPaths(filteredPaths)
    setSelected(defaultSelected)
    setToNames(defaultToNames)
  }

  const togglePath = (p: string) => {
    setSelected(prev => {
      const next = new Set(prev)
      if (next.has(p)) next.delete(p)
      else next.add(p)
      return next
    })
  }

  const insertMapper = () => {
    const mappings = paths
      .filter(p => selected.has(p))
      .map(p => ({ from: p, to: toNames[p] ?? p.split('.').pop() ?? p }))
    if (mappings.length === 0) return
    onInsertMapperNode(node.id, mappings)
  }

  return (
    <div style={{ marginTop: '12px', border: '1px solid #e0e6ed', borderRadius: '6px', overflow: 'hidden' }}>
      <button
        onClick={() => setOpen(o => !o)}
        style={{
          width: '100%', textAlign: 'left', background: '#f0f9ff', border: 'none',
          padding: '7px 10px', cursor: 'pointer', fontSize: '11px', fontWeight: 700,
          color: '#0369a1', display: 'flex', justifyContent: 'space-between', alignItems: 'center',
        }}
      >
        <span>Response Mapper</span>
        <span>{open ? '▾' : '▸'}</span>
      </button>
      {open && (
        <div style={{ padding: '10px', background: 'white' }}>
          <div style={{ fontSize: '10px', color: '#aaa', marginBottom: '6px', lineHeight: 1.5 }}>
            Paste a sample response JSON. Select the fields you want to map, then insert a
            <code> mapper</code> node wired after this one.
          </div>
          <textarea
            value={sampleJson}
            onChange={e => setSampleJson(e.target.value)}
            rows={5}
            placeholder={'{\n  "status": "ok",\n  "data": { "id": 1 }\n}'}
            spellCheck={false}
            style={{ width: '100%', boxSizing: 'border-box', fontFamily: 'monospace', fontSize: '11px', padding: '6px 8px', border: '1px solid #ddd', borderRadius: '4px', resize: 'vertical' }}
          />
          {parseError && <div style={{ color: '#e74c3c', fontSize: '11px', marginTop: '4px' }}>{parseError}</div>}
          <button
            onClick={parsePaths}
            style={{ marginTop: '6px', width: '100%', padding: '5px', background: '#0369a1', color: 'white', border: 'none', borderRadius: '4px', cursor: 'pointer', fontWeight: 600, fontSize: '11px' }}
          >
            Parse Paths
          </button>

          {paths.length > 0 && (
            <>
              <div style={{ marginTop: '8px', fontSize: '10px', fontWeight: 700, color: '#555', marginBottom: '4px' }}>
                Select fields to map:
              </div>
              {paths.map(p => (
                <div key={p} style={{ display: 'flex', alignItems: 'center', gap: '6px', marginBottom: '4px' }}>
                  <input
                    type="checkbox"
                    checked={selected.has(p)}
                    onChange={() => togglePath(p)}
                    style={{ flexShrink: 0 }}
                  />
                  <code style={{ fontSize: '10px', color: '#2c3e50', flex: '0 0 auto', minWidth: '80px' }}>{p}</code>
                  <span style={{ fontSize: '10px', color: '#aaa' }}>→</span>
                  <input
                    value={toNames[p] ?? ''}
                    onChange={e => setToNames(prev => ({ ...prev, [p]: e.target.value }))}
                    style={{ flex: 1, padding: '2px 4px', border: '1px solid #ddd', borderRadius: '3px', fontSize: '10px', fontFamily: 'monospace' }}
                    placeholder="to key"
                  />
                </div>
              ))}
              <button
                onClick={insertMapper}
                disabled={selected.size === 0}
                style={{ marginTop: '8px', width: '100%', padding: '5px', background: selected.size === 0 ? '#aaa' : '#0369a1', color: 'white', border: 'none', borderRadius: '4px', cursor: selected.size === 0 ? 'default' : 'pointer', fontWeight: 600, fontSize: '11px' }}
              >
                Insert Mapper Node
              </button>
            </>
          )}
        </div>
      )}
    </div>
  )
}

// ── Mapper Modal ──────────────────────────────────────────────────────────────

function MapperModal({ sourceJson, destJson, existingMappings, onSave, onClose }: {
  sourceJson: string
  destJson: string
  existingMappings: Array<{ from: string; to: string }>
  onSave: (mappings: Array<{ from: string; to: string }>) => void
  onClose: () => void
}) {
  const [mappings, setMappings] = useState<Array<{ from: string; to: string }>>(existingMappings)
  const [dragOverDest, setDragOverDest] = useState<string | null>(null)
  const dragKey = useRef<string>('')

  const { sourceKeys, sourceValues: mapperSourceValues } = (() => {
    try {
      const parsed = JSON.parse(sourceJson)
      const out: string[] = []
      collectLeafPaths(parsed, '', 0, out)
      return { sourceKeys: out.filter(p => p !== ''), sourceValues: buildValueMap(parsed) }
    } catch { return { sourceKeys: [] as string[], sourceValues: {} as Record<string, unknown> } }
  })()

  const destKeys: string[] = (() => {
    try {
      const parsed = JSON.parse(destJson)
      if (typeof parsed === 'object' && parsed !== null && !Array.isArray(parsed)) {
        return Object.keys(parsed as Record<string, unknown>)
      }
      return []
    } catch { return [] }
  })()

  const mappedSources = new Set(mappings.map(m => m.from))

  const removeMapping = (idx: number) => {
    setMappings(prev => prev.filter((_, i) => i !== idx))
  }

  const handleDrop = (destKey: string) => {
    const fromKey = dragKey.current
    if (!fromKey) return
    setMappings(prev => {
      // Remove existing mapping for this dest key, then add new one
      const without = prev.filter(m => m.to !== destKey)
      return [...without, { from: fromKey, to: destKey }]
    })
    setDragOverDest(null)
  }

  return (
    <div style={{ position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.6)', display: 'flex', alignItems: 'center', justifyContent: 'center', zIndex: 2000 }}>
      <div style={{ background: 'white', borderRadius: '10px', width: '800px', maxHeight: '80vh', display: 'flex', flexDirection: 'column', boxShadow: '0 20px 60px rgba(0,0,0,0.4)', overflow: 'hidden' }}>
        {/* Header */}
        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', padding: '14px 20px', borderBottom: '1px solid #e0e6ed', background: '#f8f9fa' }}>
          <div style={{ fontWeight: 700, fontSize: '15px', color: '#2c3e50' }}>Field Mapper</div>
          <button onClick={onClose} style={{ background: 'none', border: 'none', cursor: 'pointer', fontSize: '18px', color: '#666', lineHeight: 1 }}>✕</button>
        </div>

        {/* Body */}
        <div style={{ flex: 1, overflow: 'auto', padding: '16px 20px' }}>
          {/* Two columns */}
          <div style={{ display: 'flex', gap: '20px', marginBottom: '16px' }}>
            {/* Source fields */}
            <div style={{ flex: 1 }}>
              <div style={{ fontSize: '11px', fontWeight: 700, color: '#555', textTransform: 'uppercase', letterSpacing: '0.5px', marginBottom: '8px' }}>
                Source Fields <span style={{ fontWeight: 400, color: '#aaa', textTransform: 'none' }}>(drag from here)</span>
              </div>
              {sourceKeys.length === 0 && (
                <div style={{ fontSize: '11px', color: '#aaa', fontStyle: 'italic' }}>No source fields parsed — check your source JSON.</div>
              )}
              {sourceKeys.map(k => {
                const isMapped = mappedSources.has(k)
                return (
                  <div
                    key={k}
                    draggable
                    onDragStart={() => { dragKey.current = k }}
                    onDragEnd={() => { dragKey.current = '' }}
                    title={k in mapperSourceValues ? fmtVal(mapperSourceValues[k]) : undefined}
                    style={{
                      display: 'inline-flex', alignItems: 'center', gap: '4px',
                      background: isMapped ? '#d4edda' : '#e8f4fd',
                      border: `1px solid ${isMapped ? '#28a745' : '#3498db'}`,
                      borderRadius: '4px', padding: '4px 8px', marginBottom: '6px', marginRight: '6px',
                      fontFamily: 'monospace', fontSize: '11px', color: isMapped ? '#155724' : '#1a5276',
                      cursor: 'grab', userSelect: 'none', opacity: isMapped ? 0.7 : 1,
                    }}
                  >
                    {isMapped && <span style={{ fontSize: '10px' }}>✓</span>}
                    {k}
                  </div>
                )
              })}
            </div>

            {/* Destination fields */}
            <div style={{ flex: 1 }}>
              <div style={{ fontSize: '11px', fontWeight: 700, color: '#555', textTransform: 'uppercase', letterSpacing: '0.5px', marginBottom: '8px' }}>
                Destination Fields <span style={{ fontWeight: 400, color: '#aaa', textTransform: 'none' }}>(drop onto these)</span>
              </div>
              {destKeys.length === 0 && (
                <div style={{ fontSize: '11px', color: '#aaa', fontStyle: 'italic' }}>No destination fields parsed — check your destination JSON.</div>
              )}
              {destKeys.map(dk => {
                const existingMap = mappings.find(m => m.to === dk)
                const isOver = dragOverDest === dk
                return (
                  <div
                    key={dk}
                    onDragOver={e => { e.preventDefault(); setDragOverDest(dk) }}
                    onDragLeave={() => setDragOverDest(null)}
                    onDrop={() => handleDrop(dk)}
                    title={existingMap && existingMap.from in mapperSourceValues ? `${existingMap.from} = ${fmtVal(mapperSourceValues[existingMap.from])}` : undefined}
                    style={{
                      display: 'flex', alignItems: 'center', gap: '8px',
                      background: isOver ? '#d4edda' : existingMap ? '#f0fff4' : '#f5f5f5',
                      border: `1px dashed ${isOver ? '#28a745' : existingMap ? '#90d0a0' : '#ccc'}`,
                      borderRadius: '4px', padding: '5px 10px', marginBottom: '6px',
                      transition: 'background 0.1s, border-color 0.1s',
                      minHeight: '30px',
                    }}
                  >
                    <code style={{ fontSize: '11px', color: '#2c3e50', flex: '0 0 auto' }}>{dk}</code>
                    <span style={{ fontSize: '10px', color: '#aaa', flex: 1 }}>
                      {existingMap
                        ? <span style={{ color: '#27ae60', fontFamily: 'monospace' }}>← {existingMap.from}</span>
                        : isOver ? <span style={{ color: '#27ae60' }}>drop here</span> : <span style={{ color: '#ccc' }}>drop here ▼</span>
                      }
                    </span>
                  </div>
                )
              })}
            </div>
          </div>

          {/* Current mappings */}
          {mappings.length > 0 && (
            <div style={{ borderTop: '1px solid #e0e6ed', paddingTop: '12px' }}>
              <div style={{ fontSize: '11px', fontWeight: 700, color: '#555', textTransform: 'uppercase', letterSpacing: '0.5px', marginBottom: '8px' }}>
                Current Mappings
              </div>
              <div style={{ display: 'flex', flexWrap: 'wrap', gap: '6px' }}>
                {mappings.map((m, i) => (
                  <div key={i} style={{ display: 'inline-flex', alignItems: 'center', gap: '4px', background: '#f0f4ff', border: '1px solid #c5cff5', borderRadius: '4px', padding: '3px 8px', fontSize: '11px', fontFamily: 'monospace', color: '#2c3e50' }}>
                    <span style={{ color: '#3498db' }}>{m.from}</span>
                    <span style={{ color: '#aaa', fontSize: '10px' }}>→</span>
                    <span style={{ color: '#27ae60' }}>{m.to}</span>
                    <button onClick={() => removeMapping(i)} style={{ background: 'none', border: 'none', cursor: 'pointer', color: '#e74c3c', fontSize: '12px', padding: '0 0 0 4px', lineHeight: 1 }}>✕</button>
                  </div>
                ))}
              </div>
            </div>
          )}
        </div>

        {/* Footer */}
        <div style={{ display: 'flex', justifyContent: 'flex-end', gap: '10px', padding: '12px 20px', borderTop: '1px solid #e0e6ed', background: '#f8f9fa' }}>
          <button onClick={onClose} style={{ padding: '8px 16px', background: '#f0f0f0', border: 'none', borderRadius: '4px', cursor: 'pointer', fontWeight: 500, fontSize: '13px' }}>
            Cancel
          </button>
          <button
            onClick={() => onSave(mappings)}
            style={{ padding: '8px 16px', background: '#667eea', color: 'white', border: 'none', borderRadius: '4px', cursor: 'pointer', fontWeight: 600, fontSize: '13px' }}
          >
            Save Mappings
          </button>
        </div>
      </div>
    </div>
  )
}

// ── HTTP Body Builder Modal ───────────────────────────────────────────────────

function setLeafValue(obj: Record<string, unknown>, path: string, value: unknown): Record<string, unknown> {
  const segs = path.split('.')
  const result = { ...obj }
  let cur: Record<string, unknown> = result
  for (let i = 0; i < segs.length - 1; i++) {
    const seg = segs[i]
    cur[seg] = typeof cur[seg] === 'object' && cur[seg] !== null && !Array.isArray(cur[seg])
      ? { ...(cur[seg] as Record<string, unknown>) }
      : {}
    cur = cur[seg] as Record<string, unknown>
  }
  cur[segs[segs.length - 1]] = value
  return result
}

function collectLeafPathsWithValues(obj: unknown, prefix: string, depth: number, out: Array<{ path: string; value: unknown }>): void {
  if (depth > 5 || typeof obj !== 'object' || obj === null) {
    out.push({ path: prefix, value: obj })
    return
  }
  if (Array.isArray(obj)) {
    (obj as unknown[]).forEach((item, i) => {
      const childPath = prefix ? `${prefix}.${i}` : String(i)
      if (typeof item === 'object' && item !== null && depth < 5) {
        collectLeafPathsWithValues(item, childPath, depth + 1, out)
      } else {
        out.push({ path: childPath, value: item })
      }
    })
    return
  }
  const keys = Object.keys(obj as Record<string, unknown>)
  if (keys.length === 0) { out.push({ path: prefix, value: obj }); return }
  for (const k of keys) {
    const child = (obj as Record<string, unknown>)[k]
    const childPath = prefix ? `${prefix}.${k}` : k
    if (typeof child === 'object' && child !== null && depth < 5) {
      collectLeafPathsWithValues(child, childPath, depth + 1, out)
    } else {
      out.push({ path: childPath, value: child })
    }
  }
}

// Build a path→value lookup from a parsed JSON object.
function buildValueMap(parsed: unknown): Record<string, unknown> {
  const out: Array<{ path: string; value: unknown }> = []
  collectLeafPathsWithValues(parsed, '', 0, out)
  const map: Record<string, unknown> = {}
  for (const { path, value } of out) {
    if (path) map[path] = value
  }
  return map
}

// Format a value for display in a tooltip.
function fmtVal(v: unknown): string {
  if (v === null) return 'null'
  if (v === undefined) return ''
  if (typeof v === 'string') return v
  return JSON.stringify(v)
}

// Returns a tooltip string for a value cell: resolves {{key}} → "key = value" from sourceValues.
function resolveTitle(val: string, sourceValues: Record<string, unknown>): string | undefined {
  if (!val) return undefined
  const m = val.match(/^\{\{(.+?)\}\}$/)
  if (!m) return undefined
  const key = m[1]
  return key in sourceValues ? `${key} = ${fmtVal(sourceValues[key])}` : `${key} (not in context)`
}

// ── WorkflowGraphPicker ────────────────────────────────────────────────────────

function WorkflowGraphPicker({ value, onChange }: { value: string; onChange: (name: string) => void }) {
  const [names, setNames] = useState<string[]>([])
  useEffect(() => {
    fetch('/api/workflows/graphs', { credentials: 'include' })
      .then(r => r.json())
      .then(d => setNames(d.names || []))
      .catch(() => {})
  }, [])
  return (
    <select
      value={value}
      onChange={e => onChange(e.target.value)}
      style={{ ...inputStyle, background: 'white' }}
    >
      <option value="">— select graph —</option>
      {names.map(n => <option key={n} value={n}>{n}</option>)}
    </select>
  )
}

// ── BooleanToggle ─────────────────────────────────────────────────────────────

function BooleanToggle({ value, onChange }: { value: boolean; onChange: (v: boolean) => void }) {
  return (
    <label style={{ display: 'flex', alignItems: 'center', gap: '8px', cursor: 'pointer', userSelect: 'none' }}>
      <span style={{
        position: 'relative', display: 'inline-block', width: '36px', height: '20px',
        background: value ? '#6366f1' : '#4b5563',
        borderRadius: '10px', transition: 'background 0.2s', flexShrink: 0,
      }}>
        <span style={{
          position: 'absolute', top: '3px',
          left: value ? '19px' : '3px',
          width: '14px', height: '14px', background: 'white',
          borderRadius: '50%', transition: 'left 0.2s',
        }} />
        <input
          type="checkbox"
          checked={value}
          onChange={e => onChange(e.target.checked)}
          style={{ position: 'absolute', opacity: 0, width: 0, height: 0 }}
        />
      </span>
      <span style={{ fontSize: '12px', color: value ? '#a5b4fc' : '#9ca3af' }}>
        {value ? 'Yes' : 'No'}
      </span>
    </label>
  )
}

// ── ContextFieldPicker ────────────────────────────────────────────────────────

function ContextFieldPicker({ value, fieldName, debugContext, onChange }: {
  value: string
  fieldName: string
  debugContext: string
  onChange: (v: string) => void
}) {
  const ctxPaths: string[] = (() => {
    try {
      const ctx = JSON.parse(debugContext || '{}')
      const out: string[] = []
      collectLeafPaths(ctx, '', 0, out)
      return out.filter(p => p !== '' && !p.startsWith('_'))
    } catch { return [] }
  })()

  const sharedInputStyle = { width: '100%', padding: '5px 7px', border: '1px solid #d0d5dd', borderRadius: '4px', fontSize: '12px', boxSizing: 'border-box' as const, marginBottom: '4px' }

  return (
    <div>
      {ctxPaths.length > 0 && (
        <select
          value={ctxPaths.includes(value.replace(/^\{\{|\}\}$/g, '')) ? value : ''}
          onChange={e => { if (e.target.value) onChange(e.target.value) }}
          style={{ ...sharedInputStyle, background: 'white' }}
        >
          <option value="">— pick from context —</option>
          {ctxPaths.map(p => (
            <option key={p} value={`{{${p}}}`}>{p}</option>
          ))}
        </select>
      )}
      <input
        value={value}
        onChange={e => onChange(e.target.value)}
        style={{ ...sharedInputStyle, fontFamily: 'monospace', background: ctxPaths.length > 0 ? '#f8f8f8' : 'white' }}
        placeholder={ctxPaths.length > 0 ? `or type {{${fieldName}}} manually` : `{{${fieldName}}} — run a prior node to populate picker`}
      />
    </div>
  )
}

// ── ContextKeyPicker ──────────────────────────────────────────────────────────
// Like ContextFieldPicker but stores the bare key name (no {{}}),
// used when the activity needs the key name itself (e.g. format_date).

function ContextKeyPicker({ value, onChange, debugContext }: {
  value: string
  onChange: (v: string) => void
  debugContext: string
}) {
  const [picking, setPicking] = useState(false)
  const ctxKeys: string[] = (() => {
    try {
      const ctx = JSON.parse(debugContext || '{}')
      const out: string[] = []
      collectLeafPaths(ctx, '', 0, out)
      return out.filter(p => p !== '' && !p.startsWith('_'))
    } catch { return [] }
  })()

  const sharedStyle = { width: '100%', padding: '5px 7px', border: '1px solid #d0d5dd', borderRadius: '4px', fontSize: '12px', boxSizing: 'border-box' as const, marginBottom: '4px' }

  if (picking) {
    return (
      <select
        autoFocus
        value=""
        onChange={e => { if (e.target.value) { onChange(e.target.value); setPicking(false) } }}
        onBlur={() => setPicking(false)}
        style={{ ...sharedStyle, background: 'white' }}
      >
        <option value="">— pick context key —</option>
        {ctxKeys.map(k => <option key={k} value={k}>{k}</option>)}
      </select>
    )
  }

  return (
    <div style={{ display: 'flex', gap: '4px', alignItems: 'center', marginBottom: '4px' }}>
      <input
        value={value}
        onChange={e => onChange(e.target.value)}
        style={{ ...sharedStyle, marginBottom: 0, fontFamily: 'monospace', flex: 1 }}
        placeholder="select or type a context key"
      />
      {ctxKeys.length > 0 && (
        <button
          onClick={() => setPicking(true)}
          title="Pick from context"
          style={{ padding: '4px 8px', border: '1px solid #d0d5dd', borderRadius: '4px', background: 'white', cursor: 'pointer', fontSize: '12px', whiteSpace: 'nowrap' }}
        >▾</button>
      )}
    </div>
  )
}

// ── DrivePicker ───────────────────────────────────────────────────────────────

interface DrivePickerProps {
  value: string
  displayName: string
  mode: 'folder' | 'file'
  onSelect: (id: string, name: string) => void
}

function DrivePicker({ value, displayName, mode, onSelect }: DrivePickerProps) {
  const [open, setOpen] = useState(false)
  const [items, setItems] = useState<DriveItem[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [breadcrumb, setBreadcrumb] = useState<Array<{ id: string; name: string }>>([{ id: '', name: 'My Drive' }])

  const currentParent = breadcrumb[breadcrumb.length - 1]

  const [notConnected, setNotConnected] = useState(false)

  const loadFolder = async (parentId: string) => {
    setLoading(true)
    setError('')
    setNotConnected(false)
    try {
      const url = parentId
        ? `/api/google/drive/browse?parent=${encodeURIComponent(parentId)}`
        : '/api/google/drive/browse'
      const res = await fetch(url, { credentials: 'include' })
      const data = await res.json()
      if (!res.ok || data?.error) {
        const msg: string = data?.error || await res.text()
        if (msg.includes('not configured') || msg.includes('auth/start')) {
          setNotConnected(true)
          return
        }
        throw new Error(msg)
      }
      setItems((data || []).map((f: any) => ({
        id: f.id,
        name: f.name,
        type: f.is_folder ? 'folder' : 'file',
        mime_type: f.mimeType || f.mime_type,
      })))
    } catch (e: any) {
      setError(e.message || 'Failed to load Drive files')
    } finally {
      setLoading(false)
    }
  }

  const openModal = () => {
    setOpen(true)
    setBreadcrumb([{ id: '', name: 'My Drive' }])
    loadFolder('')
  }

  const navigateInto = (item: DriveItem) => {
    if (item.type !== 'folder') return
    const newCrumb = [...breadcrumb, { id: item.id, name: item.name }]
    setBreadcrumb(newCrumb)
    loadFolder(item.id)
  }

  const navigateTo = (idx: number) => {
    const newCrumb = breadcrumb.slice(0, idx + 1)
    setBreadcrumb(newCrumb)
    loadFolder(breadcrumb[idx].id)
  }

  const selectFolder = () => {
    onSelect(currentParent.id, currentParent.name)
    setOpen(false)
  }

  const selectFile = (item: DriveItem) => {
    onSelect(item.id, item.name)
    setOpen(false)
  }

  return (
    <div>
      <div style={{ display: 'flex', gap: '6px', alignItems: 'center' }}>
        <div style={{
          flex: 1, padding: '5px 8px', background: '#f8fafc', border: '1px solid #ddd',
          borderRadius: '4px', fontSize: '12px', color: value ? '#333' : '#999',
          overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap',
        }}>
          {displayName ? `${mode === 'folder' ? '📁' : '📄'} ${displayName}` : '— not selected —'}
        </div>
        <button
          onClick={openModal}
          style={{
            padding: '5px 10px', background: '#0369a1', color: 'white',
            border: 'none', borderRadius: '4px', fontSize: '11px', cursor: 'pointer',
            whiteSpace: 'nowrap', fontWeight: 600,
          }}
        >
          Browse
        </button>
      </div>
      <input
        value={value}
        onChange={e => onSelect(e.target.value, '')}
        placeholder={`{{${mode === 'file' ? 'file_id' : 'folder_id'}}} or paste ID`}
        style={{ marginTop: '4px', width: '100%', boxSizing: 'border-box', padding: '4px 8px', fontSize: '11px', fontFamily: 'monospace', border: '1px solid #e2e8f0', borderRadius: '4px', color: value.startsWith('{{') ? '#0369a1' : '#555', background: '#fafafa' }}
      />

      {open && (
        <div
          style={{
            position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.5)',
            zIndex: 9999, display: 'flex', alignItems: 'center', justifyContent: 'center',
          }}
          onClick={e => { if (e.target === e.currentTarget) setOpen(false) }}
        >
          <div style={{
            background: 'white', borderRadius: '8px', width: '520px', maxHeight: '520px',
            display: 'flex', flexDirection: 'column', boxShadow: '0 20px 60px rgba(0,0,0,0.3)',
          }}>
            {/* Header */}
            <div style={{ padding: '14px 16px', borderBottom: '1px solid #e0e6ed', display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
              <span style={{ fontWeight: 700, fontSize: '13px' }}>
                {mode === 'folder' ? 'Select a Folder' : 'Select a File'}
              </span>
              <button onClick={() => setOpen(false)} style={{ background: 'none', border: 'none', fontSize: '16px', cursor: 'pointer', color: '#666' }}>✕</button>
            </div>

            {/* Breadcrumb */}
            <div style={{ padding: '8px 16px', borderBottom: '1px solid #f0f0f0', display: 'flex', alignItems: 'center', gap: '4px', flexWrap: 'wrap' }}>
              {breadcrumb.map((crumb, idx) => (
                <span key={idx} style={{ display: 'flex', alignItems: 'center', gap: '4px' }}>
                  {idx > 0 && <span style={{ color: '#aaa', fontSize: '11px' }}>›</span>}
                  <span
                    onClick={() => idx < breadcrumb.length - 1 && navigateTo(idx)}
                    style={{
                      fontSize: '11px',
                      color: idx === breadcrumb.length - 1 ? '#333' : '#0369a1',
                      cursor: idx === breadcrumb.length - 1 ? 'default' : 'pointer',
                      fontWeight: idx === breadcrumb.length - 1 ? 600 : 400,
                    }}
                  >{crumb.name}</span>
                </span>
              ))}
            </div>

            {/* Select this folder button (folder mode, not at root) */}
            {mode === 'folder' && currentParent.id !== '' && (
              <div style={{ padding: '8px 16px', borderBottom: '1px solid #f0f0f0' }}>
                <button
                  onClick={selectFolder}
                  style={{
                    width: '100%', padding: '7px', background: '#16a34a', color: 'white',
                    border: 'none', borderRadius: '4px', fontSize: '12px', cursor: 'pointer',
                    fontWeight: 600,
                  }}
                >
                  ✓ Select "{currentParent.name}"
                </button>
              </div>
            )}

            {/* File list */}
            <div style={{ flex: 1, overflowY: 'auto', padding: '8px 0' }}>
              {loading && <div style={{ padding: '20px', textAlign: 'center', color: '#888', fontSize: '12px' }}>Loading...</div>}
              {notConnected && (
                <div style={{ padding: '24px 20px', textAlign: 'center' }}>
                  <div style={{ fontSize: '13px', fontWeight: 600, color: '#334155', marginBottom: '8px' }}>Google Drive not connected</div>
                  <div style={{ fontSize: '11px', color: '#64748b', marginBottom: '16px' }}>You need to authorize access to your Google Drive first.</div>
                  <a
                    href="/api/google/auth/start"
                    target="_blank"
                    rel="noopener noreferrer"
                    onClick={() => setOpen(false)}
                    style={{
                      display: 'inline-block', padding: '8px 16px',
                      background: '#0369a1', color: 'white', borderRadius: '5px',
                      fontSize: '12px', fontWeight: 600, textDecoration: 'none',
                    }}
                  >
                    Connect Google Drive
                  </a>
                </div>
              )}
              {error && <div style={{ padding: '12px 16px', color: '#e74c3c', fontSize: '12px' }}>{error}</div>}
              {!loading && !notConnected && !error && items.length === 0 && (
                <div style={{ padding: '20px', textAlign: 'center', color: '#aaa', fontSize: '12px' }}>
                  {mode === 'folder' ? 'No subfolders' : 'Empty folder'}
                </div>
              )}
              {!loading && items.map(item => {
                const isFolder = item.type === 'folder'
                if (mode === 'folder' && !isFolder) return null
                return (
                  <div
                    key={item.id}
                    onClick={() => isFolder ? navigateInto(item) : selectFile(item)}
                    style={{
                      display: 'flex', alignItems: 'center', gap: '8px',
                      padding: '7px 16px', cursor: 'pointer', background: 'transparent',
                    }}
                    onMouseEnter={e => { (e.currentTarget as HTMLDivElement).style.background = '#f0f4ff' }}
                    onMouseLeave={e => { (e.currentTarget as HTMLDivElement).style.background = 'transparent' }}
                  >
                    <span style={{ fontSize: '15px' }}>{isFolder ? '📁' : '📄'}</span>
                    <span style={{ fontSize: '12px', flex: 1 }}>{item.name}</span>
                    {isFolder && <span style={{ color: '#aaa', fontSize: '11px' }}>›</span>}
                  </div>
                )
              })}
            </div>
          </div>
        </div>
      )}
    </div>
  )
}

// ── PromptVarsBuilderModal ────────────────────────────────────────────────────

function PromptVarsBuilderModal({ existingVars, onSave, onClose, defaultSourceJson = '' }: {
  existingVars: Record<string, string>
  onSave: (vars: Record<string, string>) => void
  onClose: () => void
  defaultSourceJson?: string
}) {
  const [sourceText, setSourceText] = useState(defaultSourceJson)
  const [sourcePaths, setSourcePaths] = useState<string[]>([])
  const [sourceParseError, setSourceParseError] = useState('')
  const [sourceFilter, setSourceFilter] = useState('')
  const [vars, setVars] = useState<Array<{ key: string; value: string }>>(
    Object.entries(existingVars).map(([k, v]) => ({ key: k, value: v }))
  )
  const dragRef = useRef<string>('')
  const [dragOverIdx, setDragOverIdx] = useState<number | null>(null)

  useEffect(() => {
    if (!defaultSourceJson || defaultSourceJson.trim() === '' || defaultSourceJson.trim() === '{}') return
    try {
      const parsed = JSON.parse(defaultSourceJson)
      const out: string[] = []
      collectLeafPaths(parsed, '', 0, out)
      setSourcePaths(out.filter(p => p !== ''))
    } catch { /* ignore */ }
  }, [defaultSourceJson])

  const parseSource = () => {
    setSourceParseError('')
    setSourcePaths([])
    let parsed: unknown
    try { parsed = JSON.parse(sourceText) } catch { setSourceParseError('Invalid JSON.'); return }
    const out: string[] = []
    collectLeafPaths(parsed, '', 0, out)
    setSourcePaths(out.filter(p => p !== ''))
  }

  const addBlank = () => setVars(v => [...v, { key: `var${v.length + 1}`, value: '' }])

  const addFromCtx = (path: string) => {
    if (!path) return
    const lastSeg = path.split('.').pop() ?? path
    const varName = lastSeg.replace(/[^a-zA-Z0-9_]/g, '_')
    const key = vars.some(r => r.key === varName) ? path.replace(/\./g, '_') : varName
    setVars(v => [...v, { key, value: `{{${path}}}` }])
  }

  const updateKey = (i: number, k: string) => setVars(v => v.map((r, idx) => idx === i ? { ...r, key: k } : r))
  const updateVal = (i: number, val: string) => setVars(v => v.map((r, idx) => idx === i ? { ...r, value: val } : r))
  const removeRow = (i: number) => setVars(v => v.filter((_, idx) => idx !== i))

  const handleDrop = (i: number) => {
    const from = dragRef.current
    if (!from) return
    updateVal(i, `{{${from}}}`)
    setDragOverIdx(null)
  }

  const usedPaths = new Set(
    vars.map(r => { const m = r.value.match(/^\{\{(.+)\}\}$/); return m ? m[1] : null }).filter(Boolean) as string[]
  )

  const save = () => {
    const result: Record<string, string> = {}
    for (const { key, value } of vars) {
      if (key.trim()) result[key.trim()] = value
    }
    onSave(result)
  }

  return (
    <div style={{ position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.65)', display: 'flex', alignItems: 'center', justifyContent: 'center', zIndex: 2100 }}>
      <div style={{ background: 'white', borderRadius: '10px', width: '860px', maxHeight: '85vh', display: 'flex', flexDirection: 'column', boxShadow: '0 24px 64px rgba(0,0,0,0.45)', overflow: 'hidden' }}>

        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', padding: '14px 20px', background: '#1e293b', color: 'white' }}>
          <div>
            <div style={{ fontWeight: 700, fontSize: '15px' }}>Prompt Variables Builder</div>
            <div style={{ fontSize: '11px', color: '#94a3b8', marginTop: '2px' }}>Define named variables → use {'{{var_name}}'} in your prompt</div>
          </div>
          <button onClick={onClose} style={{ background: 'none', border: 'none', cursor: 'pointer', fontSize: '18px', color: '#94a3b8', lineHeight: 1 }}>✕</button>
        </div>

        <div style={{ flex: 1, overflow: 'auto', display: 'flex', gap: 0, minHeight: 0 }}>

          {/* LEFT — Context Keys */}
          <div style={{ flex: '0 0 300px', borderRight: '1px solid #e0e6ed', padding: '16px', display: 'flex', flexDirection: 'column', gap: '8px', overflow: 'auto' }}>
            <div style={{ fontSize: '11px', fontWeight: 700, color: '#555', textTransform: 'uppercase', letterSpacing: '0.5px' }}>
              Context Keys <span style={{ fontWeight: 400, color: '#aaa', textTransform: 'none' }}>(drag onto values)</span>
            </div>
            <div style={{ fontSize: '10px', color: '#aaa', lineHeight: 1.5 }}>
              Paste sample JSON from a previous node, then click Parse Keys.
            </div>
            <textarea
              value={sourceText}
              onChange={e => setSourceText(e.target.value)}
              rows={6}
              placeholder={'{\n  "first_name": "Jane",\n  "last_name": "Smith",\n  "email": "jane@example.com"\n}'}
              spellCheck={false}
              style={{ width: '100%', boxSizing: 'border-box', fontFamily: 'monospace', fontSize: '11px', padding: '6px 8px', border: '1px solid #ddd', borderRadius: '4px', resize: 'vertical' }}
            />
            {sourceParseError && <div style={{ color: '#e74c3c', fontSize: '11px' }}>{sourceParseError}</div>}
            <button
              onClick={parseSource}
              style={{ padding: '5px', background: '#3b82f6', color: 'white', border: 'none', borderRadius: '4px', cursor: 'pointer', fontWeight: 600, fontSize: '11px' }}
            >
              Parse Keys
            </button>

            {sourcePaths.length > 0 && (
              <>
                <div style={{ display: 'flex', alignItems: 'center', gap: '6px', marginTop: '4px' }}>
                  <div style={{ fontSize: '10px', fontWeight: 700, color: '#555', whiteSpace: 'nowrap' }}>SOURCE KEYS:</div>
                  <input
                    type="text"
                    placeholder="filter..."
                    value={sourceFilter}
                    onChange={e => setSourceFilter(e.target.value)}
                    style={{ flex: 1, fontSize: '10px', padding: '3px 6px', border: '1px solid #ddd', borderRadius: '4px', fontFamily: 'monospace', minWidth: 0 }}
                  />
                  {sourceFilter && (
                    <button onClick={() => setSourceFilter('')} style={{ background: 'none', border: 'none', cursor: 'pointer', color: '#aaa', fontSize: '12px', padding: 0, lineHeight: 1 }}>✕</button>
                  )}
                </div>
                <div style={{ display: 'flex', flexWrap: 'wrap', gap: '5px' }}>
                  {sourcePaths.filter(p => !sourceFilter || p.toLowerCase().includes(sourceFilter.toLowerCase())).map(p => {
                    const isUsed = usedPaths.has(p)
                    return (
                      <div
                        key={p}
                        draggable
                        onDragStart={() => { dragRef.current = p }}
                        onDragEnd={() => { dragRef.current = '' }}
                        onClick={() => addFromCtx(p)}
                        style={{
                          display: 'inline-flex', alignItems: 'center', gap: '3px',
                          background: isUsed ? '#d1fae5' : '#dbeafe',
                          border: `1px solid ${isUsed ? '#34d399' : '#93c5fd'}`,
                          borderRadius: '4px', padding: '3px 7px',
                          fontFamily: 'monospace', fontSize: '10px',
                          color: isUsed ? '#065f46' : '#1e40af',
                          cursor: 'grab', userSelect: 'none', opacity: isUsed ? 0.75 : 1,
                        }}
                        title="Drag onto a value field, or click to add as new variable"
                      >
                        {isUsed && <span style={{ fontSize: '9px' }}>✓</span>}
                        {p}
                      </div>
                    )
                  })}
                </div>
              </>
            )}
          </div>

          {/* RIGHT — Prompt Variables */}
          <div style={{ flex: 1, padding: '16px', display: 'flex', flexDirection: 'column', gap: '8px', overflow: 'auto' }}>
            <div style={{ fontSize: '11px', fontWeight: 700, color: '#555', textTransform: 'uppercase', letterSpacing: '0.5px' }}>
              Prompt Variables <span style={{ fontWeight: 400, color: '#aaa', textTransform: 'none' }}>(drop context keys onto values)</span>
            </div>
            <div style={{ fontSize: '10px', color: '#aaa', lineHeight: 1.5 }}>
              Each variable becomes <code>{'{{var_name}}'}</code> in your prompt and system prompt.
            </div>

            {vars.map(({ key, value }, i) => {
              const isOver = dragOverIdx === i
              const isMapped = value.startsWith('{{') && value.endsWith('}}')
              return (
                <div
                  key={i}
                  onDragOver={e => { e.preventDefault(); setDragOverIdx(i) }}
                  onDragLeave={() => setDragOverIdx(null)}
                  onDrop={() => handleDrop(i)}
                  style={{
                    display: 'flex', alignItems: 'center', gap: '6px',
                    background: isOver ? '#dcfce7' : isMapped ? '#f0fdf4' : '#f8fafc',
                    border: `1px dashed ${isOver ? '#22c55e' : isMapped ? '#86efac' : '#cbd5e1'}`,
                    borderRadius: '4px', padding: '6px 8px', transition: 'background 0.1s, border-color 0.1s',
                  }}
                >
                  <input
                    value={key}
                    onChange={e => updateKey(i, e.target.value)}
                    placeholder="var_name"
                    title="Used in prompt as {{var_name}}"
                    style={{ flex: '0 0 110px', fontFamily: 'monospace', fontSize: '11px', padding: '4px 6px', border: '1px solid #e2e8f0', borderRadius: '3px', background: 'white' }}
                  />
                  <span style={{ color: '#bbb', fontSize: '11px', flexShrink: 0 }}>→</span>
                  <input
                    value={value}
                    onChange={e => updateVal(i, e.target.value)}
                    placeholder="{{context_key}} or literal value"
                    style={{ flex: 1, fontFamily: 'monospace', fontSize: '11px', padding: '4px 6px', border: '1px solid #e2e8f0', borderRadius: '3px', background: isMapped ? '#f0fdf4' : 'white', color: isMapped ? '#0d9488' : '#334155', minWidth: 0 }}
                  />
                  <button onClick={() => removeRow(i)} style={{ flexShrink: 0, border: 'none', background: 'none', color: '#e74c3c', cursor: 'pointer', fontSize: '16px', padding: '0 2px', lineHeight: 1 }}>×</button>
                </div>
              )
            })}

            <button
              onClick={addBlank}
              style={{ alignSelf: 'flex-start', fontSize: '11px', padding: '4px 10px', border: '1px solid #d0d5dd', borderRadius: '4px', background: 'white', cursor: 'pointer', color: '#555' }}
            >
              + Add Variable
            </button>
          </div>
        </div>

        <div style={{ display: 'flex', justifyContent: 'flex-end', gap: '10px', padding: '12px 20px', borderTop: '1px solid #e0e6ed', background: '#f8f9fa' }}>
          <button onClick={onClose} style={{ padding: '8px 16px', background: '#f0f0f0', border: 'none', borderRadius: '4px', cursor: 'pointer', fontWeight: 500, fontSize: '13px' }}>
            Cancel
          </button>
          <button
            onClick={save}
            style={{ padding: '8px 16px', background: '#7c3aed', color: 'white', border: 'none', borderRadius: '4px', cursor: 'pointer', fontWeight: 600, fontSize: '13px' }}
          >
            Apply Variables
          </button>
        </div>
      </div>
    </div>
  )
}

// ── GdriveTemplateVarsBuilderModal ────────────────────────────────────────────

function GdriveTemplateVarsBuilderModal({ templateId, existingVars, existingImageVars, onSave, onClose, defaultSourceJson = '' }: {
  templateId: string
  existingVars: Record<string, string>
  existingImageVars: Record<string, string>
  onSave: (vars: Record<string, string>, imageVars: Record<string, string>) => void
  onClose: () => void
  defaultSourceJson?: string
}) {
  const [loading, setLoading] = useState(false)
  const [docContent, setDocContent] = useState('')
  const [fetchError, setFetchError] = useState('')
  const [sourceText, setSourceText] = useState(defaultSourceJson)
  const [sourcePaths, setSourcePaths] = useState<string[]>([])
  const [sourceParseError, setSourceParseError] = useState('')
  const [sourceFilter, setSourceFilter] = useState('')
  const [vars, setVars] = useState<Array<{ key: string; value: string }>>(
    Object.entries(existingVars).map(([k, v]) => ({ key: k, value: v }))
  )
  const [imageVars, setImageVars] = useState<Array<{ key: string; value: string }>>(
    Object.entries(existingImageVars).map(([k, v]) => ({ key: k, value: v }))
  )
  const dragRef = useRef<string>('')
  const [dragOverIdx, setDragOverIdx] = useState<number | null>(null)
  const [dragOverImgIdx, setDragOverImgIdx] = useState<number | null>(null)

  // Fetch and parse template document on mount
  useEffect(() => {
    if (!templateId) {
      setFetchError('No template selected')
      return
    }
    setLoading(true)
    setFetchError('')

    // Fetch the document content from the backend
    fetch(`/api/workflows/gdrive-content?file_id=${encodeURIComponent(templateId)}`)
      .then(r => r.json())
      .then(data => {
        if (data.error) {
          setFetchError(data.error)
        } else {
          const text = data.text || ''
          setDocContent(text)
          // Extract placeholders like {{field_name}}
          const placeholderRegex = /\{\{([a-zA-Z_][a-zA-Z0-9_]*)\}\}/g
          const matches = new Set<string>()
          let match
          while ((match = placeholderRegex.exec(text)) !== null) {
            matches.add(match[1])
          }
          // Convert to existing vars format if not already mapped
          setVars(prev => {
            const existing = new Set(prev.map(v => v.key))
            const newVars = Array.from(matches)
              .filter(key => !existing.has(key))
              .map(key => ({ key, value: '' }))
            return [...prev, ...newVars]
          })
        }
        setLoading(false)
      })
      .catch(err => {
        setFetchError(err.message)
        setLoading(false)
      })
  }, [templateId])

  useEffect(() => {
    if (!defaultSourceJson || defaultSourceJson.trim() === '' || defaultSourceJson.trim() === '{}') return
    try {
      const parsed = JSON.parse(defaultSourceJson)
      const out: string[] = []
      collectLeafPaths(parsed, '', 0, out)
      setSourcePaths(out.filter(p => p !== ''))
    } catch { /* ignore */ }
  }, [defaultSourceJson])

  const parseSource = () => {
    setSourceParseError('')
    setSourcePaths([])
    let parsed: unknown
    try { parsed = JSON.parse(sourceText) } catch { setSourceParseError('Invalid JSON.'); return }
    const out: string[] = []
    collectLeafPaths(parsed, '', 0, out)
    setSourcePaths(out.filter(p => p !== ''))
  }

  const addBlank = () => setVars(v => [...v, { key: `var${v.length + 1}`, value: '' }])

  const addFromCtx = (path: string) => {
    if (!path) return
    const lastSeg = path.split('.').pop() ?? path
    const varName = lastSeg.replace(/[^a-zA-Z0-9_]/g, '_')
    const key = vars.some(r => r.key === varName) ? path.replace(/\./g, '_') : varName
    setVars(v => [...v, { key, value: `{{${path}}}` }])
  }

  const updateKey = (i: number, k: string) => setVars(v => v.map((r, idx) => idx === i ? { ...r, key: k } : r))
  const updateVal = (i: number, val: string) => setVars(v => v.map((r, idx) => idx === i ? { ...r, value: val } : r))
  const removeRow = (i: number) => setVars(v => v.filter((_, idx) => idx !== i))

  const handleDrop = (i: number) => {
    const from = dragRef.current
    if (!from) return
    updateVal(i, `{{${from}}}`)
    setDragOverIdx(null)
  }

  const addBlankImage = () => setImageVars(v => [...v, { key: '', value: '' }])
  const updateImgKey = (i: number, k: string) => setImageVars(v => v.map((r, idx) => idx === i ? { ...r, key: k } : r))
  const updateImgVal = (i: number, val: string) => setImageVars(v => v.map((r, idx) => idx === i ? { ...r, value: val } : r))
  const removeImgRow = (i: number) => setImageVars(v => v.filter((_, idx) => idx !== i))

  const handleImgDrop = (i: number) => {
    const from = dragRef.current
    if (!from) return
    updateImgVal(i, `{{${from}}}`)
    setDragOverImgIdx(null)
  }

  const usedPaths = new Set(
    vars.map(r => { const m = r.value.match(/^\{\{(.+)\}\}$/); return m ? m[1] : null }).filter(Boolean) as string[]
  )

  const save = () => {
    const result: Record<string, string> = {}
    for (const { key, value } of vars) {
      if (key.trim()) result[key.trim()] = value
    }
    const imgResult: Record<string, string> = {}
    for (const { key, value } of imageVars) {
      if (key.trim()) imgResult[key.trim()] = value
    }
    onSave(result, imgResult)
  }

  return (
    <div style={{ position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.65)', display: 'flex', alignItems: 'center', justifyContent: 'center', zIndex: 2100 }}>
      <div style={{ background: 'white', borderRadius: '10px', width: '860px', maxHeight: '85vh', display: 'flex', flexDirection: 'column', boxShadow: '0 24px 64px rgba(0,0,0,0.45)', overflow: 'hidden' }}>

        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', padding: '14px 20px', background: '#1e293b', color: 'white' }}>
          <div>
            <div style={{ fontWeight: 700, fontSize: '15px' }}>Template Variables Builder</div>
            <div style={{ fontSize: '11px', color: '#94a3b8', marginTop: '2px' }}>Map document placeholders {'{{field_name}}'} to context values</div>
          </div>
          <button onClick={onClose} style={{ background: 'none', border: 'none', cursor: 'pointer', fontSize: '18px', color: '#94a3b8', lineHeight: 1 }}>✕</button>
        </div>

        {loading ? (
          <div style={{ padding: '40px', textAlign: 'center', color: '#666' }}>Loading document...</div>
        ) : fetchError ? (
          <div style={{ padding: '20px', color: '#e74c3c', fontSize: '13px' }}>{fetchError}</div>
        ) : (
          <div style={{ flex: 1, overflow: 'auto', display: 'flex', gap: 0, minHeight: 0 }}>

            {/* LEFT — Context Keys */}
            <div style={{ flex: '0 0 300px', borderRight: '1px solid #e0e6ed', padding: '16px', display: 'flex', flexDirection: 'column', gap: '8px', overflow: 'auto' }}>
              <div style={{ fontSize: '11px', fontWeight: 700, color: '#555', textTransform: 'uppercase', letterSpacing: '0.5px' }}>
                Context Keys <span style={{ fontWeight: 400, color: '#aaa', textTransform: 'none' }}>(drag onto values)</span>
              </div>
              <div style={{ fontSize: '10px', color: '#aaa', lineHeight: 1.5 }}>
                Paste sample JSON from a previous node, then click Parse Keys.
              </div>
              <textarea
                value={sourceText}
                onChange={e => setSourceText(e.target.value)}
                rows={6}
                placeholder={'{\n  "first_name": "Jane",\n  "last_name": "Smith",\n  "email": "jane@example.com"\n}'}
                spellCheck={false}
                style={{ width: '100%', boxSizing: 'border-box', fontFamily: 'monospace', fontSize: '11px', padding: '6px 8px', border: '1px solid #ddd', borderRadius: '4px', resize: 'vertical' }}
              />
              {sourceParseError && <div style={{ color: '#e74c3c', fontSize: '11px' }}>{sourceParseError}</div>}
              <button
                onClick={parseSource}
                style={{ padding: '5px', background: '#3b82f6', color: 'white', border: 'none', borderRadius: '4px', cursor: 'pointer', fontWeight: 600, fontSize: '11px' }}
              >
                Parse Keys
              </button>

              {sourcePaths.length > 0 && (
                <>
                  <div style={{ display: 'flex', alignItems: 'center', gap: '6px', marginTop: '4px' }}>
                    <div style={{ fontSize: '10px', fontWeight: 700, color: '#555', whiteSpace: 'nowrap' }}>SOURCE KEYS:</div>
                    <input
                      type="text"
                      placeholder="filter..."
                      value={sourceFilter}
                      onChange={e => setSourceFilter(e.target.value)}
                      style={{ flex: 1, fontSize: '10px', padding: '3px 6px', border: '1px solid #ddd', borderRadius: '4px', fontFamily: 'monospace', minWidth: 0 }}
                    />
                    {sourceFilter && (
                      <button onClick={() => setSourceFilter('')} style={{ background: 'none', border: 'none', cursor: 'pointer', color: '#aaa', fontSize: '12px', padding: 0, lineHeight: 1 }}>✕</button>
                    )}
                  </div>
                  <div style={{ display: 'flex', flexWrap: 'wrap', gap: '5px' }}>
                    {sourcePaths.filter(p => !sourceFilter || p.toLowerCase().includes(sourceFilter.toLowerCase())).map(p => {
                      const isUsed = usedPaths.has(p)
                      return (
                        <div
                          key={p}
                          draggable
                          onDragStart={() => { dragRef.current = p }}
                          onDragEnd={() => { dragRef.current = '' }}
                          onClick={() => addFromCtx(p)}
                          style={{
                            display: 'inline-flex', alignItems: 'center', gap: '3px',
                            background: isUsed ? '#d1fae5' : '#dbeafe',
                            border: `1px solid ${isUsed ? '#34d399' : '#93c5fd'}`,
                            borderRadius: '4px', padding: '3px 7px',
                            fontFamily: 'monospace', fontSize: '10px',
                            color: isUsed ? '#065f46' : '#1e40af',
                            cursor: 'grab', userSelect: 'none', opacity: isUsed ? 0.75 : 1,
                          }}
                          title="Drag onto a value field, or click to add as new variable"
                        >
                          {isUsed && <span style={{ fontSize: '9px' }}>✓</span>}
                          {p}
                        </div>
                      )
                    })}
                  </div>
                </>
              )}
            </div>

            {/* RIGHT — Template Variables */}
            <div style={{ flex: 1, padding: '16px', display: 'flex', flexDirection: 'column', gap: '8px', overflow: 'auto' }}>
              <div style={{ fontSize: '11px', fontWeight: 700, color: '#555', textTransform: 'uppercase', letterSpacing: '0.5px' }}>
                Template Placeholders <span style={{ fontWeight: 400, color: '#aaa', textTransform: 'none' }}>(drop context keys onto values)</span>
              </div>
              <div style={{ fontSize: '10px', color: '#aaa', lineHeight: 1.5 }}>
                Each variable becomes <code>{'{{field_name}}'}</code> in the document and gets replaced with a context value.
              </div>

              {vars.map(({ key, value }, i) => {
                const isOver = dragOverIdx === i
                const isMapped = value.startsWith('{{') && value.endsWith('}}')
                return (
                  <div
                    key={i}
                    onDragOver={e => { e.preventDefault(); setDragOverIdx(i) }}
                    onDragLeave={() => setDragOverIdx(null)}
                    onDrop={() => handleDrop(i)}
                    style={{
                      display: 'flex', alignItems: 'center', gap: '6px',
                      background: isOver ? '#dcfce7' : isMapped ? '#f0fdf4' : '#f8fafc',
                      border: `1px dashed ${isOver ? '#22c55e' : isMapped ? '#86efac' : '#cbd5e1'}`,
                      borderRadius: '4px', padding: '6px 8px', transition: 'background 0.1s, border-color 0.1s',
                    }}
                  >
                    <input
                      value={key}
                      onChange={e => updateKey(i, e.target.value)}
                      placeholder="var_name"
                      title="Used in document as {{var_name}}"
                      style={{ flex: '0 0 110px', fontFamily: 'monospace', fontSize: '11px', padding: '4px 6px', border: '1px solid #e2e8f0', borderRadius: '3px', background: 'white' }}
                    />
                    <span style={{ color: '#bbb', fontSize: '11px', flexShrink: 0 }}>→</span>
                    <input
                      value={value}
                      onChange={e => updateVal(i, e.target.value)}
                      placeholder="{{context_key}} or literal value"
                      style={{ flex: 1, fontFamily: 'monospace', fontSize: '11px', padding: '4px 6px', border: '1px solid #e2e8f0', borderRadius: '3px', background: isMapped ? '#f0fdf4' : 'white', color: isMapped ? '#0d9488' : '#334155', minWidth: 0 }}
                    />
                    <button onClick={() => removeRow(i)} style={{ flexShrink: 0, border: 'none', background: 'none', color: '#e74c3c', cursor: 'pointer', fontSize: '16px', padding: '0 2px', lineHeight: 1 }}>×</button>
                  </div>
                )
              })}

              <button
                onClick={addBlank}
                style={{ alignSelf: 'flex-start', fontSize: '11px', padding: '4px 10px', border: '1px solid #d0d5dd', borderRadius: '4px', background: 'white', cursor: 'pointer', color: '#555' }}
              >
                + Add Variable
              </button>

              {/* ── Replace Images subsection ── */}
              <div style={{ borderTop: '2px solid #e0e6ed', marginTop: '12px', paddingTop: '12px' }}>
                <div style={{ fontSize: '11px', fontWeight: 700, color: '#555', textTransform: 'uppercase', letterSpacing: '0.5px', marginBottom: '4px' }}>
                  Replace Images <span style={{ fontWeight: 400, color: '#aaa', textTransform: 'none' }}>(alt text → image URL)</span>
                </div>
                <div style={{ fontSize: '10px', color: '#aaa', lineHeight: 1.5, marginBottom: '6px' }}>
                  Set an image's alt text as the key and provide its replacement URL as the value.
                </div>

                {imageVars.map(({ key, value }, i) => {
                  const isOver = dragOverImgIdx === i
                  const isMapped = value.startsWith('{{') && value.endsWith('}}')
                  return (
                    <div
                      key={i}
                      onDragOver={e => { e.preventDefault(); setDragOverImgIdx(i) }}
                      onDragLeave={() => setDragOverImgIdx(null)}
                      onDrop={() => handleImgDrop(i)}
                      style={{
                        display: 'flex', alignItems: 'center', gap: '6px',
                        background: isOver ? '#dcfce7' : isMapped ? '#f0fdf4' : '#f8fafc',
                        border: `1px dashed ${isOver ? '#22c55e' : isMapped ? '#86efac' : '#cbd5e1'}`,
                        borderRadius: '4px', padding: '6px 8px', marginBottom: '6px', transition: 'background 0.1s, border-color 0.1s',
                      }}
                    >
                      <input
                        value={key}
                        onChange={e => updateImgKey(i, e.target.value)}
                        placeholder="image alt text"
                        style={{ flex: '0 0 110px', fontFamily: 'monospace', fontSize: '11px', padding: '4px 6px', border: '1px solid #e2e8f0', borderRadius: '3px', background: 'white' }}
                      />
                      <span style={{ color: '#bbb', fontSize: '11px', flexShrink: 0 }}>→</span>
                      <input
                        value={value}
                        onChange={e => updateImgVal(i, e.target.value)}
                        placeholder="{{context_key}} or image URL"
                        style={{ flex: 1, fontFamily: 'monospace', fontSize: '11px', padding: '4px 6px', border: '1px solid #e2e8f0', borderRadius: '3px', background: isMapped ? '#f0fdf4' : 'white', color: isMapped ? '#0d9488' : '#334155', minWidth: 0 }}
                      />
                      <button onClick={() => removeImgRow(i)} style={{ flexShrink: 0, border: 'none', background: 'none', color: '#e74c3c', cursor: 'pointer', fontSize: '16px', padding: '0 2px', lineHeight: 1 }}>×</button>
                    </div>
                  )
                })}

                <button
                  onClick={addBlankImage}
                  style={{ alignSelf: 'flex-start', fontSize: '11px', padding: '4px 10px', border: '1px solid #d0d5dd', borderRadius: '4px', background: 'white', cursor: 'pointer', color: '#555' }}
                >
                  + Add Image
                </button>
              </div>
            </div>
          </div>
        )}

        <div style={{ display: 'flex', justifyContent: 'flex-end', gap: '10px', padding: '12px 20px', borderTop: '1px solid #e0e6ed', background: '#f8f9fa' }}>
          <button onClick={onClose} style={{ padding: '8px 16px', background: '#f0f0f0', border: 'none', borderRadius: '4px', cursor: 'pointer', fontWeight: 500, fontSize: '13px' }}>
            Cancel
          </button>
          <button
            onClick={save}
            disabled={loading}
            style={{ padding: '8px 16px', background: loading ? '#ccc' : '#06b6d4', color: 'white', border: 'none', borderRadius: '4px', cursor: loading ? 'not-allowed' : 'pointer', fontWeight: 600, fontSize: '13px' }}
          >
            Apply Variables
          </button>
        </div>
      </div>
    </div>
  )
}

// ── HttpBodyBuilderModal ──────────────────────────────────────────────────────

type TypeHint = 'string' | 'number' | 'bool'

function HttpBodyBuilderModal({ existingBody, onSave, onClose, defaultSourceJson = '' }: {
  existingBody: string
  onSave: (body: string) => void
  onClose: () => void
  defaultSourceJson?: string
}) {
  const [sourceText, setSourceText] = useState(defaultSourceJson)
  const [bodyText, setBodyText] = useState(existingBody)
  const [sourcePaths, setSourcePaths] = useState<string[]>([])
  const [sourceParseError, setSourceParseError] = useState('')
  const [sourceFilter, setSourceFilter] = useState('')
  const [bodyParseError, setBodyParseError] = useState('')
  const [bodyObj, setBodyObj] = useState<Record<string, unknown>>(() => {
    try { return JSON.parse(existingBody) || {} } catch { return {} }
  })
  const [bodyLeaves, setBodyLeaves] = useState<Array<{ path: string; value: unknown }>>([])
  const [dragOverPath, setDragOverPath] = useState<string | null>(null)
  const dragRef = useRef<string>('')
  const [typeHints, setTypeHints] = useState<Record<string, TypeHint>>(() => {
    const hints: Record<string, TypeHint> = {}
    try {
      const parsed = JSON.parse(existingBody)
      const leaves: Array<{path: string; value: unknown}> = []
      collectLeafPathsWithValues(parsed, '', 0, leaves)
      for (const l of leaves) {
        const m = String(l.value ?? '').match(/^\{\{.+\|(number|bool|string)\}\}$/)
        if (m) hints[l.path] = m[1] as TypeHint
      }
    } catch { /* ignore */ }
    return hints
  })

  // Auto-parse defaultSourceJson when it changes (e.g. debug context passed in)
  useEffect(() => {
    if (!defaultSourceJson || defaultSourceJson.trim() === '' || defaultSourceJson.trim() === '{}') return
    try {
      const parsed = JSON.parse(defaultSourceJson)
      const out: string[] = []
      collectLeafPaths(parsed, '', 0, out)
      setSourcePaths(out.filter(p => p !== ''))
      setSourceParseError('')
    } catch { /* invalid JSON - ignore */ }
  }, [defaultSourceJson])

  // Parse source JSON into leaf paths
  const parseSource = () => {
    setSourceParseError('')
    setSourcePaths([])
    let parsed: unknown
    try { parsed = JSON.parse(sourceText) } catch { setSourceParseError('Invalid JSON.'); return }
    const out: string[] = []
    collectLeafPaths(parsed, '', 0, out)
    setSourcePaths(out.filter(p => p !== ''))
  }

  // Sanitize body text for JSON parsing: replace unquoted {{...}} with 0
  // and quoted "{{...}}" values are already valid strings — leave them.
  const sanitizeBodyForParse = (text: string): string => {
    // Replace unquoted {{...}} (not inside quotes) with placeholder number 0
    return text.replace(/:\s*(\{\{[^}]+\}\})/g, ': "__PLACEHOLDER__$1"')
  }

  // Parse body textarea JSON into leaf rows — tolerates {{...}} placeholders
  const parseBody = () => {
    setBodyParseError('')
    let parsed: Record<string, unknown>
    const sanitized = sanitizeBodyForParse(bodyText)
    try { parsed = JSON.parse(sanitized) } catch {
      // Try original as fallback
      try { parsed = JSON.parse(bodyText) } catch { setBodyParseError('Invalid JSON — check syntax.'); return }
    }
    // Restore placeholder values to original {{...}} form
    const restore = (obj: unknown): unknown => {
      if (typeof obj === 'string') return obj.replace(/^__PLACEHOLDER__/, '')
      if (Array.isArray(obj)) return obj.map(restore)
      if (obj && typeof obj === 'object') return Object.fromEntries(Object.entries(obj as Record<string, unknown>).map(([k, v]) => [k, restore(v)]))
      return obj
    }
    parsed = restore(parsed) as Record<string, unknown>
    setBodyObj(parsed)
    const out: Array<{ path: string; value: unknown }> = []
    collectLeafPathsWithValues(parsed, '', 0, out)
    setBodyLeaves(out.filter(l => l.path !== ''))
  }

  // Sync bodyText into bodyObj when user types directly
  const handleBodyTextChange = (text: string) => {
    setBodyText(text)
    try {
      const sanitized = sanitizeBodyForParse(text)
      const raw = JSON.parse(sanitized)
      const restore = (obj: unknown): unknown => {
        if (typeof obj === 'string') return obj.replace(/^__PLACEHOLDER__/, '')
        if (Array.isArray(obj)) return obj.map(restore)
        if (obj && typeof obj === 'object') return Object.fromEntries(Object.entries(obj as Record<string, unknown>).map(([k, v]) => [k, restore(v)]))
        return obj
      }
      const parsed = restore(raw) as Record<string, unknown>
      setBodyObj(parsed)
      const out: Array<{ path: string; value: unknown }> = []
      collectLeafPathsWithValues(parsed, '', 0, out)
      setBodyLeaves(out.filter(l => l.path !== ''))
      setBodyParseError('')
    } catch { /* silent — user still typing */ }
  }

  const handleDrop = (leafPath: string) => {
    const from = dragRef.current
    if (!from) return
    const updated = setLeafValue(bodyObj, leafPath, `{{${from}}}`)
    setBodyObj(updated)
    setBodyText(JSON.stringify(updated, null, 2))
    const out: Array<{ path: string; value: unknown }> = []
    collectLeafPathsWithValues(updated, '', 0, out)
    setBodyLeaves(out.filter(l => l.path !== ''))
    setDragOverPath(null)
  }

  // Apply type hints to bodyObj before saving — produces "{{key|number}}" etc. as string values.
  const applyTypeHints = (obj: Record<string, unknown>, hints: Record<string, TypeHint>, prefix = ''): Record<string, unknown> => {
    return Object.fromEntries(Object.entries(obj).map(([k, v]) => {
      const path = prefix ? `${prefix}.${k}` : k
      if (v !== null && typeof v === 'object' && !Array.isArray(v)) {
        return [k, applyTypeHints(v as Record<string, unknown>, hints, path)]
      }
      const hint = hints[path]
      if (hint && hint !== 'string' && typeof v === 'string') {
        const m = v.match(/^\{\{(.+)\}\}$/)
        if (m) {
          // Strip any existing |type suffixes before appending the new one
          const cleanPath = m[1].replace(/(\|(number|bool|string))+$/, '')
          return [k, `{{${cleanPath}|${hint}}}`]
        }
      }
      return [k, v]
    }))
  }

  // Strip ALL hint suffixes from a value string (handles repeated |number|number).
  const stripHint = (val: string): string => val.replace(/(\|(number|bool|string))+(\}\})$/, '$3')

  const usedPaths = new Set(
    bodyLeaves
      .filter(l => typeof l.value === 'string' && (l.value as string).startsWith('{{'))
      .map(l => {
        const m = stripHint(l.value as string).match(/^\{\{(.+)\}\}$/)
        return m ? m[1] : null
      })
      .filter(Boolean) as string[]
  )

  const previewJson = (() => {
    try {
      let json = JSON.stringify(applyTypeHints(bodyObj, typeHints), null, 2)
      // Remove quotes around number/bool placeholders so preview matches actual sent JSON
      json = json.replace(/"(\{\{[^}]+\|(number|bool)\}\})"/g, '$1')
      return json
    } catch { return bodyText }
  })()

  return (
    <div style={{ position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.65)', display: 'flex', alignItems: 'center', justifyContent: 'center', zIndex: 2100 }}>
      <div style={{ background: 'white', borderRadius: '10px', width: '900px', height: '90vh', display: 'flex', flexDirection: 'column', boxShadow: '0 24px 64px rgba(0,0,0,0.45)', overflow: 'hidden' }}>

        {/* Header */}
        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', padding: '14px 20px', background: '#1e293b', color: 'white' }}>
          <div style={{ fontWeight: 700, fontSize: '15px' }}>HTTP Body Builder</div>
          <button onClick={onClose} style={{ background: 'none', border: 'none', cursor: 'pointer', fontSize: '18px', color: '#94a3b8', lineHeight: 1 }}>✕</button>
        </div>

        {/* Two-column body */}
        <div style={{ flex: 1, overflow: 'auto', display: 'flex', gap: 0, minHeight: 0 }}>

          {/* LEFT — Context Keys */}
          <div style={{ flex: '0 0 300px', borderRight: '1px solid #e0e6ed', padding: '16px', display: 'flex', flexDirection: 'column', gap: '8px', overflow: 'auto' }}>
            <div style={{ fontSize: '11px', fontWeight: 700, color: '#555', textTransform: 'uppercase', letterSpacing: '0.5px' }}>
              Context Keys <span style={{ fontWeight: 400, color: '#aaa', textTransform: 'none' }}>(drag from here)</span>
            </div>
            <div style={{ fontSize: '10px', color: '#aaa', lineHeight: 1.5 }}>
              Paste sample JSON from the previous node output, then click Parse Keys.
            </div>
            <textarea
              value={sourceText}
              onChange={e => setSourceText(e.target.value)}
              rows={6}
              placeholder={'{\n  "results": [{"lat": 37.7, "lon": -122.4}],\n  "email_address": "user@example.com"\n}'}
              spellCheck={false}
              style={{ width: '100%', boxSizing: 'border-box', fontFamily: 'monospace', fontSize: '11px', padding: '6px 8px', border: '1px solid #ddd', borderRadius: '4px', resize: 'vertical' }}
            />
            {sourceParseError && <div style={{ color: '#e74c3c', fontSize: '11px' }}>{sourceParseError}</div>}
            <button
              onClick={parseSource}
              style={{ padding: '5px', background: '#3b82f6', color: 'white', border: 'none', borderRadius: '4px', cursor: 'pointer', fontWeight: 600, fontSize: '11px' }}
            >
              Parse Keys
            </button>

            {sourcePaths.length > 0 && (
              <>
                <div style={{ display: 'flex', alignItems: 'center', gap: '6px', marginTop: '4px' }}>
                  <div style={{ fontSize: '10px', fontWeight: 700, color: '#555', whiteSpace: 'nowrap' }}>SOURCE KEYS:</div>
                  <input
                    type="text"
                    placeholder="filter..."
                    value={sourceFilter}
                    onChange={e => setSourceFilter(e.target.value)}
                    style={{ flex: 1, fontSize: '10px', padding: '3px 6px', border: '1px solid #ddd', borderRadius: '4px', fontFamily: 'monospace', minWidth: 0 }}
                  />
                  {sourceFilter && (
                    <button onClick={() => setSourceFilter('')} style={{ background: 'none', border: 'none', cursor: 'pointer', color: '#aaa', fontSize: '12px', padding: 0, lineHeight: 1 }}>✕</button>
                  )}
                </div>
                <div style={{ display: 'flex', flexWrap: 'wrap', gap: '5px' }}>
                  {sourcePaths.filter(p => !sourceFilter || p.toLowerCase().includes(sourceFilter.toLowerCase())).map(p => {
                    const isUsed = usedPaths.has(p)
                    return (
                      <div
                        key={p}
                        draggable
                        onDragStart={() => { dragRef.current = p }}
                        onDragEnd={() => { dragRef.current = '' }}
                        style={{
                          display: 'inline-flex', alignItems: 'center', gap: '3px',
                          background: isUsed ? '#d1fae5' : '#dbeafe',
                          border: `1px solid ${isUsed ? '#34d399' : '#93c5fd'}`,
                          borderRadius: '4px', padding: '3px 7px',
                          fontFamily: 'monospace', fontSize: '10px',
                          color: isUsed ? '#065f46' : '#1e40af',
                          cursor: 'grab', userSelect: 'none', opacity: isUsed ? 0.75 : 1,
                        }}
                      >
                        {isUsed && <span style={{ fontSize: '9px' }}>✓</span>}
                        {p}
                      </div>
                    )
                  })}
                </div>
              </>
            )}
          </div>

          {/* RIGHT — Request Body */}
          <div style={{ flex: 1, padding: '16px', display: 'flex', flexDirection: 'column', gap: '8px', overflow: 'auto' }}>
            <div style={{ fontSize: '11px', fontWeight: 700, color: '#555', textTransform: 'uppercase', letterSpacing: '0.5px' }}>
              Request Body <span style={{ fontWeight: 400, color: '#aaa', textTransform: 'none' }}>(drop onto values)</span>
            </div>
            <div style={{ fontSize: '10px', color: '#aaa', lineHeight: 1.5 }}>
              Paste your desired body JSON structure, then click Parse Fields to get drop zones.
            </div>
            <textarea
              value={bodyText}
              onChange={e => handleBodyTextChange(e.target.value)}
              placeholder={'{\n  "latitude": "",\n  "longitude": "",\n  "birth_time": ""\n}'}
              spellCheck={false}
              style={{ flex: 1, minHeight: '180px', width: '100%', boxSizing: 'border-box', fontFamily: 'monospace', fontSize: '11px', padding: '6px 8px', border: '1px solid #ddd', borderRadius: '4px', resize: 'vertical' }}
            />
            {bodyParseError && <div style={{ color: '#e74c3c', fontSize: '11px' }}>{bodyParseError}</div>}
            <button
              onClick={parseBody}
              style={{ padding: '5px', background: '#6366f1', color: 'white', border: 'none', borderRadius: '4px', cursor: 'pointer', fontWeight: 600, fontSize: '11px' }}
            >
              Parse Fields
            </button>

            {bodyLeaves.length > 0 && (
              <>
                <div style={{ fontSize: '10px', fontWeight: 700, color: '#555', marginTop: '4px' }}>BODY FIELDS:</div>
                {bodyLeaves.map(leaf => {
                  const isOver = dragOverPath === leaf.path
                  const rawVal = String(leaf.value ?? '')
                  const displayVal = stripHint(rawVal)
                  const isMapped = displayVal.startsWith('{{') && displayVal.endsWith('}}')
                  const currentHint: TypeHint = typeHints[leaf.path] ?? 'string'

                  const updateLeafValue = (newVal: string) => {
                    const clean = stripHint(newVal)
                    const updated = setLeafValue(bodyObj, leaf.path, clean)
                    setBodyObj(updated)
                    setBodyText(JSON.stringify(updated, null, 2))
                    const out: Array<{ path: string; value: unknown }> = []
                    collectLeafPathsWithValues(updated, '', 0, out)
                    setBodyLeaves(out.filter(l => l.path !== ''))
                  }

                  const hintColors: Record<TypeHint, string> = { string: '#94a3b8', number: '#f59e0b', bool: '#8b5cf6' }

                  return (
                    <div
                      key={leaf.path}
                      onDragOver={e => { e.preventDefault(); setDragOverPath(leaf.path) }}
                      onDragLeave={() => setDragOverPath(null)}
                      onDrop={() => handleDrop(leaf.path)}
                      style={{
                        display: 'flex', alignItems: 'center', gap: '6px',
                        background: isOver ? '#dcfce7' : isMapped ? '#f0fdf4' : '#f8fafc',
                        border: `1px dashed ${isOver ? '#22c55e' : isMapped ? '#86efac' : '#cbd5e1'}`,
                        borderRadius: '4px', padding: '5px 8px', transition: 'background 0.1s, border-color 0.1s',
                        minHeight: '32px',
                      }}
                    >
                      <code style={{ fontSize: '11px', color: '#334155', flex: '0 0 auto', minWidth: '90px' }}>{leaf.path}</code>
                      <input
                        value={displayVal}
                        onChange={e => updateLeafValue(e.target.value)}
                        placeholder="value or {{key}}"
                        style={{ flex: 1, fontFamily: 'monospace', fontSize: '10px', padding: '3px 5px', border: '1px solid #e2e8f0', borderRadius: '3px', background: isMapped ? '#f0fdf4' : 'white', color: isMapped ? '#0d9488' : '#334155', minWidth: 0 }}
                      />
                      {isMapped && (
                        <select
                          value={currentHint}
                          onChange={e => setTypeHints(prev => ({ ...prev, [leaf.path]: e.target.value as TypeHint }))}
                          title="JSON type — Str keeps value quoted, Num/Bool strips quotes"
                          style={{ fontSize: '10px', padding: '2px 3px', border: `1px solid ${hintColors[currentHint]}`, borderRadius: '3px', background: 'white', color: hintColors[currentHint], flexShrink: 0, fontWeight: 600, cursor: 'pointer' }}
                        >
                          <option value="string">Str</option>
                          <option value="number">Num</option>
                          <option value="bool">Bool</option>
                        </select>
                      )}
                      {sourcePaths.length > 0 && (
                        <select
                          value=""
                          onChange={e => { if (e.target.value) updateLeafValue(`{{${e.target.value}}}`) }}
                          style={{ fontSize: '10px', padding: '3px 4px', border: '1px solid #e2e8f0', borderRadius: '3px', background: 'white', color: '#555', flexShrink: 0, maxWidth: '110px' }}
                          title="Pick a context variable"
                        >
                          <option value="">+ var</option>
                          {sourcePaths.map(p => <option key={p} value={p}>{p}</option>)}
                        </select>
                      )}
                    </div>
                  )
                })}
              </>
            )}
          </div>
        </div>

        {/* Preview */}
        <div style={{ borderTop: '1px solid #e0e6ed', padding: '10px 20px', background: '#f8fafc', maxHeight: '120px', overflow: 'auto' }}>
          <div style={{ fontSize: '10px', fontWeight: 700, color: '#555', textTransform: 'uppercase', letterSpacing: '0.5px', marginBottom: '4px' }}>Preview</div>
          <pre style={{ margin: 0, fontFamily: 'monospace', fontSize: '10px', color: '#334155', whiteSpace: 'pre-wrap', wordBreak: 'break-all' }}>{previewJson}</pre>
        </div>

        {/* Footer */}
        <div style={{ display: 'flex', justifyContent: 'flex-end', gap: '10px', padding: '12px 20px', borderTop: '1px solid #e0e6ed', background: '#f8f9fa' }}>
          <button onClick={onClose} style={{ padding: '8px 16px', background: '#f0f0f0', border: 'none', borderRadius: '4px', cursor: 'pointer', fontWeight: 500, fontSize: '13px' }}>
            Cancel
          </button>
          <button
            onClick={() => onSave(JSON.stringify(applyTypeHints(bodyObj, typeHints), null, 2))}
            style={{ padding: '8px 16px', background: '#6366f1', color: 'white', border: 'none', borderRadius: '4px', cursor: 'pointer', fontWeight: 600, fontSize: '13px' }}
          >
            Apply Body
          </button>
        </div>
      </div>
    </div>
  )
}

// ── TextSubstBuilderModal ─────────────────────────────────────────────────────

function TextSubstBuilderModal({ existingVars, onSave, onClose, defaultSourceJson = '' }: {
  existingVars: Record<string, string>
  onSave: (vars: Record<string, string>) => void
  onClose: () => void
  defaultSourceJson?: string
}) {
  const [sourcePaths, setSourcePaths] = useState<string[]>([])
  const [sourceFilter, setSourceFilter] = useState('')
  // context key whose value contains the template text
  const [templateKey, setTemplateKey] = useState('content')
  const [rows, setRows] = useState<Array<{ key: string; value: string }>>(() =>
    Object.entries(existingVars).map(([k, v]) => ({ key: k, value: v }))
  )
  const [dragOverIdx, setDragOverIdx] = useState<number | null>(null)
  const dragRef = useRef<string>('')

  // Auto-parse debug context into leaf paths + auto-parse template vars if rows are empty
  useEffect(() => {
    if (!defaultSourceJson || defaultSourceJson.trim() === '' || defaultSourceJson.trim() === '{}') return
    try {
      const parsed = JSON.parse(defaultSourceJson)
      const out: string[] = []
      collectLeafPaths(parsed, '', 0, out)
      const paths = out.filter(p => p !== '')
      setSourcePaths(paths)
      // Set templateKey to first available path if current default isn't in context
      setTemplateKey(prev => paths.includes(prev) ? prev : (paths[0] ?? prev))
      // Auto-discover template vars from the best template key if no rows yet
      if (Object.keys(existingVars).length === 0) {
        const key = paths.includes('content') ? 'content' : paths[0]
        if (key && typeof parsed[key] === 'string') parseVarsFromText(parsed[key])
      }
    } catch { /* ignore */ }
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [defaultSourceJson])

  const [parseMsg, setParseMsg] = useState('')

  const getByPath = (obj: Record<string, unknown>, path: string): unknown => {
    return path.split('.').reduce<unknown>((cur, seg) => {
      if (cur == null || typeof cur !== 'object') return undefined
      return (cur as Record<string, unknown>)[seg]
    }, obj)
  }

  const parseVarsFromText = (text: string): number => {
    const regex = /\{\{\s*([^}]+?)\s*\}\}/g
    const found = new Set<string>()
    let m: RegExpExecArray | null
    while ((m = regex.exec(text)) !== null) {
      // Strip markdown escape backslashes (e.g. about\_your\_business → about_your_business)
      const name = m[1].replace(/\\/g, '')
      if (name) found.add(name)
    }
    setRows(prev => {
      const existing = new Map(prev.map(r => [r.key, r.value]))
      const merged = [...prev]
      for (const k of found) {
        if (!existing.has(k)) merged.push({ key: k, value: '' })
      }
      return merged
    })
    return found.size
  }

  const parseFromContext = () => {
    setParseMsg('')
    let ctx: Record<string, unknown>
    try { ctx = JSON.parse(defaultSourceJson) } catch { setParseMsg('Debug context is empty — step through the previous node first.'); return }
    const text = getByPath(ctx, templateKey)
    if (typeof text !== 'string') { setParseMsg(`"${templateKey}" is not a string in context — pick a different key.`); return }
    const count = parseVarsFromText(text)
    setParseMsg(count > 0 ? `Found ${count} variable${count === 1 ? '' : 's'}.` : 'No {{variable}} tokens found in that field.')
  }

  const updateVal = (i: number, val: string) => setRows(v => v.map((r, idx) => idx === i ? { ...r, value: val } : r))
  const updateKey = (i: number, k: string) => setRows(v => v.map((r, idx) => idx === i ? { ...r, key: k } : r))
  const removeRow = (i: number) => setRows(v => v.filter((_, idx) => idx !== i))

  const handleDrop = (i: number) => {
    const from = dragRef.current
    if (!from) return
    updateVal(i, `{{${from}}}`)
    setDragOverIdx(null)
  }

  const usedPaths = new Set(
    rows.map(r => { const m = r.value.match(/^\{\{(.+)\}\}$/); return m ? m[1] : null }).filter(Boolean) as string[]
  )

  const save = () => {
    const result: Record<string, string> = {}
    for (const { key, value } of rows) {
      if (key.trim()) result[key.trim()] = value
    }
    onSave(result)
  }

  return (
    <div style={{ position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.65)', display: 'flex', alignItems: 'center', justifyContent: 'center', zIndex: 2100 }}>
      <div style={{ background: 'white', borderRadius: '10px', width: '860px', maxHeight: '85vh', display: 'flex', flexDirection: 'column', boxShadow: '0 24px 64px rgba(0,0,0,0.45)', overflow: 'hidden' }}>

        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', padding: '14px 20px', background: '#1e293b', color: 'white' }}>
          <div>
            <div style={{ fontWeight: 700, fontSize: '15px' }}>Text Substitution Builder</div>
            <div style={{ fontSize: '11px', color: '#94a3b8', marginTop: '2px' }}>Drag context keys onto {'{{placeholder}}'} tokens from your document</div>
          </div>
          <button onClick={onClose} style={{ background: 'none', border: 'none', cursor: 'pointer', fontSize: '18px', color: '#94a3b8', lineHeight: 1 }}>✕</button>
        </div>

        <div style={{ flex: 1, overflow: 'auto', display: 'flex', gap: 0, minHeight: 0 }}>

          {/* LEFT — Context Keys (from debug context) */}
          <div style={{ flex: '0 0 280px', borderRight: '1px solid #e0e6ed', padding: '16px', display: 'flex', flexDirection: 'column', gap: '8px', overflow: 'auto' }}>
            <div style={{ fontSize: '11px', fontWeight: 700, color: '#555', textTransform: 'uppercase', letterSpacing: '0.5px' }}>
              Context Keys <span style={{ fontWeight: 400, color: '#aaa', textTransform: 'none' }}>(drag onto placeholders)</span>
            </div>
            {sourcePaths.length === 0 && (
              <div style={{ fontSize: '10px', color: '#aaa', lineHeight: 1.5 }}>
                Step through the debug panel first to populate context keys.
              </div>
            )}
            {sourcePaths.length > 0 && (
              <>
                <input
                  type="text"
                  placeholder="filter..."
                  value={sourceFilter}
                  onChange={e => setSourceFilter(e.target.value)}
                  style={{ fontSize: '10px', padding: '4px 6px', border: '1px solid #ddd', borderRadius: '4px', fontFamily: 'monospace' }}
                />
                <div style={{ display: 'flex', flexWrap: 'wrap', gap: '5px' }}>
                  {sourcePaths.filter(p => !sourceFilter || p.toLowerCase().includes(sourceFilter.toLowerCase())).map(p => {
                    const isUsed = usedPaths.has(p)
                    return (
                      <div
                        key={p}
                        draggable
                        onDragStart={() => { dragRef.current = p }}
                        onDragEnd={() => { dragRef.current = '' }}
                        style={{
                          display: 'inline-flex', alignItems: 'center', gap: '3px',
                          background: isUsed ? '#d1fae5' : '#dbeafe',
                          border: `1px solid ${isUsed ? '#34d399' : '#93c5fd'}`,
                          borderRadius: '4px', padding: '3px 7px',
                          fontFamily: 'monospace', fontSize: '10px',
                          color: isUsed ? '#065f46' : '#1e40af',
                          cursor: 'grab', userSelect: 'none', opacity: isUsed ? 0.75 : 1,
                        }}
                      >
                        {isUsed && <span style={{ fontSize: '9px' }}>✓</span>}
                        {p}
                      </div>
                    )
                  })}
                </div>
              </>
            )}
          </div>

          {/* RIGHT — Document placeholders */}
          <div style={{ flex: 1, padding: '16px', display: 'flex', flexDirection: 'column', gap: '8px', overflow: 'auto' }}>

            {/* Parse from context */}
            <div style={{ fontSize: '11px', fontWeight: 700, color: '#555', textTransform: 'uppercase', letterSpacing: '0.5px' }}>
              Document Placeholders
            </div>
            <div style={{ fontSize: '10px', color: '#aaa', lineHeight: 1.5 }}>
              Pick the context key that holds your document, then click Parse to extract {'{{placeholder}}'} tokens.
            </div>
            <div style={{ display: 'flex', gap: '6px', alignItems: 'center' }}>
              {sourcePaths.length > 0 ? (
                <select
                  value={templateKey}
                  onChange={e => setTemplateKey(e.target.value)}
                  style={{ flex: 1, fontFamily: 'monospace', fontSize: '11px', padding: '5px 6px', border: '1px solid #ddd', borderRadius: '4px' }}
                >
                  {sourcePaths.map(p => <option key={p} value={p}>{p}</option>)}
                </select>
              ) : (
                <input
                  value={templateKey}
                  onChange={e => setTemplateKey(e.target.value)}
                  placeholder="e.g. content"
                  style={{ flex: 1, fontFamily: 'monospace', fontSize: '11px', padding: '5px 6px', border: '1px solid #ddd', borderRadius: '4px' }}
                />
              )}
              <button
                onClick={parseFromContext}
                style={{ padding: '5px 12px', background: '#0ea5e9', color: 'white', border: 'none', borderRadius: '4px', cursor: 'pointer', fontWeight: 600, fontSize: '11px', whiteSpace: 'nowrap' }}
              >
                Parse Variables
              </button>
            </div>

            {parseMsg && (
              <div style={{ fontSize: '11px', color: parseMsg.startsWith('Found') ? '#16a34a' : '#e74c3c', padding: '4px 0' }}>
                {parseMsg}
              </div>
            )}

            {/* Variable rows — placeholder → drop target */}
            {rows.length > 0 && (
              <div style={{ fontSize: '10px', fontWeight: 700, color: '#555', textTransform: 'uppercase', letterSpacing: '0.5px', marginTop: '4px', borderTop: '1px solid #e0e6ed', paddingTop: '8px' }}>
                Substitutions <span style={{ fontWeight: 400, color: '#aaa', textTransform: 'none' }}>(drag context keys onto each field)</span>
              </div>
            )}

            {rows.map(({ key, value }, i) => {
              const isOver = dragOverIdx === i
              const isMapped = value !== ''
              return (
                <div
                  key={i}
                  onDragOver={e => { e.preventDefault(); setDragOverIdx(i) }}
                  onDragLeave={() => setDragOverIdx(null)}
                  onDrop={() => handleDrop(i)}
                  style={{
                    display: 'flex', alignItems: 'center', gap: '6px',
                    background: isOver ? '#dcfce7' : isMapped ? '#f0fdf4' : '#f8fafc',
                    border: `1px dashed ${isOver ? '#22c55e' : isMapped ? '#86efac' : '#cbd5e1'}`,
                    borderRadius: '4px', padding: '6px 8px', transition: 'background 0.1s, border-color 0.1s',
                  }}
                >
                  <span style={{ flex: '0 0 160px', fontFamily: 'monospace', fontSize: '11px', color: '#334155', padding: '4px 0' }}>{`{{${key}}}`}</span>
                  <span style={{ color: '#bbb', fontSize: '11px', flexShrink: 0 }}>→</span>
                  <input
                    value={value}
                    onChange={e => updateVal(i, e.target.value)}
                    placeholder="drop a context key here"
                    style={{ flex: 1, fontFamily: 'monospace', fontSize: '11px', padding: '4px 6px', border: '1px solid #e2e8f0', borderRadius: '3px', background: isMapped ? '#f0fdf4' : 'white', color: isMapped ? '#0d9488' : '#334155', minWidth: 0 }}
                  />
                </div>
              )
            })}
          </div>
        </div>

        <div style={{ display: 'flex', justifyContent: 'flex-end', gap: '10px', padding: '12px 20px', borderTop: '1px solid #e0e6ed', background: '#f8f9fa' }}>
          <button onClick={onClose} style={{ padding: '8px 16px', background: '#f0f0f0', border: 'none', borderRadius: '4px', cursor: 'pointer', fontWeight: 500, fontSize: '13px' }}>Cancel</button>
          <button onClick={save} style={{ padding: '8px 16px', background: '#0ea5e9', color: 'white', border: 'none', borderRadius: '4px', cursor: 'pointer', fontWeight: 600, fontSize: '13px' }}>Apply Substitutions</button>
        </div>
      </div>
    </div>
  )
}

// ── Node properties editor ────────────────────────────────────────────────────

function NodeEditor({ node, isStart, activities, onSetStart, onChange, onDelete, onTestFromHere, onInsertMapperNode, debugContext }: {
  node: BNode; isStart: boolean
  activities: ActivityInfo[]
  onSetStart: () => void
  onChange: (n: BNode) => void
  onDelete: () => void
  onTestFromHere: (nodeId: string) => void
  onInsertMapperNode: (sourceNodeId: string, mappings: Array<{ from: string; to: string }>) => void
  debugContext: string
}) {
  const set = (patch: Partial<BNode>) => onChange({ ...node, ...patch })
  const setInput = (key: string, val: string) =>
    set({ staticInput: { ...(node.staticInput ?? {}), [key]: val } })

  // Mapper-specific local UI state
  const [mapperSourceJson, setMapperSourceJson] = useState('')
  const [mapperDestJson, setMapperDestJson] = useState('')
  const [showMapperModal, setShowMapperModal] = useState(false)

  // Auto-populate mapper source from debugContext once when it becomes non-empty
  useEffect(() => {
    if (debugContext && debugContext.trim() !== '{}' && !mapperSourceJson.trim()) {
      setMapperSourceJson(debugContext)
    }
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [debugContext])

  // LLM model list — fetched dynamically from /api/llm/models, falls back to static
  const [llmModels, setLlmModels] = useState<Array<{ group: string; models: Array<{ value: string; label: string }> }>>([])
  const [llmModelsLoading, setLlmModelsLoading] = useState(false)
  // state for the "add from context" dropdown in the prompt vars mapper
  const [llmAddVarCtxKey, setLlmAddVarCtxKey] = useState('')
  // refs for inserting context vars directly into prompt textareas
  const systemPromptRef = useRef<HTMLTextAreaElement>(null)
  const promptRef = useRef<HTMLTextAreaElement>(null)

  const fetchLlmModels = useCallback(async () => {
    setLlmModelsLoading(true)
    try {
      const res = await fetch('/api/llm/models')
      if (res.ok) {
        const data = await res.json()
        if (data.groups?.length) setLlmModels(data.groups)
      }
    } catch {
      // fall back to static list
    } finally {
      setLlmModelsLoading(false)
    }
  }, [])

  useEffect(() => {
    if (node.activityName === 'llm_prompt' && llmModels.length === 0) fetchLlmModels()
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [node.activityName])

  // HTTP Body Builder state
  const [showHttpBodyBuilder, setShowHttpBodyBuilder] = useState(false)
  const [showPromptVarsBuilder, setShowPromptVarsBuilder] = useState(false)
  const [showTextSubstBuilder, setShowTextSubstBuilder] = useState(false)
  const [showGdriveTemplateVarsBuilder, setShowGdriveTemplateVarsBuilder] = useState(false)
  const [showPbDataMapper, setShowPbDataMapper] = useState(false)
  const [showPbQueryBuilder, setShowPbQueryBuilder] = useState(false)
  const [showPbUpsertWhereBuilder, setShowPbUpsertWhereBuilder] = useState(false)
  const [showTelegramButtonMapper, setShowTelegramButtonMapper] = useState(false)
  const [showCfUpsertMapper, setShowCfUpsertMapper] = useState(false)
  const [showCfTagPicker, setShowCfTagPicker] = useState(false)
  const [showCfContactIdMapper, setShowCfContactIdMapper] = useState(false)

  if (node.activityName === 'END') {
    return (
      <div>
        <div style={{ fontWeight: 600, marginBottom: '8px', color: '#2c3e50' }}>Finish Node</div>
        <div style={{ fontSize: '12px', color: '#888', marginBottom: '12px' }}>
          Any edge connecting to this node will end the workflow.
        </div>
        <button onClick={onDelete} style={{ display: 'block', width: '100%', ...iconBtn('#e74c3c'), padding: '6px' }}>
          Delete Node
        </button>
      </div>
    )
  }

  const actMeta = activities.find(a => a.name === node.activityName)
  // Input fields that aren't wildcard (*) and aren't purely pass-through
  const inputFields = (actMeta?.meta.input_fields ?? []).filter(f => f.name !== '*')
  const isPython = node.activityName === 'python_eval'
  const isPbWriteNode = node.activityName === 'pb_update' || node.activityName === 'pb_create' || node.activityName === 'pb_upsert' || node.activityName === 'pb_delete'
  const isPbUpsert = node.activityName === 'pb_upsert'
  const isPbQuery = node.activityName === 'pb_query'
  const isHttpRequest = node.activityName === 'http_request' || node.activityName === 'gdrive_fetch_url'
  const isMapper = node.activityName === 'mapper'
  const isLlm = node.activityName === 'llm_prompt'
  const isTextSubst = node.activityName === 'text_substitute'
  const isGdriveFillTemplate = node.activityName === 'gdrive_fill_template'
  const isNotes = node.activityName === 'notes'
  const isTelegramButton = node.activityName === 'telegram_send_button'
  const isCfUpsert = node.activityName === 'cf_upsert'
  const isCfTagNode = node.activityName === 'cf_add_tag' || node.activityName === 'cf_remove_tag'
  const hasCfContactId = ['cf_get_contact', 'cf_upsert', 'cf_add_tag', 'cf_remove_tag'].includes(node.activityName ?? '')

  // Parse validity for mapper button enable
  const mapperSourceValid = (() => { try { JSON.parse(mapperSourceJson); return mapperSourceJson.trim() !== ''; } catch { return false } })()
  const mapperDestValid = (() => { try { JSON.parse(mapperDestJson); return mapperDestJson.trim() !== ''; } catch { return false } })()

  // Parse existing mappings from staticInput for the modal
  const existingMappings: Array<{ from: string; to: string }> = (() => {
    try { return JSON.parse((node.staticInput ?? {}).mappings ?? '[]') } catch { return [] }
  })()

  return (
    <div>
      <div style={{ fontWeight: 600, marginBottom: '8px', color: '#2c3e50' }}>Node: {node.id}</div>
      <div style={{ fontSize: '11px', fontFamily: 'monospace', color: isPython ? '#3572A5' : node.isHuman ? '#f39c12' : '#667eea', marginBottom: '10px' }}>
        {isPython ? '🐍 ' : node.isHuman ? '👤 ' : ''}{node.activityName}
      </div>

      <label style={labelStyle}>Label (optional)</label>
      <input value={node.label} onChange={e => set({ label: e.target.value })}
        style={inputStyle} placeholder="Display label" />

      <label style={labelStyle}>Max Retries</label>
      <input type="number" min={0} value={node.maxRetries} onChange={e => set({ maxRetries: Number(e.target.value) })}
        style={inputStyle} />

      <label style={{ ...labelStyle, display: 'flex', alignItems: 'center', gap: '6px', cursor: 'pointer' }}>
        <input type="checkbox" checked={node.isHuman} onChange={e => set({ isHuman: e.target.checked })} />
        Human-in-the-loop (pause for approval)
      </label>

      {isPython && (
        <PythonPanel
          code={(node.staticInput ?? {}).code ?? ''}
          onCodeChange={code => setInput('code', code)}
          needsConversion={node.needsConversion ?? false}
          onNeedsConversionChange={v => set({ needsConversion: v })}
        />
      )}

      {isMapper && (
        <>
          <div style={{ marginTop: '14px', marginBottom: '6px', fontSize: '11px', fontWeight: 700, color: '#444', borderTop: '1px solid #e0e6ed', paddingTop: '10px' }}>
            Field Mapper Config
          </div>
          <label style={labelStyle}>Source Data (paste sample JSON from previous node output)</label>
          <textarea
            value={mapperSourceJson}
            onChange={e => setMapperSourceJson(e.target.value)}
            rows={4}
            placeholder={'{\n  "email_address": "user@example.com",\n  "first_name": "Jane"\n}'}
            spellCheck={false}
            style={{ width: '100%', boxSizing: 'border-box', fontFamily: 'monospace', fontSize: '11px', padding: '6px 8px', border: '1px solid #ddd', borderRadius: '4px', resize: 'vertical', marginBottom: '6px' }}
          />
          <label style={labelStyle}>Destination Template (paste sample of what the output should look like)</label>
          <textarea
            value={mapperDestJson}
            onChange={e => setMapperDestJson(e.target.value)}
            rows={4}
            placeholder={'{\n  "email": "",\n  "name": "",\n  "contact_id": ""\n}'}
            spellCheck={false}
            style={{ width: '100%', boxSizing: 'border-box', fontFamily: 'monospace', fontSize: '11px', padding: '6px 8px', border: '1px solid #ddd', borderRadius: '4px', resize: 'vertical', marginBottom: '8px' }}
          />
          <button
            onClick={() => setShowMapperModal(true)}
            disabled={!mapperSourceValid || !mapperDestValid}
            style={{
              width: '100%', padding: '6px', background: (mapperSourceValid && mapperDestValid) ? '#667eea' : '#ccc',
              color: 'white', border: 'none', borderRadius: '4px', cursor: (mapperSourceValid && mapperDestValid) ? 'pointer' : 'default',
              fontWeight: 600, fontSize: '12px', marginBottom: '8px',
            }}
          >
            Open Field Mapper
          </button>
          {(node.staticInput ?? {}).mappings && (
            <div style={{ marginTop: '4px', fontSize: '10px', color: '#888' }}>
              <div style={{ fontWeight: 600, marginBottom: '2px', color: '#555' }}>Current mappings:</div>
              <code style={{ wordBreak: 'break-all', fontSize: '10px', background: '#f5f5f5', padding: '4px 6px', borderRadius: '3px', display: 'block', lineHeight: 1.6 }}>
                {(node.staticInput ?? {}).mappings}
              </code>
            </div>
          )}
          {showMapperModal && (
            <MapperModal
              sourceJson={mapperSourceJson}
              destJson={mapperDestJson}
              existingMappings={existingMappings}
              onSave={mappings => {
                setInput('source_key', 'json')
                setInput('mappings', JSON.stringify(mappings))
                setShowMapperModal(false)
              }}
              onClose={() => setShowMapperModal(false)}
            />
          )}
        </>
      )}

      {isLlm && (
        <>
          <div style={{ marginTop: '14px', marginBottom: '6px', fontSize: '11px', fontWeight: 700, color: '#444', borderTop: '1px solid #e0e6ed', paddingTop: '10px' }}>
            LLM Prompt
          </div>

          <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: '2px' }}>
            <label style={{ ...labelStyle, marginBottom: 0 }}>Model <span style={{ color: '#e74c3c' }}>*</span></label>
            <button
              onClick={fetchLlmModels}
              disabled={llmModelsLoading}
              title="Refresh model list from API keys configured in Settings"
              style={{ fontSize: '10px', padding: '2px 7px', border: '1px solid #d0d5dd', borderRadius: '4px', background: 'white', cursor: llmModelsLoading ? 'default' : 'pointer', color: '#555' }}
            >
              {llmModelsLoading ? '...' : '↻ Refresh'}
            </button>
          </div>
          <select
            value={(node.staticInput ?? {}).model ?? ''}
            onChange={e => setInput('model', e.target.value)}
            style={{ ...inputStyle, background: 'white', marginBottom: '4px' }}
          >
            <option value="">— pick from list —</option>
            {(llmModels.length > 0 ? llmModels : LLM_MODELS).map(g => (
              <optgroup key={g.group} label={g.group}>
                {g.models.map(m => (
                  <option key={m.value} value={m.value}>{m.label}</option>
                ))}
              </optgroup>
            ))}
          </select>
          <input
            value={(node.staticInput ?? {}).model ?? ''}
            onChange={e => setInput('model', e.target.value)}
            style={{ ...inputStyle, fontFamily: 'monospace', fontSize: '11px' }}
            placeholder="or type ID: openrouter/minimax/minimax-text-01"
          />

          {/* ── Prompt Variables Mapper ── */}
          {(() => {
            const rawVars = (node.staticInput ?? {}).prompt_vars
            const promptVars: Record<string, string> = (() => {
              try { return rawVars ? JSON.parse(rawVars) : {} } catch { return {} }
            })()
            const contextPaths: string[] = (() => {
              try {
                const ctx = JSON.parse(debugContext || '{}')
                const out: string[] = []
                collectLeafPaths(ctx, '', 0, out)
                return out.filter(p => p !== '' && !p.startsWith('_'))
              } catch { return [] }
            })()
            const rows = Object.entries(promptVars)
            const saveVars = (vars: Record<string, string>) =>
              setInput('prompt_vars', JSON.stringify(vars))
            const addBlankRow = () => {
              const key = `var${rows.length + 1}`
              saveVars({ ...promptVars, [key]: '' })
            }
            const addFromCtx = (ctxPath: string) => {
              if (!ctxPath) return
              // derive a clean var name from the last path segment
              const lastSeg = ctxPath.split('.').pop() ?? ctxPath
              const varName = lastSeg.replace(/[^a-zA-Z0-9_]/g, '_')
              const key = promptVars[varName] !== undefined ? ctxPath.replace(/\./g, '_') : varName
              saveVars({ ...promptVars, [key]: `{{${ctxPath}}}` })
              setLlmAddVarCtxKey('')
            }
            const removeRow = (k: string) => {
              const next = { ...promptVars }
              delete next[k]
              saveVars(next)
            }
            const updateKey = (oldKey: string, newKey: string) => {
              const next: Record<string, string> = {}
              for (const [k, v] of Object.entries(promptVars)) {
                next[k === oldKey ? newKey : k] = v
              }
              saveVars(next)
            }
            const updateVal = (key: string, val: string) =>
              saveVars({ ...promptVars, [key]: val })
            return (
              <div style={{ marginTop: '10px', marginBottom: '8px' }}>
                <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: '4px' }}>
                  <span style={{ fontSize: '11px', fontWeight: 700, color: '#444' }}>
                    Prompt Variables
                  </span>
                  <button
                    onClick={() => setShowPromptVarsBuilder(true)}
                    style={{ fontSize: '10px', padding: '3px 8px', background: '#7c3aed', color: 'white', border: 'none', borderRadius: '4px', cursor: 'pointer', fontWeight: 600 }}
                  >
                    Build Vars
                  </button>
                </div>
                {rows.map(([k, v]) => (
                  <div key={k} style={{ display: 'flex', gap: '4px', alignItems: 'center', marginBottom: '3px' }}>
                    <input
                      value={k}
                      onChange={e => updateKey(k, e.target.value)}
                      title="Variable name used in prompt as {{name}}"
                      style={{ ...inputStyle, flex: '0 0 90px', fontFamily: 'monospace', fontSize: '10px', marginBottom: 0 }}
                      placeholder="var name"
                    />
                    <span style={{ color: '#bbb', fontSize: '10px', flexShrink: 0 }}>→</span>
                    <input
                      value={v}
                      onChange={e => updateVal(k, e.target.value)}
                      title="Value — use {{context_key}} to pull from workflow context"
                      style={{ ...inputStyle, flex: 1, fontFamily: 'monospace', fontSize: '10px', marginBottom: 0 }}
                      placeholder="{{context_key}}"
                    />
                    <button onClick={() => removeRow(k)} style={{ flexShrink: 0, border: 'none', background: 'none', color: '#e74c3c', cursor: 'pointer', fontSize: '15px', padding: '0 2px', lineHeight: 1 }}>×</button>
                  </div>
                ))}
                <div style={{ display: 'flex', gap: '5px', marginTop: '4px' }}>
                  {contextPaths.length > 0 && (
                    <select
                      value={llmAddVarCtxKey}
                      onChange={e => addFromCtx(e.target.value)}
                      style={{ ...inputStyle, flex: 1, fontSize: '10px', marginBottom: 0 }}
                    >
                      <option value="">+ from context…</option>
                      {contextPaths.map(p => (
                        <option key={p} value={p}>{p}</option>
                      ))}
                    </select>
                  )}
                  <button
                    onClick={addBlankRow}
                    style={{ flexShrink: 0, fontSize: '10px', padding: '3px 8px', border: '1px solid #d0d5dd', borderRadius: '4px', background: 'white', cursor: 'pointer', color: '#555' }}
                  >+ Add</button>
                </div>
              </div>
            )
          })()}

          {(() => {
            const contextPaths: string[] = (() => {
              try {
                const ctx = JSON.parse(debugContext || '{}')
                const out: string[] = []
                collectLeafPaths(ctx, '', 0, out)
                return out.filter(p => p !== '' && !p.startsWith('_'))
              } catch { return [] }
            })()

            const insertAtCursor = (
              ref: React.RefObject<HTMLTextAreaElement | null>,
              field: string,
              key: string,
            ) => {
              const tag = `{{${key}}}`
              const el = ref.current
              if (el) {
                const start = el.selectionStart ?? el.value.length
                const end = el.selectionEnd ?? el.value.length
                const next = el.value.slice(0, start) + tag + el.value.slice(end)
                setInput(field, next)
                requestAnimationFrame(() => {
                  el.focus()
                  el.setSelectionRange(start + tag.length, start + tag.length)
                })
              } else {
                setInput(field, ((node.staticInput ?? {})[field] ?? '') + tag)
              }
            }

            const CtxInsert = ({ field, ref: tRef }: { field: string; ref: React.RefObject<HTMLTextAreaElement | null> }) =>
              contextPaths.length > 0 ? (
                <select
                  value=""
                  onChange={e => { if (e.target.value) insertAtCursor(tRef, field, e.target.value) }}
                  style={{ ...inputStyle, fontSize: '10px', marginBottom: '6px', color: '#555' }}
                >
                  <option value="">+ insert context var…</option>
                  {contextPaths.map(p => <option key={p} value={p}>{`{{${p}}}`}</option>)}
                </select>
              ) : null

            return (
              <>
                <label style={labelStyle}>System Prompt</label>
                <div style={{ fontSize: '10px', color: '#aaa', marginBottom: '2px' }}>Supports {'{{key}}'} and prompt variable interpolation</div>
                <textarea
                  ref={systemPromptRef}
                  value={(node.staticInput ?? {}).system_prompt ?? ''}
                  onChange={e => setInput('system_prompt', e.target.value)}
                  rows={3}
                  placeholder="You are a helpful assistant analyzing {{contact.first_name}}..."
                  spellCheck={false}
                  style={{ width: '100%', boxSizing: 'border-box', fontFamily: 'monospace', fontSize: '11px', padding: '5px 7px', border: '1px solid #e0e6ed', borderRadius: '4px', resize: 'vertical', marginBottom: '2px' }}
                />
                <CtxInsert field="system_prompt" ref={systemPromptRef} />

                <label style={labelStyle}>Prompt <span style={{ color: '#e74c3c' }}>*</span></label>
                <div style={{ fontSize: '10px', color: '#aaa', marginBottom: '2px' }}>Supports {'{{key}}'} template interpolation</div>
                <textarea
                  ref={promptRef}
                  value={(node.staticInput ?? {}).prompt ?? ''}
                  onChange={e => setInput('prompt', e.target.value)}
                  rows={4}
                  placeholder={'Summarize this contact: {{contact.first_name}} {{contact.last_name}}'}
                  spellCheck={false}
                  style={{ width: '100%', boxSizing: 'border-box', fontFamily: 'monospace', fontSize: '11px', padding: '5px 7px', border: '1px solid #e0e6ed', borderRadius: '4px', resize: 'vertical', marginBottom: '2px' }}
                />
                <CtxInsert field="prompt" ref={promptRef} />
              </>
            )
          })()}

          <div style={{ display: 'flex', gap: '8px' }}>
            <div style={{ flex: 1 }}>
              <label style={labelStyle}>Temperature</label>
              <input
                type="number" min={0} max={2} step={0.1}
                value={(node.staticInput ?? {}).temperature ?? '0.7'}
                onChange={e => setInput('temperature', e.target.value)}
                style={inputStyle}
                placeholder="0.7"
              />
            </div>
            <div style={{ flex: 1 }}>
              <label style={labelStyle}>Max Tokens</label>
              <input
                type="number" min={1}
                value={(node.staticInput ?? {}).max_tokens ?? '1024'}
                onChange={e => setInput('max_tokens', e.target.value)}
                style={inputStyle}
                placeholder="1024"
              />
            </div>
          </div>

          <label style={labelStyle}>Result Key</label>
          <div style={{ fontSize: '10px', color: '#aaa', marginBottom: '2px' }}>Store response under this context key (default: llm_response)</div>
          <input
            value={(node.staticInput ?? {}).result_key ?? ''}
            onChange={e => setInput('result_key', e.target.value)}
            style={inputStyle}
            placeholder="llm_response"
          />
        </>
      )}

      {isNotes && (
        <>
          <div style={{ marginTop: '14px', marginBottom: '6px', fontSize: '11px', fontWeight: 700, color: '#92610a', borderTop: '1px solid #e0e6ed', paddingTop: '10px' }}>
            Note Text
          </div>
          <div style={{ fontSize: '10px', color: '#aaa', marginBottom: '6px' }}>
            Documentation only — this node is always skipped during execution.
          </div>
          <textarea
            value={(node.staticInput ?? {}).note ?? ''}
            onChange={e => setInput('note', e.target.value)}
            rows={6}
            placeholder="Add notes about this part of the workflow..."
            spellCheck={false}
            style={{ width: '100%', boxSizing: 'border-box', fontFamily: 'sans-serif', fontSize: '12px', padding: '8px 10px', background: '#fffbeb', color: '#4a3800', border: '1px solid #d4a017', borderRadius: '4px', resize: 'vertical', lineHeight: 1.5 }}
          />
        </>
      )}

      {!isPython && !isMapper && !isLlm && !isPbQuery && !isNotes && inputFields.length > 0 && (
        <>
          <div style={{ marginTop: '14px', marginBottom: '6px', fontSize: '11px', fontWeight: 700, color: '#444', borderTop: '1px solid #e0e6ed', paddingTop: '10px' }}>
            Input Fields
          </div>
          <div style={{ fontSize: '10px', color: '#aaa', marginBottom: '8px' }}>
            Pre-set values baked into this node. Use <code>{'{{key}}'}</code> to reference the workflow context.
          </div>
          {inputFields.map(f => {
            if (isTelegramButton && f.name === 'buttons') return null
            if (isCfUpsert && f.name === 'data') return null
            if (isCfTagNode && f.name === 'tag_ids') return null
            if (hasCfContactId && f.name === 'contact_id') return null
            return (
            <div key={f.name}>
              <label style={labelStyle}>
                {f.name}
                {f.type && <span style={{ color: '#aaa', fontWeight: 400 }}> ({f.type})</span>}
                {isPbWriteNode && f.name === 'data' && (
                  <span
                    title={
                      node.activityName === 'pb_update' ? 'Merge update: only listed fields change; omitted fields are untouched.' :
                      node.activityName === 'pb_upsert' ? 'Upsert: updates if id supplied, creates otherwise.' :
                      'Create: all listed fields are set on the new record.'
                    }
                    style={{ marginLeft: '4px', color: '#6b46c1', cursor: 'help', fontSize: '12px' }}
                  >ℹ</span>
                )}
              </label>
              {f.description && (
                <div style={{ fontSize: '10px', color: '#aaa', marginBottom: '2px' }}>{f.description}</div>
              )}
              {isHttpRequest && f.name === 'body' && (
                <button
                  onClick={() => setShowHttpBodyBuilder(true)}
                  style={{
                    display: 'block', width: '100%', marginBottom: '4px',
                    padding: '5px 10px', background: '#6366f1', color: 'white',
                    border: 'none', borderRadius: '4px', cursor: 'pointer',
                    fontWeight: 600, fontSize: '11px', textAlign: 'left',
                  }}
                >
                  Build Body
                </button>
              )}
              {isTextSubst && f.name === 'vars' && (
                <button
                  onClick={() => setShowTextSubstBuilder(true)}
                  style={{
                    display: 'block', width: '100%', marginBottom: '4px',
                    padding: '5px 10px', background: '#0ea5e9', color: 'white',
                    border: 'none', borderRadius: '4px', cursor: 'pointer',
                    fontWeight: 600, fontSize: '11px', textAlign: 'left',
                  }}
                >
                  Build Substitutions
                </button>
              )}
              {isGdriveFillTemplate && f.name === 'vars' && (
                <button
                  onClick={() => setShowGdriveTemplateVarsBuilder(true)}
                  style={{
                    display: 'block', width: '100%', marginBottom: '4px',
                    padding: '5px 10px', background: '#06b6d4', color: 'white',
                    border: 'none', borderRadius: '4px', cursor: 'pointer',
                    fontWeight: 600, fontSize: '11px', textAlign: 'left',
                  }}
                >
                  Build Template Variables
                </button>
              )}
              {f.options && f.options.length > 0 ? (
                <select
                  value={(node.staticInput ?? {})[f.name] ?? ''}
                  onChange={e => setInput(f.name, e.target.value)}
                  style={{ ...inputStyle, background: 'white' }}
                >
                  <option value="">— select —</option>
                  {f.options.map(opt => <option key={opt} value={opt}>{opt}</option>)}
                </select>
              ) : (f.type === 'gdrive_folder' || f.type === 'gdrive_file') ? (
                <DrivePicker
                  value={(node.staticInput ?? {})[f.name] ?? ''}
                  displayName={(node.staticInput ?? {})[f.name + '_name'] ?? ''}
                  mode={f.type === 'gdrive_folder' ? 'folder' : 'file'}
                  onSelect={(id, name) => {
                    set({ staticInput: { ...(node.staticInput ?? {}), [f.name]: id, [f.name + '_name']: name } })
                  }}
                />
              ) : (f.type === 'workflow_graph') ? (
                <WorkflowGraphPicker
                  value={(node.staticInput ?? {})[f.name] ?? ''}
                  onChange={(name: string) => setInput(f.name, name)}
                />
              ) : (f.type === 'context_field') ? (
                <ContextFieldPicker
                  value={(node.staticInput ?? {})[f.name] ?? ''}
                  fieldName={f.name}
                  debugContext={debugContext}
                  onChange={v => setInput(f.name, v)}
                />
              ) : (f.type === 'context_key') ? (
                <ContextKeyPicker
                  value={(node.staticInput ?? {})[f.name] ?? ''}
                  onChange={v => setInput(f.name, v)}
                  debugContext={debugContext}
                />
              ) : (f.type === 'boolean') ? (
                <BooleanToggle
                  value={(node.staticInput ?? {})[f.name] === 'true'}
                  onChange={v => setInput(f.name, v ? 'true' : 'false')}
                />
              ) : (f.type === 'textarea') ? (
                <textarea
                  value={(node.staticInput ?? {})[f.name] ?? ''}
                  onChange={e => setInput(f.name, e.target.value)}
                  style={{ ...inputStyle, minHeight: '80px', resize: 'vertical', fontFamily: 'inherit' }}
                  placeholder="Notes..."
                />
              ) : (
                <input
                  value={(node.staticInput ?? {})[f.name] ?? ''}
                  onChange={e => setInput(f.name, e.target.value)}
                  style={inputStyle}
                  placeholder={f.type === 'object' ? 'JSON or {{key}}' : `{{${f.name}}} or literal`}
                />
              )}
            </div>
            )
          })}
        </>
      )}

      {isTelegramButton && (
        <>
          <button
            onClick={() => setShowTelegramButtonMapper(true)}
            style={{
              display: 'block', width: '100%', marginTop: '10px',
              padding: '6px 10px', background: '#0ea5e9', color: 'white',
              border: 'none', borderRadius: '4px', cursor: 'pointer',
              fontWeight: 600, fontSize: '11px', textAlign: 'left',
            }}
          >
            Open Button Mapper
          </button>
          {showTelegramButtonMapper && (
            <TelegramButtonMapperModal
              existingButtons={(node.staticInput ?? {}).buttons ?? ''}
              defaultSourceJson={debugContext}
              onSave={buttons => {
                setInput('buttons', buttons)
                setShowTelegramButtonMapper(false)
              }}
              onClose={() => setShowTelegramButtonMapper(false)}
            />
          )}
        </>
      )}

      {isPbWriteNode && (
        <>
          <button
            onClick={() => setShowPbDataMapper(true)}
            style={{
              display: 'block', width: '100%', marginTop: '10px',
              padding: '6px 10px', background: '#6b46c1', color: 'white',
              border: 'none', borderRadius: '4px', cursor: 'pointer',
              fontWeight: 600, fontSize: '11px', textAlign: 'left',
            }}
          >
            Open Field Mapper
          </button>
          {showPbDataMapper && (
            <PbDataMapperModal
              existingData={(node.staticInput ?? {}).data ?? ''}
              existingId={(node.staticInput ?? {}).id ?? ''}
              existingResultKey={(node.staticInput ?? {}).result_key ?? ''}
              defaultSourceJson={debugContext}
              tableName={(node.staticInput ?? {}).table_name ?? ''}
              mode={
                node.activityName === 'pb_update' ? 'update' :
                node.activityName === 'pb_upsert' ? 'upsert' :
                node.activityName === 'pb_delete' ? 'delete' : 'create'
              }
              onSave={data => setInput('data', data)}
              onSaveId={id => setInput('id', id)}
              onSaveResultKey={key => setInput('result_key', key)}
              onClose={() => setShowPbDataMapper(false)}
            />
          )}
        </>
      )}

      {isCfUpsert && (
        <>
          <button
            onClick={() => setShowCfUpsertMapper(true)}
            style={{
              display: 'block', width: '100%', marginTop: '10px',
              padding: '6px 10px', background: '#16a34a', color: 'white',
              border: 'none', borderRadius: '4px', cursor: 'pointer',
              fontWeight: 600, fontSize: '11px', textAlign: 'left',
            }}
          >
            Open CF Field Mapper
          </button>
          {showCfUpsertMapper && (
            <CfUpsertMapperModal
              existingData={(node.staticInput ?? {}).data ?? ''}
              defaultSourceJson={debugContext}
              onSave={data => {
                setInput('data', data)
                setShowCfUpsertMapper(false)
              }}
              onClose={() => setShowCfUpsertMapper(false)}
            />
          )}
        </>
      )}

      {hasCfContactId && (
        <>
          <button
            onClick={() => setShowCfContactIdMapper(true)}
            style={{
              display: 'block', width: '100%', marginTop: '10px',
              padding: '6px 10px', background: '#7c3aed', color: 'white',
              border: 'none', borderRadius: '4px', cursor: 'pointer',
              fontWeight: 600, fontSize: '11px', textAlign: 'left',
            }}
          >
            {(() => {
              const v = (node.staticInput ?? {}).contact_id ?? ''
              return v ? `Contact ID: ${v}` : 'Set Contact ID'
            })()}
          </button>
          {showCfContactIdMapper && (
            <CfContactIdMapperModal
              existingValue={(node.staticInput ?? {}).contact_id ?? ''}
              defaultSourceJson={debugContext}
              onSave={v => { setInput('contact_id', v); setShowCfContactIdMapper(false) }}
              onClose={() => setShowCfContactIdMapper(false)}
            />
          )}
        </>
      )}

      {isCfTagNode && (
        <>
          <button
            onClick={() => setShowCfTagPicker(true)}
            style={{
              display: 'block', width: '100%', marginTop: '10px',
              padding: '6px 10px',
              background: node.activityName === 'cf_add_tag' ? '#16a34a' : '#dc2626',
              color: 'white', border: 'none', borderRadius: '4px', cursor: 'pointer',
              fontWeight: 600, fontSize: '11px', textAlign: 'left',
            }}
          >
            {(() => {
              const ids: number[] = (() => { try { return JSON.parse((node.staticInput ?? {}).tag_ids ?? '[]') } catch { return [] } })()
              const verb = node.activityName === 'cf_add_tag' ? 'Add' : 'Remove'
              return ids.length > 0 ? `${verb} Tags (${ids.length} selected)` : `Open Tag Picker`
            })()}
          </button>
          {showCfTagPicker && (
            <CfTagPickerModal
              existingTagIds={(node.staticInput ?? {}).tag_ids ?? '[]'}
              mode={node.activityName === 'cf_add_tag' ? 'add' : 'remove'}
              onSave={ids => {
                setInput('tag_ids', ids)
                setShowCfTagPicker(false)
              }}
              onClose={() => setShowCfTagPicker(false)}
            />
          )}
        </>
      )}

      {isPbUpsert && (
        <>
          <button
            onClick={() => setShowPbUpsertWhereBuilder(true)}
            style={{
              display: 'block', width: '100%', marginTop: '10px',
              padding: '6px 10px', background: '#0369a1', color: 'white',
              border: 'none', borderRadius: '4px', cursor: 'pointer',
              fontWeight: 600, fontSize: '11px', textAlign: 'left',
            }}
          >
            {(() => {
              const si = node.staticInput ?? {}
              try {
                const f = JSON.parse(si.where_filters ?? '[]') as Array<{field:string;operator:string;value:string}>
                if (f.length > 0) return `Update Where (${f.length} rule${f.length > 1 ? 's' : ''})`
              } catch { /* ignore */ }
              return 'Update Where (optional)'
            })()}
          </button>
          {(() => {
            const si = node.staticInput ?? {}
            let existingFilters: QueryFilter[] = []
            try { existingFilters = (JSON.parse(si.where_filters ?? '[]') as Array<{field:string;operator:string;value:string}>).map(f => ({ ...f, id: qid() })) } catch { /* ignore */ }
            const existing: QueryConfig = {
              tableName: si.table_name ?? '',
              resultKey: '',
              filters: existingFilters,
              filterMode: (si.where_filter_mode as 'and' | 'or') || 'and',
              sortField: '',
              sortDir: 'asc',
              limit: 1,
            }
            return showPbUpsertWhereBuilder ? (
              <PbQueryBuilderModal
                existing={existing}
                defaultSourceJson={debugContext}
                onSave={cfg => {
                  const filters = cfg.filters.filter(f => f.field && f.operator)
                  const next = { ...(node.staticInput ?? {}), where_filters: JSON.stringify(filters.map(({ field, operator, value }) => ({ field, operator, value }))), where_filter_mode: cfg.filterMode }
                  onChange({ ...node, staticInput: next })
                  setShowPbUpsertWhereBuilder(false)
                }}
                onClose={() => setShowPbUpsertWhereBuilder(false)}
              />
            ) : null
          })()}
        </>
      )}

      {isPbQuery && (
        <>
          <button
            onClick={() => setShowPbQueryBuilder(true)}
            style={{
              display: 'block', width: '100%', marginTop: '10px',
              padding: '6px 10px', background: '#0ea5e9', color: 'white',
              border: 'none', borderRadius: '4px', cursor: 'pointer',
              fontWeight: 600, fontSize: '11px', textAlign: 'left',
            }}
          >
            Open Query Builder
          </button>
          {/* Summary of current query config */}
          {(node.staticInput ?? {}).table_name && (
            <div style={{ marginTop: '6px', fontSize: '11px', color: '#555', background: '#f0f9ff', border: '1px solid #bae6fd', borderRadius: '4px', padding: '6px 10px' }}>
              <strong>{(node.staticInput ?? {}).table_name}</strong>
              {(node.staticInput ?? {}).filters && (() => {
                try {
                  const fs = JSON.parse((node.staticInput ?? {}).filters)
                  return <span style={{ color: '#0284c7' }}> · {fs.length} filter{fs.length !== 1 ? 's' : ''} ({(node.staticInput ?? {}).filter_mode ?? 'and'})</span>
                } catch { return null }
              })()}
              {(node.staticInput ?? {}).sort_field && <span style={{ color: '#0284c7' }}> · sort: {(node.staticInput ?? {}).sort_field} {(node.staticInput ?? {}).sort_dir ?? 'asc'}</span>}
              {(node.staticInput ?? {}).limit && <span style={{ color: '#0284c7' }}> · limit {(node.staticInput ?? {}).limit}</span>}
              {(node.staticInput ?? {}).result_key && <span style={{ color: '#7c3aed' }}> → {(node.staticInput ?? {}).result_key}.*</span>}
            </div>
          )}
          {showPbQueryBuilder && (() => {
            const si = node.staticInput ?? {}
            let existingFilters: QueryFilter[] = []
            try { existingFilters = (JSON.parse(si.filters ?? '[]') as Array<{field:string;operator:string;value:string}>).map(f => ({ ...f, id: qid() })) } catch { /* ignore */ }
            const existing: QueryConfig = {
              tableName: si.table_name ?? '',
              resultKey: si.result_key ?? '',
              filters: existingFilters,
              filterMode: (si.filter_mode as 'and' | 'or') ?? 'and',
              sortField: si.sort_field ?? '',
              sortDir: (si.sort_dir as 'asc' | 'desc') ?? 'asc',
              limit: Number(si.limit) || 50,
            }
            return (
              <PbQueryBuilderModal
                existing={existing}
                defaultSourceJson={debugContext}
                onSave={cfg => {
                  onChange({
                    ...node,
                    staticInput: {
                      ...(node.staticInput ?? {}),
                      table_name: cfg.tableName,
                      result_key: cfg.resultKey,
                      filters: JSON.stringify(cfg.filters.map(({ id: _id, ...rest }) => rest)),
                      filter_mode: cfg.filterMode,
                      sort_field: cfg.sortField,
                      sort_dir: cfg.sortDir,
                      limit: String(cfg.limit),
                    },
                  })
                  setShowPbQueryBuilder(false)
                }}
                onClose={() => setShowPbQueryBuilder(false)}
              />
            )
          })()}
        </>
      )}

      {node.activityName === 'http_request' && (
        <HttpResponseMapperPanel
          node={node}
          onInsertMapperNode={onInsertMapperNode}
        />
      )}

      {isHttpRequest && showHttpBodyBuilder && (
        <HttpBodyBuilderModal
          existingBody={(node.staticInput ?? {}).body ?? ''}
          defaultSourceJson={debugContext}
          onSave={body => {
            setInput('body', body)
            setShowHttpBodyBuilder(false)
          }}
          onClose={() => setShowHttpBodyBuilder(false)}
        />
      )}

      {isLlm && showPromptVarsBuilder && (
        <PromptVarsBuilderModal
          existingVars={(() => { try { return JSON.parse((node.staticInput ?? {}).prompt_vars ?? '{}') } catch { return {} } })()}
          defaultSourceJson={debugContext}
          onSave={vars => {
            setInput('prompt_vars', JSON.stringify(vars))
            setShowPromptVarsBuilder(false)
          }}
          onClose={() => setShowPromptVarsBuilder(false)}
        />
      )}

      {isTextSubst && showTextSubstBuilder && (
        <TextSubstBuilderModal
          existingVars={(() => { try { return JSON.parse((node.staticInput ?? {}).vars ?? '{}') } catch { return {} } })()}
          defaultSourceJson={debugContext}
          onSave={vars => {
            setInput('vars', JSON.stringify(vars))
            setShowTextSubstBuilder(false)
          }}
          onClose={() => setShowTextSubstBuilder(false)}
        />
      )}

      {isGdriveFillTemplate && showGdriveTemplateVarsBuilder && (
        <GdriveTemplateVarsBuilderModal
          templateId={(node.staticInput ?? {}).template_id ?? ''}
          existingVars={(() => { try { return JSON.parse((node.staticInput ?? {}).vars ?? '{}') } catch { return {} } })()}
          existingImageVars={(() => { try { return JSON.parse((node.staticInput ?? {}).image_vars ?? '{}') } catch { return {} } })()}
          defaultSourceJson={debugContext}
          onSave={(vars, imageVars) => {
            set({ staticInput: { ...(node.staticInput ?? {}), vars: JSON.stringify(vars), image_vars: JSON.stringify(imageVars) } })
            setShowGdriveTemplateVarsBuilder(false)
          }}
          onClose={() => setShowGdriveTemplateVarsBuilder(false)}
        />
      )}

      {(isPbWriteNode || isPbQuery) && (
        <>
          <label style={{ ...labelStyle, marginTop: '10px' }}>
            Result Key
            <span title="Store output under this context key (e.g. &quot;contact&quot; → {{contact.id}}). Useful when multiple PB nodes share similar output keys." style={{ marginLeft: '4px', color: '#6b46c1', cursor: 'help' }}>ℹ</span>
          </label>
          <input
            value={(node.staticInput ?? {}).result_key ?? ''}
            onChange={e => setInput('result_key', e.target.value)}
            style={inputStyle}
            placeholder={isPbQuery ? 'e.g. contacts' : 'e.g. contact'}
          />
        </>
      )}

      {!isStart && (
        <button onClick={onSetStart}
          style={{ marginTop: '10px', display: 'block', width: '100%', ...iconBtn('#27ae60'), padding: '6px' }}>
          Set as Start Node
        </button>
      )}
      {isStart && (
        <div style={{ marginTop: '8px', fontSize: '12px', color: '#27ae60', fontWeight: 600 }}>✓ Start Node</div>
      )}

      <button onClick={() => onTestFromHere(node.id)}
        style={{ display: 'block', marginTop: '10px', width: '100%', ...iconBtn('#e67e22'), padding: '6px' }}>
        ▶ Test from this node
      </button>

      <CopyNodeButton node={node} />
      <CopyNodeJsonButton node={node} />

      <button onClick={onDelete} style={{ display: 'block', marginTop: '8px', width: '100%', ...iconBtn('#e74c3c'), padding: '6px' }}>
        Delete Node
      </button>
    </div>
  )
}

const NODE_CLIPBOARD_KEY = 'cbw_node_clipboard'

function CopyNodeButton({ node }: { node: BNode }) {
  const [copied, setCopied] = useState(false)
  const handleCopy = () => {
    const data = {
      activityName: node.activityName,
      label: node.label,
      maxRetries: node.maxRetries,
      isHuman: node.isHuman,
      needsConversion: node.needsConversion,
      staticInput: node.staticInput ?? {},
    }
    localStorage.setItem(NODE_CLIPBOARD_KEY, JSON.stringify(data))
    setCopied(true)
    setTimeout(() => setCopied(false), 1500)
  }
  return (
    <button onClick={handleCopy} style={{ display: 'block', marginTop: '8px', width: '100%', ...iconBtn('#0ea5e9'), padding: '6px' }}>
      {copied ? '✓ Copied' : '⎘ Copy Node'}
    </button>
  )
}

function CopyNodeJsonButton({ node }: { node: BNode }) {
  const [copied, setCopied] = useState(false)
  const handleCopy = () => {
    const data = {
      id: node.id,
      activityName: node.activityName,
      label: node.label,
      staticInput: node.staticInput ?? {},
    }
    navigator.clipboard.writeText(JSON.stringify(data, null, 2)).then(() => {
      setCopied(true)
      setTimeout(() => setCopied(false), 1500)
    })
  }
  return (
    <button onClick={handleCopy} style={{ display: 'block', marginTop: '8px', width: '100%', ...iconBtn('#7c3aed'), padding: '6px' }}>
      {copied ? '✓ JSON Copied' : '{ } Copy Node JSON'}
    </button>
  )
}

function PasteNodeButton({ onPaste }: { onPaste: (clip: Omit<BNode, 'id' | 'x' | 'y'>) => void }) {
  const [hasClip, setHasClip] = useState(() => !!localStorage.getItem(NODE_CLIPBOARD_KEY))
  // Re-check when storage changes (e.g. user copies in another tab)
  useEffect(() => {
    const handler = () => setHasClip(!!localStorage.getItem(NODE_CLIPBOARD_KEY))
    window.addEventListener('storage', handler)
    return () => window.removeEventListener('storage', handler)
  }, [])
  const handlePaste = () => {
    const raw = localStorage.getItem(NODE_CLIPBOARD_KEY)
    if (!raw) return
    try {
      const clip = JSON.parse(raw) as Omit<BNode, 'id' | 'x' | 'y'>
      onPaste(clip)
      setHasClip(true)
    } catch { /* ignore */ }
  }
  if (!hasClip) return null
  return (
    <button onClick={handlePaste} style={hBtn('#0ea5e9')}>
      ⎘ Paste Node
    </button>
  )
}

// ── Test-from-node modal ──────────────────────────────────────────────────────

function TestModal({ nodeId, graphName, onClose, onRun }: {
  nodeId: string
  graphName: string
  onClose: () => void
  onRun: (nodeId: string, context: string) => Promise<void>
}) {
  const [context, setContext] = useState('{}')
  const [running, setRunning] = useState(false)
  const [error, setError] = useState('')

  const run = async () => {
    setRunning(true); setError('')
    try {
      await onRun(nodeId, context)
    } catch (e) {
      setError(String(e))
      setRunning(false)
    }
  }

  return (
    <div style={{ position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.5)', display: 'flex', alignItems: 'center', justifyContent: 'center', zIndex: 1000 }}>
      <div style={{ background: 'white', borderRadius: '8px', padding: '24px', width: '440px', boxShadow: '0 20px 60px rgba(0,0,0,0.3)' }}>
        <h3 style={{ margin: '0 0 4px', color: '#2c3e50' }}>Test from node</h3>
        <div style={{ fontSize: '12px', color: '#888', marginBottom: '16px' }}>
          Graph: <code style={{ color: '#667eea' }}>{graphName}</code> · Starting at: <code style={{ color: '#e67e22' }}>{nodeId}</code>
        </div>
        <div style={{ fontSize: '12px', color: '#555', marginBottom: '4px', fontWeight: 600 }}>
          Initial Context (JSON)
        </div>
        <div style={{ fontSize: '11px', color: '#aaa', marginBottom: '6px' }}>
          These values will be merged into the workflow context before running. Use to provide the inputs your node expects.
        </div>
        <textarea value={context} onChange={e => setContext(e.target.value)} rows={8}
          style={{ width: '100%', fontFamily: 'monospace', fontSize: '12px', padding: '10px', border: '1px solid #e0e6ed', borderRadius: '4px', boxSizing: 'border-box', resize: 'vertical' }} />
        {error && <div style={{ color: '#e74c3c', fontSize: '12px', marginTop: '6px' }}>{error}</div>}
        <div style={{ display: 'flex', gap: '10px', justifyContent: 'flex-end', marginTop: '14px' }}>
          <button onClick={onClose} style={{ padding: '8px 16px', background: '#f0f0f0', border: 'none', borderRadius: '4px', cursor: 'pointer', fontWeight: 500 }}>Cancel</button>
          <button onClick={run} disabled={running}
            style={{ padding: '8px 16px', background: running ? '#aaa' : '#e67e22', color: 'white', border: 'none', borderRadius: '4px', cursor: running ? 'default' : 'pointer', fontWeight: 500 }}>
            {running ? 'Starting…' : '▶ Run'}
          </button>
        </div>
      </div>
    </div>
  )
}

// ── Main WorkflowBuilder ──────────────────────────────────────────────────────

export default function WorkflowBuilder() {
  const navigate = useNavigate()
  const svgRef = useRef<SVGSVGElement>(null)

  const [activities, setActivities] = useState<ActivityInfo[]>([])
  const [openCategory, setOpenCategory] = useState<string | null>(null)
  const [nodes, setNodes] = useState<BNode[]>([])
  const [edges, setEdges] = useState<BEdge[]>([])
  const [startNode, setStartNode] = useState<string | null>(null)
  const [selectedNode, setSelectedNode] = useState<string | null>(null)
  const [selectedEdge, setSelectedEdge] = useState<string | null>(null)
  const [draggingNode, setDraggingNode] = useState<string | null>(null)
  const [dragOffset, setDragOffset] = useState({ x: 0, y: 0 })
  // Drag-to-connect state
  const dragConnectFrom = useRef<string | null>(null)
  const [dragConnectLine, setDragConnectLine] = useState<{ x1: number; y1: number; x2: number; y2: number } | null>(null)
  const [connectHover, setConnectHover] = useState<string | null>(null)
  const [graphName, setGraphName] = useState('my_workflow')
  const [saveStatus, setSaveStatus] = useState<string>('')
  const [testModal, setTestModal] = useState<string | null>(null)  // nodeId being tested
  const [graphNotFound, setGraphNotFound] = useState<string>('')

  // ── Debug panel state ──────────────────────────────────────────────────────
  const [debugOpen, setDebugOpen] = useState(false)
  const [debugHeight, setDebugHeight] = useState(300)
  const [debugContext, setDebugContext] = useState('{}')
  const [debugNodeIdx, setDebugNodeIdx] = useState(0)
  const [debugResults, setDebugResults] = useState<Array<{nodeId: string; activityName: string; output?: unknown; error?: string; skipped?: boolean}>>([])
  const [debugRunning, setDebugRunning] = useState(false)
  const [debugResetKey, setDebugResetKey] = useState(0)
  const [debugCopied, setDebugCopied] = useState(false)
  const [debugCurlModal, setDebugCurlModal] = useState<string | null>(null)

  // ── Loop-aware context enrichment ─────────────────────────────────────────
  // Inject item: arr[0] into context when a node is inside a loop body.
  const enrichContextForNode = useCallback((ctx: string, nodeId: string): string => {
    for (const loopNode of nodes.filter(n => n.activityName === 'loop')) {
      const reachable = new Set<string>()
      const q = edges.filter(e => e.fromNode === loopNode.id).map(e => e.toNode)
      while (q.length > 0) {
        const id = q.shift()!
        if (reachable.has(id) || id === loopNode.id) continue
        reachable.add(id)
        const n = nodes.find(n => n.id === id)
        if (!n || n.activityName === 'loop_next') continue
        for (const e of edges.filter(e => e.fromNode === id)) q.push(e.toNode)
      }
      if (reachable.has(nodeId)) {
        const listKey = loopNode.staticInput?.list_key || 'records'
        const itemKey = loopNode.staticInput?.item_key || 'item'
        try {
          const ctxObj = JSON.parse(ctx) as Record<string, unknown>
          // Support dotted paths like "records.records"
          let raw: unknown = ctxObj
          for (const part of listKey.split('.')) {
            raw = (raw as Record<string, unknown>)?.[part]
          }
          const arr = Array.isArray(raw) ? raw : null
          if (arr && arr.length > 0) {
            return JSON.stringify({ ...ctxObj, [itemKey]: arr[0] }, null, 2)
          }
        } catch { /* ignore */ }
      }
    }
    return ctx
  }, [nodes, edges])

  const enrichedDebugContext = useMemo(() => {
    if (!selectedNode) return debugContext
    return enrichContextForNode(debugContext, selectedNode)
  }, [debugContext, selectedNode, enrichContextForNode])

  const onDebugResizeStart = (e: React.MouseEvent) => {
    e.preventDefault()
    const startY = e.clientY
    const startH = debugHeight
    const onMove = (ev: MouseEvent) => setDebugHeight(Math.max(150, Math.min(window.innerHeight - 200, startH - (ev.clientY - startY))))
    const onUp = () => { window.removeEventListener('mousemove', onMove); window.removeEventListener('mouseup', onUp) }
    window.addEventListener('mousemove', onMove)
    window.addEventListener('mouseup', onUp)
  }

  // Drag from sidebar
  const dragActivity = useRef<string | null>(null)

  useEffect(() => {
    fetch('/api/workflows/activities')
      .then(r => r.json())
      .then(d => setActivities(d.activities ?? []))
  }, [])

  // Load existing graph if ?graph=name is in the URL
  useEffect(() => {
    const params = new URLSearchParams(window.location.search)
    const editGraph = params.get('graph')
    if (!editGraph) return
    fetch('/api/workflows/graphs')
      .then(r => r.json())
      .then(data => {
        const g = data.graphs?.[editGraph]
        if (!g) {
          setGraphName(editGraph)
          setGraphNotFound(editGraph)
          return
        }
        setGraphNotFound('')
        setGraphName(g.name)
        // Walk nodes in execution order (BFS from start_node) so left-to-right layout matches firing order
        type RawNode = { id: string; activity_name: string; label?: string; max_retries: number; is_human: boolean; needs_conversion?: boolean; input?: Record<string, string>; transitions: Array<{ next_node: string; conditions: unknown[]; label?: string }> }
        const nodeMap = g.nodes as Record<string, RawNode>
        const ordered: RawNode[] = []
        const visited = new Set<string>()
        const queue: string[] = [g.start_node]
        while (queue.length > 0) {
          const id = queue.shift()!
          if (visited.has(id) || !nodeMap[id]) continue
          visited.add(id)
          ordered.push(nodeMap[id])
          for (const t of nodeMap[id].transitions ?? []) {
            if (t.next_node && !visited.has(t.next_node)) queue.push(t.next_node)
          }
        }
        // Any nodes not reachable from start (disconnected) go at the end
        for (const n of Object.values(nodeMap) as RawNode[]) {
          if (!visited.has(n.id)) ordered.push(n)
        }
        let col = 0, row = 0
        const loadedNodes: BNode[] = ordered.map(n => {
          const x = 80 + col * 220; const y = 100 + row * 120
          col++; if (col > 4) { col = 0; row++ }
          return {
            id: n.id, activityName: n.activity_name,
            x, y, label: n.label ?? '', maxRetries: n.max_retries ?? 0,
            isHuman: n.is_human ?? false, needsConversion: n.needs_conversion ?? false,
            staticInput: (n.input ?? {}) as Record<string, string>,
          }
        })
        const nodeEntries = ordered
        const loadedEdges: BEdge[] = []
        for (const n of nodeEntries) {
          for (const t of n.transitions ?? []) {
            if (!t.next_node) continue
            const conditions: CondRow[] = (t.conditions ?? []).map((c: unknown) => {
              const rc = c as { key?: string; operator?: string; value?: unknown }
              return {
                key: rc.key ?? '',
                operator: rc.operator ?? 'eq',
                value: rc.value != null ? String(rc.value) : '',
              }
            })
            loadedEdges.push({
              id: newEid(), fromNode: n.id, toNode: t.next_node,
              conditions, label: t.label ?? '',
            })
          }
        }
        setNodes(loadedNodes)
        setEdges(loadedEdges)
        setStartNode(g.start_node)
      })
  }, [])

  // ── SVG coordinate helper ─────────────────────────────────────────────────

  const svgCoords = useCallback((clientX: number, clientY: number) => {
    if (!svgRef.current) return { x: clientX, y: clientY }
    const rect = svgRef.current.getBoundingClientRect()
    return { x: clientX - rect.left, y: clientY - rect.top }
  }, [])

  // ── Canvas drop (from sidebar) ────────────────────────────────────────────

  const onCanvasDrop = (e: React.DragEvent) => {
    e.preventDefault()
    if (!dragActivity.current) return
    const { x, y } = svgCoords(e.clientX, e.clientY)
    const name = dragActivity.current
    dragActivity.current = null

    const id = newNid()
    const activityName = name === '__human__' ? 'human_input' : name === '__python__' ? 'python_eval' : name
    const isHumanNode = activityName === 'human_input' || activityName === 'python_eval'
    const defaultStaticInput: Record<string, string> =
      activityName === 'python_eval' ? { code: 'print("input was:", input)\nresult = {"doubled": input.get("value", 0) * 2, "msg": "hello from pyodide"}' }
      : activityName === 'loop' ? { list_key: 'records', item_key: 'item' }
      : {}
    const node: BNode = {
      id, activityName, x: x - NODE_W / 2, y: y - NODE_H / 2,
      label: '', maxRetries: 0, isHuman: isHumanNode, needsConversion: false, staticInput: defaultStaticInput,
    }
    setNodes(prev => {
      const next = [...prev, node]
      // First non-END node becomes start
      if (activityName !== 'END' && !startNode) setStartNode(id)
      return next
    })
    setSelectedNode(id)
    setSelectedEdge(null)
  }

  // ── Node dragging (move on canvas) ────────────────────────────────────────

  const onNodeMouseDown = (e: React.MouseEvent, nodeId: string) => {
    if (dragConnectFrom.current) return // don't drag while connecting
    e.stopPropagation()
    const node = nodes.find(n => n.id === nodeId)!
    const { x, y } = svgCoords(e.clientX, e.clientY)
    setDraggingNode(nodeId)
    setDragOffset({ x: x - node.x, y: y - node.y })
    setSelectedNode(nodeId)
    setSelectedEdge(null)
  }

  const onSvgMouseMove = (e: React.MouseEvent) => {
    const { x, y } = svgCoords(e.clientX, e.clientY)
    if (draggingNode) {
      setNodes(prev => prev.map(n =>
        n.id === draggingNode ? { ...n, x: x - dragOffset.x, y: y - dragOffset.y } : n
      ))
    }
    if (dragConnectFrom.current) {
      // Find source port position
      const src = nodes.find(n => n.id === dragConnectFrom.current)
      if (src) {
        setDragConnectLine({ x1: src.x + NODE_W + PORT_R, y1: src.y + NODE_H / 2, x2: x, y2: y })
        // Check if hovering over a valid target node
        const target = nodes.find(n =>
          n.id !== dragConnectFrom.current &&
          x >= n.x && x <= n.x + NODE_W && y >= n.y && y <= n.y + NODE_H
        )
        setConnectHover(target?.id ?? null)
      }
    }
  }

  const onSvgMouseUp = (e: React.MouseEvent) => {
    if (dragConnectFrom.current) {
      const { x, y } = svgCoords(e.clientX, e.clientY)
      const target = nodes.find(n =>
        n.id !== dragConnectFrom.current &&
        x >= n.x && x <= n.x + NODE_W && y >= n.y && y <= n.y + NODE_H
      )
      if (target) createEdge(dragConnectFrom.current, target.id)
      dragConnectFrom.current = null
      setDragConnectLine(null)
      setConnectHover(null)
    }
    setDraggingNode(null)
  }

  // ── Drag-to-connect: start on output port mousedown ───────────────────────

  const onOutputPortMouseDown = (e: React.MouseEvent, nodeId: string) => {
    e.stopPropagation()
    e.preventDefault()
    dragConnectFrom.current = nodeId
    const src = nodes.find(n => n.id === nodeId)!
    const { x, y } = svgCoords(e.clientX, e.clientY)
    setDragConnectLine({ x1: src.x + NODE_W + PORT_R, y1: src.y + NODE_H / 2, x2: x, y2: y })
    setSelectedNode(null)
    setSelectedEdge(null)
  }

  const createEdge = (fromNode: string, toNode: string) => {
    const id = newEid()
    const edge: BEdge = { id, fromNode, toNode, conditions: [], label: '' }
    setEdges(prev => [...prev, edge])
    setSelectedEdge(id)
    setSelectedNode(null)
  }

  // ── Canvas click (deselect) ───────────────────────────────────────────────

  const onSvgClick = () => {
    setSelectedNode(null)
    setSelectedEdge(null)
  }

  // ── Save graph ────────────────────────────────────────────────────────────

  const saveGraph = async () => {
    if (!graphName.trim()) { setSaveStatus('Graph name is required.'); return }
    if (nodes.filter(n => n.activityName !== 'END').length === 0) {
      setSaveStatus('Add at least one activity node.'); return
    }
    if (!startNode) { setSaveStatus('Set a start node.'); return }

    const graphNodes: Record<string, object> = {}
    for (const n of nodes) {
      if (n.activityName === 'END') continue
      const outEdges = edges.filter(e => e.fromNode === n.id)
      const transitions = outEdges.map(e => ({
        conditions: e.conditions
          .filter(c => c.key || ['exists', 'not_exists'].includes(c.operator))
          .map(c => ({ key: c.key, operator: c.operator, value: parseVal(c.value) })),
        next_node: e.toNode === 'END' || nodes.find(x => x.id === e.toNode)?.activityName === 'END'
          ? ''
          : e.toNode,
        label: e.label || undefined,
      }))
      // Loop nodes require a "body" label on the first transition
      if (n.activityName === 'loop' && transitions.length > 0 && !transitions.some(t => t.label === 'body')) {
        transitions[0] = { ...transitions[0], label: 'body' }
      }
      const si = n.staticInput ?? {}
      const inputPayload = n.activityName === 'python_eval'
        ? { code: si.code ?? '' }
        : Object.keys(si).length > 0 ? si : undefined
      graphNodes[n.id] = {
        id: n.id,
        activity_name: n.activityName,
        max_retries: n.maxRetries,
        transitions,
        is_human: n.isHuman,
        ...(n.label ? { label: n.label } : {}),
        ...(n.needsConversion ? { needs_conversion: true } : {}),
        ...(inputPayload !== undefined ? { input: inputPayload } : {}),
      }
    }

    const graph = { name: graphName, start_node: startNode, nodes: graphNodes }
    const res = await fetch('/api/workflows/graphs', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ name: graphName, graph }),
    })
    if (res.ok) {
      setSaveStatus('Saved!')
      setTimeout(() => setSaveStatus(''), 2000)
      return true
    } else {
      const txt = await res.text()
      setSaveStatus(`Error: ${txt}`)
      return false
    }
  }

  // ── Insert mapper node after http_request ─────────────────────────────────

  const insertMapperNode = useCallback((sourceNodeId: string, mappings: Array<{ from: string; to: string }>) => {
    const sourceNode = nodes.find(n => n.id === sourceNodeId)
    if (!sourceNode) return

    const mapperId = newNid()
    // Build staticInput: mappings is an array; we store as JSON in a "mappings" key
    const mappingsPayload = mappings.map(m => ({ from: m.from, to: m.to }))
    const mapperNode: BNode = {
      id: mapperId,
      activityName: 'mapper',
      x: sourceNode.x + 220,
      y: sourceNode.y,
      label: 'mapper',
      maxRetries: 0,
      isHuman: false,
      needsConversion: false,
      staticInput: { mappings: JSON.stringify(mappingsPayload) },
    }

    // Find existing outbound edges from sourceNode and redirect them to come from mapperNode
    setEdges(prev => {
      const outboundEdges = prev.filter(e => e.fromNode === sourceNodeId)
      const otherEdges = prev.filter(e => e.fromNode !== sourceNodeId)
      // Re-wire existing outbound edges to go from mapperNode instead
      const rewiredEdges = outboundEdges.map(e => ({ ...e, id: newEid(), fromNode: mapperId }))
      // New edge: sourceNode → mapperNode
      const newEdge: BEdge = { id: newEid(), fromNode: sourceNodeId, toNode: mapperId, conditions: [], label: '' }
      return [...otherEdges, newEdge, ...rewiredEdges]
    })

    setNodes(prev => [...prev, mapperNode])
    setSelectedNode(mapperId)
  }, [nodes])

  // ── Debug: ordered node walk from startNode ───────────────────────────────

  // debugNodes is built dynamically as the user steps through the workflow.
  // After each node executes, we evaluate its outgoing edge conditions against
  // the current context to append the correct next node (respecting branches).
  const [debugNodes, setDebugNodes] = useState<BNode[]>(() => {
    if (!startNode || nodes.length === 0) return []
    const first = nodes.find(n => n.id === startNode)
    return first ? [first] : []
  })

  // Evaluate one condition row against ctx (mirrors Go evalCondition logic).
  const evalDebugCondition = useCallback((cond: CondRow, ctx: Record<string, unknown>): boolean => {
    let key = cond.key.trim()
    if (key.startsWith('{{') && key.endsWith('}}')) key = key.slice(2, -2).trim()
    const value = resolvePath(key, ctx)
    const toNum = (v: unknown): number | null => {
      const n = Number(v)
      return isNaN(n) ? null : n
    }
    switch (cond.operator) {
      case 'exists':     return value !== undefined && value !== null
      case 'not_exists': return value === undefined || value === null
      case 'eq':         return String(value ?? '') === String(cond.value)
      case 'neq':        return String(value ?? '') !== String(cond.value)
      case 'contains':   return String(value ?? '').includes(String(cond.value))
      case 'gte': { const a = toNum(value), b = toNum(cond.value); return a !== null && b !== null && a >= b }
      case 'lte': { const a = toNum(value), b = toNum(cond.value); return a !== null && b !== null && a <= b }
      case 'gt':  { const a = toNum(value), b = toNum(cond.value); return a !== null && b !== null && a > b }
      case 'lt':  { const a = toNum(value), b = toNum(cond.value); return a !== null && b !== null && a < b }
      default:           return false
    }
  }, [])

  // Find the next node ID by evaluating outgoing edge conditions against ctx.
  const findNextDebugNodeId = useCallback((fromNodeId: string, ctx: Record<string, unknown>): string | null => {
    const outEdges = edges.filter(e => e.fromNode === fromNodeId)
    for (const edge of outEdges) {
      const allMatch = edge.conditions.length === 0 || edge.conditions.every(c => evalDebugCondition(c, ctx))
      if (allMatch) return edge.toNode && edge.toNode !== 'END' ? edge.toNode : null
    }
    return null
  }, [edges, evalDebugCondition])

  // Append the next node to debugNodes after a step, based on condition evaluation.
  const appendNextDebugNode = useCallback((fromNodeId: string, ctx: Record<string, unknown>) => {
    const nextId = findNextDebugNodeId(fromNodeId, ctx)
    if (!nextId) return
    const nextNode = nodes.find(n => n.id === nextId)
    if (nextNode) setDebugNodes(prev => [...prev, nextNode])
  }, [findNextDebugNodeId, nodes])

  // ── Template resolver (type-preserving) ───────────────────────────────────
  // Walks a value recursively. When a string is exactly "{{path}}", it returns
  // the resolved value at that path (preserving type — array, object, etc.).
  // When a string contains templates mixed with other text, string-interpolates.
  function resolvePath(path: string, ctx: Record<string, unknown>): unknown {
    const parts = path.split('.')
    let cur: unknown = ctx
    for (const p of parts) cur = (cur as Record<string, unknown>)?.[p]
    return cur
  }

  function applyTypeHintCoercion(raw: unknown, hint: string): unknown {
    if (hint === 'number') {
      const n = Number(raw)
      return isNaN(n) ? raw : n
    }
    if (hint === 'bool') {
      const s = String(raw).toLowerCase()
      return s === 'true' || s === '1' || s === 'yes'
    }
    return raw  // 'string' or unknown hint — keep as-is
  }

  function resolveTemplates(val: unknown, ctx: Record<string, unknown>): unknown {
    if (typeof val === 'string') {
      // Full-string match: {{key}} or {{key|hint}}
      const full = val.match(/^\{\{([^{}|]+?)(?:\|(number|bool|string))?\}\}$/)
      if (full) {
        const resolved = resolvePath(full[1], ctx)
        if (resolved === undefined || resolved === null) return ''
        return full[2] ? applyTypeHintCoercion(resolved, full[2]) : resolved
      }
      // Inline replacements — strip hints, substitute as strings
      return val.replace(/\{\{([^{}|]+?)(?:\|(number|bool|string))?\}\}/g, (_, path, hint) => {
        const resolved = resolvePath(path, ctx)
        if (resolved == null) return ''
        if (hint) {
          const coerced = applyTypeHintCoercion(resolved, hint)
          return typeof coerced === 'object' ? JSON.stringify(coerced) : String(coerced)
        }
        return typeof resolved === 'object' ? JSON.stringify(resolved) : String(resolved)
      })
    }
    if (Array.isArray(val)) return val.map(v => resolveTemplates(v, ctx))
    if (val && typeof val === 'object') {
      const result: Record<string, unknown> = {}
      for (const [k, v] of Object.entries(val as Record<string, unknown>)) {
        result[k] = resolveTemplates(v, ctx)
      }
      return result
    }
    return val
  }

  // ── Debug: step one node ──────────────────────────────────────────────────

  const debugStep = useCallback(async () => {
    const node = debugNodes[debugNodeIdx]
    if (!node) return

    setDebugRunning(true)
    let input: Record<string, unknown> = {}
    try { input = JSON.parse(enrichContextForNode(debugContext, node.id)) } catch { /* keep empty */ }

    const mergedInput: Record<string, unknown> = { ...input }
    for (const [k, v] of Object.entries(node.staticInput ?? {})) {
      let parsed: unknown = v
      if (typeof v === 'string') { try { parsed = JSON.parse(v) } catch { /* keep */ } }
      mergedInput[k] = resolveTemplates(parsed, input)
    }

    let data: { output?: unknown; error?: string; skipped?: boolean } = {}
    try {
      const resp = await fetch('/api/workflows/execute-node', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ activity: node.activityName, input: mergedInput }),
      })
      const ct = resp.headers.get('content-type') ?? ''
      if (!ct.includes('application/json')) {
        data = { error: `Session expired — please refresh the page and log in again (got ${resp.status})` }
      } else {
        data = await resp.json()
      }
    } catch (e) {
      data = { error: String(e) }
    }

    if (data.skipped) {
      setDebugResults(prev => [...prev, { nodeId: node.id, activityName: node.activityName, output: input, skipped: true }])
      appendNextDebugNode(node.id, input)
      setDebugNodeIdx(i => i + 1)
      setDebugRunning(false)
      return
    }

    let mergedCtx = { ...input }
    if (!data.error && data.output && typeof data.output === 'object') {
      mergedCtx = { ...mergedCtx, ...(data.output as Record<string, unknown>) }
    }
    setDebugResults(prev => [...prev, { nodeId: node.id, activityName: node.activityName, output: mergedCtx, error: data.error }])

    if (!data.error) {
      try {
        setDebugContext(JSON.stringify(mergedCtx, null, 2))
      } catch { /* keep as-is */ }
      appendNextDebugNode(node.id, mergedCtx)
      setDebugNodeIdx(i => i + 1)
    }

    setDebugRunning(false)
  }, [debugNodeIdx, debugContext, debugNodes, enrichContextForNode, appendNextDebugNode])

  // ── Debug: run all remaining nodes ────────────────────────────────────────

  const debugRunAll = useCallback(async () => {
    let idx = debugNodeIdx
    let ctx = debugContext
    let results = [...debugResults]
    let currentNodes = [...debugNodes]

    while (idx < currentNodes.length) {
      setDebugRunning(true)
      const node = currentNodes[idx]
      if (!node) break

      let input: Record<string, unknown> = {}
      try { input = JSON.parse(enrichContextForNode(ctx, node.id)) } catch { /* keep empty */ }

      const mergedInput: Record<string, unknown> = { ...input }
      for (const [k, v] of Object.entries(node.staticInput ?? {})) {
        let parsed: unknown = v
        if (typeof v === 'string') { try { parsed = JSON.parse(v) } catch { /* keep */ } }
        mergedInput[k] = resolveTemplates(parsed, input)
      }

      let data: { output?: unknown; error?: string; skipped?: boolean } = {}
      try {
        const resp = await fetch('/api/workflows/execute-node', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ activity: node.activityName, input: mergedInput }),
        })
        const ct = resp.headers.get('content-type') ?? ''
        if (!ct.includes('application/json')) {
          data = { error: `Session expired — please refresh the page and log in again (got ${resp.status})` }
        } else {
          data = await resp.json()
        }
      } catch (e) {
        data = { error: String(e) }
      }

      if (data.skipped) {
        results = [...results, { nodeId: node.id, activityName: node.activityName, output: input, skipped: true }]
        const nextId = findNextDebugNodeId(node.id, input)
        if (nextId) { const nextNode = nodes.find(n => n.id === nextId); if (nextNode) currentNodes = [...currentNodes, nextNode] }
        idx++
        setDebugResults(results)
        setDebugNodeIdx(idx)
        setDebugNodes(currentNodes)
        await new Promise(r => setTimeout(r, 300))
        continue
      }

      let mergedCtxObj = { ...input }
      if (!data.error && data.output && typeof data.output === 'object') {
        mergedCtxObj = { ...mergedCtxObj, ...(data.output as Record<string, unknown>) }
        try { ctx = JSON.stringify(mergedCtxObj, null, 2) } catch { /* keep */ }
      }

      results = [...results, { nodeId: node.id, activityName: node.activityName, output: mergedCtxObj, error: data.error }]

      if (!data.error) {
        const nextId = findNextDebugNodeId(node.id, mergedCtxObj)
        if (nextId) { const nextNode = nodes.find(n => n.id === nextId); if (nextNode) currentNodes = [...currentNodes, nextNode] }
        idx++
      }

      setDebugResults(results)
      setDebugContext(ctx)
      setDebugNodeIdx(idx)
      setDebugNodes(currentNodes)

      if (data.error) break
      await new Promise(r => setTimeout(r, 300))
    }
    setDebugRunning(false)
  }, [debugNodeIdx, debugContext, debugResults, debugNodes, enrichContextForNode, findNextDebugNodeId, nodes])

  // ── Test from node ────────────────────────────────────────────────────────

  const runTestFromNode = async (nodeId: string, contextJson: string) => {
    // Save the graph first
    const ok = await saveGraph()
    if (!ok) throw new Error(saveStatus || 'Save failed — check graph name and nodes')

    let ctx: Record<string, unknown> = {}
    try { ctx = JSON.parse(contextJson) } catch { throw new Error('Invalid JSON in context') }

    const res = await fetch('/api/workflows', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ graph_name: graphName, start_node: nodeId, context: ctx }),
    })
    if (!res.ok) {
      const txt = await res.text()
      throw new Error(txt)
    }
    const data = await res.json()
    navigate(`/workflows/${data.workflow.id}`)
  }

  // ── Render helpers ────────────────────────────────────────────────────────

  const selNode = selectedNode ? nodes.find(n => n.id === selectedNode) : null
  const selEdge = selectedEdge ? edges.find(e => e.id === selectedEdge) : null

  // Debug ring lookup sets
  const debugCurrentNodeId = debugOpen && debugNodes[debugNodeIdx]?.id
  const debugCompletedIds = new Set(debugResults.filter(r => !r.error).map(r => r.nodeId))
  const debugFailedIds = new Set(debugResults.filter(r => !!r.error).map(r => r.nodeId))
  const lastDebugResult = debugResults[debugResults.length - 1]

  const svgW = Math.max(800, ...nodes.map(n => n.x + NODE_W + 100))
  const svgH = Math.max(600, ...nodes.map(n => n.y + NODE_H + 100))

  // Build edge paths
  function edgePath(fromNode: string, toNode: string) {
    const src = nodes.find(n => n.id === fromNode)
    const dst = nodes.find(n => n.id === toNode)
    if (!src || !dst) return null
    const x1 = src.x + NODE_W + PORT_R
    const y1 = src.y + NODE_H / 2
    const x2 = dst.x - PORT_R
    const y2 = dst.y + NODE_H / 2
    const mx = (x1 + x2) / 2
    return { x1, y1, x2, y2, mx }
  }

  return (
    <div style={{ display: 'flex', height: '100vh', fontFamily: '-apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif' }}>
      <Nav />
      <div style={{ display: 'flex', flexDirection: 'column', flex: 1, overflow: 'hidden', background: '#f5f7fa' }}>

      {/* Header */}
      <header style={{ background: 'white', borderBottom: '1px solid #e0e6ed', padding: '10px 16px', display: 'flex', alignItems: 'center', gap: '12px', boxShadow: '0 2px 4px rgba(0,0,0,0.05)' }}>
        <h1 style={{ margin: 0, fontSize: '18px', color: '#667eea', fontWeight: 600 }}>Workflow Builder</h1>
        <div style={{ flex: 1 }} />
        <label style={{ fontSize: '12px', color: '#666', fontWeight: 600 }}>Graph Name:</label>
        <input value={graphName} onChange={e => setGraphName(e.target.value)}
          style={{ fontFamily: 'monospace', padding: '5px 8px', border: '1px solid #e0e6ed', borderRadius: '4px', fontSize: '13px', width: '180px' }} />
        <button onClick={saveGraph} style={hBtn('#27ae60')}>Save Graph</button>
        <PasteNodeButton onPaste={clip => {
          const id = newNid()
          setNodes(ns => [...ns, { id, x: 200, y: 80 + ns.length * 80, ...clip }])
          setSelectedNode(id)
        }} />
        <button
          onClick={() => { setDebugOpen(o => { if (!o) { const first = nodes.find(n => n.id === startNode); setDebugNodes(first ? [first] : []); setDebugNodeIdx(0); setDebugResults([]) } return !o }) }}
          style={{ ...hBtn(debugOpen ? '#5a3ea0' : '#7c5cbf'), outline: debugOpen ? '2px solid #b39ddb' : 'none', outlineOffset: '2px' }}
        >
          ▶ Debug
        </button>
        {saveStatus && <span style={{ fontSize: '12px', color: saveStatus.startsWith('Error') ? '#e74c3c' : '#27ae60', fontWeight: 600 }}>{saveStatus}</span>}
      </header>

      <div style={{ display: 'flex', flex: 1, overflow: 'hidden' }}>

        {/* Left sidebar — activity library */}
        <div style={{ width: '220px', background: 'white', borderRight: '1px solid #e0e6ed', padding: '12px', overflowY: 'auto', flexShrink: 0 }}>
          <div style={{ fontSize: '11px', fontWeight: 700, color: '#888', textTransform: 'uppercase', letterSpacing: '0.5px', marginBottom: '8px' }}>
            Activities
          </div>
          <div style={{ fontSize: '10px', color: '#aaa', marginBottom: '10px' }}>Drag onto canvas</div>

          {/* END / Finish node — always pinned, not in API */}
          <div
            draggable
            onDragStart={() => { dragActivity.current = 'END' }}
            style={{
              background: '#fff0f0', border: '1px solid #f9c0c0', borderRadius: '6px',
              padding: '8px 10px', cursor: 'grab', marginBottom: '10px',
              fontSize: '12px', fontFamily: 'monospace', color: '#c0392b',
              fontWeight: 700, userSelect: 'none',
            }}
          >
            ⏹ END (Finish)
          </div>

          {activities.length === 0 && (
            <div style={{ fontSize: '12px', color: '#aaa' }}>Loading activities…</div>
          )}

          {(() => {
            const groups: Record<string, ActivityInfo[]> = {}
            for (const a of activities) {
              const cat = a.meta?.category ?? 'Other'
              if (!groups[cat]) groups[cat] = []
              groups[cat].push(a)
            }
            const sorted = Object.entries(groups).sort(([a], [b]) => a.localeCompare(b))
            sorted.forEach(([, acts]) => acts.sort((a, b) => a.name.localeCompare(b.name)))
            return sorted.map(([cat, acts], i) => (
              <CategoryGroup
                key={cat}
                category={cat}
                activities={acts}
                onDragStart={name => { dragActivity.current = name }}
                open={openCategory === cat}
                onOpen={() => setOpenCategory(prev => prev === cat ? null : cat)}
              />
            ))
          })()}
        </div>

        {/* Canvas + debug panel column */}
        <div style={{ flex: 1, display: 'flex', flexDirection: 'column', overflow: 'hidden' }}>

        {/* Canvas */}
        <div style={{ flex: 1, overflow: 'auto', position: 'relative' }}>
          {graphNotFound && (
            <div style={{ background: '#fff3cd', border: '1px solid #ffc107', borderRadius: '6px', padding: '12px 16px', margin: '12px', fontSize: '13px', color: '#856404' }}>
              Graph "{graphNotFound}" was not found — it may have been created before the last server restart. The graph name has been pre-filled below. Rebuild it and click Save Graph.
            </div>
          )}
          {dragConnectLine && (
            <div style={{ position: 'absolute', top: '8px', left: '50%', transform: 'translateX(-50%)', background: '#667eea', color: 'white', padding: '4px 14px', borderRadius: '20px', fontSize: '12px', fontWeight: 600, zIndex: 10, pointerEvents: 'none' }}>
              {connectHover ? '⚡ Release to connect' : 'Drag to a node to connect…'}
            </div>
          )}
          <svg
            ref={svgRef}
            width={svgW} height={svgH}
            style={{ display: 'block', cursor: dragConnectLine ? 'crosshair' : draggingNode ? 'grabbing' : 'default' }}
            onDragOver={e => e.preventDefault()}
            onDrop={onCanvasDrop}
            onMouseMove={onSvgMouseMove}
            onMouseUp={onSvgMouseUp}
            onClick={onSvgClick}
          >
            <defs>
              <marker id="arrowB" markerWidth="8" markerHeight="8" refX="6" refY="3" orient="auto">
                <path d="M0,0 L0,6 L8,3 z" fill="#aaa" />
              </marker>
            </defs>

            {/* Grid */}
            <defs>
              <pattern id="grid" width="24" height="24" patternUnits="userSpaceOnUse">
                <path d="M 24 0 L 0 0 0 24" fill="none" stroke="#e8ecf0" strokeWidth="0.5" />
              </pattern>
            </defs>
            <rect width={svgW} height={svgH} fill="url(#grid)" />

            {/* Edges */}
            {edges.map(e => {
              const p = edgePath(e.fromNode, e.toNode)
              if (!p) return null
              const isSelected = e.id === selectedEdge
              return (
                <g key={e.id} style={{ cursor: 'pointer' }}
                  onClick={(ev) => { ev.stopPropagation(); setSelectedEdge(e.id); setSelectedNode(null) }}>
                  {/* Wider invisible hit area */}
                  <path d={`M${p.x1},${p.y1} C${p.mx},${p.y1} ${p.mx},${p.y2} ${p.x2},${p.y2}`}
                    fill="none" stroke="transparent" strokeWidth={12} />
                  <path d={`M${p.x1},${p.y1} C${p.mx},${p.y1} ${p.mx},${p.y2} ${p.x2},${p.y2}`}
                    fill="none" stroke={isSelected ? '#667eea' : '#aaa'} strokeWidth={isSelected ? 2 : 1.5}
                    markerEnd="url(#arrowB)" />
                  {(e.label || e.conditions.length > 0) && (
                    <text x={(p.x1 + p.x2) / 2} y={(p.y1 + p.y2) / 2 - 6}
                      textAnchor="middle" fontSize={10} fill={isSelected ? '#667eea' : '#888'}>
                      {e.label || e.conditions.map(c => `${c.key} ${c.operator} ${c.value}`).join(' & ')}
                    </text>
                  )}
                </g>
              )
            })}

            {/* Connect-hover highlight */}
            {connectHover && (() => {
              const hn = nodes.find(n => n.id === connectHover)
              return hn ? (
                <rect x={hn.x - 3} y={hn.y - 3} width={NODE_W + 6} height={NODE_H + 6}
                  rx={10} fill="none" stroke="#22c55e" strokeWidth={3}
                  strokeDasharray="6 3" opacity={0.9}>
                  <animate attributeName="stroke-dashoffset" from="0" to="-18" dur="0.4s" repeatCount="indefinite" />
                </rect>
              ) : null
            })()}

            {/* Live drag-connect line */}
            {dragConnectLine && (
              <line
                x1={dragConnectLine.x1} y1={dragConnectLine.y1}
                x2={dragConnectLine.x2} y2={dragConnectLine.y2}
                stroke={connectHover ? '#22c55e' : '#667eea'} strokeWidth={2}
                strokeDasharray="6 4" opacity={0.85} pointerEvents="none"
              />
            )}

            {/* Nodes */}
            {nodes.map(n => {
              const isEnd = n.activityName === 'END'
              const isMuxer = n.activityName === 'muxer'
              const isCondenser = n.activityName === 'condenser'
              const isPython = n.activityName === 'python_eval'
              const isLoop = n.activityName === 'loop'
              const isLoopNext = n.activityName === 'loop_next'
              const isNotes = n.activityName === 'notes'
              const isStart = n.id === startNode
              const isSelected = n.id === selectedNode
              const fill = isEnd ? '#fff0f0' : isMuxer ? '#eef6ff' : isCondenser ? '#efffef' : isPython ? '#f0f0ff' : isLoop ? '#f0fffe' : isLoopNext ? '#f5fff5' : isNotes ? '#fffbeb' : n.isHuman ? '#fffbf0' : isStart ? '#f0f4ff' : 'white'
              const borderColor = isSelected ? '#667eea' : isMuxer ? '#a0c8f0' : isCondenser ? '#90d0a0' : isPython ? '#3572A5' : isLoop ? '#1abc9c' : isLoopNext ? '#82c996' : isNotes ? '#d4a017' : isStart ? '#667eea' : isEnd ? '#f9c0c0' : n.isHuman ? '#f9e0a0' : '#ccc'

              return (
                <g key={n.id}
                  style={{ cursor: draggingNode === n.id ? 'grabbing' : 'grab' }}
                  onMouseDown={e => onNodeMouseDown(e, n.id)}
                  onClick={e => { e.stopPropagation(); setSelectedNode(n.id); setSelectedEdge(null) }}
                >
                  {isSelected && (
                    <rect x={n.x - 3} y={n.y - 3} width={NODE_W + 6} height={NODE_H + 6}
                      rx={9} fill="none" stroke="#667eea" strokeWidth={2} strokeDasharray="4 2" />
                  )}
                  {debugOpen && debugCurrentNodeId === n.id && (
                    <rect x={n.x - 4} y={n.y - 4} width={NODE_W + 8} height={NODE_H + 8}
                      rx={10} fill="none" stroke="#f39c12" strokeWidth={3} />
                  )}
                  {debugOpen && debugCompletedIds.has(n.id) && debugCurrentNodeId !== n.id && (
                    <rect x={n.x - 3} y={n.y - 3} width={NODE_W + 6} height={NODE_H + 6}
                      rx={9} fill="none" stroke="#27ae60" strokeWidth={2} />
                  )}
                  {debugOpen && debugFailedIds.has(n.id) && (
                    <rect x={n.x - 3} y={n.y - 3} width={NODE_W + 6} height={NODE_H + 6}
                      rx={9} fill="none" stroke="#e74c3c" strokeWidth={2} />
                  )}
                  <rect x={n.x} y={n.y} width={NODE_W} height={NODE_H} rx={6}
                    fill={fill} stroke={borderColor} strokeWidth={isSelected ? 2 : 1} />

                  {isEnd ? (
                    <>
                      <text x={n.x + NODE_W / 2} y={n.y + NODE_H / 2 + 5}
                        textAnchor="middle" fontSize={13} fontWeight={700} fill="#c0392b">
                        ⏹ END
                      </text>
                      {/* Input port only */}
                      <circle cx={n.x - PORT_R} cy={n.y + NODE_H / 2} r={PORT_R}
                        fill={'#e0e6ed'} stroke="#aaa" strokeWidth={1}
                        style={{ cursor: 'default' }} />
                    </>
                  ) : isMuxer ? (
                    <>
                      <text x={n.x + NODE_W / 2} y={n.y + 20}
                        textAnchor="middle" fontSize={12} fontWeight={700} fill="#1a6aa0">
                        ⇉ {n.label || n.id}
                      </text>
                      <text x={n.x + NODE_W / 2} y={n.y + 36}
                        textAnchor="middle" fontSize={10} fill="#4080a0" fontFamily="monospace">muxer</text>
                      <circle cx={n.x - PORT_R} cy={n.y + NODE_H / 2} r={PORT_R}
                        fill={'#e0e6ed'} stroke="#aaa" strokeWidth={1}
                        style={{ cursor: 'default' }} />
                      <circle cx={n.x + NODE_W + PORT_R} cy={n.y + NODE_H / 2} r={PORT_R}
                        fill={dragConnectFrom.current === n.id ? '#667eea' : '#e0e6ed'} stroke="#aaa" strokeWidth={1}
                        style={{ cursor: 'crosshair' }}
                        onMouseDown={e => onOutputPortMouseDown(e, n.id)} />
                    </>
                  ) : isCondenser ? (
                    <>
                      <text x={n.x + NODE_W / 2} y={n.y + 20}
                        textAnchor="middle" fontSize={12} fontWeight={700} fill="#1a7a40">
                        ⇇ {n.label || n.id}
                      </text>
                      <text x={n.x + NODE_W / 2} y={n.y + 36}
                        textAnchor="middle" fontSize={10} fill="#408050" fontFamily="monospace">condenser</text>
                      <circle cx={n.x - PORT_R} cy={n.y + NODE_H / 2} r={PORT_R}
                        fill={'#e0e6ed'} stroke="#aaa" strokeWidth={1}
                        style={{ cursor: 'default' }} />
                      <circle cx={n.x + NODE_W + PORT_R} cy={n.y + NODE_H / 2} r={PORT_R}
                        fill={dragConnectFrom.current === n.id ? '#667eea' : '#e0e6ed'} stroke="#aaa" strokeWidth={1}
                        style={{ cursor: 'crosshair' }}
                        onMouseDown={e => onOutputPortMouseDown(e, n.id)} />
                    </>
                  ) : isLoop ? (
                    <>
                      <text x={n.x + NODE_W / 2} y={n.y + 20}
                        textAnchor="middle" fontSize={12} fontWeight={700} fill="#0e8c72">
                        ↻ {n.label || n.id}
                      </text>
                      <text x={n.x + NODE_W / 2} y={n.y + 36}
                        textAnchor="middle" fontSize={10} fill="#1abc9c" fontFamily="monospace">loop</text>
                      <circle cx={n.x - PORT_R} cy={n.y + NODE_H / 2} r={PORT_R}
                        fill={'#e0e6ed'} stroke="#aaa" strokeWidth={1}
                        style={{ cursor: 'default' }} />
                      <circle cx={n.x + NODE_W + PORT_R} cy={n.y + NODE_H / 2} r={PORT_R}
                        fill={dragConnectFrom.current === n.id ? '#667eea' : '#e0e6ed'} stroke="#aaa" strokeWidth={1}
                        style={{ cursor: 'crosshair' }}
                        onMouseDown={e => onOutputPortMouseDown(e, n.id)} />
                    </>
                  ) : isLoopNext ? (
                    <>
                      <text x={n.x + NODE_W / 2} y={n.y + 20}
                        textAnchor="middle" fontSize={12} fontWeight={700} fill="#1a6a30">
                        ↺ {n.label || n.id}
                      </text>
                      <text x={n.x + NODE_W / 2} y={n.y + 36}
                        textAnchor="middle" fontSize={10} fill="#82c996" fontFamily="monospace">loop_next</text>
                      <circle cx={n.x - PORT_R} cy={n.y + NODE_H / 2} r={PORT_R}
                        fill={'#e0e6ed'} stroke="#aaa" strokeWidth={1}
                        style={{ cursor: 'default' }} />
                      <circle cx={n.x + NODE_W + PORT_R} cy={n.y + NODE_H / 2} r={PORT_R}
                        fill={dragConnectFrom.current === n.id ? '#667eea' : '#e0e6ed'} stroke="#aaa" strokeWidth={1}
                        style={{ cursor: 'crosshair' }}
                        onMouseDown={e => onOutputPortMouseDown(e, n.id)} />
                    </>
                  ) : isNotes ? (
                    <>
                      <text x={n.x + NODE_W / 2} y={n.y + 18}
                        textAnchor="middle" fontSize={11} fontWeight={700} fill="#92610a">
                        📝 {n.label || 'note'}
                      </text>
                      <text x={n.x + NODE_W / 2} y={n.y + 34}
                        textAnchor="middle" fontSize={9} fill="#b07e20" fontFamily="sans-serif"
                        style={{ fontStyle: 'italic' }}>
                        {((n.staticInput ?? {}).note ?? '').slice(0, 22) || '(no text)'}
                      </text>
                      <circle cx={n.x - PORT_R} cy={n.y + NODE_H / 2} r={PORT_R}
                        fill={'#e0e6ed'} stroke="#aaa" strokeWidth={1}
                        style={{ cursor: 'default' }} />
                      <circle cx={n.x + NODE_W + PORT_R} cy={n.y + NODE_H / 2} r={PORT_R}
                        fill={dragConnectFrom.current === n.id ? '#667eea' : '#e0e6ed'} stroke="#aaa" strokeWidth={1}
                        style={{ cursor: 'crosshair' }}
                        onMouseDown={e => onOutputPortMouseDown(e, n.id)} />
                    </>
                  ) : isPython ? (
                    <>
                      <text x={n.x + 10} y={n.y + 18} fontSize={12} fill="#3572A5">🐍</text>
                      <text x={n.x + NODE_W / 2} y={n.y + (n.label ? 18 : 22)}
                        textAnchor="middle" fontSize={11} fontWeight={600} fill="#3572A5">
                        🐍 {n.label || n.id}
                      </text>
                      <text x={n.x + NODE_W / 2} y={n.y + 36}
                        textAnchor="middle" fontSize={10} fill="#3572A5" fontFamily="monospace">python_eval</text>
                      {isStart && (
                        <text x={n.x + NODE_W / 2} y={n.y + NODE_H - 4}
                          textAnchor="middle" fontSize={9} fill="#27ae60">START</text>
                      )}
                      {n.needsConversion && (
                        <text x={n.x + NODE_W - 6} y={n.y + 14} textAnchor="end" fontSize={11}>🔄</text>
                      )}
                      <circle cx={n.x - PORT_R} cy={n.y + NODE_H / 2} r={PORT_R}
                        fill={'#e0e6ed'} stroke="#aaa" strokeWidth={1}
                        style={{ cursor: 'default' }} />
                      <circle cx={n.x + NODE_W + PORT_R} cy={n.y + NODE_H / 2} r={PORT_R}
                        fill={dragConnectFrom.current === n.id ? '#667eea' : '#e0e6ed'} stroke="#aaa" strokeWidth={1}
                        style={{ cursor: 'crosshair' }}
                        onMouseDown={e => onOutputPortMouseDown(e, n.id)} />
                    </>
                  ) : (
                    <>
                      {n.isHuman && (
                        <text x={n.x + 10} y={n.y + 18} fontSize={12} fill="#555">👤</text>
                      )}
                      <text x={n.x + NODE_W / 2} y={n.y + (n.label ? 18 : 22)}
                        textAnchor="middle" fontSize={11} fontWeight={600} fill="#2c3e50">
                        {n.label || n.id}
                      </text>
                      <text x={n.x + NODE_W / 2} y={n.y + 36}
                        textAnchor="middle" fontSize={10} fill="#667eea" fontFamily="monospace">
                        {n.activityName}
                      </text>
                      {isStart && (
                        <text x={n.x + NODE_W / 2} y={n.y + NODE_H - 4}
                          textAnchor="middle" fontSize={9} fill="#27ae60">START</text>
                      )}

                      {/* Input port (left) */}
                      <circle cx={n.x - PORT_R} cy={n.y + NODE_H / 2} r={PORT_R}
                        fill={'#e0e6ed'} stroke="#aaa" strokeWidth={1}
                        style={{ cursor: 'default' }} />
                      {/* Output port (right) */}
                      <circle cx={n.x + NODE_W + PORT_R} cy={n.y + NODE_H / 2} r={PORT_R}
                        fill={dragConnectFrom.current === n.id ? '#667eea' : '#e0e6ed'} stroke="#aaa" strokeWidth={1}
                        style={{ cursor: 'crosshair' }}
                        onMouseDown={e => onOutputPortMouseDown(e, n.id)} />
                    </>
                  )}
                </g>
              )
            })}
          </svg>
        </div>

        {/* Debug panel (inside canvas+debug column, below canvas) */}
        {debugOpen && (
          <div style={{ height: `${debugHeight}px`, borderTop: '2px solid #7c5cbf', background: '#1a1a2e', display: 'flex', flexDirection: 'column', flexShrink: 0 }}>
            {/* Resize handle */}
            <div
              onMouseDown={onDebugResizeStart}
              style={{ height: '6px', cursor: 'ns-resize', background: '#2d2d4e', flexShrink: 0, display: 'flex', alignItems: 'center', justifyContent: 'center' }}
            >
              <div style={{ width: '40px', height: '2px', background: '#5a4a8a', borderRadius: '2px' }} />
            </div>
            {/* Two-column body */}
            <div style={{ display: 'flex', flex: 1, overflow: 'hidden' }}>
              {/* Left: context editor */}
              <div style={{ width: '45%', display: 'flex', flexDirection: 'column', borderRight: '1px solid #2d2d4e', padding: '8px' }}>
                <div style={{ fontSize: '10px', fontWeight: 700, color: '#9b8fd4', textTransform: 'uppercase', letterSpacing: '0.5px', marginBottom: '4px' }}>
                  Context (editable)
                </div>
                <textarea
                  key={debugResetKey}
                  value={debugContext}
                  onChange={e => setDebugContext(e.target.value)}
                  spellCheck={false}
                  style={{
                    flex: 1, width: '100%', boxSizing: 'border-box', fontFamily: 'monospace', fontSize: '11px',
                    padding: '6px', background: '#0d0d1a', color: '#c9d1d9', border: '1px solid #2d2d4e',
                    borderRadius: '4px', resize: 'none', outline: 'none', lineHeight: 1.5,
                  }}
                />
                <div style={{ fontSize: '10px', color: '#6b7280', marginTop: '4px', fontFamily: 'monospace' }}>
                  {debugNodes[debugNodeIdx]
                    ? `Node ${debugNodeIdx + 1}/${debugNodes.length}: ${debugNodes[debugNodeIdx].id} → ${debugNodes[debugNodeIdx].activityName}`
                    : debugNodes.length > 0
                      ? `Done — ${debugNodes.length} node${debugNodes.length !== 1 ? 's' : ''} completed`
                      : 'No nodes in walk (set a start node)'}
                </div>
              </div>

              {/* Right: step output */}
              <div style={{ flex: 1, display: 'flex', flexDirection: 'column', padding: '8px' }}>
                <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '4px' }}>
                  <div style={{ display: 'flex', alignItems: 'center', gap: '6px' }}>
                    <div style={{ fontSize: '10px', fontWeight: 700, color: '#9b8fd4', textTransform: 'uppercase', letterSpacing: '0.5px' }}>
                      Context After Step
                    </div>
                    {(() => {
                      const curlVal = lastDebugResult && !lastDebugResult.error && lastDebugResult.output
                        ? (lastDebugResult.output as Record<string, unknown>)['curl']
                        : undefined
                      return typeof curlVal === 'string' ? (
                        <button
                          onClick={() => setDebugCurlModal(curlVal)}
                          style={{ background: '#313244', color: '#89b4fa', border: 'none', borderRadius: '8px', padding: '1px 8px', fontSize: '10px', fontFamily: 'monospace', fontWeight: 700, cursor: 'pointer' }}
                        >
                          curl
                        </button>
                      ) : null
                    })()}
                  </div>
                  {lastDebugResult && !lastDebugResult.error && (
                    <button
                      onClick={() => {
                        const out = lastDebugResult.output as Record<string, unknown> | undefined
                        const filtered = out ? Object.fromEntries(Object.entries(out).filter(([k]) => k !== 'curl')) : out
                        navigator.clipboard.writeText(JSON.stringify(filtered, null, 2))
                        setDebugCopied(true)
                        setTimeout(() => setDebugCopied(false), 1500)
                      }}
                      style={{ fontSize: '10px', padding: '2px 8px', background: debugCopied ? '#166534' : '#2d2d4e', border: `1px solid ${debugCopied ? '#4ade80' : '#4a4a6a'}`, borderRadius: '4px', color: debugCopied ? '#4ade80' : '#9b8fd4', cursor: 'pointer', transition: 'all 0.2s' }}
                    >
                      {debugCopied ? '✓ Copied' : 'Copy JSON'}
                    </button>
                  )}
                </div>
                <div style={{ flex: 1, overflowY: 'auto', fontFamily: 'monospace', fontSize: '11px', lineHeight: 1.5 }}>
                  {lastDebugResult ? (
                    lastDebugResult.skipped ? (
                      <div style={{ color: '#fbbf24', whiteSpace: 'pre-wrap' }}>
                        Skipped: {lastDebugResult.activityName} (node returned ErrSkip — context unchanged)
                      </div>
                    ) : lastDebugResult.error ? (
                      <div style={{ color: '#f87171', whiteSpace: 'pre-wrap' }}>
                        Error in {lastDebugResult.activityName}:{'\n'}{lastDebugResult.error}
                      </div>
                    ) : (
                      <>
                        <div style={{ color: '#86efac', whiteSpace: 'pre-wrap' }}>
                          {JSON.stringify(
                            lastDebugResult.output && typeof lastDebugResult.output === 'object'
                              ? Object.fromEntries(Object.entries(lastDebugResult.output as Record<string, unknown>).filter(([k]) => k !== 'curl'))
                              : lastDebugResult.output,
                            null, 2
                          )}
                        </div>
                        {(() => {
                          const ctx = (lastDebugResult.output && typeof lastDebugResult.output === 'object')
                            ? lastDebugResult.output as Record<string, unknown>
                            : {}
                          const outEdges = edges.filter(e => e.fromNode === lastDebugResult.nodeId)
                          if (outEdges.length === 0) return null
                          return (
                            <div style={{ marginTop: '10px', borderTop: '1px solid #2d2d4e', paddingTop: '8px' }}>
                              <div style={{ fontSize: '10px', fontWeight: 700, color: '#9b8fd4', textTransform: 'uppercase', letterSpacing: '0.5px', marginBottom: '6px' }}>
                                Transition Evaluation
                              </div>
                              {outEdges.map(edge => {
                                const toNode = nodes.find(n => n.id === edge.toNode)
                                const label = toNode?.label || toNode?.activityName || edge.toNode || 'END'
                                if (edge.conditions.length === 0) {
                                  return (
                                    <div key={edge.id} style={{ marginBottom: '4px', color: '#94a3b8', fontSize: '11px' }}>
                                      <span style={{ color: '#4ade80', fontWeight: 700 }}>✓</span> → <span style={{ color: '#e2e8f0' }}>{label}</span> <span style={{ color: '#64748b' }}>(unconditional)</span>
                                    </div>
                                  )
                                }
                                return (
                                  <div key={edge.id} style={{ marginBottom: '8px' }}>
                                    <div style={{ color: '#94a3b8', fontSize: '10px', marginBottom: '2px' }}>→ <span style={{ color: '#e2e8f0' }}>{label}</span></div>
                                    {edge.conditions.map((c, i) => {
                                      let key = c.key.trim()
                                      if (key.startsWith('{{') && key.endsWith('}}')) key = key.slice(2, -2).trim()
                                      const resolved = resolvePath(key, ctx)
                                      const result = evalDebugCondition(c, ctx)
                                      return (
                                        <div key={i} style={{ fontSize: '11px', fontFamily: 'monospace', paddingLeft: '8px', marginBottom: '2px' }}>
                                          <span style={{ color: result ? '#4ade80' : '#f87171', fontWeight: 700 }}>{result ? '✓' : '✗'}</span>
                                          {' '}
                                          <span style={{ color: '#fbbf24' }}>{c.key}</span>
                                          {' = '}
                                          <span style={{ color: '#7dd3fc' }}>{JSON.stringify(resolved)}</span>
                                          <span style={{ color: '#64748b' }}> ({typeof resolved})</span>
                                          {' '}
                                          <span style={{ color: '#c084fc' }}>{c.operator}</span>
                                          {' '}
                                          <span style={{ color: '#86efac' }}>{JSON.stringify(c.value)}</span>
                                          <span style={{ color: '#64748b' }}> ({typeof c.value})</span>
                                        </div>
                                      )
                                    })}
                                  </div>
                                )
                              })}
                            </div>
                          )
                        })()}
                      </>
                    )
                  ) : (
                    <div style={{ color: '#4b5563' }}>No steps run yet. Click "Step" to execute the current node.</div>
                  )}
                </div>
              </div>
            </div>

            {/* Bottom button bar */}
            <div style={{ display: 'flex', alignItems: 'center', gap: '8px', padding: '6px 8px', borderTop: '1px solid #2d2d4e', flexShrink: 0 }}>
              {debugNodeIdx >= debugNodes.length && debugNodes.length > 0 ? (
                <span style={{ fontSize: '12px', color: '#4ade80', fontWeight: 600 }}>Done</span>
              ) : (
                <>
                  <button
                    onClick={() => debugStep()}
                    disabled={debugRunning || debugNodeIdx >= debugNodes.length}
                    style={{
                      ...iconBtn('#7c5cbf'),
                      opacity: (debugRunning || debugNodeIdx >= debugNodes.length) ? 0.5 : 1,
                      fontSize: '12px', padding: '4px 10px',
                    }}
                  >
                    ▶ Step{debugNodes[debugNodeIdx] ? `: ${debugNodes[debugNodeIdx].activityName}` : ''}
                  </button>
                  <button
                    onClick={debugRunAll}
                    disabled={debugRunning || debugNodeIdx >= debugNodes.length}
                    style={{
                      ...iconBtn('#5a3ea0'),
                      opacity: (debugRunning || debugNodeIdx >= debugNodes.length) ? 0.5 : 1,
                      fontSize: '12px', padding: '4px 10px',
                    }}
                  >
                    Run All
                  </button>
                </>
              )}
              <button
                onClick={() => {
                  if (debugNodeIdx > 0) {
                    setDebugNodeIdx(debugNodeIdx - 1)
                    setDebugResults(prev => prev.slice(0, -1))
                  }
                }}
                disabled={debugRunning || debugNodeIdx === 0}
                style={{ ...iconBtn('#374151'), opacity: (debugRunning || debugNodeIdx === 0) ? 0.5 : 1, fontSize: '12px', padding: '4px 8px' }}
              >
                ← Back
              </button>
              <select
                value={debugNodeIdx}
                onChange={e => {
                  const idx = Number(e.target.value)
                  setDebugNodeIdx(idx)
                  setDebugResults(prev => prev.slice(0, idx))
                }}
                disabled={debugRunning}
                style={{ fontSize: '11px', padding: '3px 6px', background: '#2d2d4e', color: '#9b8fd4', border: '1px solid #4a4a6a', borderRadius: '4px', cursor: 'pointer' }}
              >
                {debugNodes.map((n, i) => (
                  <option key={n.id} value={i}>{i + 1}. {n.activityName}</option>
                ))}
                {debugNodes.length > 0 && (
                  <option value={debugNodes.length}>Done</option>
                )}
              </select>
              <button
                onClick={() => { setDebugNodeIdx(0); setDebugResults([]); setDebugResetKey(k => k + 1); const first = nodes.find(n => n.id === startNode); setDebugNodes(first ? [first] : []) }}
                disabled={debugRunning}
                style={{ ...iconBtn('#374151'), opacity: debugRunning ? 0.5 : 1, fontSize: '12px', padding: '4px 10px' }}
              >
                Reset
              </button>
              {debugRunning && (
                <span style={{ fontSize: '11px', color: '#9b8fd4', fontFamily: 'monospace' }}>running…</span>
              )}
              <div style={{ flex: 1 }} />
              <span style={{ fontSize: '10px', color: '#4b5563', fontFamily: 'monospace' }}>
                {debugResults.length}/{debugNodes.length} steps
              </span>
            </div>
          </div>
        )}

        </div>{/* end canvas+debug column */}

        {/* Right panel — properties */}
        <div style={{ width: '240px', background: 'white', borderLeft: '1px solid #e0e6ed', padding: '14px', overflowY: 'auto', flexShrink: 0 }}>
          {selNode ? (
            <NodeEditor
              node={selNode}
              isStart={selNode.id === startNode}
              activities={activities}
              onSetStart={() => setStartNode(selNode.id)}
              onChange={updated => setNodes(prev => prev.map(n => n.id === updated.id ? updated : n))}
              onDelete={() => {
                setNodes(prev => prev.filter(n => n.id !== selNode.id))
                setEdges(prev => prev.filter(e => e.fromNode !== selNode.id && e.toNode !== selNode.id))
                if (startNode === selNode.id) setStartNode(null)
                setSelectedNode(null)
              }}
              onTestFromHere={nodeId => setTestModal(nodeId)}
              onInsertMapperNode={insertMapperNode}
              debugContext={enrichedDebugContext}
            />
          ) : selEdge ? (
            <EdgeEditor
              edge={selEdge}
              onChange={updated => setEdges(prev => prev.map(e => e.id === updated.id ? updated : e))}
              onDelete={() => { setEdges(prev => prev.filter(e => e.id !== selEdge.id)); setSelectedEdge(null) }}
            />
          ) : (
            <div>
              <div style={{ fontWeight: 700, fontSize: '13px', color: '#555', marginBottom: '12px' }}>Canvas</div>
              <div style={{ fontSize: '12px', color: '#aaa', lineHeight: 1.7 }}>
                <div>• Drag activities from the left panel onto the canvas</div>
                <div>• Drag from the right ● port of a node to connect to another node</div>
                <div>• Click the left ● port of another node to complete the edge</div>
                <div>• Click a node or edge to edit its properties</div>
                <div>• Drag a node to reposition it</div>
              </div>
              <div style={{ marginTop: '16px', fontSize: '12px', color: '#888' }}>
                {nodes.length} nodes · {edges.length} edges
              </div>
            </div>
          )}
        </div>
      </div>

      {debugCurlModal !== null && (
        <div onClick={() => setDebugCurlModal(null)} style={{ position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.6)', display: 'flex', alignItems: 'center', justifyContent: 'center', zIndex: 2000 }}>
          <div onClick={e => e.stopPropagation()} style={{ background: '#1e1e2e', borderRadius: '10px', padding: '20px', width: 'min(720px, 90vw)', boxShadow: '0 24px 80px rgba(0,0,0,0.5)', display: 'flex', flexDirection: 'column', gap: '12px' }}>
            <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
              <span style={{ fontFamily: 'monospace', fontSize: '12px', fontWeight: 700, color: '#89b4fa', textTransform: 'uppercase', letterSpacing: '0.5px' }}>curl command</span>
              <div style={{ display: 'flex', gap: '8px' }}>
                <button
                  onClick={() => { navigator.clipboard.writeText(debugCurlModal) }}
                  style={{ background: '#313244', color: '#cdd6f4', border: 'none', borderRadius: '6px', padding: '5px 14px', fontSize: '12px', fontWeight: 600, cursor: 'pointer' }}
                >Copy</button>
                <button onClick={() => setDebugCurlModal(null)} style={{ background: '#313244', color: '#cdd6f4', border: 'none', borderRadius: '6px', padding: '5px 10px', fontSize: '14px', cursor: 'pointer' }}>✕</button>
              </div>
            </div>
            <pre style={{ background: '#181825', color: '#cdd6f4', padding: '14px 16px', borderRadius: '6px', fontSize: '12px', fontFamily: 'monospace', overflowX: 'auto', margin: 0, lineHeight: 1.7, whiteSpace: 'pre-wrap', wordBreak: 'break-all' }}>
              {debugCurlModal}
            </pre>
          </div>
        </div>
      )}

      {testModal && (
        <TestModal
          nodeId={testModal}
          graphName={graphName}
          onClose={() => setTestModal(null)}
          onRun={runTestFromNode}
        />
      )}
      </div>
    </div>
  )
}

// ── Style helpers ─────────────────────────────────────────────────────────────

const labelStyle: React.CSSProperties = {
  fontSize: '11px', fontWeight: 600, color: '#666', display: 'block',
  marginBottom: '3px', marginTop: '8px',
}
const inputStyle: React.CSSProperties = {
  width: '100%', padding: '5px 7px', border: '1px solid #e0e6ed', borderRadius: '4px',
  fontSize: '12px', boxSizing: 'border-box', marginBottom: '2px',
}
function iconBtn(bg: string): React.CSSProperties {
  return { background: bg, color: 'white', border: 'none', padding: '4px 8px', borderRadius: '4px', cursor: 'pointer', fontSize: '12px' }
}
function hBtn(bg: string): React.CSSProperties {
  return { background: bg, color: 'white', border: 'none', padding: '6px 12px', borderRadius: '4px', cursor: 'pointer', fontSize: '13px', fontWeight: 500 }
}
