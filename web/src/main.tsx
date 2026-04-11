import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom'
import LogViewer from './LogViewer'
import WorkflowList from './WorkflowList'
import WorkflowHistory from './WorkflowHistory'
import WorkflowDetail from './WorkflowDetail'
import WorkflowBuilder from './WorkflowBuilder'
import Triggers from './Triggers'
import Settings from './Settings'
import TelegramMessages from './TelegramMessages'

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <BrowserRouter>
      <Routes>
        <Route path="/logs" element={<LogViewer />} />
        <Route path="/workflows/builder" element={<WorkflowBuilder />} />
        <Route path="/workflows/:id" element={<WorkflowDetail />} />
        <Route path="/workflows" element={<WorkflowList />} />
        <Route path="/history" element={<WorkflowHistory />} />
        <Route path="/triggers" element={<Triggers />} />
        <Route path="/triggers/*" element={<Triggers />} />
        <Route path="/settings" element={<Settings />} />
        <Route path="/telegram/messages" element={<TelegramMessages />} />
        <Route path="/" element={<Navigate to="/workflows" replace />} />
        <Route path="*" element={<Navigate to="/workflows" replace />} />
      </Routes>
    </BrowserRouter>
  </StrictMode>,
)
