import { useState, useEffect } from 'react'
import { useLocation, useNavigate } from 'react-router-dom'

const STORAGE_KEY = 'cbw_nav_collapsed'

// ── SVG Icons ─────────────────────────────────────────────────────────────────

function IconWorkflows() {
  return (
    <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <rect x="3" y="3" width="7" height="7" rx="1" />
      <rect x="14" y="3" width="7" height="7" rx="1" />
      <rect x="3" y="14" width="7" height="7" rx="1" />
      <rect x="14" y="14" width="7" height="7" rx="1" />
    </svg>
  )
}

function IconTriggers() {
  return (
    <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <path d="M13 2L3 14h9l-1 8 10-12h-9l1-8z" />
    </svg>
  )
}

function IconTelegram() {
  return (
    <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <line x1="22" y1="2" x2="11" y2="13" />
      <polygon points="22 2 15 22 11 13 2 9 22 2" />
    </svg>
  )
}

function IconSettings() {
  return (
    <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <circle cx="12" cy="12" r="3" />
      <path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83-2.83l.06-.06A1.65 1.65 0 0 0 4.68 15a1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 2.83-2.83l.06.06A1.65 1.65 0 0 0 9 4.68a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 2.83l-.06.06A1.65 1.65 0 0 0 19.4 9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z" />
    </svg>
  )
}

function IconHistory() {
  return (
    <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <polyline points="1 4 1 10 7 10" />
      <path d="M3.51 15a9 9 0 1 0 .49-4.5" />
    </svg>
  )
}

function IconLogs() {
  return (
    <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <polyline points="4 17 10 11 4 5" />
      <line x1="12" y1="19" x2="20" y2="19" />
    </svg>
  )
}

function IconAdmin() {
  return (
    <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z" />
    </svg>
  )
}

function IconLogout() {
  return (
    <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4" />
      <polyline points="16 17 21 12 16 7" />
      <line x1="21" y1="12" x2="9" y2="12" />
    </svg>
  )
}

function IconChevron({ collapsed }: { collapsed: boolean }) {
  return (
    <svg
      width="16"
      height="16"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
      style={{ transform: collapsed ? 'rotate(180deg)' : 'rotate(0deg)', transition: 'transform 200ms ease' }}
    >
      <polyline points="15 18 9 12 15 6" />
    </svg>
  )
}

// ── Nav items config ──────────────────────────────────────────────────────────

interface NavItem {
  label: string
  path: string
  icon: React.ReactNode
  /** Use a plain <a href> instead of navigate() */
  external?: boolean
  /** Exact match for active check */
  exact?: boolean
}

const NAV_ITEMS: NavItem[] = [
  { label: 'Workflows',  path: '/workflows',         icon: <IconWorkflows />,  exact: true  },
  { label: 'History',    path: '/history',           icon: <IconHistory />,    exact: true  },
  { label: 'Triggers',   path: '/triggers',          icon: <IconTriggers />,   exact: true  },
  { label: 'Telegram',   path: '/telegram/messages', icon: <IconTelegram />,   exact: true  },
  { label: 'Settings',   path: '/settings',          icon: <IconSettings />,   exact: true  },
  { label: 'Logs',       path: '/logs',              icon: <IconLogs />,       external: true },
  { label: 'Admin',      path: '/_/',                icon: <IconAdmin />,      external: true },
  { label: 'Logout',     path: '/logout',            icon: <IconLogout />,     external: true },
]

// ── Styles ────────────────────────────────────────────────────────────────────

const SIDEBAR_BG = '#16213e'
const SIDEBAR_BORDER = '#1a2a50'
const ACTIVE_BG = '#667eea'
const HOVER_BG = '#1e2f5c'
const TEXT_COLOR = '#c8d0e8'
const TEXT_MUTED = '#6b7fa3'
const ACCENT = '#667eea'

// ── NavItemRow ────────────────────────────────────────────────────────────────

function NavItemRow({
  item,
  collapsed,
  active,
}: {
  item: NavItem
  collapsed: boolean
  active: boolean
}) {
  const navigate = useNavigate()
  const [hover, setHover] = useState(false)

  const bg = active ? ACTIVE_BG : hover ? HOVER_BG : 'transparent'
  const color = active ? '#fff' : TEXT_COLOR

  const content = (
    <>
      <span style={{
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        flexShrink: 0,
        width: '20px',
        color: active ? '#fff' : hover ? '#a0b0e0' : TEXT_COLOR,
        transition: 'color 150ms',
      }}>
        {item.icon}
      </span>
      {!collapsed && (
        <span style={{
          fontSize: '13px',
          fontWeight: active ? 600 : 500,
          letterSpacing: '0.01em',
          whiteSpace: 'nowrap',
          overflow: 'hidden',
          opacity: collapsed ? 0 : 1,
          transition: 'opacity 150ms',
        }}>
          {item.label}
        </span>
      )}
    </>
  )

  const rowStyle: React.CSSProperties = {
    display: 'flex',
    alignItems: 'center',
    gap: '12px',
    padding: collapsed ? '10px 0' : '10px 14px',
    justifyContent: collapsed ? 'center' : 'flex-start',
    borderRadius: '6px',
    cursor: 'pointer',
    background: bg,
    color,
    textDecoration: 'none',
    transition: 'background 150ms',
    position: 'relative',
    margin: '1px 8px',
  }

  if (item.external) {
    return (
      <a
        href={item.path}
        style={rowStyle}
        title={collapsed ? item.label : undefined}
        onMouseEnter={() => setHover(true)}
        onMouseLeave={() => setHover(false)}
      >
        {content}
        {collapsed && hover && (
          <Tooltip label={item.label} />
        )}
      </a>
    )
  }

  return (
    <div
      role="button"
      tabIndex={0}
      onClick={() => navigate(item.path)}
      onKeyDown={e => { if (e.key === 'Enter' || e.key === ' ') navigate(item.path) }}
      style={rowStyle}
      title={collapsed ? item.label : undefined}
      onMouseEnter={() => setHover(true)}
      onMouseLeave={() => setHover(false)}
    >
      {content}
      {collapsed && hover && (
        <Tooltip label={item.label} />
      )}
    </div>
  )
}

function Tooltip({ label }: { label: string }) {
  return (
    <div style={{
      position: 'absolute',
      left: 'calc(100% + 10px)',
      top: '50%',
      transform: 'translateY(-50%)',
      background: '#0d1528',
      color: '#e2e8f0',
      fontSize: '12px',
      fontWeight: 500,
      padding: '5px 10px',
      borderRadius: '5px',
      whiteSpace: 'nowrap',
      pointerEvents: 'none',
      zIndex: 9999,
      boxShadow: '0 2px 8px rgba(0,0,0,0.4)',
    }}>
      {label}
    </div>
  )
}

// ── Nav ───────────────────────────────────────────────────────────────────────

export default function Nav() {
  const location = useLocation()
  const [collapsed, setCollapsed] = useState<boolean>(() => {
    try {
      return localStorage.getItem(STORAGE_KEY) === 'true'
    } catch {
      return false
    }
  })

  const [toggleHover, setToggleHover] = useState(false)

  useEffect(() => {
    try {
      localStorage.setItem(STORAGE_KEY, String(collapsed))
    } catch { /* ignore */ }
  }, [collapsed])

  const width = collapsed ? 52 : 200

  function isActive(item: NavItem): boolean {
    if (item.external) return false
    if (item.exact) return location.pathname === item.path
    return location.pathname.startsWith(item.path)
  }

  return (
    <div style={{
      width,
      minWidth: width,
      height: '100vh',
      background: SIDEBAR_BG,
      borderRight: `1px solid ${SIDEBAR_BORDER}`,
      display: 'flex',
      flexDirection: 'column',
      transition: 'width 200ms ease, min-width 200ms ease',
      overflow: 'hidden',
      flexShrink: 0,
      position: 'relative',
      zIndex: 100,
    }}>

      {/* Logo / brand */}
      <div style={{
        padding: collapsed ? '18px 0' : '18px 16px',
        display: 'flex',
        alignItems: 'center',
        justifyContent: collapsed ? 'center' : 'flex-start',
        gap: '10px',
        borderBottom: `1px solid ${SIDEBAR_BORDER}`,
        flexShrink: 0,
      }}>
        {/* CBW monogram mark */}
        <div style={{
          width: '28px',
          height: '28px',
          background: `linear-gradient(135deg, ${ACCENT} 0%, #9b59b6 100%)`,
          borderRadius: '6px',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          flexShrink: 0,
          fontSize: '11px',
          fontWeight: 800,
          color: '#fff',
          letterSpacing: '-0.5px',
        }}>
          CB
        </div>
        {!collapsed && (
          <span style={{
            fontSize: '14px',
            fontWeight: 700,
            color: '#e2e8f0',
            letterSpacing: '0.02em',
            whiteSpace: 'nowrap',
          }}>
            CBW
          </span>
        )}
      </div>

      {/* Nav items */}
      <nav style={{ flex: 1, overflowY: 'auto', overflowX: 'hidden', paddingTop: '8px', paddingBottom: '8px' }}>
        {NAV_ITEMS.map(item => (
          <NavItemRow
            key={item.path}
            item={item}
            collapsed={collapsed}
            active={isActive(item)}
          />
        ))}
      </nav>

      {/* Collapse toggle */}
      <div style={{ borderTop: `1px solid ${SIDEBAR_BORDER}`, padding: '8px', flexShrink: 0 }}>
        <button
          onClick={() => setCollapsed(c => !c)}
          onMouseEnter={() => setToggleHover(true)}
          onMouseLeave={() => setToggleHover(false)}
          title={collapsed ? 'Expand sidebar' : 'Collapse sidebar'}
          style={{
            width: '100%',
            display: 'flex',
            alignItems: 'center',
            justifyContent: collapsed ? 'center' : 'flex-start',
            gap: '10px',
            padding: collapsed ? '8px 0' : '8px 10px',
            background: toggleHover ? HOVER_BG : 'transparent',
            border: 'none',
            borderRadius: '6px',
            cursor: 'pointer',
            color: TEXT_MUTED,
            transition: 'background 150ms',
          }}
        >
          <IconChevron collapsed={collapsed} />
          {!collapsed && (
            <span style={{ fontSize: '12px', color: TEXT_MUTED, whiteSpace: 'nowrap' }}>
              Collapse
            </span>
          )}
        </button>
      </div>
    </div>
  )
}
