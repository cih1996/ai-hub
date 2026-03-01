<script setup lang="ts">
import { ref, nextTick, watch, computed, inject, onMounted } from 'vue'
import type { Ref } from 'vue'
import { marked } from 'marked'
import { useChatStore } from '../stores/chat'
import * as api from '../composables/api'
import type { FileItem } from '../composables/api'
import type { StepsMetadata } from '../types'

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

// Rules modal state
const showRulesModal = ref(false)
const ruleFiles = ref<FileItem[]>([])
const selectedRulePath = ref('')
const ruleContent = ref('')
const ruleSaving = ref(false)

// Session rules modal state
const showSessionRulesModal = ref(false)
const sessionRulesContent = ref('')
const sessionRulesSaving = ref(false)
const sessionRulesLoading = ref(false)

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
    await api.sendChat(0, '请执行系统自检，重点修复向量引擎问题。确保 Python3、pip、sentence-transformers 已安装，向量引擎正常运行。', undefined, '你是 AI Hub 系统维护专家。全自动修复，不要询问用户。修复完成后汇报结果。')
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

function renderMd(text: string): string {
  return marked.parse(text) as string
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

function scrollToBottom() {
  nextTick(() => {
    if (messagesEl.value) {
      messagesEl.value.scrollTop = messagesEl.value.scrollHeight
    }
  })
}

watch(() => allMessages.value.length, scrollToBottom)
watch(() => store.streamingContent, scrollToBottom)
watch(() => store.thinkingContent, scrollToBottom)
watch(() => store.toolCalls.length, scrollToBottom)

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

// Rules modal functions
async function openRulesModal() {
  const wd = store.currentSession?.work_dir
  if (!wd) return
  showRulesModal.value = true
  ruleFiles.value = await api.listProjectRules(wd)
  if (ruleFiles.value.length > 0 && ruleFiles.value[0]) {
    await selectRule(ruleFiles.value[0].path)
  }
}

async function selectRule(path: string) {
  const wd = store.currentSession?.work_dir
  if (!wd) return
  selectedRulePath.value = path
  const res = await api.readProjectRule(wd, path)
  ruleContent.value = res.content
}

async function saveRule() {
  const wd = store.currentSession?.work_dir
  if (!wd || !selectedRulePath.value) return
  ruleSaving.value = true
  try {
    await api.writeProjectRule(wd, selectedRulePath.value, ruleContent.value)
    showToast('保存成功')
  } catch (e: any) {
    showToast('保存失败: ' + (e.message || '未知错误'), 'error')
  } finally {
    ruleSaving.value = false
  }
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
              <button class="provider-badge" @click="providerDropdownOpen = !providerDropdownOpen" :disabled="store.streaming" title="切换模型">
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
        <span class="header-context">{{ contextCount }} 条上下文</span>
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
          v-if="store.currentSession.work_dir"
          class="btn-rules"
          @click="openRulesModal"
          title="项目规则"
        >
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M14 2H6a2 2 0 00-2 2v16a2 2 0 002 2h12a2 2 0 002-2V8z"/>
            <polyline points="14 2 14 8 20 8"/>
          </svg>
          规则
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
            <button v-if="store.currentSession.work_dir" @click="openRulesModal">项目规则</button>
            <button @click="openSessionRulesModal">会话角色</button>
            <div class="more-menu-divider"></div>
            <div class="more-menu-label">切换模型</div>
            <button
              v-for="p in store.providers"
              :key="p.id"
              :class="{ 'more-menu-active': String(p.id) === String(store.currentSession.provider_id) }"
              :disabled="store.streaming"
              @click.stop="onSwitchProvider(String(p.id)); moreMenuOpen = false"
            >{{ p.name }} · {{ p.model_id }}</button>
          </div>
        </div>
      </div>
    </div>

    <div class="messages" ref="messagesEl">
      <!-- __CONTINUE_HERE__ -->
      <div class="messages-inner">
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
      <div class="input-wrapper" :class="{ disabled: store.streaming }">
        <textarea
          ref="textareaEl"
          v-model="input"
          :disabled="store.streaming"
          @keydown="onKeydown"
          @input="autoResize"
          @compositionstart="isComposing = true"
          @compositionend="isComposing = false"
          :placeholder="store.streaming ? 'AI is responding...' : 'Type a message... (Shift+Enter for new line)'"
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

    <!-- Rules modal -->
    <Teleport to="body">
      <div v-if="showRulesModal" class="modal-overlay" @click="showRulesModal = false">
        <div class="rules-modal" @click.stop>
          <div class="rules-modal-header">
            <span class="rules-modal-title">项目规则</span>
            <span class="rules-modal-dir">{{ displayWorkDir }}</span>
            <button class="rules-modal-close" @click="showRulesModal = false">
              <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <path d="M18 6L6 18M6 6l12 12"/>
              </svg>
            </button>
          </div>
          <div class="rules-modal-body">
            <div class="rules-file-list">
              <div
                v-for="f in ruleFiles"
                :key="f.path"
                class="rules-file-item"
                :class="{ active: f.path === selectedRulePath }"
                @click="selectRule(f.path)"
              >
                {{ f.name }}
              </div>
              <div v-if="ruleFiles.length === 0" class="rules-empty">暂无规则文件</div>
            </div>
            <div class="rules-editor">
              <textarea
                v-model="ruleContent"
                class="rules-textarea"
                placeholder="规则内容..."
                :disabled="!selectedRulePath"
              />
              <div class="rules-editor-actions">
                <button class="btn-save-rule" :disabled="!selectedRulePath || ruleSaving" @click="saveRule">
                  {{ ruleSaving ? '保存中...' : '保存' }}
                </button>
              </div>
            </div>
          </div>
        </div>
      </div>
    </Teleport>

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
.input-wrapper {
  max-width: 720px; margin: 0 auto;
  display: flex; align-items: flex-end; gap: 8px;
  background: var(--bg-secondary); border: 1px solid var(--border);
  border-radius: var(--radius-lg); padding: 8px 12px;
  transition: border-color var(--transition);
}
.input-wrapper:focus-within { border-color: var(--accent); }
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
