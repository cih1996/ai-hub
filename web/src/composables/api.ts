import type { Provider, Session, Message, ConversationLog, Trigger, Channel, TokenUsage, TokenUsageStats, CompressionSettings } from '../types'

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
export const getCompressionSettings = () => request<CompressionSettings>('/compression/settings')
export const updateCompressionSettings = (settings: CompressionSettings) =>
  request<CompressionSettings>('/compression/settings', { method: 'PUT', body: JSON.stringify(settings) })

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
export const getMessagesPaginated = (sessionId: number, limit = 20, beforeId?: number) => {
  const params = new URLSearchParams({ limit: String(limit) })
  if (beforeId && beforeId > 0) params.set('before_id', String(beforeId))
  return request<{ messages: Message[]; has_more: boolean; total?: number }>(
    `/sessions/${sessionId}/messages?${params.toString()}`
  )
}

// Paginated conversation logs: archived user inputs and final AI outputs only.
export const getConversationLogsPaginated = (sessionId: number, limit = 50, beforeId?: number) => {
  const params = new URLSearchParams({ limit: String(limit) })
  if (beforeId && beforeId > 0) params.set('before_id', String(beforeId))
  return request<{ logs: ConversationLog[]; has_more: boolean; total?: number }>(
    `/sessions/${sessionId}/logs?${params.toString()}`
  )
}

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
export interface LastRawRequest {
  system_prompt: string
  query: string
  context_count: number
  captured_at: string
  anthropic_request?: AnthropicRequest
  estimated_tokens?: number
  provider_max_tokens?: number
  threshold_percent?: number
  threshold_tokens?: number
  usage_percent?: number
  compression_enabled?: boolean
  would_trigger_compression?: boolean
  compression_triggered?: boolean
}

export const getLastRawRequest = (id: number) =>
  request<LastRawRequest>(`/sessions/${id}/last-request`)

// Get real context usage (based on actual API input_tokens, not rough estimate)
export interface ContextUsageResponse {
  estimated_tokens: number
  provider_max_tokens: number
  threshold_percent: number
  threshold_tokens: number
  display_percent: number
  compression_enabled: boolean
}
export const getSessionContextUsage = (id: number) =>
  request<ContextUsageResponse>(`/sessions/${id}/context-usage`)

// Truncate messages from a given message ID inclusive (used for retry-message feature).
// Deletes the user message itself AND all subsequent messages (AI reply etc.)
export const truncateMessages = (sessionId: number, fromMsgId: number) =>
  request<{ ok: boolean }>(`/sessions/${sessionId}/messages?from=${fromMsgId}`, { method: 'DELETE' })

// Switch session provider
export const switchProvider = (id: number, providerId: string) =>
  request<{ ok: boolean; provider_id: string; provider_name: string }>(`/sessions/${id}/provider`, { method: 'PUT', body: JSON.stringify({ provider_id: providerId }) })


// Chat
export interface ChatAttachmentPayload {
  type: 'image'
  mime_type: string
  data: string
  name?: string
}

export const sendChat = (sessionId: number, content: string, workDir?: string, sessionRules?: string, providerId?: string, groupName?: string, attachments?: ChatAttachmentPayload[]) =>
  request<{ session_id: number; status: string }>('/chat/send', {
    method: 'POST',
    body: JSON.stringify({ session_id: sessionId, content, work_dir: workDir || '', session_rules: sessionRules || '', provider_id: providerId || '', group_name: groupName || '', attachments: attachments || [] }),
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
  size?: number
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
  when_to_use?: string
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

export interface SkillImportCandidate {
  id: string
  dir_name: string
  name: string
  description: string
  when_to_use?: string
  file_count: number
  files: string[]
  exists: boolean
}
export interface SkillImportPreview {
  archive_name: string
  mode: string
  candidates: SkillImportCandidate[]
  warnings: string[]
}
export async function previewSkillImport(file: File): Promise<SkillImportPreview> {
  const fd = new FormData()
  fd.append('file', file)
  const res = await fetch(BASE + '/skill-import/preview', { method: 'POST', body: fd })
  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: res.statusText }))
    throw new Error(err.error || res.statusText)
  }
  return res.json()
}
export async function importSkills(file: File, skills: string[], overwrite = false): Promise<{ ok: boolean; imported: string[]; warnings: string[] }> {
  const fd = new FormData()
  fd.append('file', file)
  for (const s of skills) fd.append('skills', s)
  fd.append('overwrite', overwrite ? 'true' : 'false')
  const res = await fetch(BASE + '/skill-import', { method: 'POST', body: fd })
  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: res.statusText }))
    throw new Error(err.error || res.statusText)
  }
  return res.json()
}
export function skillExportUrl(name: string) {
  return `${BASE}/skill-export/${encodeURIComponent(name)}`
}

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

export interface ScopedFileRich {
  file_name: string
  preview: string
  type: string
  source_session_id: number
  created_at: string  // RFC3339
  updated_at: string  // RFC3339
  scope: string
  origin: string      // "session" | "team" | "global"
  size?: number       // bytes
}

export const listScopedFiles = (scope: string, opts?: { session_id?: number; level?: string; type?: string }) => {
  const params = new URLSearchParams()
  if (scope) params.set('scope', scope)
  if (opts?.session_id) params.set('session_id', String(opts.session_id))
  if (opts?.level) params.set('level', opts.level)
  if (opts?.type) params.set('type', opts.type)
  return request<{ files: ScopedFileRich[]; total: number }>(`/files/scoped/list?${params.toString()}`)
}

export const readScopedFile = (scope: string, fileName: string, sessionId?: number, type = 'memory') =>
  request<{ file_name: string; content: string; scope: string }>('/files/scoped/read', {
    method: 'POST',
    body: JSON.stringify({ scope, file_name: fileName, ...(sessionId ? { session_id: sessionId } : {}), type }),
  })

export const writeScopedFile = (scope: string, fileName: string, content: string, sessionId?: number, type = 'memory') =>
  request<{ ok: boolean; file_name: string; scope: string }>('/files/scoped/write', {
    method: 'POST',
    body: JSON.stringify({ scope: scope || '', file_name: fileName, content, ...(sessionId ? { session_id: sessionId } : {}), type }),
  })

export const deleteScopedFile = (scope: string, fileName: string, sessionId?: number, type = 'memory') =>
  request<{ ok: boolean; file_name: string }>('/files/scoped/delete', {
    method: 'POST',
    body: JSON.stringify({ scope: scope || '', file_name: fileName, ...(sessionId ? { session_id: sessionId } : {}), type }),
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

