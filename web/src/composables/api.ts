import type { Provider, Session, Message, Trigger, Channel, TokenUsage, TokenUsageStats, CompressSettings } from '../types'

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

// Paginated messages: returns { messages, has_more }
export const getMessagesPaginated = (sessionId: number, limit = 50, beforeId?: number) => {
  const params = new URLSearchParams({ limit: String(limit) })
  if (beforeId && beforeId > 0) params.set('before_id', String(beforeId))
  return request<{ messages: Message[]; has_more: boolean; total?: number }>(
    `/sessions/${sessionId}/messages?${params.toString()}`
  )
}

// Compress session context
export const compressSession = (id: number) =>
  request<{ ok: boolean }>(`/sessions/${id}/compress`, { method: 'POST' })

// Reset session context (delete messages, keep session config)
export const resetSession = (id: number, keepLast = 0) =>
  request<{ ok: boolean; deleted_count: number; kept_count: number }>(`/sessions/${id}/reset`, {
    method: 'POST',
    body: JSON.stringify({ confirm: true, keep_last: keepLast }),
  })

// Groups
export interface Group {
  id: number
  name: string
  icon: string
  description: string
  session_count: number
  created_at: string
  updated_at: string
}
export const listGroups = () => request<Group[]>('/groups')

// Anthropic API request structure (as sent by Claude Code CLI through the proxy)
export interface AnthropicMessage {
  role: 'user' | 'assistant'
  content: string | Array<{ type: string; text?: string; [key: string]: unknown }>
}
export interface AnthropicRequest {
  model?: string
  max_tokens?: number
  system?: string | Array<{ type: string; text?: string }>
  messages?: AnthropicMessage[]
  [key: string]: unknown
}

// Get last raw request sent to Claude Code CLI
export const getLastRawRequest = (id: number) =>
  request<{
    system_prompt: string
    query: string
    context_count: number
    captured_at: string
    anthropic_request?: AnthropicRequest
  }>(`/sessions/${id}/last-request`)

// Truncate messages from a given message ID inclusive (used for retry-message feature).
// Deletes the user message itself AND all subsequent messages (AI reply etc.)
export const truncateMessages = (sessionId: number, fromMsgId: number) =>
  request<{ ok: boolean }>(`/sessions/${sessionId}/messages?from=${fromMsgId}`, { method: 'DELETE' })

// Switch session provider
export const switchProvider = (id: number, providerId: string) =>
  request<{ ok: boolean; provider_id: string; provider_name: string }>(`/sessions/${id}/provider`, { method: 'PUT', body: JSON.stringify({ provider_id: providerId }) })

// Toggle attention mode
export const toggleAttention = (id: number, enabled: boolean) =>
  request<{ ok: boolean; attention_enabled: boolean; session_id: number }>(`/sessions/${id}/attention`, { method: 'PUT', body: JSON.stringify({ enabled }) })

// Chat
export const sendChat = (sessionId: number, content: string, workDir?: string, sessionRules?: string, providerId?: string, groupName?: string) =>
  request<{ session_id: number; status: string }>('/chat/send', {
    method: 'POST',
    body: JSON.stringify({ session_id: sessionId, content, work_dir: workDir || '', session_rules: sessionRules || '', provider_id: providerId || '', group_name: groupName || '' }),
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
export const getSkillContent = (name: string) =>
  request<{ name: string; dir: string; content: string }>(`/skills/${encodeURIComponent(name)}`)
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

// Memory search (three-layer merge: session → team → global)
export interface MemorySearchResult {
  id: string
  document: string
  similarity: number
  level: string       // "session" | "team" | "global"
  origin: string      // "session" | "team" | "global"
  type: string        // "memory"
  hit_count: number
  read_count: number
  source_session_id: number
  created_at: string  // RFC3339
  updated_at: string  // RFC3339
  metadata: Record<string, any>
}
export const searchMemory = (query: string, topK: number = 10, sessionId?: number) =>
  request<{ results: MemorySearchResult[] }>('/vector/search_memory', {
    method: 'POST',
    body: JSON.stringify({ query, top_k: topK, ...(sessionId ? { session_id: sessionId } : {}) }),
  })

// Read memory file content by scope + file_name
export const readMemoryFile = (scope: string, fileName: string) =>
  request<{ file_name: string; content: string; scope: string }>('/vector/read', {
    method: 'POST',
    body: JSON.stringify({ scope, file_name: fileName }),
  })

export const vectorHealth = () =>
  request<{ ready: boolean; disabled: boolean; error?: string; fix_hint?: string }>('/vector/health')

// List .md files in a vector scope dir (knowledge / memory / rules)
export const listVectorFiles = (scope: string) =>
  request<string[]>(`/vector/list?scope=${encodeURIComponent(scope)}`)

// Rich file item returned by /vector/list_files
export interface VectorFileRich {
  file_name: string
  preview: string
  type: string
  source_session_id: number
  created_at: string  // RFC3339
  updated_at: string  // RFC3339
  scope: string
  origin: string      // "session" | "team" | "global"
}

// List .md files with metadata (source_session_id, updated_at) sorted by mod time desc
export const listVectorFilesRich = (scope: string, opts?: { session_id?: number; level?: string }) => {
  const params = new URLSearchParams({ scope })
  if (opts?.session_id) params.set('session_id', String(opts.session_id))
  if (opts?.level) params.set('level', opts.level)
  return request<{ files: VectorFileRich[]; total: number }>(`/vector/list_files?${params}`)
}

// Read a single file from any valid scope
export const readVectorFile = (scope: string, fileName: string) =>
  request<{ file_name: string; content: string; scope: string }>('/vector/read', {
    method: 'POST',
    body: JSON.stringify({ scope, file_name: fileName }),
  })

// Write a single file to any valid vector scope
// When sessionId is provided and scope is empty, backend auto-resolves session-level scope
export const writeVectorFile = (scope: string, fileName: string, content: string, sessionId?: number) =>
  request<{ ok: boolean; file_name: string; scope: string }>('/vector/write', {
    method: 'POST',
    body: JSON.stringify({ scope: scope || '', file_name: fileName, content, ...(sessionId ? { session_id: sessionId } : {}) }),
  })

// Delete a single file from any valid vector scope
// When sessionId is provided and scope is empty, backend auto-resolves session-level scope
export const deleteVectorFile = (scope: string, fileName: string, sessionId?: number) =>
  request<{ ok: boolean; file_name: string }>('/vector/delete', {
    method: 'POST',
    body: JSON.stringify({ scope: scope || '', file_name: fileName, ...(sessionId ? { session_id: sessionId } : {}) }),
  })

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

// Export / Import
export interface ImportResult {
  ok: boolean
  sessions_imported: number
  session_id_map: Record<string, number>
  team_files_imported: number
  warnings: string[]
}

export const exportSessionUrl = (id: number) => `${BASE}/export/session/${id}`
export const exportTeamUrl = (name: string) => `${BASE}/export/team/${encodeURIComponent(name)}`

export async function importArchive(file: File): Promise<ImportResult> {
  const form = new FormData()
  form.append('file', file)
  const res = await fetch(`${BASE}/import`, { method: 'POST', body: form })
  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: res.statusText }))
    throw new Error(err.error || res.statusText)
  }
  return res.json()
}

// Services
export interface Service {
  id: number
  name: string
  command: string
  work_dir: string
  port: number
  log_path: string
  pid: number
  status: string
  auto_start: boolean
  created_at: string
  updated_at: string
}
export const listServices = () => request<Service[]>('/services')
export const createService = (data: Partial<Service>) =>
  request<Service>('/services', { method: 'POST', body: JSON.stringify(data) })
export const updateService = (id: number, data: Partial<Service>) =>
  request<Service>('/services/' + id, { method: 'PUT', body: JSON.stringify(data) })
export const deleteService = (id: number) =>
  request('/services/' + id, { method: 'DELETE' })
export const startService = (id: number) =>
  request<Service>('/services/' + id + '/start', { method: 'POST' })
export const stopService = (id: number) =>
  request<Service>('/services/' + id + '/stop', { method: 'POST' })
export const restartService = (id: number) =>
  request<Service>('/services/' + id + '/restart', { method: 'POST' })
export const getServiceLogs = (id: number, lines = 100) =>
  request<{ logs: string; error?: string }>('/services/' + id + '/logs?lines=' + lines)

// Compress settings
export const getCompressSettings = () => request<CompressSettings>('/settings/compress')
export const updateCompressSettings = (s: CompressSettings) =>
  request<{ ok: boolean }>('/settings/compress', { method: 'PUT', body: JSON.stringify(s) })

// Schemas
export interface SchemaItem {
  id: number
  name: string
  definition: string
  writers: string
  created_at: string
  updated_at: string
}
export const listSchemas = () => request<SchemaItem[]>('/schemas')
export const getSchemaByName = (name: string) => request<SchemaItem>('/schemas/' + encodeURIComponent(name))
export const createSchemaApi = (name: string, definition: object, writers?: number[]) =>
  request<SchemaItem>('/schemas', { method: 'POST', body: JSON.stringify({ name, definition, ...(writers && writers.length > 0 ? { writers } : {}) }) })
export const updateSchemaApi = (name: string, definition: object, writers?: number[]) =>
  request<SchemaItem>('/schemas/' + encodeURIComponent(name), { method: 'PUT', body: JSON.stringify({ definition, ...(writers !== undefined ? { writers } : {}) }) })
export const deleteSchemaApi = (name: string) =>
  request<{ ok: boolean }>('/schemas/' + encodeURIComponent(name), { method: 'DELETE' })

// Shadow AI
export interface ShadowAIConfig {
  patrol_interval: string
  extract_interval: string
  deep_scan_interval: string
  self_clean_interval: string
  context_reset_threshold: number
}
export interface ShadowAIStatus {
  enabled: boolean
  session_id: number
  status: string
  config: ShadowAIConfig
  triggers?: Array<{
    id: number
    content: string
    trigger_time: string
    enabled: boolean
    status: string
    fired_count: number
  }>
}
export const getShadowAIStatus = () => request<ShadowAIStatus>('/shadow-ai/status')
export const enableShadowAI = (config?: Partial<ShadowAIConfig>) =>
  request<{ ok: boolean; session_id: number; triggers: number[] }>('/shadow-ai/enable', {
    method: 'POST',
    body: JSON.stringify(config || {}),
  })
export const disableShadowAI = () =>
  request<{ ok: boolean; message: string }>('/shadow-ai/disable', { method: 'POST' })
export const updateShadowAIConfig = (config: Partial<ShadowAIConfig>) =>
  request<{ ok: boolean; config: ShadowAIConfig }>('/shadow-ai/config', {
    method: 'PUT',
    body: JSON.stringify(config),
  })
export const getShadowAILogs = (lines = 50) =>
  request<{ content: string; exists: boolean }>(`/shadow-ai/logs?lines=${lines}`)

// Structured Memory
export interface StructuredCategory {
  category: string
  label: string
  has_data: boolean
  fixed: boolean
}
export const listStructuredMemory = () =>
  request<StructuredCategory[]>('/structured-memory')
export const getStructuredMemory = (category: string) =>
  request<{ category: string; label: string; content: string }>(`/structured-memory/${encodeURIComponent(category)}`)
export const putStructuredMemory = (category: string, content: string) =>
  request<{ ok: boolean }>(`/structured-memory/${encodeURIComponent(category)}`, {
    method: 'PUT',
    body: JSON.stringify({ content }),
  })

// Changelog
export interface ChangelogEntry {
  id: number
  file_name: string
  scope: string
  change_type: string
  session_id: number
  diff: string
  schema: string
  version: number
  content: string
  created_at: string
}
export const getChangelog = (fileName: string, scope = 'memory', limit = 20) =>
  request<{ changelog: ChangelogEntry[]; file_name: string; scope: string }>(
    `/changelog?file_name=${encodeURIComponent(fileName)}&scope=${encodeURIComponent(scope)}&limit=${limit}`
  )

// Changelog Rollback
export const rollbackChangelog = (fileName: string, scope: string, version: number) =>
  request<{ ok: boolean; rolled_back_to: number; new_version: number }>('/changelog/rollback', {
    method: 'POST',
    body: JSON.stringify({ file_name: fileName, scope, version }),
  })

// Hooks (Event Hooks)
export interface Hook {
  id: number
  event: string
  condition: string
  target_session: number
  payload: string
  enabled: boolean
  fired_count: number
  created_at: string
  updated_at: string
}
export const listHooks = () => request<Hook[]>('/hooks')
export const getHook = (id: number) => request<Hook>(`/hooks/${id}`)
export const createHook = (h: Partial<Hook>) =>
  request<Hook>('/hooks', { method: 'POST', body: JSON.stringify(h) })
export const updateHook = (id: number, h: Partial<Hook>) =>
  request<Hook>(`/hooks/${id}`, { method: 'PUT', body: JSON.stringify(h) })
export const deleteHook = (id: number) =>
  request<{ ok: boolean }>(`/hooks/${id}`, { method: 'DELETE' })
export const enableHook = (id: number) =>
  request<{ ok: boolean }>(`/hooks/${id}/enable`, { method: 'POST' })
export const disableHook = (id: number) =>
  request<{ ok: boolean }>(`/hooks/${id}/disable`, { method: 'POST' })

// Injection Router
export interface InjectionRoute {
  id: number
  keywords: string
  inject_categories: string
  created_at: string
  updated_at: string
}
export const listInjectionRoutes = () =>
  request<{ routes: InjectionRoute[]; categories: string[]; fixed: string[]; conditional: string[] }>('/injection-router')
export const createInjectionRoute = (keywords: string, inject_categories: string) =>
  request<InjectionRoute>('/injection-router', { method: 'POST', body: JSON.stringify({ keywords, inject_categories }) })
export const updateInjectionRoute = (id: number, data: Partial<InjectionRoute>) =>
  request<{ ok: boolean }>(`/injection-router/${id}`, { method: 'PUT', body: JSON.stringify(data) })
export const deleteInjectionRoute = (id: number) =>
  request<{ ok: boolean }>(`/injection-router/${id}`, { method: 'DELETE' })
