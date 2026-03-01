import type { Provider, Session, Message, Trigger, Channel, TokenUsage, TokenUsageStats } from '../types'

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
export const setProviderDefault = (id: string) =>
  request<{ ok: boolean }>(`/providers/${id}/default`, { method: 'PUT' })
export interface ClaudeAuthStatus {
  logged_in: boolean
  auth_method: string
  email: string
  raw: string
  error?: string
}
export const getClaudeAuthStatus = () => request<ClaudeAuthStatus>('/claude/auth-status')

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

// Truncate messages from a given message ID inclusive (used for retry-message feature).
// Deletes the user message itself AND all subsequent messages (AI reply etc.)
export const truncateMessages = (sessionId: number, fromMsgId: number) =>
  request<{ ok: boolean }>(`/sessions/${sessionId}/messages?from=${fromMsgId}`, { method: 'DELETE' })

// Switch session provider
export const switchProvider = (id: number, providerId: string) =>
  request<{ ok: boolean; provider_id: string; provider_name: string }>(`/sessions/${id}/provider`, { method: 'PUT', body: JSON.stringify({ provider_id: providerId }) })

// Chat
export const sendChat = (sessionId: number, content: string, workDir?: string, sessionRules?: string, providerId?: string) =>
  request<{ session_id: number; status: string }>('/chat/send', {
    method: 'POST',
    body: JSON.stringify({ session_id: sessionId, content, work_dir: workDir || '', session_rules: sessionRules || '', provider_id: providerId || '' }),
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

// Vector search
export interface VectorSearchResult {
  id: string
  document: string
  similarity: number
  metadata: Record<string, any>
}
export const vectorSearch = (scope: string, query: string, topK: number = 5) =>
  request<{ results: VectorSearchResult[] }>('/vector/search', {
    method: 'POST',
    body: JSON.stringify({ scope, query, top_k: topK }),
  })

export const vectorHealth = () =>
  request<{ ready: boolean; disabled: boolean; error?: string; fix_hint?: string }>('/vector/health')

// Channels
export const listChannels = () => request<Channel[]>('/channels')
export const createChannel = (ch: Partial<Channel>) =>
  request<Channel>('/channels', { method: 'POST', body: JSON.stringify(ch) })
export const updateChannel = (id: number, ch: Partial<Channel>) =>
  request<Channel>(`/channels/${id}`, { method: 'PUT', body: JSON.stringify(ch) })
export const deleteChannel = (id: number) =>
  request<{ ok: boolean }>(`/channels/${id}`, { method: 'DELETE' })

// Token usage
export const getMessageTokenUsage = (messageId: number) =>
  request<TokenUsage>(`/token-usage/message/${messageId}`)
export const getSessionTokenUsage = (sessionId: number) =>
  request<{ stats: TokenUsageStats; records: TokenUsage[] }>(`/token-usage/session/${sessionId}`)
export const getSystemTokenUsage = (start?: string, end?: string) => {
  const params = new URLSearchParams()
  if (start) params.set('start', start)
  if (end) params.set('end', end)
  const qs = params.toString()
  return request<TokenUsageStats>(`/token-usage/system${qs ? '?' + qs : ''}`)
}

export interface DailyTokenUsage {
  date: string
  input_tokens: number
  output_tokens: number
  cache_creation_input_tokens: number
  cache_read_input_tokens: number
}
export const getDailyTokenUsage = (start?: string, end?: string) => {
  const params = new URLSearchParams()
  if (start) params.set('start', start)
  if (end) params.set('end', end)
  const qs = params.toString()
  return request<DailyTokenUsage[]>(`/token-usage/daily${qs ? '?' + qs : ''}`)
}

export interface SessionTokenRanking {
  session_id: number
  title: string
  input_tokens: number
  output_tokens: number
  cache_creation_input_tokens: number
  cache_read_input_tokens: number
  total: number
}
export const getTokenUsageRanking = (start?: string, end?: string, limit = 10) => {
  const params = new URLSearchParams()
  if (start) params.set('start', start)
  if (end) params.set('end', end)
  params.set('limit', String(limit))
  const qs = params.toString()
  return request<SessionTokenRanking[]>(`/token-usage/ranking${qs ? '?' + qs : ''}`)
}

export interface HourlyTokenUsage {
  hour: string
  input_tokens: number
  output_tokens: number
  cache_creation_input_tokens: number
  cache_read_input_tokens: number
}
export const getHourlyTokenUsage = (start?: string, end?: string, sessionId?: number) => {
  const params = new URLSearchParams()
  if (start) params.set('start', start)
  if (end) params.set('end', end)
  if (sessionId && sessionId > 0) params.set('session_id', String(sessionId))
  const qs = params.toString()
  return request<HourlyTokenUsage[]>(`/token-usage/hourly${qs ? '?' + qs : ''}`)
}
