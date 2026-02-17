<script setup lang="ts">
import { ref, nextTick, watch, computed } from 'vue'
import { marked } from 'marked'
import { useChatStore } from '../stores/chat'
import * as api from '../composables/api'
import type { FileItem } from '../composables/api'

const store = useChatStore()
const input = ref('')
const messagesEl = ref<HTMLElement>()
const textareaEl = ref<HTMLTextAreaElement>()
const stepsExpanded = ref(false)
const isComposing = ref(false)

// Rules modal state
const showRulesModal = ref(false)
const ruleFiles = ref<FileItem[]>([])
const selectedRulePath = ref('')
const ruleContent = ref('')
const ruleSaving = ref(false)

marked.setOptions({ breaks: true, gfm: true })

function renderMd(text: string): string {
  return marked.parse(text) as string
}

const allMessages = computed(() => [...store.messages])

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

function send() {
  const text = input.value.trim()
  if (!text || store.streaming) return
  store.sendMessage(text)
  input.value = ''
  stepsExpanded.value = false
  autoResize()
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
  } finally {
    ruleSaving.value = false
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
    <!-- Chat header bar -->
    <div v-if="store.currentSession" class="chat-header">
      <div class="header-left">
        <div class="header-title">{{ store.currentSession.title }}</div>
        <div class="header-workdir">{{ displayWorkDir }}</div>
      </div>
      <div class="header-right">
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
      </div>
    </div>

    <div class="messages" ref="messagesEl">
      <!-- __CONTINUE_HERE__ -->
      <div class="messages-inner">
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
            <div
              v-if="msg.role === 'assistant'"
              class="message-content md-content"
              v-html="renderMd(msg.content)"
            />
            <div v-else class="message-content">{{ msg.content }}</div>
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
                  {{ stepCount > 0 ? `${stepCount} step${stepCount > 1 ? 's' : ''}` : 'Processing...' }}
                </span>
                <svg class="chevron" :class="{ expanded: stepsExpanded }" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                  <path d="M6 9l6 6 6-6"/>
                </svg>
              </div>
              <div v-if="stepsExpanded" class="activity-body">
                <div v-if="store.thinkingContent" class="thinking-section">
                  <div class="section-label">
                    <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                      <circle cx="12" cy="12" r="10"/><path d="M12 6v6l4 2"/>
                    </svg>
                    Thinking
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
                    <span class="tool-name">{{ tc.name }}</span>
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
  </div>
</template>

<style scoped>
.chat-panel {
  flex: 1;
  display: flex;
  flex-direction: column;
  min-height: 0;
}
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
.tool-status.running { color: var(--accent); }
.tool-status.done { color: var(--success); }
.tool-name {
  font-weight: 600; color: var(--text-primary); font-size: 13px;
}
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
  background: rgba(0, 0, 0, 0.5);
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
  background: var(--accent); color: #fff;
  transition: opacity var(--transition);
}
.btn-save-rule:hover:not(:disabled) { opacity: 0.9; }
.btn-save-rule:disabled { opacity: 0.5; cursor: not-allowed; }
</style>
