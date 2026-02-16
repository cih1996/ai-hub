<script setup lang="ts">
import { ref, nextTick, watch, computed } from 'vue'
import { marked } from 'marked'
import { useChatStore } from '../stores/chat'

const store = useChatStore()
const input = ref('')
const messagesEl = ref<HTMLElement>()
const textareaEl = ref<HTMLTextAreaElement>()
const stepsExpanded = ref(false)
const isComposing = ref(false)

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

function formatToolInput(raw: string): string {
  if (!raw) return ''
  try {
    const obj = JSON.parse(raw)
    if (typeof obj === 'object' && obj !== null) {
      // Bash: show command
      if (obj.command) return obj.command
      // Read/Edit/Write: show file path
      if (obj.file_path) return obj.file_path
      // Grep/Search: show pattern + path
      if (obj.pattern) return obj.pattern + (obj.path ? ` in ${obj.path}` : '')
      // Fallback: show first string value
      const firstVal = Object.values(obj).find((v) => typeof v === 'string' && (v as string).length > 0)
      if (firstVal) return String(firstVal).slice(0, 300)
    }
  } catch {
    // Partial JSON — try to extract useful bits
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
    <div class="messages" ref="messagesEl">
      <div class="messages-inner">
        <!-- Previous messages -->
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
  </div>
</template>

<style scoped>
.chat-panel {
  flex: 1;
  display: flex;
  flex-direction: column;
  min-height: 0;
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
</style>
