import type { Provider, Session, Message, Trigger } from '../types'

const BASE = '/api/v1'

async function request<T>(url: string, options?: RequestInit): Promise<T> {
  const res = await fetch(BASE + url, {
    headers: { 'Content-Type': 'application/json' },
    ...options,
  })
  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: res.statusText }))
    throw new Error(err.error || res.statusText)
  }
  return res.json()
}

// Providers
export const listProviders = () => request<Provider[]>('/providers')
export const createProvider = (p: Partial<Provider>) =>
  request<Provider>('/providers', { method: 'POST', body: JSON.stringify(p) })
export const updateProvider = (id: string, p: Partial<Provider>) =>
  request<Provider>(`/providers/${id}`, { method: 'PUT', body: JSON.stringify(p) })
export const deleteProvider = (id: string) =>
  request<{ ok: boolean }>(`/providers/${id}`, { method: 'DELETE' })

// Sessions
export const listSessions = () => request<Session[]>('/sessions')
export const getSession = (id: number) => request<Session>(`/sessions/${id}`)
export const updateSession = (id: number, s: Partial<Session>) =>
  request<Session>(`/sessions/${id}`, { method: 'PUT', body: JSON.stringify(s) })
export const deleteSession = (id: number) =>
  request<{ ok: boolean }>(`/sessions/${id}`, { method: 'DELETE' })
export const getMessages = (sessionId: number) =>
  request<Message[]>(`/sessions/${sessionId}/messages`)

// Compress session context
export const compressSession = (id: number) =>
  request<{ ok: boolean }>(`/sessions/${id}/compress`, { method: 'POST' })

// Restart session (kill CLI process, refresh rules)
export const restartSession = (id: number) =>
  request<{ ok: boolean }>(`/sessions/${id}/restart`, { method: 'POST' })

// Chat
export const sendChat = (sessionId: number, content: string, workDir?: string) =>
  request<{ session_id: number; status: string }>('/chat/send', {
    method: 'POST',
    body: JSON.stringify({ session_id: sessionId, content, work_dir: workDir || '' }),
  })

// Status
export interface DepsStatus {
  node_installed: boolean
  node_version: string
  npm_installed: boolean
  npm_version: string
  claude_installed: boolean
  claude_version: string
  installing: boolean
  install_error: string
  install_hint: string
}
export const getStatus = () => request<DepsStatus>('/status')
export const getVersion = () => request<{ version: string }>('/version')
export const retryInstall = () =>
  request<{ ok: boolean }>('/status/retry-install', { method: 'POST' })

// Files (manage page)
export interface FileItem {
  name: string
  path: string
  exists: boolean
}
export const listFiles = (scope: string) =>
  request<FileItem[]>(`/files?scope=${scope}`)
export const readFileContent = (scope: string, path: string) =>
  request<{ content: string }>(`/files/content?scope=${encodeURIComponent(scope)}&path=${encodeURIComponent(path)}`)
export const writeFileContent = (scope: string, path: string, content: string) =>
  request<{ ok: boolean }>('/files/content', {
    method: 'PUT',
    body: JSON.stringify({ scope, path, content }),
  })
export const createFileApi = (scope: string, path: string, content: string) =>
  request<{ ok: boolean }>('/files', {
    method: 'POST',
    body: JSON.stringify({ scope, path, content }),
  })
export const deleteFileApi = (scope: string, path: string) =>
  request<{ ok: boolean }>(`/files?scope=${encodeURIComponent(scope)}&path=${encodeURIComponent(path)}`, {
    method: 'DELETE',
  })
export interface TemplateVar {
  name: string
  desc: string
  value: string
}
export const getTemplateVars = () => request<TemplateVar[]>('/files/variables')
export const getDefaultFile = (path: string) =>
  request<{ content: string }>(`/files/default?path=${encodeURIComponent(path)}`)

// Skills
export interface SkillItem {
  name: string
  description: string
  source: string
  path: string
  enabled: boolean
}
export const listSkills = () => request<SkillItem[]>('/skills')
export const toggleSkill = (name: string, source: string, enable: boolean) =>
  request<{ ok: boolean }>('/skills/toggle', {
    method: 'POST',
    body: JSON.stringify({ name, source, enable }),
  })

// MCP
export interface McpServerItem {
  name: string
  type: string
  url: string
  command: string
  enabled: boolean
}
export const listMcpServers = () => request<McpServerItem[]>('/mcp')
export const toggleMcpServer = (name: string, enable: boolean) =>
  request<{ ok: boolean }>('/mcp/toggle', {
    method: 'POST',
    body: JSON.stringify({ name, enable }),
  })

// Project-level rules
export const listProjectRules = (workDir: string) =>
  request<FileItem[]>(`/project-rules?work_dir=${encodeURIComponent(workDir)}`)
export const readProjectRule = (workDir: string, path: string) =>
  request<{ content: string }>(`/project-rules/content?work_dir=${encodeURIComponent(workDir)}&path=${encodeURIComponent(path)}`)
export const writeProjectRule = (workDir: string, path: string, content: string) =>
  request<{ ok: boolean }>('/project-rules/content', {
    method: 'PUT',
    body: JSON.stringify({ work_dir: workDir, path, content }),
  })

// Session rules
export const getSessionRules = (id: number) =>
  request<{ session_id: number; content: string }>(`/session-rules/${id}`)
export const putSessionRules = (id: number, content: string) =>
  request<{ ok: boolean }>(`/session-rules/${id}`, {
    method: 'PUT',
    body: JSON.stringify({ content }),
  })
export const deleteSessionRules = (id: number) =>
  request<{ ok: boolean }>(`/session-rules/${id}`, { method: 'DELETE' })

// Triggers
export const listTriggers = (sessionId?: number) =>
  request<Trigger[]>(sessionId ? `/triggers?session_id=${sessionId}` : '/triggers')
export const createTrigger = (t: Partial<Trigger>) =>
  request<Trigger>('/triggers', { method: 'POST', body: JSON.stringify(t) })
export const updateTrigger = (id: number, t: Partial<Trigger>) =>
  request<Trigger>(`/triggers/${id}`, { method: 'PUT', body: JSON.stringify(t) })
export const deleteTrigger = (id: number) =>
  request<{ ok: boolean }>(`/triggers/${id}`, { method: 'DELETE' })
