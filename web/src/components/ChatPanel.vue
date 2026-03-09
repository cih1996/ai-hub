<script setup lang="ts">
import { ref, nextTick, watch, computed, inject, onMounted } from 'vue'
import type { Ref } from 'vue'
import { marked } from 'marked'
import { useChatStore } from '../stores/chat'
import * as api from '../composables/api'
import type { StepsMetadata, Message } from '../types'

const isMobile = inject<Ref<boolean>>('isMobile', ref(false))
const openSidebar = inject<() => void>('openSidebar', () => {})

const store = useChatStore()
const input = ref('')
const messagesEl = ref<HTMLElement>()
const textareaEl = ref<HTMLTextAreaElement>()
const stepsExpanded = ref(false)
const isComposing = ref(false)
const moreMenuOpen = ref(false)
const providerDropdownOpen = ref(false)
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

// Toast state
const toastMsg = ref('')
const toastType = ref<'success' | 'error'>('success')
const toastVisible = ref(false)
let toastTimer: ReturnType<typeof setTimeout>

function quickAction(message: string) {
  store.sendMessage(message)
}

function showToast(msg: string, type: 'success' | 'error' = 'success') {
  toastMsg.value = msg
  toastType.value = type
  toastVisible.value = true
  clearTimeout(toastTimer)
  toastTimer = setTimeout(() => { toastVisible.value = false }, 2500)
}

// Toggle attention mode
async function toggleAttention() {
  const session = store.currentSession
  if (!session) return
  try {
    const newState = !session.attention_enabled
    await api.toggleAttention(session.id, newState)
    session.attention_enabled = newState
    showToast(newState ? '注意力模式已开启' : '注意力模式已关闭', 'success')
  } catch (e: unknown) {
    showToast('切换失败: ' + (e instanceof Error ? e.message : String(e)), 'error')
  }
}

// Open attention rules modal (right-click or long-press on attention button)
async function openAttentionRulesModal() {
  const session = store.currentSession
  if (!session) return
  attentionRulesLoading.value = true
  showAttentionRulesModal.value = true
  try {
    const res = await api.getAttentionRules(session.id)
    // Load system rules (read-only)
    systemActivationRule.value = res.system_activation_rule || ''
    systemReviewRule.value = res.system_review_rule || ''
    // Load custom rules (editable)
    activationCustom.value = res.activation_custom || ''
    reviewCustom.value = res.review_custom || ''
  } catch (e: unknown) {
    showToast('加载注意力规则失败: ' + (e instanceof Error ? e.message : String(e)), 'error')
    systemActivationRule.value = ''
    systemReviewRule.value = ''
    activationCustom.value = ''
    reviewCustom.value = ''
  } finally {
    attentionRulesLoading.value = false
  }
}

// Save attention rules (new v2 format)
async function saveAttentionRules() {
  const session = store.currentSession
  if (!session) return
  attentionRulesSaving.value = true
  try {
    await api.updateAttentionRulesV2(session.id, activationCustom.value, reviewCustom.value)
    // Update local session state
    const rulesData = { activation_custom: activationCustom.value, review_custom: reviewCustom.value }
    session.attention_rules = JSON.stringify(rulesData)
    showToast('注意力规则已保存', 'success')
    showAttentionRulesModal.value = false
  } catch (e: unknown) {
    showToast('保存失败: ' + (e instanceof Error ? e.message : String(e)), 'error')
  } finally {
    attentionRulesSaving.value = false
  }
}

// Clear attention rules
async function clearAttentionRules() {
  const session = store.currentSession
  if (!session) return
  attentionRulesSaving.value = true
  try {
    await api.updateAttentionRulesV2(session.id, '', '')
    activationCustom.value = ''
    reviewCustom.value = ''
    session.attention_rules = ''
    showToast('注意力规则已清除', 'success')
  } catch (e: unknown) {
    showToast('清除失败: ' + (e instanceof Error ? e.message : String(e)), 'error')
  } finally {
    attentionRulesSaving.value = false
  }
}

// Long press handlers for mobile
function onAttentionTouchStart() {
  longPressTriggered.value = false
  longPressTimer = setTimeout(() => {
    longPressTriggered.value = true
    openAttentionRulesModal()
  }, 500)
}

function onAttentionTouchEnd() {
  if (longPressTimer) {
    clearTimeout(longPressTimer)
    longPressTimer = null
  }
}

function onAttentionClick() {
  // If long press was triggered, don't toggle
  if (longPressTriggered.value) {
    longPressTriggered.value = false
    return
  }
  toggleAttention()
}

// Session rules modal state
const showSessionRulesModal = ref(false)
const sessionRulesContent = ref('')
const sessionRulesSaving = ref(false)
const sessionRulesLoading = ref(false)

// Attention rules modal state
const showAttentionRulesModal = ref(false)
const attentionRulesSaving = ref(false)
const attentionRulesLoading = ref(false)
// New v2 structure: system rules (read-only) + custom rules (editable)
const systemActivationRule = ref('')
const systemReviewRule = ref('')
const activationCustom = ref('')
const reviewCustom = ref('')
// Long press support for mobile
let longPressTimer: ReturnType<typeof setTimeout> | null = null
const longPressTriggered = ref(false)

// Memory modal state
const showMemoryModal = ref(false)
const memoryLoading = ref(false)
const memoryFiles = ref<api.VectorFileRich[]>([])
const memoryLevelFilter = ref<'all' | 'session' | 'team' | 'global'>('all')
const memorySelectedFile = ref<api.VectorFileRich | null>(null)
const memoryFileContent = ref('')
const memoryFileLoading = ref(false)
const memoryFileSaving = ref(false)
const memoryEditing = ref(false)
const memoryCreating = ref(false)
const memoryNewFileName = ref('')

// Raw request modal state
const showRawRequestModal = ref(false)
const rawRequestLoading = ref(false)
const rawRequestData = ref<{
  system_prompt: string
  query: string
  context_count: number
  captured_at: string
  anthropic_request?: api.AnthropicRequest
} | null>(null)
const rawRequestTab = ref<'messages' | 'fullchat' | 'raw' | 'system' | 'query'>('system')
// Track which rows are expanded in the visual Messages tab
const expandedRows = ref<Set<number>>(new Set())

// Full chat history state (lazy-loaded in fullchat tab)
const fullChatMessages = ref<Message[]>([])
const fullChatHasMore = ref(false)
const fullChatTotal = ref(0)
const fullChatLoading = ref(false)
const fullChatLoaded = ref(false)
const expandedFullChatRows = ref<Set<number>>(new Set())

async function loadFullChat() {
  const sid = store.currentSession?.id
  if (!sid || fullChatLoading.value) return
  fullChatLoading.value = true
  try {
    const beforeId = fullChatMessages.value.length > 0
      ? fullChatMessages.value[fullChatMessages.value.length - 1]!.id
      : undefined
    const res = await api.getMessagesPaginated(sid, 30, beforeId)
    // API returns ASC order within the batch; we want newest-first display,
    // so reverse each batch and append (older messages go to the end)
    const batch = [...res.messages].reverse()
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

function previewText(text: string, len: number): string {
  const clean = stripErrorTags(text).replace(/\n/g, ' ')
  return clean.length > len ? clean.slice(0, len) + '…' : clean
}

// Format the complete Anthropic API request body for the Raw tab.
// Displays the entire POST body (model, max_tokens, tools, temperature, etc.)
function formatAnthropicMessages(req: api.AnthropicRequest | undefined): string {
  if (!req) return ''
  return JSON.stringify(req, null, 2)
}

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
  expandedFullChatRows.value = new Set()
  try {
    rawRequestData.value = await api.getLastRawRequest(sid)
    // Default to Messages tab if proxy data is available (it's the most informative)
    rawRequestTab.value = rawRequestData.value?.anthropic_request?.messages ? 'messages' : 'system'
  } catch {
    rawRequestData.value = null
    rawRequestTab.value = 'system'
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

// Vector engine health banner
const vectorHealthy = ref(true)
const vectorError = ref('')
const vectorFixing = ref(false)

onMounted(async () => {
  try {
    const h = await api.vectorHealth()
    vectorHealthy.value = h.ready
    if (!h.ready) vectorError.value = h.error || h.fix_hint || '向量引擎未就绪'
  } catch {
    // API not available, skip banner
  }
})

async function fixVectorEngine() {
  vectorFixing.value = true
  try {
    await api.sendChat(0, '请执行系统自检，重点检查向量引擎状态。确保模型已下载到 ~/.ai-hub/models/，向量引擎正常运行。', undefined, '你是 AI Hub 系统维护专家。全自动修复，不要询问用户。修复完成后汇报结果。')
    vectorError.value = '正在自动修复，请在新会话中查看进度...'
  } catch (e: any) {
    vectorError.value = '修复启动失败: ' + e.message
  }
  vectorFixing.value = false
}

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

const displayWorkDir = computed(() => {
  const wd = store.currentSession?.work_dir
  if (!wd) return '~ (系统默认)'
  const home = '~'
  // Try to shorten with ~ prefix
  return wd.replace(/^\/Users\/[^/]+/, home)
})

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
  if (n >= 1000000) return (n / 1000000).toFixed(1) + 'M'
  if (n >= 1000) return (n / 1000).toFixed(1) + 'K'
  return String(n)
}

function formatUsageLine(u: { input_tokens: number; output_tokens: number; cache_creation_input_tokens?: number; cache_read_input_tokens?: number }): string {
  let parts = [`${u.input_tokens.toLocaleString()} in`]
  if (u.cache_creation_input_tokens) parts.push(`${formatTokenNum(u.cache_creation_input_tokens)} cache_w`)
  if (u.cache_read_input_tokens) parts.push(`${formatTokenNum(u.cache_read_input_tokens)} cache_r`)
  parts.push(`${u.output_tokens.toLocaleString()} out`)
  return parts.join(' / ')
}

function send() {
  const text = input.value.trim()
  if (!text || store.streaming) return
  store.sendMessage(text)
  input.value = ''
  stepsExpanded.value = false
  autoResize()
}

async function onSwitchProvider(providerId: string) {
  providerDropdownOpen.value = false
  await store.switchProviderForSession(providerId)
}

function onKeydown(e: KeyboardEvent) {
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

// Session rules functions
async function openSessionRulesModal() {
  const sid = store.currentSession?.id
  if (!sid) return
  showSessionRulesModal.value = true
  sessionRulesLoading.value = true
  try {
    const res = await api.getSessionRules(sid)
    sessionRulesContent.value = res.content || ''
  } catch {
    sessionRulesContent.value = ''
  } finally {
    sessionRulesLoading.value = false
  }
}

async function saveSessionRules() {
  const sid = store.currentSession?.id
  if (!sid) return
  sessionRulesSaving.value = true
  try {
    await api.putSessionRules(sid, sessionRulesContent.value)
    showToast('保存成功')
  } catch (e: any) {
    showToast('保存失败: ' + (e.message || '未知错误'), 'error')
  } finally {
    sessionRulesSaving.value = false
  }
}

async function deleteSessionRules() {
  const sid = store.currentSession?.id
  if (!sid) return
  await api.deleteSessionRules(sid)
  sessionRulesContent.value = ''
}

// Memory functions
const filteredMemoryFiles = computed(() => {
  if (memoryLevelFilter.value === 'all') return memoryFiles.value
  return memoryFiles.value.filter(f => f.origin === memoryLevelFilter.value)
})

async function openMemoryModal() {
  const sid = store.currentSession?.id
  if (!sid) return
  showMemoryModal.value = true
  memorySelectedFile.value = null
  memoryFileContent.value = ''
  memoryEditing.value = false
  memoryCreating.value = false
  await loadMemoryFiles()
}

async function loadMemoryFiles() {
  const sid = store.currentSession?.id
  if (!sid) return
  memoryLoading.value = true
  try {
    const res = await api.listVectorFilesRich('', { session_id: sid, level: 'all' })
    memoryFiles.value = res.files || []
  } catch {
    memoryFiles.value = []
  } finally {
    memoryLoading.value = false
  }
}

async function selectMemoryFile(file: api.VectorFileRich) {
  memorySelectedFile.value = file
  memoryEditing.value = false
  memoryCreating.value = false
  memoryFileLoading.value = true
  try {
    const res = await api.readVectorFile(file.scope, file.file_name)
    memoryFileContent.value = res.content || ''
  } catch {
    memoryFileContent.value = ''
  } finally {
    memoryFileLoading.value = false
  }
}

async function saveMemoryFile() {
  if (!memorySelectedFile.value) return
  memoryFileSaving.value = true
  try {
    await api.writeVectorFile(memorySelectedFile.value.scope, memorySelectedFile.value.file_name, memoryFileContent.value)
    showToast('保存成功')
    memoryEditing.value = false
    await loadMemoryFiles()
  } catch (e: any) {
    showToast('保存失败: ' + (e.message || '未知错误'), 'error')
  } finally {
    memoryFileSaving.value = false
  }
}

async function deleteMemoryFile() {
  if (!memorySelectedFile.value) return
  if (!confirm(`确定删除「${memorySelectedFile.value.file_name}」？`)) return
  try {
    await api.deleteVectorFile(memorySelectedFile.value.scope, memorySelectedFile.value.file_name)
    showToast('已删除')
    memorySelectedFile.value = null
    memoryFileContent.value = ''
    await loadMemoryFiles()
  } catch (e: any) {
    showToast('删除失败: ' + (e.message || '未知错误'), 'error')
  }
}

function startCreateMemory() {
  memoryCreating.value = true
  memoryEditing.value = false
  memorySelectedFile.value = null
  memoryNewFileName.value = ''
  memoryFileContent.value = ''
}

async function createMemoryFile() {
  const sid = store.currentSession?.id
  if (!sid || !memoryNewFileName.value.trim()) return
  let fileName = memoryNewFileName.value.trim()
  if (!fileName.endsWith('.md')) fileName += '.md'
  memoryFileSaving.value = true
  try {
    // Pass session_id with empty scope, let backend auto-resolve session-level scope
    // (handles both team sessions and standalone sessions via _standalone fallback)
    const res = await api.writeVectorFile('', fileName, memoryFileContent.value, sid)
    showToast('创建成功')
    memoryCreating.value = false
    await loadMemoryFiles()
    // Select the newly created file using the scope returned by backend
    const newFile = memoryFiles.value.find(f => f.file_name === fileName && f.scope === res.scope)
    if (newFile) selectMemoryFile(newFile)
  } catch (e: any) {
    showToast('创建失败: ' + (e.message || '未知错误'), 'error')
  } finally {
    memoryFileSaving.value = false
  }
}

function memoryOriginLabel(origin: string): string {
  switch (origin) {
    case 'session': return '会话'
    case 'team': return '团队'
    case 'global': return '全局'
    default: return origin
  }
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
    <!-- Vector engine health banner -->
    <div v-if="!vectorHealthy" class="vector-banner">
      <span class="vector-banner-icon">⚠️</span>
      <span class="vector-banner-text">向量引擎未就绪：{{ vectorError }}</span>
      <button class="vector-banner-btn" :disabled="vectorFixing" @click="fixVectorEngine">
        {{ vectorFixing ? '修复中...' : '一键修复' }}
      </button>
      <button class="vector-banner-close" @click="vectorHealthy = true">✕</button>
    </div>
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
          <div v-else class="header-title" @click="startEditTitle" title="点击编辑标题">{{ store.currentSession.title }}</div>
          <div class="header-sub-row">
            <div class="header-workdir">{{ displayWorkDir }}</div>
            <div class="provider-switcher" v-if="store.currentProvider">
              <button class="provider-badge" @click="providerDropdownOpen = !providerDropdownOpen" :disabled="store.streaming || store.providerSwitching" title="切换模型">
                {{ store.currentProvider.name }} · {{ store.currentProvider.model_id }}
                <svg width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M6 9l6 6 6-6"/></svg>
              </button>
              <div v-if="providerDropdownOpen" class="provider-dropdown">
                <button
                  v-for="p in store.providers"
                  :key="p.id"
                  class="provider-option"
                  :class="{ active: String(p.id) === String(store.currentSession.provider_id) }"
                  @click="onSwitchProvider(String(p.id))"
                >
                  <span class="provider-option-name">{{ p.name }}</span>
                  <span class="provider-option-model">{{ p.model_id }}</span>
                </button>
              </div>
            </div>
          </div>
        </div>
      </div>
      <div class="header-right" v-if="!isMobile">
        <span v-if="sessionTokenStats" class="header-token-stats" title="本会话累计 Token 用量">
          <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="4" y="4" width="16" height="16" rx="2"/><circle cx="9" cy="9" r="1.5"/><circle cx="15" cy="9" r="1.5"/><circle cx="9" cy="15" r="1.5"/><circle cx="15" cy="15" r="1.5"/></svg>
          {{ formatTokenNum(sessionTokenStats.total_input_tokens + sessionTokenStats.total_output_tokens + (sessionTokenStats.total_cache_creation_tokens || 0) + (sessionTokenStats.total_cache_read_tokens || 0)) }}
        </span>
        <span class="header-context header-context-btn" @click="openRawRequest" title="查看最后一次原始请求">{{ contextCount }} 条上下文</span>
        <button
          class="btn-compress"
          @click="store.compressContext()"
          :disabled="store.streaming"
          title="压缩上下文（重置 CLI 会话，保留最近对话摘要）"
        >
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <polyline points="4 14 10 14 10 20"/>
            <polyline points="20 10 14 10 14 4"/>
            <line x1="14" y1="10" x2="21" y2="3"/>
            <line x1="3" y1="21" x2="10" y2="14"/>
          </svg>
          压缩
        </button>
        <button
          class="btn-rules"
          @click="openSessionRulesModal"
          title="会话规则（角色设定）"
        >
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M20 21v-2a4 4 0 00-4-4H8a4 4 0 00-4 4v2"/>
            <circle cx="12" cy="7" r="4"/>
          </svg>
          角色
        </button>
        <button
          class="btn-rules"
          @click="openMemoryModal"
          title="记忆库"
        >
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2z"/>
            <path d="M12 6v6l4 2"/>
          </svg>
          记忆
        </button>
      </div>
      <!-- Mobile: more menu -->
      <div v-if="isMobile" class="header-right-mobile">
        <span v-if="sessionTokenStats" class="header-token-stats" title="Token">
          <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="4" y="4" width="16" height="16" rx="2"/><circle cx="9" cy="9" r="1.5"/><circle cx="15" cy="9" r="1.5"/><circle cx="9" cy="15" r="1.5"/><circle cx="15" cy="15" r="1.5"/></svg>
          {{ formatTokenNum(sessionTokenStats.total_input_tokens + sessionTokenStats.total_output_tokens + (sessionTokenStats.total_cache_creation_tokens || 0) + (sessionTokenStats.total_cache_read_tokens || 0)) }}
        </span>
        <div class="more-menu-wrapper">
          <button class="btn-more" @click="moreMenuOpen = !moreMenuOpen">
            <svg width="18" height="18" viewBox="0 0 24 24" fill="currentColor">
              <circle cx="12" cy="5" r="2"/><circle cx="12" cy="12" r="2"/><circle cx="12" cy="19" r="2"/>
            </svg>
          </button>
          <div v-if="moreMenuOpen" class="more-menu" @click="moreMenuOpen = false">
            <button @click="store.compressContext()" :disabled="store.streaming">压缩上下文</button>
            <button @click="openSessionRulesModal">会话角色</button>
            <button @click="openMemoryModal">记忆库</button>
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
            <div class="quick-card" @click="quickAction('请查看当前系统状态，包括版本、进程、向量引擎、各会话运行情况。')">
              <span class="quick-card-icon"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="12" width="4" height="8" rx="1"/><rect x="10" y="8" width="4" height="12" rx="1"/><rect x="17" y="4" width="4" height="16" rx="1"/></svg></span>
              <span class="quick-card-label">查看系统状态</span>
              <span class="quick-card-desc">版本、进程、引擎状态</span>
            </div>
          </div>
        </div>
        <div
          v-for="msg in allMessages"
          :key="msg.id"
          class="message"
          :class="msg.role"
        >
          <div class="message-avatar">
            <div v-if="msg.role === 'user'" class="avatar user-avatar">U</div>
            <div v-else class="avatar ai-avatar">
              <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <path d="M12 2L2 7l10 5 10-5-10-5z"/>
                <path d="M2 17l10 5 10-5"/>
                <path d="M2 12l10 5 10-5"/>
              </svg>
            </div>
          </div>
          <div class="message-body">
            <div class="message-role">{{ msg.role === 'user' ? 'You' : 'AI' }}</div>
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
              v-if="msg.role === 'assistant'"
              class="message-content md-content"
              v-html="renderMd(msg.content)"
            />
            <div v-else class="message-content">{{ msg.content }}</div>
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

        <!-- Streaming text content (BELOW activity) -->
        <div v-if="store.streaming && store.streamingContent" class="message assistant">
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
            <div class="message-role">AI</div>
            <div class="message-content md-content" v-html="renderMd(store.streamingContent)" />
          </div>
        </div>

        <!-- Pure waiting state -->
        <div v-if="store.streaming && !store.streamingContent && !store.thinkingContent && store.toolCalls.length === 0" class="message assistant">
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
            <div class="typing-indicator">
              <span></span><span></span><span></span>
            </div>
          </div>
        </div>

      </div>
    </div>

    <div class="input-area">
      <div class="input-row">
        <div class="input-wrapper" :class="{ disabled: store.streaming, 'attention-active': store.currentSession?.attention_enabled }">
          <button
            class="btn-attention"
            :class="{ active: store.currentSession?.attention_enabled, 'has-rules': store.currentSession?.attention_rules }"
            @click="onAttentionClick"
            @contextmenu.prevent="openAttentionRulesModal"
            @touchstart.prevent="onAttentionTouchStart"
            @touchend="onAttentionTouchEnd"
            @touchcancel="onAttentionTouchEnd"
            :title="store.currentSession?.attention_enabled ? '关闭注意力模式（右键/长按配置规则）' : '开启注意力模式（右键/长按配置规则）'"
          >
            <svg class="attention-icon" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <circle cx="12" cy="12" r="10"/>
              <circle cx="12" cy="12" r="6"/>
              <circle cx="12" cy="12" r="2"/>
              <line x1="12" y1="2" x2="12" y2="4"/>
              <line x1="12" y1="20" x2="12" y2="22"/>
              <line x1="2" y1="12" x2="4" y2="12"/>
              <line x1="20" y1="12" x2="22" y2="12"/>
            </svg>
          </button>
          <textarea
            ref="textareaEl"
            v-model="input"
            :disabled="store.streaming"
            @keydown="onKeydown"
            @input="autoResize"
            @compositionstart="isComposing = true"
            @compositionend="isComposing = false"
            :placeholder="store.streaming ? 'AI is responding...' : (store.currentSession?.attention_enabled ? '注意力模式：AI 会先规划再执行...' : 'Type a message... (Shift+Enter for new line)')"
            rows="1"
          />
          <div class="input-actions">
            <button v-if="store.streaming" class="btn-stop" @click="store.stopStreaming()" title="Stop">
              <svg width="16" height="16" viewBox="0 0 24 24" fill="currentColor">
                <rect x="6" y="6" width="12" height="12" rx="2"/>
              </svg>
            </button>
            <button v-else class="btn-send" :disabled="!input.trim()" @click="send" title="Send">
              <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <path d="M22 2L11 13M22 2l-7 20-4-9-9-4 20-7z"/>
              </svg>
            </button>
          </div>
        </div>
      </div>
    </div>

    <!-- Session rules modal -->
    <Teleport to="body">
      <div v-if="showSessionRulesModal" class="modal-overlay" @click="showSessionRulesModal = false">
        <div class="rules-modal" @click.stop>
          <div class="rules-modal-header">
            <span class="rules-modal-title">会话规则</span>
            <span class="rules-modal-dir">会话 #{{ store.currentSession?.id }}</span>
            <button class="rules-modal-close" @click="showSessionRulesModal = false">
              <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <path d="M18 6L6 18M6 6l12 12"/>
              </svg>
            </button>
          </div>
          <div class="session-rules-body">
            <div v-if="sessionRulesLoading" class="rules-empty">加载中...</div>
            <template v-else>
              <textarea
                v-model="sessionRulesContent"
                class="rules-textarea session-rules-textarea"
                placeholder="输入会话角色规则（Markdown 格式）...&#10;&#10;例如：&#10;你是一名测试工程师，负责..."
              />
              <div class="rules-editor-actions session-rules-actions">
                <button
                  class="btn-delete-rule"
                  @click="deleteSessionRules"
                  :disabled="!sessionRulesContent"
                >
                  清除
                </button>
                <button
                  class="btn-save-rule"
                  :disabled="sessionRulesSaving"
                  @click="saveSessionRules"
                >
                  {{ sessionRulesSaving ? '保存中...' : '保存' }}
                </button>
              </div>
            </template>
          </div>
        </div>
      </div>
    </Teleport>

    <!-- Attention rules modal (v2: dual panel) -->
    <Teleport to="body">
      <div v-if="showAttentionRulesModal" class="modal-overlay" @click="showAttentionRulesModal = false">
        <div class="rules-modal attention-rules-modal-v2" @click.stop>
          <div class="rules-modal-header">
            <span class="rules-modal-title">注意力规则配置</span>
            <span class="rules-modal-dir">会话 #{{ store.currentSession?.id }}</span>
            <button class="rules-modal-close" @click="showAttentionRulesModal = false">
              <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <path d="M18 6L6 18M6 6l12 12"/>
              </svg>
            </button>
          </div>
          <div class="attention-rules-body-v2">
            <div v-if="attentionRulesLoading" class="rules-empty">加载中...</div>
            <template v-else>
              <!-- Activation Rules Panel -->
              <div class="attention-panel">
                <div class="attention-panel-header">
                  <span class="attention-panel-title">激活规则</span>
                  <span class="attention-panel-desc">用户发消息时注入，要求 AI 输出结构化计划</span>
                </div>
                <div class="attention-system-rule">
                  <div class="attention-system-label">系统内置（只读）</div>
                  <div class="attention-system-content">{{ systemActivationRule }}</div>
                </div>
                <div class="attention-custom-rule">
                  <div class="attention-custom-label">自定义补充</div>
                  <textarea
                    v-model="activationCustom"
                    class="attention-custom-textarea"
                    placeholder="补充激活规则（可选）...&#10;例如：&#10;- 涉及数据库操作时必须说明影响范围&#10;- 修改配置文件前列出所有变更项"
                  />
                </div>
              </div>

              <!-- Review Rules Panel -->
              <div class="attention-panel">
                <div class="attention-panel-header">
                  <span class="attention-panel-title">审核规则</span>
                  <span class="attention-panel-desc">检测到计划后触发，独立 AI 审核是否符合规则</span>
                </div>
                <div class="attention-system-rule">
                  <div class="attention-system-label">系统内置（只读）</div>
                  <div class="attention-system-content">{{ systemReviewRule }}</div>
                </div>
                <div class="attention-custom-rule">
                  <div class="attention-custom-label">自定义补充</div>
                  <textarea
                    v-model="reviewCustom"
                    class="attention-custom-textarea"
                    placeholder="补充审核规则（可选）...&#10;例如：&#10;- 禁止删除生产数据库&#10;- 必须检查是否遗漏单元测试"
                  />
                </div>
              </div>

              <div class="rules-editor-actions attention-rules-actions">
                <button
                  class="btn-delete-rule"
                  @click="clearAttentionRules"
                  :disabled="(!activationCustom && !reviewCustom) || attentionRulesSaving"
                >
                  清除全部
                </button>
                <button
                  class="btn-save-rule"
                  :disabled="attentionRulesSaving"
                  @click="saveAttentionRules"
                >
                  {{ attentionRulesSaving ? '保存中...' : '保存' }}
                </button>
              </div>
            </template>
          </div>
        </div>
      </div>
    </Teleport>

    <!-- Memory modal -->
    <Teleport to="body">
      <div v-if="showMemoryModal" class="modal-overlay" @click="showMemoryModal = false">
        <div class="memory-modal" @click.stop>
          <div class="rules-modal-header">
            <span class="rules-modal-title">记忆库</span>
            <span class="rules-modal-dir">会话 #{{ store.currentSession?.id }}</span>
            <button class="rules-modal-close" @click="showMemoryModal = false">
              <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <path d="M18 6L6 18M6 6l12 12"/>
              </svg>
            </button>
          </div>
          <div class="memory-body">
            <!-- Left: file list -->
            <div class="memory-sidebar">
              <div class="memory-filter-bar">
                <button
                  v-for="lv in (['all', 'session', 'team', 'global'] as const)"
                  :key="lv"
                  :class="['memory-filter-btn', { active: memoryLevelFilter === lv }]"
                  @click="memoryLevelFilter = lv"
                >{{ lv === 'all' ? '全部' : memoryOriginLabel(lv) }}</button>
                <button class="memory-add-btn" @click="startCreateMemory" title="新建记忆">+</button>
              </div>
              <div v-if="memoryLoading" class="rules-empty">加载中...</div>
              <div v-else-if="filteredMemoryFiles.length === 0" class="rules-empty">暂无记忆文件</div>
              <div v-else class="memory-file-list">
                <div
                  v-for="f in filteredMemoryFiles"
                  :key="f.scope + '/' + f.file_name"
                  :class="['memory-file-item', { active: memorySelectedFile?.file_name === f.file_name && memorySelectedFile?.scope === f.scope }]"
                  @click="selectMemoryFile(f)"
                >
                  <div class="memory-file-name">{{ f.file_name }}</div>
                  <div class="memory-file-meta">
                    <span :class="'memory-origin memory-origin-' + f.origin">{{ memoryOriginLabel(f.origin) }}</span>
                    <span class="memory-file-time">{{ f.updated_at ? new Date(f.updated_at).toLocaleDateString() : '' }}</span>
                  </div>
                </div>
              </div>
            </div>
            <!-- Right: content -->
            <div class="memory-content">
              <template v-if="memoryCreating">
                <div class="memory-create-header">
                  <input
                    v-model="memoryNewFileName"
                    class="memory-filename-input"
                    placeholder="文件名（如：工作总结.md）"
                  />
                </div>
                <textarea
                  v-model="memoryFileContent"
                  class="rules-textarea memory-textarea"
                  placeholder="输入记忆内容..."
                />
                <div class="rules-editor-actions memory-actions">
                  <button class="btn-delete-rule" @click="memoryCreating = false">取消</button>
                  <button
                    class="btn-save-rule"
                    :disabled="!memoryNewFileName.trim() || memoryFileSaving"
                    @click="createMemoryFile"
                  >{{ memoryFileSaving ? '创建中...' : '创建' }}</button>
                </div>
              </template>
              <template v-else-if="memorySelectedFile">
                <div v-if="memoryFileLoading" class="rules-empty">加载中...</div>
                <template v-else>
                  <textarea
                    v-model="memoryFileContent"
                    class="rules-textarea memory-textarea"
                    :readonly="!memoryEditing"
                    :placeholder="memoryEditing ? '编辑记忆内容...' : ''"
                  />
                  <div class="rules-editor-actions memory-actions">
                    <button class="btn-delete-rule" @click="deleteMemoryFile">删除</button>
                    <template v-if="memoryEditing">
                      <button class="btn-delete-rule" @click="memoryEditing = false; selectMemoryFile(memorySelectedFile!)">取消</button>
                      <button
                        class="btn-save-rule"
                        :disabled="memoryFileSaving"
                        @click="saveMemoryFile"
                      >{{ memoryFileSaving ? '保存中...' : '保存' }}</button>
                    </template>
                    <button v-else class="btn-save-rule" @click="memoryEditing = true">编辑</button>
                  </div>
                </template>
              </template>
              <div v-else class="rules-empty">← 选择一个记忆文件查看</div>
            </div>
          </div>
        </div>
      </div>
    </Teleport>

    <!-- Raw request modal -->
    <Teleport to="body">
      <div v-if="showRawRequestModal" class="modal-overlay" @click="showRawRequestModal = false">
        <div class="raw-req-modal" @click.stop>
          <div class="rules-modal-header">
            <span class="rules-modal-title">原始请求</span>
            <span class="rules-modal-dir">会话 #{{ store.currentSession?.id }}</span>
            <button class="rules-modal-close" @click="showRawRequestModal = false">
              <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <path d="M18 6L6 18M6 6l12 12"/>
              </svg>
            </button>
          </div>
          <div v-if="rawRequestLoading" class="raw-req-loading">加载中...</div>
          <div v-else-if="!rawRequestData" class="raw-req-loading">暂无数据（请先发送一条消息）</div>
          <template v-else>
            <div class="raw-req-meta">
              <template v-if="getActualMsgCount(rawRequestData.anthropic_request) !== null">
                <span class="raw-req-meta-actual">实际发送 {{ getActualMsgCount(rawRequestData.anthropic_request) }} 条消息</span>
              </template>
              <template v-else>
                <span>上下文 {{ rawRequestData.context_count }} 条</span>
              </template>
              <span>·</span>
              <span>{{ new Date(rawRequestData.captured_at).toLocaleString('zh-CN') }}</span>
            </div>
            <div class="raw-req-tabs">
              <button :class="['raw-req-tab', rawRequestTab === 'messages' && 'active']" @click="rawRequestTab = 'messages'"
                v-if="rawRequestData.anthropic_request?.messages">
                Messages <span class="raw-req-tab-badge">{{ parsedMessageRows.length }}</span>
              </button>
              <button :class="['raw-req-tab', rawRequestTab === 'fullchat' && 'active']" @click="rawRequestTab = 'fullchat'">
                完整对话 <span v-if="fullChatTotal" class="raw-req-tab-badge">{{ fullChatTotal }}</span>
              </button>
              <button :class="['raw-req-tab', rawRequestTab === 'raw' && 'active']" @click="rawRequestTab = 'raw'"
                v-if="rawRequestData.anthropic_request?.messages">
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
                <div class="fullchat-list" @scroll="onFullChatScroll">
                  <div v-if="fullChatLoading && fullChatMessages.length === 0" class="raw-req-loading">加载中...</div>
                  <template v-else>
                    <div v-for="msg in fullChatMessages" :key="msg.id"
                      class="fullchat-row" @click="toggleFullChatRow(msg.id)">
                      <div class="fullchat-row-header">
                        <span :class="['fullchat-role-badge', msg.role === 'user' ? 'role-user' : 'role-assistant']">
                          {{ msg.role === 'user' ? 'user' : 'assistant' }}
                        </span>
                        <span class="fullchat-id">#{{ msg.id }}</span>
                        <span class="fullchat-preview" v-if="!expandedFullChatRows.has(msg.id)">{{ previewText(msg.content, 60) }}</span>
                        <span class="fullchat-expand-icon">{{ expandedFullChatRows.has(msg.id) ? '▼' : '▶' }}</span>
                      </div>
                      <pre v-if="expandedFullChatRows.has(msg.id)" class="fullchat-full-content" @click.stop>{{ stripErrorTags(msg.content) }}</pre>
                    </div>
                    <div v-if="fullChatLoading" class="fullchat-status">加载更多...</div>
                    <div v-else-if="!fullChatHasMore && fullChatMessages.length > 0" class="fullchat-status">已加载全部 {{ fullChatTotal }} 条消息</div>
                  </template>
                </div>
              </template>
              <template v-else-if="rawRequestTab === 'raw'">
                <pre class="raw-req-pre">{{ formatAnthropicMessages(rawRequestData.anthropic_request) }}</pre>
              </template>
              <template v-else>
                <pre class="raw-req-pre">{{ rawRequestTab === 'system' ? rawRequestData.system_prompt : rawRequestData.query }}</pre>
              </template>            </div>
          </template>
        </div>
      </div>
    </Teleport>

    <!-- Toast -->
    <Teleport to="body">
      <div v-if="toastVisible" class="toast" :class="toastType">{{ toastMsg }}</div>
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
/* Vector health banner */
.vector-banner {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 24px;
  background: var(--warning-bg);
  border-bottom: 1px solid var(--warning-border);
  font-size: 13px;
  color: var(--warning-text);
}
.vector-banner-icon { font-size: 16px; }
.vector-banner-text { flex: 1; }
.vector-banner-btn {
  padding: 4px 12px;
  border: 1px solid var(--warning-border);
  border-radius: 4px;
  background: var(--bg-primary);
  color: var(--warning-text);
  cursor: pointer;
  font-size: 12px;
  white-space: nowrap;
}
.vector-banner-btn:hover { background: var(--warning-border); color: var(--btn-text); }
.vector-banner-btn:disabled { opacity: 0.6; cursor: not-allowed; }
.vector-banner-close {
  background: none;
  border: none;
  color: var(--warning-text);
  cursor: pointer;
  font-size: 16px;
  padding: 0 4px;
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
}
.header-title:hover {
  background: var(--bg-hover);
}
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
.raw-req-modal {
  background: var(--bg-secondary);
  border: 1px solid var(--border);
  border-radius: 12px;
  width: 760px; max-width: 92vw;
  max-height: 84vh;
  display: flex; flex-direction: column;
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
  max-height: 60vh; overflow-y: auto;
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
  display: flex; align-items: center; gap: 4px;
  font-size: 12px; color: var(--accent);
  background: var(--accent-soft); padding: 2px 8px;
  border-radius: var(--radius-sm); white-space: nowrap;
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
.message-avatar { flex-shrink: 0; padding-top: 2px; }
.avatar {
  width: 28px; height: 28px; border-radius: 50%;
  display: flex; align-items: center; justify-content: center;
  font-size: 12px; font-weight: 600;
}
.user-avatar { background: var(--bg-tertiary); color: var(--text-secondary); }
.ai-avatar { background: var(--accent-soft); color: var(--accent); }
.message-body { flex: 1; min-width: 0; }
.message-role {
  font-size: 12px; font-weight: 600; color: var(--text-secondary);
  margin-bottom: 4px; text-transform: uppercase; letter-spacing: 0.5px;
}
.message-content {
  font-size: 14px; line-height: 1.7; color: var(--text-primary); word-break: break-word;
}
.message.user .message-content { white-space: pre-wrap; }
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
/* Input area */
.input-area {
  padding: 16px 24px 24px;
  border-top: 1px solid var(--border);
  background: var(--bg-primary);
}
.input-row {
  max-width: 720px; margin: 0 auto;
  display: flex; align-items: flex-end;
}
.btn-attention {
  flex-shrink: 0;
  position: relative;
  width: 32px; height: 32px;
  display: flex; align-items: center; justify-content: center;
  border-radius: var(--radius);
  background: transparent;
  border: none;
  transition: all var(--transition);
  cursor: pointer;
  margin-right: 4px;
}
.btn-attention:hover {
  background: var(--accent-soft);
}
.btn-attention.active {
  background: linear-gradient(135deg, #f59e0b 0%, #ef4444 100%);
  border-radius: var(--radius);
}
.btn-attention .attention-icon {
  color: var(--text-secondary);
  transition: color var(--transition);
}
.btn-attention:hover .attention-icon {
  color: var(--accent);
}
.btn-attention.active .attention-icon {
  color: white;
}
.input-wrapper {
  flex: 1;
  display: flex; align-items: flex-end; gap: 4px;
  background: var(--bg-secondary); border: 1px solid var(--border);
  border-radius: var(--radius-lg); padding: 8px 12px;
  transition: border-color var(--transition), box-shadow var(--transition);
}
.input-wrapper:focus-within { border-color: var(--accent); }
.input-wrapper.attention-active {
  border-color: #f59e0b;
  box-shadow: 0 0 0 1px rgba(245, 158, 11, 0.2);
}
.input-wrapper.disabled { opacity: 0.7; }
.input-wrapper.disabled textarea { cursor: not-allowed; }
.input-wrapper textarea {
  flex: 1; resize: none; font-size: 14px; line-height: 1.5;
  padding: 4px 0; max-height: 200px;
  background: transparent; color: var(--text-primary);
}
.input-wrapper textarea::placeholder { color: var(--text-muted); }
.input-actions { flex-shrink: 0; display: flex; align-items: center; }
.btn-send, .btn-stop {
  width: 32px; height: 32px;
  display: flex; align-items: center; justify-content: center;
  border-radius: var(--radius); transition: all var(--transition);
}
.btn-send { color: var(--accent); }
.btn-send:hover:not(:disabled) { background: var(--accent-soft); }
.btn-send:disabled { color: var(--text-muted); cursor: not-allowed; }
.btn-stop { color: var(--danger); }
.btn-stop:hover { background: rgba(239, 68, 68, 0.1); }
/* Rules modal */
.modal-overlay {
  position: fixed; inset: 0;
  background: var(--overlay);
  display: flex; align-items: center; justify-content: center;
  z-index: 1000;
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
}
.session-rules-textarea {
  min-height: 300px;
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
/* Attention rules modal */
.attention-rules-modal {
  width: 600px; max-width: 95vw;
}
.attention-rules-body {
  display: flex; flex-direction: column; flex: 1; min-height: 0; padding: 16px;
}
.attention-rules-hint {
  font-size: 13px; color: var(--text-secondary);
  margin-bottom: 12px; line-height: 1.5;
  padding: 10px 12px;
  background: var(--bg-tertiary);
  border-radius: var(--radius);
  border-left: 3px solid #f59e0b;
}
.attention-rules-textarea {
  min-height: 250px;
}
.attention-rules-actions {
  gap: 8px;
}
.btn-attention.has-rules::after {
  content: '';
  position: absolute;
  top: 4px; right: 4px;
  width: 6px; height: 6px;
  background: #f59e0b;
  border-radius: 50%;
}
/* Attention rules modal v2 (dual panel) */
.attention-rules-modal-v2 {
  width: 800px; max-width: 95vw;
  max-height: 85vh;
}
.attention-rules-body-v2 {
  padding: 16px 20px;
  overflow-y: auto;
  display: flex; flex-direction: column; gap: 20px;
}
.attention-panel {
  background: var(--bg-tertiary);
  border-radius: var(--radius-lg);
  padding: 16px;
  border: 1px solid var(--border);
}
.attention-panel-header {
  display: flex; align-items: baseline; gap: 12px;
  margin-bottom: 12px;
  padding-bottom: 8px;
  border-bottom: 1px solid var(--border);
}
.attention-panel-title {
  font-size: 14px; font-weight: 600;
  color: var(--text-primary);
}
.attention-panel-desc {
  font-size: 12px; color: var(--text-muted);
}
.attention-system-rule {
  margin-bottom: 12px;
}
.attention-system-label {
  font-size: 11px; font-weight: 500;
  color: var(--text-muted);
  margin-bottom: 6px;
  text-transform: uppercase;
  letter-spacing: 0.5px;
}
.attention-system-content {
  background: var(--bg-secondary);
  border: 1px solid var(--border);
  border-radius: var(--radius);
  padding: 10px 12px;
  font-size: 12px; line-height: 1.5;
  color: var(--text-secondary);
  white-space: pre-wrap;
  max-height: 120px;
  overflow-y: auto;
}
.attention-custom-rule {
  margin-top: 8px;
}
.attention-custom-label {
  font-size: 11px; font-weight: 500;
  color: var(--accent);
  margin-bottom: 6px;
}
.attention-custom-textarea {
  width: 100%;
  min-height: 80px;
  padding: 10px 12px;
  font-size: 13px; line-height: 1.5;
  background: var(--bg-primary);
  border: 1px solid var(--border);
  border-radius: var(--radius);
  color: var(--text-primary);
  resize: vertical;
}
.attention-custom-textarea:focus {
  outline: none;
  border-color: var(--accent);
}
.attention-custom-textarea::placeholder {
  color: var(--text-muted);
}
/* Memory modal -->
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
/* Mobile styles */
@media (max-width: 768px) {
  .chat-header { padding: 8px 12px; gap: 8px; }
  .header-left { display: flex; align-items: center; gap: 8px; flex: 1; min-width: 0; }
  .header-title { font-size: 13px; }
  .header-workdir { display: none; }
  .provider-switcher { display: none; }
  .header-sub-row { margin-top: 0; }
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
