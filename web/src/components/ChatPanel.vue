<script setup lang="ts">
import { ref, nextTick, watch, computed, inject } from 'vue'
import type { Ref } from 'vue'
import { marked } from 'marked'
import { useChatStore } from '../stores/chat'
import * as api from '../composables/api'
import type { StepsMetadata, ConversationLog, Session } from '../types'
import SessionConfigDrawer from './SessionConfigDrawer.vue'
import EnergyProgress from './EnergyProgress.vue'

const isMobile = inject<Ref<boolean>>('isMobile', ref(false))
const openSidebar = inject<() => void>('openSidebar', () => {})

const store = useChatStore()
const input = ref('')
const messagesEl = ref<HTMLElement>()
const textareaEl = ref<HTMLTextAreaElement>()
const fileInputEl = ref<HTMLInputElement>()
const stepsExpanded = ref(false)
const isComposing = ref(false)
const moreMenuOpen = ref(false)
const providerDropdownOpen = ref(false)
const mobilePlusPanelOpen = ref(false)
const slashItems = ref<api.SkillItem[]>([])
const slashLoading = ref(false)
const slashSelectedIndex = ref(0)
const slashMenuOpen = ref(false)
const slashSpacePressed = ref(false) // 标记用户是否按了空格
const isPasting = ref(false) // 标记是否正在粘贴
const providerDropdownLoading = ref(false)

const isInputFocused = ref(false)

function handleInputFocus() {
  isInputFocused.value = true
  store.triggerInputFocus()
}

function handleInputBlur() {
  isInputFocused.value = false
}

// Slash tags state
interface SlashTag {
  id: string
  name: string
  source: string
}
const slashTags = ref<SlashTag[]>([])

function removeSlashTag(id: string) {
  slashTags.value = slashTags.value.filter(t => t.id !== id)
}

// Attachments state
interface Attachment {
  id: string
  type: 'image' | 'file'
  name: string
  preview?: string  // base64 data URL for images
  file?: File
  mimeType?: string
}
const attachments = ref<Attachment[]>([])

// Track expanded state for historical message steps (by message id)
const historyStepsExpanded = ref<Record<number, boolean>>({})

// Tool name Chinese mapping
const toolNameMap: Record<string, string> = {
  Read: '读取文件',
  Edit: '编辑文件',
  Write: '写入文件',
  Bash: '执行命令',
  Grep: '搜索内容',
  Glob: '查找文件',
  WebFetch: '获取网页',
  WebSearch: '搜索网页',
  Task: '子任务',
  TodoWrite: '任务清单',
  Thinking: '思考中',
  NotebookEdit: '编辑笔记本',
  AskUserQuestion: '询问用户',
  Skill: '调用技能',
  ToolSearch: '搜索工具',
}

// Tool color category mapping
function toolColorClass(name: string): string {
  if (name === 'Thinking' || name === '思考中') return 'step-thinking'
  if (['Read', 'Write', 'Edit', 'NotebookEdit'].includes(name)) return 'step-file'
  if (name === 'Bash') return 'step-bash'
  if (['Grep', 'Glob', 'WebSearch', 'ToolSearch'].includes(name)) return 'step-search'
  return 'step-default'
}

// Get current session for template
const currentSession = computed(() => store.currentSession)

// Draft management - save/restore input per session
const DRAFT_KEY_PREFIX = 'chat_draft_'

function saveDraft(sessionId: number) {
  if (input.value.trim()) {
    localStorage.setItem(DRAFT_KEY_PREFIX + sessionId, input.value)
  } else {
    localStorage.removeItem(DRAFT_KEY_PREFIX + sessionId)
  }
}

function loadDraft(sessionId: number): string {
  return localStorage.getItem(DRAFT_KEY_PREFIX + sessionId) || ''
}

watch(currentSession, (newSession, oldSession) => {
  // Save draft for old session
  if (oldSession?.id) {
    saveDraft(oldSession.id)
  }

  // Load draft for new session
  if (newSession?.id) {
    input.value = loadDraft(newSession.id)
    nextTick(() => autoResize())
  }
}, { immediate: true })

// Save draft on input change (debounced)
let saveTimer: ReturnType<typeof setTimeout> | null = null
watch(input, () => {
  if (saveTimer) clearTimeout(saveTimer)
  saveTimer = setTimeout(() => {
    if (currentSession.value?.id) {
      saveDraft(currentSession.value.id)
    }
  }, 500)
})

// Get session avatar URL
function getSessionAvatar(session?: Session): string {
  const s = session || store.currentSession
  if (s?.icon) {
    return `/avatars/${s.icon}`
  }
  // Default avatar based on session ID
  const id = s?.id || 1
  const index = (id % 50) + 1
  return `/avatars/avatar${index}.svg`
}

// 获取光标所在的斜杠上下文
function getSlashContext() {
  if (slashSpacePressed.value) return null

  const ta = textareaEl.value
  if (!ta) return null
  const val = input.value
  const cursor = ta.selectionStart ?? val.length
  const textBefore = val.slice(0, cursor)

  // 查找光标前最近的斜杠
  const slashIdx = textBefore.lastIndexOf('/')
  if (slashIdx === -1) return null

  // 提取斜杠后的查询文本
  const afterSlash = textBefore.slice(slashIdx + 1)
  const spaceIdx = afterSlash.indexOf(' ')
  const query = (spaceIdx === -1 ? afterSlash : afterSlash.slice(0, spaceIdx)).toLowerCase()

  return {
    lineStart: slashIdx,
    lineEnd: cursor,
    query
  }
}

const slashQuery = computed(() => getSlashContext()?.query || '')

const showSlashMenu = computed(() => slashMenuOpen.value && isInputFocused.value)

const filteredSlashItems = computed(() => {
  const query = slashQuery.value
  const enabled = slashItems.value.filter(item => item.enabled)
  const filtered = query
    ? enabled.filter(item => {
        const haystack = `${item.name} ${item.description || ''} ${item.when_to_use || ''}`.toLowerCase()
        return haystack.includes(query)
      })
    : enabled
  return filtered
    .sort((a, b) => {
      if (a.source === 'command' && b.source !== 'command') return -1
      if (a.source !== 'command' && b.source === 'command') return 1
      return a.name.localeCompare(b.name)
    })
    .slice(0, 12)
})

async function ensureSlashItemsLoaded() {
  if (slashItems.value.length > 0 || slashLoading.value) return
  slashLoading.value = true
  try {
    slashItems.value = await api.listSkills()
  } catch {
    slashItems.value = []
  } finally {
    slashLoading.value = false
  }
}

function slashItemType(item: api.SkillItem) {
  return item.source === 'command' ? '命令' : '技能'
}

function scrollSlashActiveIntoView() {
  nextTick(() => {
    const menu = document.querySelector('.slash-command-menu') as HTMLElement | null
    const active = document.querySelector('.slash-command-item.active') as HTMLElement | null
    if (!menu || !active) return
    active.scrollIntoView({ block: 'nearest' })
  })
}

function applySlashItem(item: api.SkillItem) {
  // 添加标签
  slashTags.value.push({
    id: generateId(),
    name: item.name,
    source: item.source
  })

  // 清除输入框中的斜杠文本
  const ctx = getSlashContext()
  if (ctx) {
    input.value = input.value.slice(0, ctx.lineStart) + input.value.slice(ctx.lineEnd)
  }

  // 关闭菜单
  slashMenuOpen.value = false
  slashSelectedIndex.value = 0
  slashSpacePressed.value = true

  nextTick(() => {
    textareaEl.value?.focus()
  })
}

// 光标移动时重新检测斜杠状态
function updateSlashOnCursorMove() {
  // 如果已按空格，不打开菜单
  if (slashSpacePressed.value) {
    slashMenuOpen.value = false
    return
  }

  const ctx = getSlashContext()
  slashMenuOpen.value = !!ctx
  if (ctx) {
    ensureSlashItemsLoaded()
  }
}

watch(input, () => {
  // 粘贴时不弹出菜单
  if (isPasting.value) return

  const ctx = getSlashContext()

  // 如果最后一个字符是 /，重置标志
  if (input.value.endsWith('/')) {
    slashSpacePressed.value = false
  }

  slashMenuOpen.value = !!ctx
  if (ctx) {
    ensureSlashItemsLoaded()
    slashSelectedIndex.value = 0
    scrollSlashActiveIntoView()
  }
})

watch(filteredSlashItems, (items) => {
  if (slashSelectedIndex.value >= items.length) slashSelectedIndex.value = 0
  scrollSlashActiveIntoView()
})

const currentTeamMembers = computed(() => {
  const current = store.currentSession
  if (!current || !current.group_name) return []
  return store.sessions.filter(s => s.group_name === current.group_name).sort((a, b) => a.id - b.id)
})

function localizeToolName(name: string): string {
  return toolNameMap[name] || name
}

function parseMetadata(metadata?: string): StepsMetadata | null {
  if (!metadata) return null
  try {
    return JSON.parse(metadata) as StepsMetadata
  } catch {
    return null
  }
}

function hasMessageContent(content?: string): boolean {
  return !!content?.trim()
}

function quickAction(message: string) {
  store.sendMessage(message)
}

// Drawer state
const showConfigDrawer = ref(false)

// Context menu for team members
const memberCtxMenu = ref<{ x: number; y: number; session: Session } | null>(null)

function openMemberCtxMenu(e: MouseEvent, member: Session) {
  memberCtxMenu.value = { x: e.clientX, y: e.clientY, session: member }
}

function closeMemberCtxMenu() {
  memberCtxMenu.value = null
}

function exportMemberSession(s: Session) {
  closeMemberCtxMenu()
  const a = document.createElement('a')
  a.href = api.exportSessionUrl(s.id)
  a.download = ''
  document.body.appendChild(a)
  a.click()
  document.body.removeChild(a)
}

// Delete confirmation for member session
const memberDeleteTarget = ref<Session | null>(null)

function confirmMemberDelete() {
  if (memberDeleteTarget.value) {
    store.deleteSessionById(memberDeleteTarget.value.id)
    memberDeleteTarget.value = null
  }
}

// Raw request modal state
const showRawRequestModal = ref(false)
const rawRequestLoading = ref(false)
const rawRequestData = ref<api.LastRawRequest | null>(null)
const rawRequestTab = ref<'messages' | 'fullchat' | 'raw' | 'system' | 'query'>('system')
// Track which rows are expanded in the visual Messages tab
const expandedRows = ref<Set<number>>(new Set())

// Full conversation log state (lazy-loaded in fullchat tab)
const fullChatMessages = ref<ConversationLog[]>([])
const fullChatHasMore = ref(false)
const fullChatTotal = ref(0)
const fullChatLoading = ref(false)
const fullChatLoaded = ref(false)
const expandedFullChatRows = ref<Set<number>>(new Set())
const fullchatSearchQuery = ref('')
const fullchatMatchIndices = ref<number[]>([])
const fullchatCurrentMatchIdx = ref(-1)

async function loadFullChat() {
  const sid = store.currentSession?.id
  if (!sid || fullChatLoading.value) return
  fullChatLoading.value = true
  try {
    const beforeId = fullChatMessages.value.length > 0
      ? fullChatMessages.value[fullChatMessages.value.length - 1]!.id
      : undefined
    const res = await api.getConversationLogsPaginated(sid, 30, beforeId)
    // API returns ASC order within the batch; we want newest-first display,
    // so reverse each batch and append (older logs go to the end)
    const batch = [...res.logs].reverse()
    fullChatMessages.value.push(...batch)
    fullChatHasMore.value = res.has_more
    if (res.total != null) fullChatTotal.value = res.total
    fullChatLoaded.value = true
  } catch {
    // silent
  } finally {
    fullChatLoading.value = false
  }
}

function onFullChatScroll(e: Event) {
  const el = e.target as HTMLElement
  if (el.scrollTop + el.clientHeight >= el.scrollHeight - 50 && fullChatHasMore.value) {
    loadFullChat()
  }
}

function toggleFullChatRow(id: number) {
  if (expandedFullChatRows.value.has(id)) {
    expandedFullChatRows.value.delete(id)
  } else {
    expandedFullChatRows.value.add(id)
  }
}

function stripErrorTags(text: string): string {
  return text.replace(errorTagPattern, '').trim()
}

// Fullchat search
const fullchatTotalMatches = computed(() => fullchatMatchIndices.value.length)
const fullchatMatchDisplay = computed(() =>
  fullchatMatchIndices.value.length > 0
    ? `${fullchatCurrentMatchIdx.value + 1}/${fullchatMatchIndices.value.length}`
    : fullchatSearchQuery.value ? '无结果' : ''
)

function fullchatOnSearch() {
  const q = fullchatSearchQuery.value.trim().toLowerCase()
  if (!q) {
    fullchatMatchIndices.value = []
    fullchatCurrentMatchIdx.value = -1
    return
  }
  const indices: number[] = []
  fullChatMessages.value.forEach((msg, i) => {
    if (msg.content && msg.content.toLowerCase().includes(q)) indices.push(i)
  })
  fullchatMatchIndices.value = indices
  fullchatCurrentMatchIdx.value = indices.length > 0 ? 0 : -1
  if (indices.length > 0) fullchatScrollToMatch(0)
}

function fullchatScrollToMatch(idx: number) {
  const msgIdx = fullchatMatchIndices.value[idx]
  if (msgIdx == null) return
  const el = document.querySelector(`.fullchat-list [data-fc-idx="${msgIdx}"]`)
  if (el) {
    el.scrollIntoView({ block: 'center', behavior: 'smooth' })
    expandedFullChatRows.value.add(fullChatMessages.value[msgIdx]!.id)
  }
}

function fullchatSearchNext() {
  if (fullchatMatchIndices.value.length === 0) return
  fullchatCurrentMatchIdx.value = (fullchatCurrentMatchIdx.value + 1) % fullchatMatchIndices.value.length
  fullchatScrollToMatch(fullchatCurrentMatchIdx.value)
}

function fullchatSearchPrev() {
  if (fullchatMatchIndices.value.length === 0) return
  fullchatCurrentMatchIdx.value = (fullchatCurrentMatchIdx.value - 1 + fullchatMatchIndices.value.length) % fullchatMatchIndices.value.length
  fullchatScrollToMatch(fullchatCurrentMatchIdx.value)
}

function fullchatHighlightPreview(content: string): string {
  if (!fullchatSearchQuery.value.trim()) return escapeHtml(previewText(content, 60))
  return highlightMatches(previewText(content, 60), fullchatSearchQuery.value)
}

function fullchatHighlightContent(content: string): string {
  if (!fullchatSearchQuery.value.trim()) return escapeHtml(stripErrorTags(content))
  return highlightMatches(stripErrorTags(content), fullchatSearchQuery.value)
}

function escapeHtml(s: string): string {
  return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;')
}

function highlightMatches(text: string, query: string): string {
  const escaped = escapeHtml(text)
  const q = query.trim()
  if (!q) return escaped
  const re = new RegExp(`(${q.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')})`, 'gi')
  return escaped.replace(re, '<mark class="fc-highlight">$1</mark>')
}

function previewText(text: string, len: number): string {
  const clean = stripErrorTags(text).replace(/\n/g, ' ')
  return clean.length > len ? clean.slice(0, len) + '…' : clean
}

function formatMessageTime(dateStr: string): string {
  if (!dateStr) return ''
  const date = new Date(dateStr)
  if (isNaN(date.getTime())) return ''
  const now = new Date()
  const pad = (n: number) => String(n).padStart(2, '0')
  const time = `${pad(date.getHours())}:${pad(date.getMinutes())}`
  const today = new Date(now.getFullYear(), now.getMonth(), now.getDate())
  const msgDay = new Date(date.getFullYear(), date.getMonth(), date.getDate())
  const diffDays = Math.floor((today.getTime() - msgDay.getTime()) / 86400000)
  if (diffDays === 0) return time
  if (diffDays === 1) return `昨天 ${time}`
  if (diffDays === 2) return `前天 ${time}`
  if (date.getFullYear() === now.getFullYear()) return `${date.getMonth() + 1}月${date.getDate()}号 ${time}`
  return `${date.getFullYear()}年${date.getMonth() + 1}月${date.getDate()}号 ${time}`
}

// Format the complete Anthropic API request body for the Raw tab.
// Displays the entire POST body (model, max_tokens, tools, temperature, etc.)
function formatAnthropicMessages(req: api.AnthropicRequest | undefined): string {
  if (!req) return ''
  return JSON.stringify(req, null, 2)
}

const rawContentSizeKB = computed(() => {
  const raw = formatAnthropicMessages(rawRequestData.value?.anthropic_request)
  if (!raw) return null
  const bytes = new Blob([raw]).size
  return (bytes / 1024).toFixed(1)
})

// Get actual messages count from the Anthropic request
function getActualMsgCount(req: api.AnthropicRequest | undefined): number | null {
  if (!req?.messages) return null
  return req.messages.length
}

// Parsed row for visual Messages tab
interface ParsedRow {
  rowIndex: number
  role: string
  type: string
  preview: string
  full: string
  toolName?: string                      // tool_use: tool name (e.g. Bash, Read)
  toolId?: string                        // tool_use: block id
  toolUseId?: string                     // tool_result: linked tool_use_id
  toolInput?: Record<string, unknown>    // tool_use: input parameters
}

// Build flat list of content rows (system + messages)
function buildParsedRows(req: api.AnthropicRequest | undefined): ParsedRow[] {
  if (!req) return []
  const rows: ParsedRow[] = []
  let idx = 0
  function addRow(role: string, type: string, rawContent: unknown, blockData: unknown,
                  toolName?: string, toolId?: string, toolUseId?: string, toolInput?: Record<string, unknown>) {
    const text = typeof rawContent === 'string' ? rawContent : JSON.stringify(rawContent)
    const preview = text.replace(/\s+/g, ' ').trim().slice(0, 60) + (text.length > 60 ? '\u2026' : '')
    const full = typeof blockData === 'string' ? blockData : JSON.stringify(blockData, null, 2)
    rows.push({ rowIndex: idx++, role, type, preview, full, toolName, toolId, toolUseId, toolInput })
  }
  if (req.system) {
    if (typeof req.system === 'string') {
      addRow('system', 'text', req.system, req.system)
    } else if (Array.isArray(req.system)) {
      for (const block of req.system) addRow('system', block.type || 'text', block.text ?? block, block)
    }
  }
  for (const msg of (req.messages || [])) {
    const content = msg.content
    if (typeof content === 'string') {
      addRow(msg.role, 'text', content, content)
    } else if (Array.isArray(content)) {
      for (const block of content) {
        let display: unknown
        if (block.type === 'text') {
          display = block.text ?? ''
          addRow(msg.role, 'text', display, block)
        } else if (block.type === 'tool_use') {
          display = (block.name || 'tool') + ': ' + JSON.stringify(block.input).slice(0, 80)
          addRow(msg.role, 'tool_use', display, block,
            block.name as string, block.id as string, undefined, block.input as Record<string, unknown>)
        } else if (block.type === 'tool_result') {
          const c = (block as { content?: unknown }).content
          display = typeof c === 'string' ? c : JSON.stringify(c)
          addRow(msg.role, 'tool_result', display, block,
            undefined, undefined, (block as { tool_use_id?: string }).tool_use_id)
        } else if (block.type === 'thinking') {
          display = (block as { thinking?: string }).thinking ?? ''
          addRow(msg.role, block.type, display, block)
        } else {
          display = JSON.stringify(block)
          addRow(msg.role, block.type || 'text', display, block)
        }
      }
    }
  }
  return rows
}

const parsedMessageRows = computed<ParsedRow[]>(() => buildParsedRows(rawRequestData.value?.anthropic_request))

// ---- tool_use ↔ tool_result ID association ----
const highlightedToolId = ref<string | null>(null)

// Bidirectional map: toolId → { useIdx, resultIdx }
const toolPairMap = computed(() => {
  const map = new Map<string, { useIdx?: number; resultIdx?: number }>()
  for (const row of parsedMessageRows.value) {
    if (row.type === 'tool_use' && row.toolId) {
      if (!map.has(row.toolId)) map.set(row.toolId, {})
      map.get(row.toolId)!.useIdx = row.rowIndex
    }
    if (row.type === 'tool_result' && row.toolUseId) {
      if (!map.has(row.toolUseId)) map.set(row.toolUseId, {})
      map.get(row.toolUseId)!.resultIdx = row.rowIndex
    }
  }
  return map
})

// toolId → tool name (for showing name on tool_result cards)
const toolNameById = computed(() => {
  const map = new Map<string, string>()
  for (const row of parsedMessageRows.value) {
    if (row.type === 'tool_use' && row.toolId && row.toolName) {
      map.set(row.toolId, row.toolName)
    }
  }
  return map
})

function truncateId(id?: string): string {
  if (!id) return ''
  return id.length > 20 ? id.slice(0, 20) + '…' : id
}

function hasToolPair(toolId?: string): boolean {
  if (!toolId) return false
  const pair = toolPairMap.value.get(toolId)
  return !!pair && pair.useIdx != null && pair.resultIdx != null
}

function getLinkedToolName(toolUseId?: string): string | undefined {
  if (!toolUseId) return undefined
  return toolNameById.value.get(toolUseId)
}

function isToolHighlighted(row: ParsedRow): boolean {
  if (!highlightedToolId.value) return false
  return row.toolId === highlightedToolId.value || row.toolUseId === highlightedToolId.value
}

function jumpToPair(fromType: 'tool_use' | 'tool_result', toolId: string | undefined) {
  if (!toolId) return
  const pair = toolPairMap.value.get(toolId)
  if (!pair) return
  const targetIdx = fromType === 'tool_use' ? pair.resultIdx : pair.useIdx
  if (targetIdx == null) return
  // Expand target row so it's visible
  const s = new Set(expandedRows.value)
  s.add(targetIdx)
  expandedRows.value = s
  // Highlight pair
  highlightedToolId.value = toolId
  // Scroll to target
  nextTick(() => {
    const el = document.querySelector(`[data-row-index="${targetIdx}"]`)
    if (el) el.scrollIntoView({ behavior: 'smooth', block: 'center' })
  })
  // Clear highlight after 2.5s
  setTimeout(() => { if (highlightedToolId.value === toolId) highlightedToolId.value = null }, 2500)
}

function toggleRowExpand(rowIndex: number) {
  const s = new Set(expandedRows.value)
  if (s.has(rowIndex)) s.delete(rowIndex)
  else s.add(rowIndex)
  expandedRows.value = s
}

async function openRawRequest() {
  const sid = store.currentSession?.id
  if (!sid) return
  showRawRequestModal.value = true
  rawRequestLoading.value = true
  rawRequestData.value = null
  expandedRows.value = new Set()
  // Reset fullchat state on each open
  fullChatMessages.value = []
  fullChatHasMore.value = false
  fullChatTotal.value = 0
  fullChatLoaded.value = false
  fullchatSearchQuery.value = ''
  fullchatMatchIndices.value = []
  fullchatCurrentMatchIdx.value = -1
  expandedFullChatRows.value = new Set()
  try {
    rawRequestData.value = await api.getLastRawRequest(sid)
    // Default to full log for sessions after context reset or when raw request is unavailable.
    rawRequestTab.value = rawRequestData.value?.anthropic_request?.messages ? 'messages' : 'fullchat'
  } catch {
    rawRequestData.value = null
    rawRequestTab.value = 'fullchat'
  } finally {
    rawRequestLoading.value = false
  }
}

// Lazy-load fullchat when tab is first activated
watch(rawRequestTab, (tab) => {
  if (tab === 'fullchat' && !fullChatLoaded.value) {
    loadFullChat()
  }
})

// Title editing state
const editingTitle = ref(false)
const titleInput = ref('')
const titleInputEl = ref<HTMLInputElement>()

function startEditTitle() {
  if (!store.currentSession) return
  titleInput.value = store.currentSession.title
  editingTitle.value = true
  nextTick(() => titleInputEl.value?.focus())
}

async function saveTitle() {
  const s = store.currentSession
  if (!s || !titleInput.value.trim()) {
    editingTitle.value = false
    return
  }
  const newTitle = titleInput.value.trim()
  if (newTitle !== s.title) {
    await api.updateSession(s.id, { title: newTitle })
    s.title = newTitle
  }
  editingTitle.value = false
}

function cancelEditTitle() {
  editingTitle.value = false
}

function healthBadgeLabel(score: string): string {
  const map: Record<string, string> = { green: '健康', yellow: '注意', red: '异常' }
  return map[score] || score
}

marked.setOptions({ breaks: true, gfm: true })

// Strip <!--error:xxx--> and <!--warning:xxx--> tags before rendering
const errorTagPattern = /<!--(?:error|warning):\s*.+?-->/g

function renderMd(text: string): string {
  return marked.parse(text.replace(errorTagPattern, '')) as string
}

const allMessages = computed(() => [...store.messages])

// ID of the last user message — retry button only shows on this one
const lastUserMsgId = computed(() => {
  const userMsgs = allMessages.value.filter((m) => m.role === 'user')
  return userMsgs.length > 0 ? userMsgs[userMsgs.length - 1]!.id : -1
})

async function retryMessage(msgId: number, content: string) {
  if (store.streaming) return
  // Delete the user message itself + all messages after it (AI reply etc.)
  // Using inclusive "from" semantics: the original user msg is removed then re-sent fresh
  if (store.currentSessionId > 0) {
    try {
      await api.truncateMessages(store.currentSessionId, msgId)
    } catch { /* ignore, still retry */ }
  }
  // Remove from idx inclusive: the user message itself + anything after (AI reply)
  const idx = store.messages.findIndex((m) => m.id === msgId)
  if (idx !== -1) {
    store.messages.splice(idx)
  }
  // sendMessage will push a fresh user message and trigger the stream
  store.sendMessage(content)
}

const stepCount = computed(() => {
  let n = store.toolCalls.length
  if (store.thinkingContent) n++
  return n
})

const hasActivity = computed(() =>
  store.streaming && (store.thinkingContent || store.toolCalls.length > 0)
)

const contextCount = computed(() => store.messages.length)



function scrollToBottom(retry = true) {
  nextTick(() => {
    if (messagesEl.value) {
      messagesEl.value.scrollTop = messagesEl.value.scrollHeight
    }
    // Retry after a short delay to catch late DOM renders (images, code blocks, etc.)
    if (retry) {
      setTimeout(() => {
        if (messagesEl.value) {
          messagesEl.value.scrollTop = messagesEl.value.scrollHeight
        }
      }, 150)
    }
  })
}

// Scroll-to-top detection for loading more messages
function onMessagesScroll() {
  if (!messagesEl.value || !store.hasMoreMessages || store.loadingMore) return
  if (messagesEl.value.scrollTop < 80) {
    loadMore()
  }
}

async function loadMore() {
  if (!messagesEl.value) return
  const el = messagesEl.value
  const prevScrollHeight = el.scrollHeight
  await store.loadMoreMessages()
  // Preserve scroll position: after prepending, restore relative position
  nextTick(() => {
    const newScrollHeight = el.scrollHeight
    el.scrollTop = newScrollHeight - prevScrollHeight
  })
}

watch(() => allMessages.value.length, (newLen, oldLen) => {
  // Only auto-scroll when new messages are appended (not prepended via loadMore)
  if (!store.loadingMore && newLen > oldLen) scrollToBottom()
})
watch(() => store.streamingContent, () => scrollToBottom(false))
watch(() => store.thinkingContent, () => scrollToBottom(false))
watch(() => store.toolCalls.length, () => scrollToBottom())

// Session token stats
const sessionTokenStats = ref<{ total_input_tokens: number; total_output_tokens: number; total_cache_creation_tokens: number; total_cache_read_tokens: number; count: number } | null>(null)

// Energy progress bar
const energyPercent = computed(() => {
  const usage = store.contextUsage
  if (!usage || !usage.compression_enabled || usage.threshold_tokens <= 0) return 0
  return Math.min(100, usage.display_percent)
})
const energyThreshold = computed(() => store.contextUsage?.threshold_percent || 0)
const showEnergy = computed(() => store.contextUsage?.compression_enabled && store.currentSessionId > 0)

// Load token usage when session changes
watch(() => store.currentSessionId, async (id) => {
  sessionTokenStats.value = null
  if (id > 0) {
    try {
      const data = await api.getSessionTokenUsage(id)
      sessionTokenStats.value = data.stats
      for (const r of data.records) {
        if (r.message_id) store.tokenUsageMap[r.message_id] = r
      }
    } catch { /* ignore */ }
  }
  scrollToBottom()
}, { immediate: true })

// Update session stats when new token_usage arrives via WS
watch(() => store.latestTokenUsage, (usage) => {
  if (usage && usage.session_id === store.currentSessionId && sessionTokenStats.value) {
    sessionTokenStats.value.total_input_tokens += usage.input_tokens
    sessionTokenStats.value.total_output_tokens += usage.output_tokens
    sessionTokenStats.value.total_cache_creation_tokens = (sessionTokenStats.value.total_cache_creation_tokens || 0) + (usage.cache_creation_input_tokens || 0)
    sessionTokenStats.value.total_cache_read_tokens = (sessionTokenStats.value.total_cache_read_tokens || 0) + (usage.cache_read_input_tokens || 0)
    sessionTokenStats.value.count++
  }
})

function formatTokenNum(n: number): string {
  if (n >= 1048576) return (n / 1048576).toFixed(1) + ' MB'
  if (n >= 1024) return (n / 1024).toFixed(1) + ' KB'
  return n + ' B'
}

function formatUsageLine(u: { input_tokens: number; output_tokens: number; cache_creation_input_tokens?: number; cache_read_input_tokens?: number }): string {
  let parts = [`${u.input_tokens.toLocaleString()} in`]
  if (u.cache_creation_input_tokens) parts.push(`${formatTokenNum(u.cache_creation_input_tokens)} cache_w`)
  if (u.cache_read_input_tokens) parts.push(`${formatTokenNum(u.cache_read_input_tokens)} cache_r`)
  parts.push(`${u.output_tokens.toLocaleString()} out`)
  return parts.join(' / ')
}

async function buildImageAttachments(): Promise<api.ChatAttachmentPayload[]> {
  const result: api.ChatAttachmentPayload[] = []
  for (const att of attachments.value) {
    if (att.type !== 'image' || !att.preview) continue
    const commaIndex = att.preview.indexOf(',')
    if (commaIndex === -1) continue
    const header = att.preview.slice(0, commaIndex)
    const data = att.preview.slice(commaIndex + 1)
    const mimeMatch = header.match(/^data:(image\/[^;]+);base64$/i)
    const mimeType = att.mimeType || mimeMatch?.[1] || 'image/png'
    result.push({
      type: 'image',
      mime_type: mimeType,
      data,
      name: att.name,
    })
  }
  return result
}

async function send() {
  const text = input.value.trim()
  const hasAttachments = attachments.value.length > 0
  const hasTags = slashTags.value.length > 0
  if (!text && !hasAttachments && !hasTags) return

  // 合并标签和文本
  let finalText = ''
  if (hasTags) {
    finalText = slashTags.value.map(t => `/${t.name}`).join(' ')
    if (text) finalText += ' ' + text
  } else {
    finalText = text
  }

  const imageAttachments = await buildImageAttachments()
  store.sendMessage(finalText.trim(), imageAttachments)
  input.value = ''
  attachments.value = []
  slashTags.value = []
  stepsExpanded.value = false
  autoResize()
}

// Attachment handling
function generateId(): string {
  return Math.random().toString(36).slice(2, 10)
}

function removeAttachment(id: string) {
  attachments.value = attachments.value.filter(a => a.id !== id)
}

function handleFileSelect(e: Event) {
  const input = e.target as HTMLInputElement
  if (!input.files) return
  for (const file of Array.from(input.files)) {
    addFileAsAttachment(file)
  }
  input.value = '' // Reset for re-selection
}

function addFileAsAttachment(file: File) {
  const isImage = file.type.startsWith('image/')
  const att: Attachment = {
    id: generateId(),
    type: isImage ? 'image' : 'file',
    name: file.name,
    file,
    mimeType: file.type || undefined,
  }

  if (isImage) {
    const reader = new FileReader()
    reader.onload = () => {
      att.preview = reader.result as string
      attachments.value = [...attachments.value] // Trigger reactivity
    }
    reader.readAsDataURL(file)
  }

  attachments.value.push(att)
}

function handlePaste(e: ClipboardEvent) {
  const items = e.clipboardData?.items
  if (!items) return

  // 标记正在粘贴，防止触发斜杠菜单
  isPasting.value = true
  nextTick(() => {
    isPasting.value = false
  })

  for (const item of Array.from(items)) {
    if (item.type.startsWith('image/')) {
      e.preventDefault()
      const file = item.getAsFile()
      if (file) {
        const att: Attachment = {
          id: generateId(),
          type: 'image',
          name: `粘贴图片_${Date.now()}.png`,
          file,
          mimeType: file.type || item.type || 'image/png',
        }
        const reader = new FileReader()
        reader.onload = () => {
          att.preview = reader.result as string
          attachments.value = [...attachments.value]
        }
        reader.readAsDataURL(file)
        attachments.value.push(att)
      }
      return
    }
  }
}

function openFileDialog() {
  fileInputEl.value?.click()
}

async function toggleProviderDropdown() {
  if (store.streaming || store.providerSwitching) return
  providerDropdownOpen.value = !providerDropdownOpen.value
  if (providerDropdownOpen.value && store.providers.length === 0 && !providerDropdownLoading.value) {
    providerDropdownLoading.value = true
    try {
      await store.loadProviders()
    } finally {
      providerDropdownLoading.value = false
    }
  }
}

async function onSwitchProvider(providerId: string) {
  providerDropdownOpen.value = false
  await store.switchProviderForSession(providerId)
}

// 点击外部关闭模型下拉
watch(providerDropdownOpen, (open) => {
  if (open) {
    nextTick(() => {
      document.addEventListener('click', closeProviderDropdown, true)
    })
  } else {
    document.removeEventListener('click', closeProviderDropdown, true)
  }
})
function closeProviderDropdown(e: MouseEvent) {
  const el = e.target as HTMLElement
  if (!el.closest('.provider-switcher')) {
    providerDropdownOpen.value = false
  }
}

function onKeydown(e: KeyboardEvent) {
  if (showSlashMenu.value && filteredSlashItems.value.length > 0) {
    if (e.key === 'ArrowDown') {
      e.preventDefault()
      slashSelectedIndex.value = (slashSelectedIndex.value + 1) % filteredSlashItems.value.length
      scrollSlashActiveIntoView()
      return
    }
    if (e.key === 'ArrowUp') {
      e.preventDefault()
      slashSelectedIndex.value = (slashSelectedIndex.value - 1 + filteredSlashItems.value.length) % filteredSlashItems.value.length
      scrollSlashActiveIntoView()
      return
    }
    if (e.key === 'Tab') {
      e.preventDefault()
      applySlashItem(filteredSlashItems.value[slashSelectedIndex.value]!)
      return
    }
    if (e.key === 'Enter' && !e.shiftKey && !isComposing.value) {
      e.preventDefault()
      applySlashItem(filteredSlashItems.value[slashSelectedIndex.value]!)
      return
    }
    if (e.key === 'Escape') {
      e.preventDefault()
      slashMenuOpen.value = false
      return
    }
    // 空格键关闭菜单并标记
    if (e.key === ' ') {
      e.preventDefault()
      slashSpacePressed.value = true
      slashMenuOpen.value = false
      return
    }
  }

  if (e.key === 'Enter' && !e.shiftKey && !isComposing.value) {
    e.preventDefault()
    send()
  }
}

function autoResize() {
  const el = textareaEl.value
  if (!el) return
  el.style.height = 'auto'
  el.style.height = Math.min(el.scrollHeight, 200) + 'px'
}

// Reset context with confirmation dialog
const showResetConfirm = ref(false)
const resetKeepLast = ref(0)

async function executeReset() {
  showResetConfirm.value = false
  await store.resetContext(resetKeepLast.value)
}



function formatToolInput(raw: string): string {
  if (!raw) return ''
  try {
    const obj = JSON.parse(raw)
    if (typeof obj === 'object' && obj !== null) {
      if (obj.command) return obj.command
      if (obj.file_path) return obj.file_path
      if (obj.pattern) return obj.pattern + (obj.path ? ` in ${obj.path}` : '')
      const firstVal = Object.values(obj).find((v) => typeof v === 'string' && (v as string).length > 0)
      if (firstVal) return String(firstVal).slice(0, 300)
    }
  } catch {
    const cmdMatch = raw.match(/"command"\s*:\s*"((?:[^"\\]|\\.)*)/)
    if (cmdMatch?.[1]) return cmdMatch[1].replace(/\\"/g, '"').replace(/\\n/g, '\n')
    const fileMatch = raw.match(/"file_path"\s*:\s*"((?:[^"\\]|\\.)*)/)
    if (fileMatch?.[1]) return fileMatch[1]
    const patternMatch = raw.match(/"pattern"\s*:\s*"((?:[^"\\]|\\.)*)/)
    if (patternMatch?.[1]) return patternMatch[1]
  }
  return raw.length > 300 ? raw.slice(0, 300) + '...' : raw
}
</script>

<template>
  <div class="chat-panel">
    <div v-if="store.usageLimitWarning" class="quota-banner">
      <span class="quota-banner-icon">⚠️</span>
      <span class="quota-banner-text">{{ store.usageLimitWarning }}</span>
      <button class="quota-banner-close" @click="store.clearUsageLimitWarning()">✕</button>
    </div>
    <!-- Mobile: always show top bar with hamburger -->
    <div v-if="isMobile && !store.currentSession" class="chat-header">
      <div class="header-left">
        <button class="btn-hamburger" @click="openSidebar" title="菜单">
          <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <line x1="3" y1="6" x2="21" y2="6"/><line x1="3" y1="12" x2="21" y2="12"/><line x1="3" y1="18" x2="21" y2="18"/>
          </svg>
        </button>
        <div class="header-title-group">
          <div class="header-title">AI Hub</div>
        </div>
      </div>
    </div>
    <!-- Chat header bar -->
    <div v-if="store.currentSession" class="chat-header">
      <div class="header-left">
        <button v-if="isMobile" class="btn-hamburger" @click="openSidebar" title="菜单">
          <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <line x1="3" y1="6" x2="21" y2="6"/><line x1="3" y1="12" x2="21" y2="12"/><line x1="3" y1="18" x2="21" y2="18"/>
          </svg>
        </button>
        <div class="header-title-group">
          <input
            v-if="editingTitle"
            ref="titleInputEl"
            v-model="titleInput"
            class="header-title-input"
            @keydown.enter="saveTitle"
            @keydown.esc="cancelEditTitle"
            @blur="saveTitle"
          />
          <div v-else class="header-title" @click="startEditTitle" title="点击编辑标题">
            {{ store.currentSession.title }}
            <span
              v-if="store.currentSession.health_score"
              class="health-badge"
              :class="'health-' + store.currentSession.health_score"
              :title="'健康度: ' + healthBadgeLabel(store.currentSession.health_score)"
            >{{ healthBadgeLabel(store.currentSession.health_score) }}</span>
          </div>
          <div class="header-sub-row">
            <span v-if="store.currentSession.group_name" class="header-team-badge" :title="'团队: ' + store.currentSession.group_name">
              <svg width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <path d="M17 21v-2a4 4 0 00-4-4H5a4 4 0 00-4 4v2"/>
                <circle cx="9" cy="7" r="4"/>
                <path d="M23 21v-2a4 4 0 00-3-3.87"/>
                <path d="M16 3.13a4 4 0 010 7.75"/>
              </svg>
              {{ store.currentSession.group_name }}
            </span>
          </div>
        </div>
      </div>
      <div class="header-right" v-if="!isMobile">
        <EnergyProgress v-if="showEnergy" :percent="energyPercent" :threshold-percent="energyThreshold" />
        <button
          class="btn-rules"
          @click="showConfigDrawer = true"
          title="配置（角色与记忆）"
        >
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2z"/>
            <path d="M12 6v6l4 2"/>
          </svg>
          配置
        </button>
      </div>
      <!-- Mobile: more menu -->
      <div v-if="isMobile" class="header-right-mobile">
        <EnergyProgress v-if="showEnergy" :percent="energyPercent" :threshold-percent="energyThreshold" />
        <div class="more-menu-wrapper">
          <button class="btn-more" @click="moreMenuOpen = !moreMenuOpen">
            <svg width="18" height="18" viewBox="0 0 24 24" fill="currentColor">
              <circle cx="12" cy="5" r="2"/><circle cx="12" cy="12" r="2"/><circle cx="12" cy="19" r="2"/>
            </svg>
          </button>
          <div v-if="moreMenuOpen" class="more-menu" @click="moreMenuOpen = false">
            <button @click="showConfigDrawer = true">会话配置</button>
            <div class="more-menu-divider"></div>
            <div class="more-menu-label">切换模型</div>
            <button
              v-for="p in store.providers"
              :key="p.id"
              :class="{ 'more-menu-active': String(p.id) === String(store.currentSession.provider_id) }"
              :disabled="store.streaming || store.providerSwitching"
              @click.stop="onSwitchProvider(String(p.id)); moreMenuOpen = false"
            >{{ p.name }} · {{ p.model_id }}</button>
          </div>
        </div>
      </div>
    </div>

    <!-- Team Members Bar -->
    <div v-if="currentTeamMembers.length > 0" class="team-members-bar">
      <div
        v-for="member in currentTeamMembers"
        :key="member.id"
        class="team-member-item"
        :class="{ active: member.id === store.currentSessionId }"
        @click="store.selectSession(member.id)"
        @contextmenu.prevent="openMemberCtxMenu($event, member)"
      >
        <img :src="getSessionAvatar(member)" class="member-avatar" />
        <span class="member-name" :title="member.title">{{ member.title }}</span>
        <button class="member-btn-delete" @click.stop="memberDeleteTarget = member" title="删除成员会话">
          <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M18 6L6 18M6 6l12 12"/>
          </svg>
        </button>
      </div>
    </div>

    <div class="messages" ref="messagesEl" @scroll="onMessagesScroll">
      <!-- __CONTINUE_HERE__ -->
      <div class="messages-inner">
        <!-- Load more indicator -->
        <div v-if="store.hasMoreMessages" class="load-more-hint" @click="loadMore">
          <span v-if="store.loadingMore" class="load-more-spinner"></span>
          <span v-else>↑ 加载更早的消息</span>
        </div>
        <!-- Quick action cards for empty chat -->
        <div v-if="allMessages.length === 0 && !store.streaming" class="quick-actions">
          <div class="quick-actions-title">快捷操作</div>
          <div class="quick-actions-grid">
            <div class="quick-card" @click="quickAction('请执行系统自检，检查所有组件状态并自动修复问题。')">
              <span class="quick-card-icon"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="11" cy="11" r="8"/><line x1="21" y1="21" x2="16.65" y2="16.65"/></svg></span>
              <span class="quick-card-label">初始化系统</span>
              <span class="quick-card-desc">自检环境、修复依赖</span>
            </div>
            <div class="quick-card" @click="quickAction('请帮我部署 QQ 机器人，对接到 AI Hub。')">
              <span class="quick-card-icon"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="2" y="3" width="20" height="14" rx="2"/><line x1="8" y1="21" x2="16" y2="21"/><line x1="12" y1="17" x2="12" y2="21"/><circle cx="9" cy="10" r="1" fill="currentColor" stroke="none"/><circle cx="15" cy="10" r="1" fill="currentColor" stroke="none"/></svg></span>
              <span class="quick-card-label">部署 QQ 机器人</span>
              <span class="quick-card-desc">安装 NapCat、扫码登录</span>
            </div>
            <div class="quick-card" @click="quickAction('请帮我部署飞书自建应用，对接到 AI Hub。')">
              <span class="quick-card-icon"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M4 4l8 4 8-4"/><path d="M4 4v12l8 4V8z"/><path d="M20 4v12l-8 4V8z"/></svg></span>
              <span class="quick-card-label">部署飞书应用</span>
              <span class="quick-card-desc">创建应用、配置机器人</span>
            </div>
            <div class="quick-card" @click="quickAction('请查看当前系统状态，包括版本、进程、各会话运行情况。')">
              <span class="quick-card-icon"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="12" width="4" height="8" rx="1"/><rect x="10" y="8" width="4" height="12" rx="1"/><rect x="17" y="4" width="4" height="16" rx="1"/></svg></span>
              <span class="quick-card-label">查看系统状态</span>
              <span class="quick-card-desc">版本、进程、会话状态</span>
            </div>
          </div>
        </div>
        <div
          v-for="msg in allMessages"
          :key="msg.id"
          class="message"
          :class="[msg.role, msg.role === 'user' ? 'flex-row-reverse' : 'flex-row']"
        >
          <div class="message-avatar">
            <div v-if="msg.role === 'user'" class="avatar user-avatar">U</div>
            <img v-else :src="getSessionAvatar()" class="avatar ai-avatar-img" />
          </div>
          <div class="message-body">
            <div class="message-header" :class="msg.role === 'user' ? 'text-right' : 'text-left'">
              <span class="message-role">{{ msg.role === 'user' ? '你' : currentSession?.title || 'AI' }}</span>
              <span class="message-time">{{ formatMessageTime(msg.created_at) }}</span>
            </div>
            <!-- Historical steps panel (for assistant messages with metadata) -->
            <div v-if="msg.role === 'assistant' && parseMetadata(msg.metadata)" class="activity-block history-steps">
              <div class="activity-header" @click="historyStepsExpanded[msg.id] = !historyStepsExpanded[msg.id]">
                <svg class="done-check" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                  <path d="M20 6L9 17l-5-5"/>
                </svg>
                <span class="activity-label">
                  {{ parseMetadata(msg.metadata)!.steps.length }} 个步骤
                </span>
                <svg class="chevron" :class="{ expanded: historyStepsExpanded[msg.id] }" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                  <path d="M6 9l6 6 6-6"/>
                </svg>
              </div>
              <div v-if="historyStepsExpanded[msg.id]" class="activity-body">
                <div v-if="parseMetadata(msg.metadata)!.thinking" class="thinking-section">
                  <div class="section-label step-thinking">
                    <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                      <circle cx="12" cy="12" r="10"/><path d="M12 6v6l4 2"/>
                    </svg>
                    思考中
                  </div>
                  <div class="thinking-text">{{ parseMetadata(msg.metadata)!.thinking }}</div>
                </div>
                <div v-for="(step, idx) in parseMetadata(msg.metadata)!.steps.filter(s => s.type === 'tool')" :key="idx" class="tool-item">
                  <div class="tool-header">
                    <span class="tool-status done">
                      <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                        <path d="M20 6L9 17l-5-5"/>
                      </svg>
                    </span>
                    <span class="tool-name" :class="toolColorClass(step.name || '')">{{ localizeToolName(step.name || '') }}</span>
                  </div>
                  <div v-if="step.input" class="tool-input">{{ formatToolInput(step.input) }}</div>
                </div>
              </div>
            </div>
            <div
              v-if="msg.role === 'assistant' && hasMessageContent(msg.content)"
              class="message-content md-content"
              v-html="renderMd(msg.content)"
            ></div>
            <div v-else-if="msg.role !== 'assistant'" class="message-content md-content" v-html="renderMd(msg.content)"></div>
            <!-- Retry button: only for the last user message, always visible -->
            <button
              v-if="msg.role === 'user' && msg.id === lastUserMsgId && !store.streaming"
              class="btn-retry"
              @click="retryMessage(msg.id, msg.content)"
              title="重新发送"
            >
              <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round">
                <polyline points="1 4 1 10 7 10"/>
                <path d="M3.51 15a9 9 0 1 0 .49-3.87"/>
              </svg>
            </button>
            <div v-if="msg.role === 'assistant' && store.tokenUsageMap[msg.id]" class="token-usage">
              <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="4" y="4" width="16" height="16" rx="2"/><circle cx="9" cy="9" r="1.5"/><circle cx="15" cy="9" r="1.5"/><circle cx="9" cy="15" r="1.5"/><circle cx="15" cy="15" r="1.5"/></svg>
              <span>{{ formatUsageLine(store.tokenUsageMap[msg.id]!) }}</span>
            </div>
          </div>
        </div>

        <!-- Activity panel: thinking + tool calls (ABOVE streaming text) -->
        <div v-if="hasActivity" class="message assistant">
          <div class="message-avatar">
            <div class="avatar ai-avatar">
              <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <path d="M12 2L2 7l10 5 10-5-10-5z"/>
                <path d="M2 17l10 5 10-5"/>
                <path d="M2 12l10 5 10-5"/>
              </svg>
            </div>
          </div>
          <div class="message-body">
            <div class="activity-block">
              <div class="activity-header" @click="stepsExpanded = !stepsExpanded">
                <svg class="spin-icon" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                  <path d="M21 12a9 9 0 11-6.219-8.56"/>
                </svg>
                <span class="activity-label">
                  {{ stepCount > 0 ? `${stepCount} 个步骤` : '处理中...' }}
                </span>
                <svg class="chevron" :class="{ expanded: stepsExpanded }" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                  <path d="M6 9l6 6 6-6"/>
                </svg>
              </div>
              <div v-if="stepsExpanded" class="activity-body">
                <div v-if="store.thinkingContent" class="thinking-section">
                  <div class="section-label step-thinking">
                    <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                      <circle cx="12" cy="12" r="10"/><path d="M12 6v6l4 2"/>
                    </svg>
                    思考中
                  </div>
                  <div class="thinking-text">{{ store.thinkingContent }}</div>
                </div>
                <div v-for="tc in store.toolCalls" :key="tc.id" class="tool-item">
                  <div class="tool-header">
                    <span class="tool-status" :class="tc.status">
                      <svg v-if="tc.status === 'running'" class="spin-icon" width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                        <path d="M21 12a9 9 0 11-6.219-8.56"/>
                      </svg>
                      <svg v-else width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                        <path d="M20 6L9 17l-5-5"/>
                      </svg>
                    </span>
                    <span class="tool-name" :class="toolColorClass(tc.name)">{{ localizeToolName(tc.name) }}</span>
                  </div>
                  <div v-if="tc.input" class="tool-input">{{ formatToolInput(tc.input) }}</div>
                </div>
              </div>
            </div>
          </div>
        </div>

        <!-- Streaming message (combines waiting state and content) -->
        <div v-if="store.streaming" class="message assistant flex-row">
          <div class="message-avatar">
            <img :src="getSessionAvatar()" class="avatar ai-avatar-img" />
          </div>
          <div class="message-body">
            <div class="message-header text-left">
              <span class="message-role">{{ currentSession?.title || 'AI' }}</span>
            </div>
            <!-- Show compressing indicator -->
            <div v-if="store.compressing && !store.streamingContent" class="message-content">
              <div class="compressing-indicator">
                <span class="compressing-icon">⏳</span> 正在自动压缩上下文，即将继续回复…
              </div>
            </div>
            <!-- Show recovery indicator (session lost, auto-recovering) -->
            <div v-else-if="store.recovering && !store.streamingContent" class="message-content">
              <div class="compressing-indicator">
                <span class="compressing-icon">🔄</span> 会话已自动刷新，正在恢复上下文…
              </div>
            </div>
            <!-- Show typing indicator when no content yet -->
            <div v-else-if="!store.streamingContent && !store.thinkingContent && store.toolCalls.length === 0" class="message-content">
              <div class="typing-indicator">
                <span></span><span></span><span></span>
              </div>
            </div>
            <!-- Show streaming content when available -->
            <div v-else-if="store.streamingContent" class="message-content md-content" v-html="renderMd(store.streamingContent)"></div>
          </div>
        </div>

      </div>
    </div>

    <div class="input-area">
      <!-- Attachments preview -->
      <div v-if="attachments.length > 0" class="attachments-preview">
        <div v-for="att in attachments" :key="att.id" class="attachment-item">
          <img v-if="att.type === 'image' && att.preview" :src="att.preview" class="attachment-thumb" />
          <div v-else class="attachment-file">
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <path d="M14 2H6a2 2 0 00-2 2v16a2 2 0 002 2h12a2 2 0 002-2V8z"/>
              <polyline points="14 2 14 8 20 8"/>
            </svg>
          </div>
          <span class="attachment-name">{{ att.name }}</span>
          <button class="attachment-remove" @click="removeAttachment(att.id)" title="移除">
            <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <path d="M18 6L6 18M6 6l12 12"/>
            </svg>
          </button>
        </div>
      </div>

      <!-- Pending queue indicator -->
      <div v-if="store.pendingQueueCount > 0" class="queue-panel">
        <div class="queue-header">
          <span class="queue-title">{{ store.pendingQueueCount }} 条消息排队中</span>
          <span class="queue-hint">AI 完成后自动处理</span>
        </div>
      </div>

      <div class="unified-input-container" :class="{ 'is-focused': isInputFocused, 'is-mobile': isMobile }">
        
        <!-- Mobile Left: Plus Button -->
        <button v-if="isMobile" class="mobile-plus-btn" @click="mobilePlusPanelOpen = !mobilePlusPanelOpen" title="更多功能">
          <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <circle cx="12" cy="12" r="10"/>
            <line x1="12" y1="8" x2="12" y2="16"/>
            <line x1="8" y1="12" x2="16" y2="12"/>
          </svg>
        </button>

        <div class="textarea-wrapper">
          <div v-if="showSlashMenu" class="slash-command-menu">
            <div class="slash-menu-header">
              <span>可用命令</span>
              <small v-if="slashLoading">加载中...</small>
              <small v-else>{{ filteredSlashItems.length }} 项</small>
            </div>
            <div v-if="!slashLoading && filteredSlashItems.length === 0" class="slash-menu-empty">没有匹配的命令或技能</div>
            <button
              v-for="(item, idx) in filteredSlashItems"
              :key="item.source + ':' + item.name"
              class="slash-command-item"
              :class="{ active: idx === slashSelectedIndex }"
              @mousedown.prevent="applySlashItem(item)"
            >
              <div class="slash-command-main">
                <span class="slash-command-name">/{{ item.name }}</span>
                <span class="slash-command-type" :class="item.source">{{ slashItemType(item) }}</span>
              </div>
              <div class="slash-command-desc">{{ item.when_to_use || item.description || '无说明' }}</div>
            </button>
            <div class="slash-menu-hint">↑↓ 选择 · Enter/Tab 插入 · Esc 清空</div>
          </div>
          <div class="textarea-with-tags">
            <div v-if="slashTags.length > 0" class="tags-overlay">
              <span v-for="tag in slashTags" :key="tag.id" class="inline-tag" @click="removeSlashTag(tag.id)">
                {{ tag.name }} ×
              </span>
            </div>
            <textarea
              ref="textareaEl"
              v-model="input"
              @keydown="onKeydown"
              @keyup="updateSlashOnCursorMove"
              @click="updateSlashOnCursorMove"
              @input="autoResize"
              @paste="handlePaste"
              @focus="handleInputFocus"
              @blur="handleInputBlur"
              @compositionstart="isComposing = true"
              @compositionend="isComposing = false"
              :placeholder="store.streaming ? 'AI 正在回复，输入的消息将排队等待...' : (isMobile ? '输入消息...' : '输入消息... (可粘贴图片)')"
              rows="1"
              :class="{ 'has-tags': slashTags.length > 0 }"
            />
          </div>
        </div>

        <!-- Mobile Right: Send/Stop -->
        <div v-if="isMobile" class="mobile-send-wrapper">
          <button v-if="store.streaming" class="send-btn stop" @click="store.stopStreaming()" title="停止">
            <svg width="14" height="14" viewBox="0 0 24 24" fill="currentColor"><rect x="6" y="6" width="12" height="12" rx="2"/></svg>
          </button>
          <button v-else class="send-btn" :class="{ 'active': input.trim() || attachments.length > 0 }" :disabled="!input.trim() && attachments.length === 0" @click="send" title="发送">
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M5 12h14M12 5l7 7-7 7"/></svg>
          </button>
        </div>

        <!-- Desktop Toolbar -->
        <div v-if="!isMobile" class="unified-bottom-toolbar">
          <div class="toolbar-left">
            <div class="provider-switcher" v-if="store.currentProvider">
              <button class="model-badge" @click="toggleProviderDropdown" :disabled="store.streaming || store.providerSwitching" title="切换模型">
                <span class="model-name-text">{{ store.currentProvider.model_id }}</span>
                <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M6 9l6 6 6-6"/></svg>
              </button>
              <div v-if="providerDropdownOpen" class="model-dropdown">
                <div v-if="providerDropdownLoading" class="provider-empty">加载模型中...</div>
                <div v-else-if="store.providers.length === 0" class="provider-empty">暂无可用模型，请先到设置里添加供应商</div>
                <button
                  v-for="p in store.providers"
                  :key="p.id"
                  class="provider-option"
                  :class="{ active: String(p.id) === String(store.currentSession?.provider_id) }"
                  @click="onSwitchProvider(String(p.id))"
                >
                  <span class="provider-option-name">{{ p.name }}</span>
                  <span class="provider-option-model">{{ p.model_id }}</span>
                </button>
              </div>
            </div>

            <button class="tool-btn" @click="openFileDialog" title="添加附件">
              <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <path d="M21.44 11.05l-9.19 9.19a6 6 0 01-8.49-8.49l9.19-9.19a4 4 0 015.66 5.66l-9.2 9.19a2 2 0 01-2.83-2.83l8.49-8.48"/>
              </svg>
            </button>
            <input ref="fileInputEl" type="file" accept="image/*" multiple style="display: none" @change="handleFileSelect" />
          </div>

          <div class="toolbar-right">
            <span class="context-badge" @click="openRawRequest" title="查看上下文详情">
              <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <path d="M14 2H6a2 2 0 00-2 2v16a2 2 0 002 2h12a2 2 0 002-2V8z"/>
                <polyline points="14 2 14 8 20 8"/>
                <line x1="16" y1="13" x2="8" y2="13"/>
                <line x1="16" y1="17" x2="8" y2="17"/>
                <polyline points="10 9 9 9 8 9"/>
              </svg>
              {{ contextCount }} 条上下文
            </span>

            <button v-if="store.streaming" class="send-btn stop" @click="store.stopStreaming()" title="停止">
              <svg width="14" height="14" viewBox="0 0 24 24" fill="currentColor"><rect x="6" y="6" width="12" height="12" rx="2"/></svg>
            </button>
            <button v-else class="send-btn" :class="{ 'active': input.trim() || attachments.length > 0 }" :disabled="!input.trim() && attachments.length === 0" @click="send" title="发送">
              <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M5 12h14M12 5l7 7-7 7"/></svg>
            </button>
          </div>
        </div>

        <!-- Mobile Floating Panel -->
        <div v-if="isMobile && mobilePlusPanelOpen" class="mobile-plus-panel">
          <div class="mobile-panel-grid">
            <div class="mobile-panel-item" @click="openFileDialog(); mobilePlusPanelOpen = false">
              <div class="mobile-panel-icon">
                <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                  <path d="M21.44 11.05l-9.19 9.19a6 6 0 01-8.49-8.49l9.19-9.19a4 4 0 015.66 5.66l-9.2 9.19a2 2 0 01-2.83-2.83l8.49-8.48"/>
                </svg>
              </div>
              <span>发图片</span>
            </div>
            <div class="mobile-panel-item" @click="openRawRequest(); mobilePlusPanelOpen = false">
              <div class="mobile-panel-icon">
                <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                  <path d="M14 2H6a2 2 0 00-2 2v16a2 2 0 002 2h12a2 2 0 002-2V8z"/>
                  <polyline points="14 2 14 8 20 8"/>
                  <line x1="16" y1="13" x2="8" y2="13"/>
                  <line x1="16" y1="17" x2="8" y2="17"/>
                  <polyline points="10 9 9 9 8 9"/>
                </svg>
              </div>
              <span>上下文</span>
            </div>
            <div class="mobile-panel-item" @click="toggleProviderDropdown">
              <div class="mobile-panel-icon">
                <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M6 9l6 6 6-6"/></svg>
              </div>
              <span>切换模型</span>
            </div>
          </div>
          <div v-if="providerDropdownOpen" class="mobile-provider-list">
             <div v-if="providerDropdownLoading" class="provider-empty">加载模型中...</div>
             <div v-else-if="store.providers.length === 0" class="provider-empty">暂无可用模型，请先到设置里添加供应商</div>
             <button
                v-for="p in store.providers"
                :key="p.id"
                class="mobile-provider-option"
                :class="{ active: String(p.id) === String(store.currentSession?.provider_id) }"
                @click="onSwitchProvider(String(p.id)); providerDropdownOpen = false; mobilePlusPanelOpen = false"
              >
                <span class="provider-option-name">{{ p.name }}</span>
                <span class="provider-option-model">{{ p.model_id }}</span>
              </button>
          </div>
        </div>
      </div>
    </div>

    <!-- Session Config Drawer -->
    <SessionConfigDrawer v-model:visible="showConfigDrawer" />

    <!-- Raw request drawer -->
    <Teleport to="body">
      <div class="drawer-overlay" :class="{ 'is-visible': showRawRequestModal }" @click="showRawRequestModal = false">
        <div class="drawer-content raw-req-drawer" :class="{ 'is-visible': showRawRequestModal }" @click.stop>
          <div class="drawer-header">
            <div class="drawer-title">
              <span>上下文详情</span>
              <span class="drawer-subtitle">会话 #{{ store.currentSession?.id }}</span>
            </div>
            <button class="btn-close" @click="showRawRequestModal = false">
              <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <path d="M18 6L6 18M6 6l12 12"/>
              </svg>
            </button>
          </div>
          <div v-if="rawRequestLoading" class="raw-req-loading">加载中...</div>
          <template v-else>
            <div class="raw-req-meta">
              <template v-if="rawRequestData && getActualMsgCount(rawRequestData.anthropic_request) !== null">
                <span class="raw-req-meta-actual">实际发送 {{ getActualMsgCount(rawRequestData.anthropic_request) }} 条消息</span>
              </template>
              <template v-else-if="rawRequestData">
                <span>上下文 {{ rawRequestData.context_count }} 条</span>
              </template>
              <template v-else>
                <span>全量日志归档</span>
              </template>
              <template v-if="rawRequestData">
                <span>·</span>
                <span>{{ new Date(rawRequestData.captured_at).toLocaleString('zh-CN') }}</span>
                <template v-if="rawContentSizeKB">
                  <span>·</span>
                  <span class="raw-req-size-badge">Raw {{ rawContentSizeKB }} KB</span>
                </template>
              </template>
            </div>
            <div class="raw-req-tabs">
              <button :class="['raw-req-tab', rawRequestTab === 'messages' && 'active']" @click="rawRequestTab = 'messages'"
                v-if="rawRequestData?.anthropic_request?.messages">
                Messages <span class="raw-req-tab-badge">{{ parsedMessageRows.length }}</span>
              </button>
              <button :class="['raw-req-tab', rawRequestTab === 'fullchat' && 'active']" @click="rawRequestTab = 'fullchat'">
                全量日志 <span v-if="fullChatTotal" class="raw-req-tab-badge">{{ fullChatTotal }}</span>
              </button>
              <button :class="['raw-req-tab', rawRequestTab === 'raw' && 'active']" @click="rawRequestTab = 'raw'"
                v-if="rawRequestData?.anthropic_request?.messages">
                Raw
              </button>
              <button :class="['raw-req-tab', rawRequestTab === 'system' && 'active']" @click="rawRequestTab = 'system'">
                System Prompt
              </button>
              <button :class="['raw-req-tab', rawRequestTab === 'query' && 'active']" @click="rawRequestTab = 'query'">
                Query
              </button>            </div>
            <div class="raw-req-body">
              <template v-if="rawRequestTab === 'messages'">
                <div class="raw-msg-list">
                  <div v-for="row in parsedMessageRows" :key="row.rowIndex"
                    :data-row-index="row.rowIndex"
                    :class="['raw-msg-row',
                             row.type === 'tool_use' && 'raw-msg-row-tool-use',
                             row.type === 'tool_result' && 'raw-msg-row-tool-result',
                             isToolHighlighted(row) && 'tool-highlighted']"
                    @click="toggleRowExpand(row.rowIndex)">
                    <div class="raw-msg-row-header">
                      <span :class="['raw-msg-role-badge', 'role-' + row.role]">{{ row.role }}</span>
                      <span :class="['raw-msg-type-badge', 'type-' + row.type]">{{ row.type }}</span>

                      <!-- tool_use: name badge + id + jump button -->
                      <template v-if="row.type === 'tool_use'">
                        <span class="tool-name-badge">{{ row.toolName }}</span>
                        <span class="tool-id-label" :title="row.toolId">{{ truncateId(row.toolId) }}</span>
                        <button v-if="hasToolPair(row.toolId)" class="tool-jump-btn" @click.stop="jumpToPair('tool_use', row.toolId)" title="跳转到对应 result">
                          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M7 13l5 5 5-5M7 6l5 5 5-5"/></svg>
                        </button>
                      </template>

                      <!-- tool_result: linked name + id + jump button + preview -->
                      <template v-else-if="row.type === 'tool_result'">
                        <span v-if="getLinkedToolName(row.toolUseId)" class="tool-name-badge tool-name-result">{{ getLinkedToolName(row.toolUseId) }}</span>
                        <span class="tool-id-label" :title="row.toolUseId">{{ truncateId(row.toolUseId) }}</span>
                        <button v-if="hasToolPair(row.toolUseId)" class="tool-jump-btn" @click.stop="jumpToPair('tool_result', row.toolUseId)" title="跳转到对应 call">
                          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M7 11l5-5 5 5M7 18l5-5 5 5"/></svg>
                        </button>
                        <span class="raw-msg-preview">{{ row.preview }}</span>
                      </template>

                      <!-- Default: preview text -->
                      <template v-else>
                        <span class="raw-msg-preview">{{ row.preview }}</span>
                      </template>

                      <svg class="raw-msg-chevron" :class="{ 'is-open': expandedRows.has(row.rowIndex) }"
                        width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5">
                        <path d="M6 9l6 6 6-6"/>
                      </svg>
                    </div>

                    <!-- tool_use expanded: structured params card -->
                    <div v-if="row.type === 'tool_use' && expandedRows.has(row.rowIndex)" class="tool-params-card" @click.stop>
                      <div v-if="row.toolInput && Object.keys(row.toolInput).length" class="tool-params-list">
                        <div v-for="(value, key) in row.toolInput" :key="String(key)" class="tool-param-item">
                          <div class="tool-param-key">{{ key }}</div>
                          <pre class="tool-param-value">{{ typeof value === 'string' ? value : JSON.stringify(value, null, 2) }}</pre>
                        </div>
                      </div>
                      <pre v-else class="raw-msg-full-pre">{{ row.full }}</pre>
                    </div>

                    <!-- Other types expanded: raw JSON -->
                    <pre v-else-if="expandedRows.has(row.rowIndex)" class="raw-msg-full-pre" @click.stop>{{ row.full }}</pre>
                  </div>
                </div>
              </template>
              <template v-else-if="rawRequestTab === 'fullchat'">
                <div class="fullchat-search-bar">
                  <input v-model="fullchatSearchQuery" @input="fullchatOnSearch" @keydown.enter="fullchatSearchNext"
                    class="fullchat-search-input" placeholder="搜索日志内容..." />
                  <span class="fullchat-match-info" v-if="fullchatSearchQuery.trim()">{{ fullchatMatchDisplay }}</span>
                  <button class="fullchat-nav-btn" :disabled="!fullchatTotalMatches" @click="fullchatSearchPrev" title="上一个">
                    <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M18 15l-6-6-6 6"/></svg>
                  </button>
                  <button class="fullchat-nav-btn" :disabled="!fullchatTotalMatches" @click="fullchatSearchNext" title="下一个">
                    <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M6 9l6 6 6-6"/></svg>
                  </button>
                </div>
                <div class="fullchat-list" @scroll="onFullChatScroll">
                  <div v-if="fullChatLoading && fullChatMessages.length === 0" class="raw-req-loading">加载中...</div>
                  <template v-else>
                    <div v-for="(msg, fcIdx) in fullChatMessages" :key="msg.id"
                      :data-fc-idx="fcIdx"
                      :class="['fullchat-row', fullchatMatchIndices[fullchatCurrentMatchIdx] === fcIdx && 'fullchat-row-active-match']"
                      @click="toggleFullChatRow(msg.id)">
                      <div class="fullchat-row-header">
                        <span :class="['fullchat-role-badge', msg.role === 'user' ? 'role-user' : 'role-assistant']">
                          {{ msg.role === 'user' ? 'user' : 'assistant' }}
                        </span>
                        <span class="fullchat-id">#{{ msg.id }} · msg#{{ msg.message_id }}</span>
                        <span class="fullchat-time">{{ formatMessageTime(msg.created_at) }}</span>
                        <span class="fullchat-preview" v-if="!expandedFullChatRows.has(msg.id)" v-html="fullchatHighlightPreview(msg.content)"></span>
                        <span class="fullchat-expand-icon">{{ expandedFullChatRows.has(msg.id) ? '▼' : '▶' }}</span>
                      </div>
                      <pre v-if="expandedFullChatRows.has(msg.id)" class="fullchat-full-content" @click.stop v-html="fullchatHighlightContent(msg.content)"></pre>
                    </div>
                    <div v-if="fullChatLoading" class="fullchat-status">加载更多...</div>
                    <div v-else-if="!fullChatHasMore && fullChatMessages.length > 0" class="fullchat-status">已加载全部 {{ fullChatTotal }} 条日志</div>
                  </template>
                </div>
              </template>
              <template v-else-if="rawRequestTab === 'raw' && rawRequestData">
                <pre class="raw-req-pre">{{ formatAnthropicMessages(rawRequestData.anthropic_request) }}</pre>
              </template>
              <template v-else-if="rawRequestData">
                <pre class="raw-req-pre">{{ rawRequestTab === 'system' ? rawRequestData.system_prompt : rawRequestData.query }}</pre>
              </template>            </div>
          </template>
        </div>
      </div>
    </Teleport>

    <!-- Reset Confirm Modal -->
    <Teleport to="body">
      <div v-if="showResetConfirm" class="modal-overlay" @click="showResetConfirm = false">
        <div class="reset-confirm-modal" @click.stop>
          <div class="rules-modal-header">
            <span class="rules-modal-title">重置上下文</span>
            <button class="rules-modal-close" @click="showResetConfirm = false">
              <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <path d="M18 6L6 18M6 6l12 12"/>
              </svg>
            </button>
          </div>
          <div class="reset-confirm-body">
            <p class="reset-warning">此操作将清空当前会话的所有消息记录，但保留会话配置（规则、团队、定时器等）。操作不可逆。</p>
            <div class="reset-option">
              <label>保留最近消息数</label>
              <input
                type="number"
                v-model.number="resetKeepLast"
                min="0"
                max="100"
                placeholder="0 = 全部清空"
                class="reset-input"
              />
              <span class="reset-hint">设为 0 表示清空所有消息</span>
            </div>
          </div>
          <div class="reset-confirm-actions">
            <button class="btn-cancel" @click="showResetConfirm = false">取消</button>
            <button class="btn-danger" @click="executeReset">确认重置</button>
          </div>
        </div>
      </div>
    </Teleport>

    <!-- Context menu (right-click on team member) -->
    <Teleport to="body">
      <div v-if="memberCtxMenu" class="ctx-overlay" @click="closeMemberCtxMenu" @contextmenu.prevent="closeMemberCtxMenu">
        <div class="ctx-menu" :style="{ left: memberCtxMenu.x + 'px', top: memberCtxMenu.y + 'px' }" @click.stop>
          <button class="ctx-item" @click="exportMemberSession(memberCtxMenu.session)">
            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <path d="M21 15v4a2 2 0 01-2 2H5a2 2 0 01-2-2v-4"/>
              <polyline points="17 8 12 3 7 8"/>
              <line x1="12" y1="3" x2="12" y2="15"/>
            </svg>
            <span>导出会话</span>
          </button>
          <button class="ctx-item ctx-danger" @click="closeMemberCtxMenu(); memberDeleteTarget = memberCtxMenu!.session">
            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <path d="M18 6L6 18M6 6l12 12"/>
            </svg>
            <span>删除会话</span>
          </button>
        </div>
      </div>
    </Teleport>

    <!-- Delete member confirmation modal -->
    <Teleport to="body">
      <div v-if="memberDeleteTarget" class="modal-overlay" @click="memberDeleteTarget = null">
        <div class="modal-box" @click.stop>
          <p class="modal-title">确认删除</p>
          <p class="modal-desc">删除成员会话「{{ memberDeleteTarget.title }}」？此操作不可撤销。</p>
          <div class="modal-actions">
            <button class="modal-btn cancel" @click="memberDeleteTarget = null">取消</button>
            <button class="modal-btn confirm" @click="confirmMemberDelete">删除</button>
          </div>
        </div>
      </div>
    </Teleport>

  </div>
</template>

<style scoped>
.chat-panel {
  flex: 1;
  display: flex;
  flex-direction: column;
  min-height: 0;
}
.quota-banner {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 24px;
  background: rgba(245, 158, 11, 0.12);
  border-bottom: 1px solid rgba(245, 158, 11, 0.35);
  font-size: 13px;
  color: var(--warning-text);
}
.quota-banner-icon { font-size: 16px; }
.quota-banner-text { flex: 1; }
.quota-banner-close {
  background: none;
  border: none;
  color: var(--warning-text);
  cursor: pointer;
  font-size: 16px;
  padding: 0 4px;
}
/* Quick action cards */
.quick-actions {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 60px 24px 24px;
  gap: 16px;
}
.quick-actions-title {
  font-size: 16px;
  font-weight: 600;
  color: var(--text-secondary);
  margin-bottom: 8px;
}
.quick-actions-grid {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 12px;
  max-width: 480px;
  width: 100%;
}
.quick-card {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 6px;
  padding: 16px 12px;
  border: 1px solid var(--border);
  border-radius: 10px;
  background: var(--bg-secondary);
  cursor: pointer;
  transition: all 0.15s;
  text-align: center;
}
.quick-card:hover {
  border-color: var(--accent);
  background: var(--bg-primary);
  transform: translateY(-1px);
  box-shadow: 0 2px 8px rgba(0,0,0,0.06);
}
.quick-card-icon { width: 28px; height: 28px; color: var(--accent); display: flex; align-items: center; justify-content: center; }
.quick-card-icon svg { width: 100%; height: 100%; }
.quick-card-label { font-size: 13px; font-weight: 600; color: var(--text-primary); }
.quick-card-desc { font-size: 11px; color: var(--text-secondary); }
/* Chat header */
.chat-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 10px 24px;
  border-bottom: 1px solid var(--border);
  background: var(--bg-secondary);
  flex-shrink: 0;
}
.header-left { min-width: 0; }
.header-title {
  font-size: 14px;
  font-weight: 600;
  color: var(--text-primary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  cursor: pointer;
  border-radius: var(--radius-sm);
  padding: 2px 4px;
  margin: -2px -4px;
  display: flex;
  align-items: center;
  gap: 6px;
}
.header-title:hover {
  background: var(--bg-hover);
}
.health-badge {
  font-size: 10px;
  padding: 1px 6px;
  border-radius: 9999px;
  font-weight: 600;
  flex-shrink: 0;
  line-height: 1.4;
}
.health-green { background: rgba(34,197,94,0.15); color: #22c55e; }
.health-yellow { background: rgba(234,179,8,0.15); color: #eab308; }
.health-red { background: rgba(239,68,68,0.15); color: #ef4444; }
.header-title-input {
  font-size: 14px;
  font-weight: 600;
  color: var(--text-primary);
  background: var(--bg-primary);
  border: 1px solid var(--accent);
  border-radius: var(--radius-sm);
  padding: 2px 4px;
  margin: -2px -4px;
  outline: none;
  width: 100%;
}
.header-team-badge {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  font-size: 11px;
  font-weight: 500;
  color: var(--accent);
  background: var(--accent-soft, rgba(124, 106, 239, 0.1));
  padding: 2px 8px;
  border-radius: 10px;
  white-space: nowrap;
  flex-shrink: 0;
}
.header-team-badge svg {
  opacity: 0.8;
}
.header-workdir {
  font-size: 12px;
  color: var(--text-muted);
  margin-top: 2px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.header-right {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-shrink: 0;
}
.header-context {
  font-size: 12px;
  color: var(--text-muted);
}
.header-context-btn {
  cursor: pointer;
  border-radius: var(--radius-sm);
  padding: 2px 6px;
  transition: background var(--transition), color var(--transition);
}
.header-context-btn:hover {
  background: var(--bg-hover);
  color: var(--text-secondary);
}
.drawer-overlay {
  position: fixed;
  inset: 0;
  background: var(--overlay);
  z-index: 2000;
  opacity: 0;
  pointer-events: none;
  transition: opacity 0.3s ease;
}
.drawer-overlay.is-visible {
  opacity: 1;
  pointer-events: auto;
}
.drawer-content {
  position: fixed;
  top: 0;
  right: 0;
  bottom: 0;
  width: 760px;
  max-width: 100vw;
  background: var(--bg-primary);
  box-shadow: -4px 0 24px rgba(0, 0, 0, 0.1);
  z-index: 2001;
  transform: translateX(100%);
  transition: transform 0.3s cubic-bezier(0.16, 1, 0.3, 1);
  display: flex;
  flex-direction: column;
}
.drawer-content.is-visible {
  transform: translateX(0);
}
.drawer-header {
  padding: 16px 20px;
  border-bottom: 1px solid var(--border);
  display: flex;
  align-items: center;
  justify-content: space-between;
  background: var(--bg-secondary);
}
.drawer-title {
  display: flex;
  align-items: baseline;
  gap: 12px;
  font-size: 16px;
  font-weight: 600;
  color: var(--text-primary);
}
.drawer-subtitle {
  font-size: 12px;
  font-weight: normal;
  color: var(--text-muted);
}
.btn-close {
  background: transparent;
  border: none;
  color: var(--text-muted);
  cursor: pointer;
  padding: 4px;
  border-radius: 6px;
  display: flex;
  align-items: center;
  justify-content: center;
  transition: all 0.2s;
}
.btn-close:hover {
  background: var(--bg-hover);
  color: var(--text-primary);
}
.raw-req-meta {
  display: flex; align-items: center; gap: 8px;
  padding: 8px 20px;
  font-size: 12px; color: var(--text-muted);
  border-bottom: 1px solid var(--border);
}
.raw-req-tabs {
  display: flex; gap: 0;
  padding: 0 20px;
  border-bottom: 1px solid var(--border);
}
.raw-req-tab {
  padding: 8px 16px;
  font-size: 13px; color: var(--text-muted);
  border-bottom: 2px solid transparent;
  margin-bottom: -1px;
  transition: all var(--transition);
}
.raw-req-tab:hover { color: var(--text-primary); }
.raw-req-tab.active { color: var(--accent); border-bottom-color: var(--accent); font-weight: 500; }
.raw-req-tab-badge {
  display: inline-flex; align-items: center; justify-content: center;
  background: var(--accent-soft); color: var(--accent);
  border-radius: 10px; font-size: 10px; font-weight: 600;
  padding: 0 5px; min-width: 16px; height: 16px;
  margin-left: 4px; vertical-align: middle;
}
.raw-req-meta-actual {
  color: var(--accent); font-weight: 500;
}
.raw-req-size-badge {
  font-size: 11px; font-weight: 600; padding: 1px 6px;
  border-radius: 3px; background: rgba(139, 92, 246, 0.1); color: #8b5cf6;
}
.raw-req-body {
  flex: 1; min-height: 0;
  overflow-y: auto;
  padding: 16px 20px;
}
.raw-req-pre {
  margin: 0;
  font-family: 'SF Mono', 'Menlo', 'Monaco', 'Consolas', monospace;
  font-size: 12px; line-height: 1.6;
  color: var(--text-primary);
  white-space: pre-wrap; word-break: break-word;
}
.raw-req-loading {
  padding: 40px 20px;
  text-align: center; color: var(--text-muted); font-size: 13px;
}

/* ===== Full Chat Tab ===== */
.fullchat-list {
  display: flex; flex-direction: column; gap: 4px;
  flex: 1; min-height: 0; overflow-y: auto;
}
.fullchat-row {
  border: 1px solid var(--border); border-radius: 6px;
  padding: 6px 10px; cursor: pointer; font-size: 12px;
}
.fullchat-row:hover { background: var(--bg-hover, var(--bg-tertiary)); }
.fullchat-row-header {
  display: flex; align-items: center; gap: 6px; min-height: 22px;
}
.fullchat-role-badge {
  font-size: 10px; font-weight: 600; padding: 1px 6px;
  border-radius: 3px; text-transform: uppercase; flex-shrink: 0;
}
.fullchat-role-badge.role-user {
  background: rgba(59, 130, 246, 0.12); color: #3b82f6;
}
.fullchat-role-badge.role-assistant {
  background: rgba(34, 197, 94, 0.12); color: #22c55e;
}
.fullchat-id {
  font-size: 10px; color: var(--text-muted); flex-shrink: 0;
}
.fullchat-time {
  font-size: 10px; color: var(--text-muted); flex-shrink: 0; opacity: 0.7;
}
.fullchat-preview {
  flex: 1; overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
  color: var(--text-secondary); font-size: 11px;
}
.fullchat-expand-icon {
  font-size: 10px; color: var(--text-muted); flex-shrink: 0;
}
.fullchat-full-content {
  margin: 6px 0 2px; padding: 8px; font-size: 12px; line-height: 1.5;
  background: var(--bg-secondary); border-radius: 4px;
  white-space: pre-wrap; word-break: break-word; max-height: 400px; overflow-y: auto;
}
.fullchat-status {
  text-align: center; padding: 10px; font-size: 11px; color: var(--text-muted);
}
.fullchat-search-bar {
  display: flex; align-items: center; gap: 6px;
  padding: 0 0 10px; flex-shrink: 0;
}
.fullchat-search-input {
  flex: 1; min-width: 0;
  padding: 6px 10px; border: 1px solid var(--border); border-radius: 6px;
  font-size: 12px; background: var(--bg-primary); color: var(--text-primary);
  outline: none;
}
.fullchat-search-input:focus { border-color: var(--accent); }
.fullchat-match-info {
  font-size: 11px; color: var(--text-muted); white-space: nowrap; min-width: 40px; text-align: center;
}
.fullchat-nav-btn {
  width: 26px; height: 26px; display: flex; align-items: center; justify-content: center;
  border: 1px solid var(--border); border-radius: 4px; background: var(--bg-primary);
  color: var(--text-secondary); cursor: pointer; flex-shrink: 0;
}
.fullchat-nav-btn:hover:not(:disabled) { background: var(--bg-hover, var(--bg-tertiary)); }
.fullchat-nav-btn:disabled { opacity: 0.3; cursor: default; }
.fullchat-row-active-match { border-color: var(--accent) !important; background: rgba(var(--accent-rgb, 59,130,246), 0.06); }
mark.fc-highlight {
  background: rgba(255, 213, 0, 0.35); color: inherit; padding: 0 1px; border-radius: 2px;
}

/* ===== Visual Messages Tab ===== */
.raw-msg-list { display: flex; flex-direction: column; gap: 4px; }
.raw-msg-row {
  border: 1px solid var(--border); border-radius: 6px;
  cursor: pointer; transition: background var(--transition); overflow: hidden;
}
.raw-msg-row:hover { background: var(--bg-hover, var(--bg-tertiary)); }
.raw-msg-row-header {
  display: flex; align-items: center; gap: 8px;
  padding: 7px 12px; min-height: 36px; flex-wrap: nowrap;
}
.raw-msg-role-badge {
  display: inline-flex; align-items: center;
  font-size: 10px; font-weight: 600; text-transform: uppercase;
  padding: 2px 7px; border-radius: 4px; white-space: nowrap; flex-shrink: 0;
}
.raw-msg-role-badge.role-user     { background: #e8f0fe; color: #1a73e8; }
.raw-msg-role-badge.role-assistant{ background: #e6f4ea; color: #1e8e3e; }
.raw-msg-role-badge.role-system   { background: #f3e8fd; color: #8430ce; }
.raw-msg-type-badge {
  display: inline-flex; align-items: center;
  font-size: 10px; font-weight: 600; padding: 2px 7px;
  border-radius: 10px; white-space: nowrap; flex-shrink: 0; color: #fff;
}
.raw-msg-type-badge.type-text         { background: #1a73e8; }
.raw-msg-type-badge.type-tool_use     { background: #f28b00; }
.raw-msg-type-badge.type-tool_result  { background: #34a853; }
.raw-msg-type-badge.type-thinking     { background: #9334e6; }
.raw-msg-type-badge:not(.type-text):not(.type-tool_use):not(.type-tool_result):not(.type-thinking) { background: #5f6368; }
.raw-msg-preview {
  flex: 1; min-width: 0; font-size: 12px; color: var(--text-muted);
  white-space: nowrap; overflow: hidden; text-overflow: ellipsis;
}
.raw-msg-chevron { flex-shrink: 0; color: var(--text-muted); transition: transform 0.18s ease; }
.raw-msg-chevron.is-open { transform: rotate(180deg); }
.raw-msg-full-pre {
  margin: 0; padding: 10px 14px; border-top: 1px solid var(--border);
  font-family: 'SF Mono', 'Menlo', 'Monaco', 'Consolas', monospace;
  font-size: 12px; line-height: 1.6; color: var(--text-primary);
  white-space: pre-wrap; word-break: break-word;
  background: var(--bg-primary); max-height: 320px; overflow-y: auto;
}

/* ===== tool_use / tool_result card enhancements ===== */
.raw-msg-row-tool-use  { border-color: rgba(242, 139, 0, 0.3); }
.raw-msg-row-tool-result { border-color: rgba(52, 168, 83, 0.3); }

/* Highlight animation when jumping between pairs */
.tool-highlighted {
  animation: tool-highlight-pulse 2.5s ease-out;
}
@keyframes tool-highlight-pulse {
  0%   { background: rgba(66, 133, 244, 0.18); box-shadow: 0 0 0 2px rgba(66, 133, 244, 0.4); }
  100% { background: transparent; box-shadow: none; }
}

/* Tool name badge */
.tool-name-badge {
  display: inline-flex; align-items: center;
  font-size: 11px; font-weight: 600;
  padding: 2px 8px; border-radius: 4px;
  background: rgba(242, 139, 0, 0.12); color: #d47700;
  white-space: nowrap; flex-shrink: 0;
}
.tool-name-badge.tool-name-result {
  background: rgba(52, 168, 83, 0.12); color: #1e8e3e;
}

/* Tool ID label (truncated) */
.tool-id-label {
  font-size: 10px; color: var(--text-muted);
  font-family: 'SF Mono', 'Menlo', monospace;
  white-space: nowrap; overflow: hidden; text-overflow: ellipsis;
  max-width: 160px; flex-shrink: 1;
}

/* Jump button */
.tool-jump-btn {
  display: inline-flex; align-items: center; justify-content: center;
  width: 22px; height: 22px; border-radius: 4px; flex-shrink: 0;
  color: var(--accent); background: var(--accent-soft);
  transition: all 0.15s ease;
}
.tool-jump-btn:hover {
  background: var(--accent); color: #fff;
}

/* ===== Structured tool params card ===== */
.tool-params-card {
  border-top: 1px solid var(--border);
  background: var(--bg-primary);
  max-height: 360px; overflow-y: auto;
}
.tool-params-list {
  display: flex; flex-direction: column;
}
.tool-param-item {
  display: flex; flex-direction: column; gap: 2px;
  padding: 8px 14px;
  border-bottom: 1px solid var(--border);
}
.tool-param-item:last-child { border-bottom: none; }
.tool-param-key {
  font-size: 10px; font-weight: 600; text-transform: uppercase;
  color: var(--accent); letter-spacing: 0.5px;
}
.tool-param-value {
  margin: 0;
  font-family: 'SF Mono', 'Menlo', 'Monaco', 'Consolas', monospace;
  font-size: 12px; line-height: 1.5; color: var(--text-primary);
  white-space: pre-wrap; word-break: break-word;
}
.header-token-stats {
  display: flex; align-items: center; gap: 3px;
  font-size: 11px; color: var(--text-muted);
  white-space: nowrap;
}
.btn-rules {
  display: flex;
  align-items: center;
  gap: 4px;
  padding: 4px 10px;
  border-radius: var(--radius);
  font-size: 12px;
  color: var(--text-secondary);
  background: var(--bg-tertiary);
  transition: all var(--transition);
}
.btn-rules:hover {
  background: var(--bg-hover);
  color: var(--text-primary);
}
.btn-compress {
  display: flex;
  align-items: center;
  gap: 4px;
  padding: 4px 10px;
  border-radius: var(--radius);
  font-size: 12px;
  color: var(--text-secondary);
  background: var(--bg-tertiary);
  transition: all var(--transition);
}
.btn-compress:hover:not(:disabled) {
  background: var(--bg-hover);
  color: var(--text-primary);
}
.btn-compress:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}
.btn-reset:hover:not(:disabled) {
  background: rgba(234, 67, 53, 0.1);
  color: #d93025;
}

/* Reset confirm modal */
.reset-confirm-modal {
  background: var(--bg-primary);
  border-radius: var(--radius-lg);
  width: 440px;
  max-width: 95vw;
  box-shadow: 0 20px 60px rgba(0,0,0,0.3);
  overflow: hidden;
}
.reset-confirm-body {
  padding: 20px 24px;
}
.reset-warning {
  color: var(--text-secondary);
  font-size: 13px;
  line-height: 1.6;
  margin: 0 0 16px;
  padding: 10px 12px;
  background: rgba(234, 67, 53, 0.06);
  border-radius: var(--radius);
  border-left: 3px solid #d93025;
}
.reset-option {
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.reset-option label {
  font-size: 12px;
  font-weight: 600;
  color: var(--text-secondary);
}
.reset-input {
  width: 120px;
  padding: 6px 10px;
  border: 1px solid var(--border);
  border-radius: var(--radius);
  background: var(--bg-secondary);
  color: var(--text-primary);
  font-size: 13px;
}
.reset-hint {
  font-size: 11px;
  color: var(--text-muted);
}
.reset-confirm-actions {
  display: flex;
  justify-content: flex-end;
  gap: 10px;
  padding: 14px 24px;
  border-top: 1px solid var(--border);
}
.btn-cancel {
  padding: 6px 16px;
  border-radius: var(--radius);
  font-size: 13px;
  background: var(--bg-tertiary);
  color: var(--text-secondary);
  transition: all var(--transition);
}
.btn-cancel:hover {
  background: var(--bg-hover);
}
.btn-danger {
  padding: 6px 16px;
  border-radius: var(--radius);
  font-size: 13px;
  background: #d93025;
  color: #fff;
  transition: all var(--transition);
}
.btn-danger:hover {
  background: #c62828;
}
.messages {
  flex: 1;
  overflow-y: auto;
  padding: 24px 0;
}
.messages-inner {
  max-width: 720px;
  margin: 0 auto;
  padding: 0 24px;
}
.load-more-hint {
  text-align: center;
  padding: 12px 0;
  color: var(--text-secondary, #888);
  font-size: 13px;
  cursor: pointer;
  user-select: none;
}
.load-more-hint:hover {
  color: var(--text-primary, #333);
}
.load-more-spinner {
  display: inline-block;
  width: 16px;
  height: 16px;
  border: 2px solid var(--text-secondary, #888);
  border-top-color: transparent;
  border-radius: 50%;
  animation: spin 0.6s linear infinite;
}
@keyframes spin {
  to { transform: rotate(360deg); }
}
.message {
  display: flex;
  gap: 12px;
  margin-bottom: 24px;
}
.message.flex-row { flex-direction: row; }
.message.flex-row-reverse { flex-direction: row-reverse; }
.message-avatar { flex-shrink: 0; padding-top: 2px; }
.avatar {
  width: 32px; height: 32px; border-radius: 8px;
  display: flex; align-items: center; justify-content: center;
  font-size: 12px; font-weight: 600;
  box-shadow: 0 1px 3px rgba(0,0,0,0.1);
}
.user-avatar { background: var(--accent); color: white; }
.ai-avatar-img {
  width: 32px; height: 32px; border-radius: 8px;
  object-fit: cover;
  background: var(--bg-secondary);
}
.message-body { flex: 1; min-width: 0; max-width: 85%; }
.message-header { margin-bottom: 4px; }
.message-header.text-right { text-align: right; }
.message-header.text-left { text-align: left; }
.message-role {
  font-size: 11px; font-weight: 600; color: var(--text-muted);
  text-transform: none; letter-spacing: 0;
}
.message-time {
  font-size: 10px; color: var(--text-muted); margin-left: 6px; opacity: 0.6;
}
.message-content {
  font-size: 14px; line-height: 1.7; color: var(--text-primary); word-break: break-word;
  padding: 12px 16px;
  border-radius: 12px;
  box-shadow: 0 1px 2px rgba(0,0,0,0.05);
}
/* User message: purple bubble, right-aligned */
.message.user .message-content {
  background: var(--accent);
  color: white;
  border-radius: 12px 4px 12px 12px;
}
.message.user .message-content :deep(a) { color: white; text-decoration: underline; }
.message.user .message-content :deep(code) { background: rgba(255,255,255,0.2); color: white; }
.message.user .message-content :deep(img),
.message.user .md-content :deep(img) {
  max-width: 300px !important;
  max-height: 200px !important;
  border-radius: 8px;
  object-fit: contain;
  display: block;
  margin: 8px 0;
}
/* AI message: card style, left-aligned */
.message.assistant .message-content {
  background: var(--bg-secondary);
  border: 1px solid var(--border);
  border-radius: 4px 12px 12px 12px;
}
/* AI message images: responsive width */
.message.assistant .message-content :deep(img),
.message.assistant .md-content :deep(img) {
  max-width: 100%;
  height: auto;
  border-radius: 8px;
  display: block;
  margin: 8px 0;
}
/* Retry button on last user message */
.btn-retry {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  margin-top: 6px;
  width: 24px;
  height: 24px;
  color: var(--text-muted);
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  background: transparent;
  cursor: pointer;
  transition: all var(--transition);
}
.btn-retry:hover {
  color: var(--accent);
  border-color: var(--accent);
  background: var(--accent-soft);
}
.token-usage {
  display: flex; align-items: center; gap: 4px; margin-top: 6px;
  font-size: 11px; color: var(--text-muted); user-select: none;
}
.token-usage svg { opacity: 0.6; }
/* Activity block */
.activity-block {
  background: var(--bg-tertiary);
  border: 1px solid var(--border);
  border-radius: var(--radius);
  overflow: hidden;
}
.activity-header {
  display: flex; align-items: center; gap: 8px;
  padding: 10px 14px; font-size: 13px; color: var(--text-secondary);
  cursor: pointer; user-select: none; transition: background var(--transition);
}
.activity-header:hover { background: var(--bg-hover); }
.activity-label { flex: 1; }
.chevron { transition: transform var(--transition); }
.chevron.expanded { transform: rotate(180deg); }
.activity-body {
  padding: 0 14px 12px;
  max-height: 400px;
  overflow-y: auto;
}

/* Thinking */
.thinking-section { margin-bottom: 10px; }
.section-label {
  display: flex; align-items: center; gap: 6px;
  font-size: 11px; font-weight: 600; color: var(--text-muted);
  text-transform: uppercase; letter-spacing: 0.5px; margin-bottom: 6px;
}
.thinking-text {
  font-size: 12px; line-height: 1.5; color: var(--text-muted);
  white-space: pre-wrap; max-height: 150px; overflow-y: auto;
  padding: 8px; background: var(--bg-primary); border-radius: var(--radius-sm);
}

/* Tool calls */
.tool-item {
  padding: 8px 0;
  border-top: 1px solid var(--border);
}
.tool-item:first-child { border-top: none; }
.thinking-section + .tool-item { border-top: 1px solid var(--border); }
.tool-header {
  display: flex; align-items: center; gap: 8px; font-size: 13px;
}
.tool-status { display: flex; align-items: center; flex-shrink: 0; }
.tool-status.running { color: var(--info); }
.tool-status.done { color: var(--success); }
.tool-name {
  font-weight: 600; color: var(--text-primary); font-size: 13px;
}
/* Step color categories */
.step-thinking { color: #8b5cf6; }
.step-file { color: var(--success); }
.step-bash { color: var(--warning); }
.step-search { color: #06b6d4; }
.step-default { color: var(--text-primary); }
/* History steps: done check icon */
.done-check { color: var(--success); }
.history-steps { margin-bottom: 8px; }
.tool-input {
  margin-top: 4px; padding: 6px 8px;
  background: var(--bg-primary); border-radius: var(--radius-sm);
  font-size: 11px; font-family: 'SF Mono', 'Fira Code', monospace;
  color: var(--text-muted); white-space: pre-wrap; word-break: break-all;
  max-height: 80px; overflow-y: auto;
}
/* Animations */
.spin-icon { animation: spin 1s linear infinite; }
@keyframes spin { to { transform: rotate(360deg); } }

/* Typing indicator */
.typing-indicator { display: flex; gap: 4px; padding: 8px 0; }
.typing-indicator span {
  width: 6px; height: 6px; border-radius: 50%;
  background: var(--text-muted); animation: bounce 1.4s ease-in-out infinite;
}
.typing-indicator span:nth-child(2) { animation-delay: 0.16s; }
.typing-indicator span:nth-child(3) { animation-delay: 0.32s; }
@keyframes bounce {
  0%, 80%, 100% { transform: scale(0.6); opacity: 0.4; }
  40% { transform: scale(1); opacity: 1; }
}
.compressing-indicator {
  display: flex; align-items: center; gap: 6px;
  padding: 8px 0; color: var(--text-muted); font-size: 13px;
}
.compressing-icon { animation: spin 1.5s linear infinite; display: inline-block; }
/* Input area */
.input-area {
  padding: 16px 24px 24px;
  border-top: 1px solid var(--border);
  background: var(--bg-primary);
}
.input-row {
  max-width: 720px; margin: 0 auto;
  display: flex; flex-direction: column; gap: 8px;
}
.queue-panel {
  background: var(--bg-secondary); border: 1px solid var(--border); border-radius: var(--radius);
  padding: 8px 10px; font-size: 12px;
}
.queue-header { display: flex; align-items: center; gap: 8px; margin-bottom: 6px; }
.queue-title { font-weight: 600; color: var(--accent); font-size: 11px; }
.queue-hint { font-size: 10px; color: var(--text-muted); }
.queue-items { display: flex; flex-direction: column; gap: 4px; }
.queue-item {
  display: flex; align-items: center; justify-content: space-between; gap: 8px;
  padding: 4px 8px; background: var(--bg-primary); border: 1px solid var(--border); border-radius: 4px;
}
.queue-item-text { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: var(--text-secondary); }
.queue-item-remove {
  width: 18px; height: 18px; display: flex; align-items: center; justify-content: center;
  border-radius: 3px; color: var(--text-muted); flex-shrink: 0; cursor: pointer;
}
.queue-item-remove:hover { color: var(--danger); background: rgba(239,68,68,0.1); }
@media (max-width: 768px) {
  .queue-panel { padding: 6px 8px; }
  .queue-item-text { font-size: 11px; }
}

/* === Unified Input Area === */
.unified-input-container {
  background: transparent;
  border: none;
  border-radius: 0;
  padding: 4px 0 0 0;
  display: flex;
  flex-direction: column;
  gap: 8px;
  position: relative;
  overflow: visible;
}
.unified-input-container.is-focused {
  /* No outline to blend in */
}
[data-theme="dark"] .unified-input-container {
  background: transparent;
  box-shadow: none;
}
[data-theme="dark"] .unified-input-container.is-focused {
  box-shadow: none;
}

.unified-input-container .textarea-wrapper {
  display: flex;
  flex-direction: column;
  flex: 1;
  position: relative;
}

.slash-command-menu {
  position: absolute;
  left: 0;
  right: 0;
  bottom: calc(100% + 10px);
  z-index: 120;
  max-height: min(420px, 55vh);
  overflow-y: auto;
  background: var(--bg-primary);
  border: 1px solid var(--border);
  border-radius: 12px;
  box-shadow: 0 12px 36px rgba(0, 0, 0, 0.16);
  padding: 8px;
}
.slash-menu-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 6px 8px 8px;
  color: var(--text-muted);
  font-size: 12px;
}
.slash-menu-empty {
  padding: 18px 10px;
  text-align: center;
  color: var(--text-muted);
  font-size: 13px;
}
.slash-command-item {
  width: 100%;
  border: none;
  background: transparent;
  color: var(--text-primary);
  text-align: left;
  padding: 9px 10px;
  border-radius: 8px;
  cursor: pointer;
}
.slash-command-item:hover,
.slash-command-item.active {
  background: var(--bg-hover);
}
.slash-command-main {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  margin-bottom: 3px;
}
.slash-command-name {
  font-size: 14px;
  font-weight: 600;
  color: var(--accent);
}
.slash-command-type {
  flex-shrink: 0;
  font-size: 11px;
  color: var(--text-muted);
  border: 1px solid var(--border);
  border-radius: 999px;
  padding: 2px 7px;
}
.slash-command-type.command {
  color: var(--accent);
}
.slash-command-desc {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--text-secondary);
  font-size: 12px;
}
.slash-menu-hint {
  padding: 8px 8px 2px;
  color: var(--text-muted);
  font-size: 11px;
  border-top: 1px solid var(--border);
  margin-top: 6px;
}

.unified-input-container textarea {
  width: 100%;
  resize: none;
  font-size: 15px;
  line-height: 1.6;
  padding: 4px;
  max-height: 240px;
  background: transparent;
  color: var(--text-primary);
  border: none;
  outline: none;
  min-height: 48px;
}
.unified-input-container textarea::placeholder {
  color: var(--text-muted);
}
.unified-input-container textarea.has-tags {
  padding-top: 32px;
}

/* Textarea with tags */
.textarea-with-tags {
  position: relative;
  width: 100%;
}
.tags-overlay {
  position: absolute;
  top: 4px;
  left: 4px;
  right: 4px;
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
  z-index: 1;
}
.inline-tag {
  display: inline-block;
  background: #3b82f6;
  color: #fff;
  padding: 2px 8px;
  border-radius: 4px;
  font-size: 13px;
  font-weight: 500;
  white-space: nowrap;
  cursor: pointer;
  user-select: none;
  transition: background 0.15s;
}
.inline-tag:hover {
  background: #2563eb;
}

.unified-bottom-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0;
  gap: 12px;
}

/* === Mobile Unified Input Area Overrides === */
.unified-input-container.is-mobile {
  flex-direction: row;
  align-items: flex-end;
  padding: 8px 0;
  gap: 10px;
}

.mobile-plus-btn {
  background: transparent;
  border: none;
  color: var(--text-secondary);
  padding: 4px;
  display: flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  border-radius: 50%;
  margin-bottom: 2px;
}
.mobile-plus-btn:hover {
  background: var(--bg-hover);
  color: var(--text-primary);
}

.unified-input-container.is-mobile .textarea-wrapper {
  background: var(--bg-secondary);
  border-radius: 20px;
  padding: 4px 12px;
  min-height: 36px;
  display: flex;
  align-items: stretch;
}

.unified-input-container.is-mobile .slash-command-menu {
  left: -46px;
  right: -46px;
  max-height: 50vh;
}

.unified-input-container.is-mobile textarea {
  min-height: 24px;
  padding: 2px 0;
  margin: 0;
}

.mobile-send-wrapper {
  display: flex;
  align-items: center;
  justify-content: center;
  margin-bottom: 2px;
}

.mobile-plus-panel {
  position: absolute;
  bottom: calc(100% + 12px);
  left: -12px;
  right: -12px;
  background: var(--bg-secondary);
  border-top: 1px solid var(--border);
  border-radius: 16px 16px 0 0;
  padding: 20px 16px;
  box-shadow: 0 -4px 20px rgba(0,0,0,0.08);
  z-index: 100;
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.mobile-panel-grid {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 16px;
}

.mobile-panel-item {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 8px;
  font-size: 12px;
  color: var(--text-secondary);
  cursor: pointer;
}

.mobile-panel-icon {
  width: 48px;
  height: 48px;
  border-radius: 12px;
  background: var(--bg-primary);
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--text-primary);
}

.mobile-provider-list {
  display: flex;
  flex-direction: column;
  gap: 4px;
  max-height: 160px;
  overflow-y: auto;
  background: var(--bg-primary);
  border-radius: 12px;
  padding: 8px;
}

.mobile-provider-option {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 10px 12px;
  border-radius: 8px;
  background: transparent;
  border: none;
  color: var(--text-secondary);
  text-align: left;
}
.mobile-provider-option.active {
  background: var(--accent-soft);
  color: var(--accent);
}
.mobile-provider-option .provider-option-name {
  font-weight: 500;
}
.mobile-provider-option .provider-option-model {
  font-size: 11px;
  color: var(--text-muted);
}

.toolbar-left {
  display: flex;
  align-items: center;
  gap: 6px;
  flex: 1;
  min-width: 0;
}

.toolbar-right {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-shrink: 0;
}

.tool-btn {
  background: transparent;
  border: none;
  color: var(--text-secondary);
  cursor: pointer;
  padding: 6px;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 8px;
  transition: all 0.2s ease;
  flex-shrink: 0;
}
.tool-btn:hover {
  background: var(--bg-hover);
  color: var(--text-primary);
}

.model-badge {
  background: var(--bg-secondary);
  border: 1px solid transparent;
  padding: 4px 10px;
  font-size: 12px;
  font-weight: 500;
  color: var(--text-secondary);
  border-radius: 16px;
  display: flex;
  align-items: center;
  gap: 4px;
  cursor: pointer;
  transition: all 0.2s ease;
  max-width: 100%;
}
.model-badge:hover {
  background: var(--bg-hover);
  color: var(--text-primary);
}
.model-badge svg {
  flex-shrink: 0;
}
.model-name-text {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.model-dropdown {
  position: absolute;
  left: 0;
  bottom: calc(100% + 8px);
  box-shadow: 0 12px 32px rgba(0,0,0,0.15), 0 4px 12px rgba(0,0,0,0.08);
  border: 1px solid rgba(var(--border-rgb, 100,100,100), 0.2);
  border-radius: 12px;
  backdrop-filter: blur(20px);
  -webkit-backdrop-filter: blur(20px);
  background: rgba(var(--bg-primary-rgb, 255,255,255), 0.92);
  padding: 6px;
  min-width: 260px;
  max-width: 360px;
  max-height: 400px;
  overflow-x: hidden;
  overflow-y: auto;
  z-index: 1000;
  display: flex;
  flex-direction: column;
  gap: 2px;
}
[data-theme="dark"] .model-dropdown {
  background: rgba(30,30,30, 0.92);
  border: 1px solid rgba(255,255,255, 0.1);
}
.model-dropdown .provider-option {
  padding: 10px 14px;
  border-radius: 8px;
  font-size: 13px;
  transition: background 0.15s;
  white-space: nowrap;
}
.model-dropdown .provider-option:hover {
  background: var(--bg-hover);
}
.model-dropdown .provider-option.active {
  background: var(--accent-soft);
  color: var(--accent);
}
.model-dropdown .provider-option-name {
  font-weight: 600;
}
.model-dropdown .provider-option-model {
  font-size: 11px;
  color: var(--text-muted);
}
.model-dropdown .provider-empty {
  padding: 16px;
  text-align: center;
  color: var(--text-muted);
  font-size: 13px;
}

.context-badge {
  font-size: 12px;
  color: var(--text-muted);
  display: flex;
  align-items: center;
  gap: 4px;
  cursor: pointer;
  transition: color 0.2s;
  padding: 4px 8px;
  border-radius: 8px;
  white-space: nowrap;
}
.context-badge:hover {
  color: var(--text-primary);
  background: var(--bg-hover);
}

.send-btn {
  width: 32px;
  height: 32px;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 50%;
  border: none;
  cursor: pointer;
  background: rgba(var(--text-primary-rgb, 0,0,0), 0.05);
  color: var(--text-muted);
  transition: all 0.2s ease;
}
[data-theme="dark"] .send-btn {
  background: rgba(255,255,255, 0.1);
}
.send-btn.active {
  background: var(--text-primary);
  color: var(--bg-primary);
}
[data-theme="dark"] .send-btn.active {
  background: #fff;
  color: #000;
}
.send-btn.active:hover {
  transform: scale(1.05);
  opacity: 0.9;
}
.send-btn.stop {
  background: var(--text-primary);
  color: var(--bg-primary);
}

/* Attachments preview */
.attachments-preview {
  max-width: 720px;
  margin: 0 auto 8px;
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}
.attachment-item {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 6px 8px;
  background: var(--bg-secondary);
  border: 1px solid var(--border);
  border-radius: var(--radius);
  max-width: 200px;
}
.attachment-thumb {
  width: 40px;
  height: 40px;
  object-fit: cover;
  border-radius: 4px;
}
.attachment-file {
  width: 40px;
  height: 40px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: var(--bg-tertiary);
  border-radius: 4px;
  color: var(--text-muted);
}
.attachment-name {
  flex: 1;
  font-size: 12px;
  color: var(--text-secondary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.attachment-remove {
  flex-shrink: 0;
  width: 20px;
  height: 20px;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 50%;
  color: var(--text-muted);
  transition: all var(--transition);
  cursor: pointer;
}
.attachment-remove:hover {
  background: rgba(239, 68, 68, 0.1);
  color: var(--danger);
}

/* Rules modal */
.modal-overlay {
  position: fixed; inset: 0;
  background: var(--overlay);
  display: flex; align-items: center; justify-content: center;
  z-index: 1000;
}
.modal-box {
  background: var(--bg-secondary);
  border: 1px solid var(--border);
  border-radius: 12px;
  padding: 24px;
  width: 340px;
  max-width: 90vw;
}
.modal-title {
  font-size: 15px;
  font-weight: 600;
  color: var(--text-primary);
  margin-bottom: 8px;
}
.modal-desc {
  font-size: 13px;
  color: var(--text-secondary);
  margin-bottom: 20px;
  line-height: 1.5;
  word-break: break-all;
}
.modal-actions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}
.modal-btn {
  padding: 6px 16px;
  border-radius: var(--radius);
  font-size: 13px;
  font-weight: 500;
  cursor: pointer;
  transition: all var(--transition);
}
.modal-btn.cancel {
  color: var(--text-secondary);
  background: var(--bg-hover);
}
.modal-btn.cancel:hover {
  color: var(--text-primary);
}
.modal-btn.confirm {
  color: var(--btn-text);
  background: var(--danger, #ef4444);
}
.modal-btn.confirm:hover {
  opacity: 0.9;
}
.rules-modal {
  background: var(--bg-secondary);
  border: 1px solid var(--border);
  border-radius: 12px;
  width: 680px; max-width: 90vw;
  max-height: 80vh;
  display: flex; flex-direction: column;
}
.rules-modal-header {
  display: flex; align-items: center; gap: 12px;
  padding: 16px 20px;
  border-bottom: 1px solid var(--border);
}
.rules-modal-title {
  font-size: 15px; font-weight: 600; color: var(--text-primary);
}
.rules-modal-dir {
  font-size: 12px; color: var(--text-muted); flex: 1;
}
.rules-modal-icon {
  flex-shrink: 0;
}
.rules-modal-title-group {
  display: flex; flex-direction: column; gap: 2px; flex: 1;
}
.rules-modal-close {
  width: 28px; height: 28px;
  display: flex; align-items: center; justify-content: center;
  border-radius: var(--radius-sm);
  color: var(--text-muted);
  transition: all var(--transition);
}
.rules-modal-close:hover {
  color: var(--text-primary); background: var(--bg-hover);
}
.rules-modal-body {
  display: flex; flex: 1; min-height: 0;
}
.rules-file-list {
  width: 160px; border-right: 1px solid var(--border);
  overflow-y: auto; padding: 8px;
}
.rules-file-item {
  padding: 6px 10px; border-radius: var(--radius-sm);
  font-size: 12px; color: var(--text-secondary);
  cursor: pointer; transition: all var(--transition);
  white-space: nowrap; overflow: hidden; text-overflow: ellipsis;
}
.rules-file-item:hover { background: var(--bg-hover); }
.rules-file-item.active {
  background: var(--accent-soft); color: var(--accent);
}
.rules-empty {
  padding: 16px; text-align: center;
  font-size: 12px; color: var(--text-muted);
}
.rules-editor {
  flex: 1; display: flex; flex-direction: column; min-width: 0;
}
.rules-textarea {
  flex: 1; resize: none; padding: 12px 16px;
  font-size: 13px; line-height: 1.6;
  font-family: 'SF Mono', 'Fira Code', monospace;
  background: transparent; color: var(--text-primary);
  min-height: 300px;
}
.rules-textarea::placeholder { color: var(--text-muted); }
.rules-editor-actions {
  padding: 8px 16px; border-top: 1px solid var(--border);
  display: flex; justify-content: flex-end;
}
.btn-save-rule {
  padding: 6px 16px; border-radius: var(--radius);
  font-size: 13px; font-weight: 500;
  background: var(--accent); color: var(--btn-text);
  transition: opacity var(--transition);
}
.btn-save-rule:hover:not(:disabled) { opacity: 0.9; }
.btn-save-rule:disabled { opacity: 0.5; cursor: not-allowed; }
/* Session rules */
.session-rules-body {
  display: flex; flex-direction: column; flex: 1; min-height: 0;
  padding: 16px 20px;
}
.session-group-selector {
  margin-bottom: 16px;
  padding-bottom: 16px;
  border-bottom: 1px solid var(--border);
}
.group-label {
  display: block;
  font-size: 12px;
  font-weight: 500;
  color: var(--text-secondary);
  margin-bottom: 6px;
}
.group-select-row {
  display: flex;
  gap: 8px;
  align-items: center;
}
.group-select {
  flex: 1;
  padding: 8px 12px;
  border-radius: var(--radius);
  border: 1px solid var(--border);
  background: var(--bg-secondary);
  color: var(--text-primary);
  font-size: 13px;
  cursor: pointer;
}
.group-select:focus {
  outline: none;
  border-color: var(--accent);
}
.btn-save-group {
  padding: 8px 16px;
  border-radius: var(--radius);
  background: var(--accent);
  color: white;
  font-size: 13px;
  font-weight: 500;
  white-space: nowrap;
}
.btn-save-group:hover:not(:disabled) {
  opacity: 0.9;
}
.btn-save-group:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}
.session-rules-textarea {
  min-height: 300px;
  border: 1px solid var(--border);
  border-radius: var(--radius);
  background: var(--bg-secondary);
}
.session-rules-actions {
  gap: 8px;
}
.btn-delete-rule {
  padding: 6px 16px; border-radius: var(--radius);
  font-size: 13px; font-weight: 500;
  background: var(--bg-tertiary); color: var(--danger);
  transition: opacity var(--transition);
}
.btn-delete-rule:hover:not(:disabled) { opacity: 0.8; }
.btn-delete-rule:disabled { opacity: 0.4; cursor: not-allowed; }
/* Memory modal */
.memory-modal {
  background: var(--bg-secondary);
  border: 1px solid var(--border);
  border-radius: 12px;
  width: 900px; max-width: 95vw;
  max-height: 80vh;
  display: flex; flex-direction: column;
}
.memory-body {
  display: flex; flex: 1; min-height: 0; overflow: hidden;
}
.memory-sidebar {
  width: 260px; min-width: 200px;
  border-right: 1px solid var(--border);
  display: flex; flex-direction: column;
}
.memory-filter-bar {
  display: flex; gap: 4px; padding: 8px 10px;
  border-bottom: 1px solid var(--border);
  flex-wrap: wrap; align-items: center;
}
.memory-filter-btn {
  padding: 3px 10px; border-radius: 12px;
  font-size: 11px; font-weight: 500;
  background: var(--bg-tertiary); color: var(--text-secondary);
  transition: all var(--transition);
}
.memory-filter-btn.active {
  background: var(--accent); color: var(--btn-text);
}
.memory-add-btn {
  margin-left: auto; width: 24px; height: 24px;
  border-radius: 50%; font-size: 16px; font-weight: 600;
  background: var(--accent); color: var(--btn-text);
  display: flex; align-items: center; justify-content: center;
  line-height: 1;
}
.memory-file-list {
  flex: 1; overflow-y: auto; padding: 4px 0;
}
.memory-file-item {
  padding: 8px 12px; cursor: pointer;
  border-bottom: 1px solid var(--border-light, rgba(128,128,128,0.08));
  transition: background var(--transition);
}
.memory-file-item:hover { background: var(--bg-hover); }
.memory-file-item.active { background: var(--accent-soft); }
.memory-file-name {
  font-size: 12px; font-weight: 500; color: var(--text-primary);
  white-space: nowrap; overflow: hidden; text-overflow: ellipsis;
}
.memory-file-meta {
  display: flex; align-items: center; gap: 6px; margin-top: 3px;
}
.memory-origin {
  font-size: 10px; padding: 1px 6px; border-radius: 8px; font-weight: 500;
}
.memory-origin-session { background: rgba(59,130,246,0.15); color: #3b82f6; }
.memory-origin-team { background: rgba(168,85,247,0.15); color: #a855f7; }
.memory-origin-global { background: rgba(34,197,94,0.15); color: #22c55e; }
.memory-file-time {
  font-size: 10px; color: var(--text-muted);
}
.memory-content {
  flex: 1; display: flex; flex-direction: column; min-width: 0;
}
.memory-textarea {
  min-height: 200px;
}
.memory-textarea[readonly] {
  cursor: default; opacity: 0.85;
}
.memory-actions {
  gap: 8px;
}
.memory-create-header {
  padding: 10px 16px; border-bottom: 1px solid var(--border);
}
.memory-filename-input {
  width: 100%; padding: 6px 10px;
  font-size: 13px; border-radius: var(--radius);
  background: var(--bg-tertiary); color: var(--text-primary);
  border: 1px solid var(--border);
}
@media (max-width: 640px) {
  .memory-modal { width: 98vw; max-height: 90vh; }
  .memory-sidebar { width: 100%; border-right: none; border-bottom: 1px solid var(--border); max-height: 40vh; }
  .memory-body { flex-direction: column; }
}
/* Toast */
.toast {
  position: fixed;
  top: 20px;
  left: 50%;
  transform: translateX(-50%);
  padding: 8px 20px;
  border-radius: 8px;
  font-size: 13px;
  font-weight: 500;
  z-index: 2000;
  animation: toast-in 0.2s ease;
}
.toast.success {
  background: var(--success);
  color: var(--btn-text);
}
.toast.error {
  background: var(--danger);
  color: var(--btn-text);
}
@keyframes toast-in {
  from { opacity: 0; transform: translateX(-50%) translateY(-10px); }
  to { opacity: 1; transform: translateX(-50%) translateY(0); }
}
/* Provider switcher */
.header-sub-row { display: flex; align-items: center; gap: 8px; margin-top: 2px; }
.provider-switcher { position: relative; }
.provider-badge {
  display: flex; align-items: center; gap: 4px;
  font-size: 11px; color: var(--accent); background: var(--accent-soft);
  padding: 1px 8px; border-radius: var(--radius-sm); cursor: pointer;
  transition: all var(--transition); white-space: nowrap;
}
.provider-badge:hover:not(:disabled) { opacity: 0.8; }
.provider-badge:disabled { opacity: 0.5; cursor: not-allowed; }
.provider-dropdown {
  position: absolute; left: 0; top: 100%; margin-top: 4px;
  background: var(--bg-secondary); border: 1px solid var(--border);
  border-radius: var(--radius); box-shadow: 0 4px 12px rgba(0,0,0,0.15);
  min-width: 200px; z-index: 100; overflow: hidden;
}
.provider-option {
  display: flex; align-items: center; justify-content: space-between; gap: 8px;
  width: 100%; padding: 8px 12px; font-size: 12px; color: var(--text-secondary);
  transition: all var(--transition); text-align: left;
}
.provider-option:hover { background: var(--bg-hover); color: var(--text-primary); }
.provider-option.active { color: var(--accent); background: var(--accent-soft); }
.provider-option-name { font-weight: 500; }
.provider-option-model { font-size: 11px; color: var(--text-muted); }
.provider-empty {
  padding: 12px;
  color: var(--text-muted);
  font-size: 12px;
  text-align: center;
  white-space: nowrap;
}
/* More menu extras */
.more-menu-divider { height: 1px; background: var(--border); margin: 4px 0; }
.more-menu-label { padding: 6px 14px; font-size: 11px; color: var(--text-muted); font-weight: 600; }
.more-menu-active { color: var(--accent) !important; }
/* Hamburger button */
.btn-hamburger {
  display: flex; align-items: center; justify-content: center;
  width: 36px; height: 36px; border-radius: var(--radius);
  color: var(--text-secondary); transition: all var(--transition); flex-shrink: 0;
}
.btn-hamburger:hover { background: var(--bg-hover); color: var(--text-primary); }
.header-title-group { min-width: 0; flex: 1; }
/* More menu */
.header-right-mobile {
  display: flex; align-items: center; gap: 8px; flex-shrink: 0;
}
.more-menu-wrapper { position: relative; }
.btn-more {
  display: flex; align-items: center; justify-content: center;
  width: 32px; height: 32px; border-radius: var(--radius);
  color: var(--text-secondary); transition: all var(--transition);
}
.btn-more:hover { background: var(--bg-hover); }
.more-menu {
  position: absolute; right: 0; top: 100%; margin-top: 4px;
  background: var(--bg-secondary); border: 1px solid var(--border);
  border-radius: var(--radius); box-shadow: var(--shadow);
  min-width: 140px; z-index: 100; overflow: hidden;
}
.more-menu button {
  display: block; width: 100%; text-align: left;
  padding: 10px 14px; font-size: 13px; color: var(--text-secondary);
  transition: all var(--transition);
}
.more-menu button:hover { background: var(--bg-hover); color: var(--text-primary); }
.more-menu button:disabled { opacity: 0.4; cursor: not-allowed; }

/* Team Members Bar */
.team-members-bar {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 24px;
  background: var(--bg-secondary);
  border-bottom: 1px solid var(--border);
  overflow-x: auto;
  flex-shrink: 0;
}
.team-members-bar::-webkit-scrollbar {
  height: 4px;
}
.team-members-bar::-webkit-scrollbar-thumb {
  background: var(--border);
  border-radius: 4px;
}
.team-member-item {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 4px 10px;
  border-radius: 16px;
  background: var(--bg-primary);
  border: 1px solid var(--border);
  cursor: pointer;
  transition: all 0.2s ease;
  white-space: nowrap;
  flex-shrink: 0;
}
.team-member-item:hover {
  background: var(--bg-hover);
  border-color: var(--text-muted);
}
.team-member-item.active {
  background: var(--accent-soft);
  border-color: var(--accent);
  color: var(--accent);
}
.team-member-item.active .member-name {
  color: var(--accent);
  font-weight: 600;
}
.member-avatar {
  width: 18px;
  height: 18px;
  border-radius: 4px;
  flex-shrink: 0;
}
.member-name {
  font-size: 12px;
  color: var(--text-secondary);
  max-width: 120px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  line-height: 1.2;
}
.member-btn-delete {
  opacity: 0.5;
  width: 20px;
  height: 20px;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 4px;
  color: var(--text-muted);
  transition: all var(--transition);
  background: none;
  border: none;
  cursor: pointer;
  padding: 0;
  margin-left: 2px;
}
.team-member-item:hover .member-btn-delete {
  opacity: 1;
}
.member-btn-delete:hover {
  color: var(--danger);
  background: rgba(239, 68, 68, 0.1);
}

/* Mobile styles */
@media (max-width: 768px) {
  .chat-header { padding: 8px 12px; gap: 8px; }
  .header-left { display: flex; align-items: center; gap: 8px; flex: 1; min-width: 0; }
  .header-title { font-size: 13px; }
  .header-workdir { display: none; }
  .provider-switcher { display: none; }
  .header-sub-row { margin-top: 0; }
  .team-members-bar { padding: 8px 12px; }
  .messages { padding: 12px 0; }
  .messages-inner { padding: 0 12px; }
  .quick-actions { padding: 32px 12px 12px; }
  .quick-actions-grid { grid-template-columns: 1fr; max-width: 100%; }
  .input-area { padding: 8px 12px 12px; padding-bottom: calc(12px + env(safe-area-inset-bottom)); }
  .input-wrapper { border-radius: var(--radius); }
  .input-wrapper textarea { font-size: 16px; }
  .rules-modal { width: 100vw; max-width: 100vw; height: 100vh; height: 100dvh; max-height: 100vh; max-height: 100dvh; border-radius: 0; }
  .rules-modal-body { flex-direction: column; }
  .rules-file-list { width: 100%; border-right: none; border-bottom: 1px solid var(--border); max-height: 120px; overflow-y: auto; display: flex; flex-wrap: wrap; padding: 6px; gap: 4px; }
  .rules-file-item { white-space: nowrap; }
  .message { gap: 8px; margin-bottom: 16px; }
}


</style>
