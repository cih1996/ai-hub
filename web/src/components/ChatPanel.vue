<script setup lang="ts">
import { ref, nextTick, watch, computed } from 'vue'
import { marked } from 'marked'
import { useChatStore } from '../stores/chat'

const store = useChatStore()
const input = ref('')
const messagesEl = ref<HTMLElement>()
const textareaEl = ref<HTMLTextAreaElement>()

// Configure marked
marked.setOptions({ breaks: true, gfm: true })

function renderMd(text: string): string {
  return marked.parse(text) as string
}

const allMessages = computed(() => {
  const msgs = [...store.messages]
  if (store.streaming && store.streamingContent) {
    msgs.push({
      id: -1,
      session_id: store.currentSessionId,
      role: 'assistant',
      content: store.streamingContent,
      created_at: new Date().toISOString(),
    })
  }
  return msgs
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

function send() {
  const text = input.value.trim()
  if (!text || store.streaming) return
  store.sendMessage(text)
  input.value = ''
  autoResize()
}

function onKeydown(e: KeyboardEvent) {
  if (e.key === 'Enter' && !e.shiftKey) {
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
</script>

<template>
  <div class="chat-panel">
    <!-- Messages -->
    <div class="messages" ref="messagesEl">
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

        <div v-if="store.streaming && !store.streamingContent" class="message assistant">
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
            <div class="typing-indicator">
              <span></span><span></span><span></span>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- Input -->
    <div class="input-area">
      <div class="input-wrapper">
        <textarea
          ref="textareaEl"
          v-model="input"
          @keydown="onKeydown"
          @input="autoResize"
          placeholder="Type a message... (Shift+Enter for new line)"
          rows="1"
        />
        <div class="input-actions">
          <button
            v-if="store.streaming"
            class="btn-stop"
            @click="store.stopStreaming()"
            title="Stop"
          >
            <svg width="16" height="16" viewBox="0 0 24 24" fill="currentColor">
              <rect x="6" y="6" width="12" height="12" rx="2"/>
            </svg>
          </button>
          <button
            v-else
            class="btn-send"
            :disabled="!input.trim()"
            @click="send"
            title="Send"
          >
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
  max-width: 768px;
  margin: 0 auto;
  padding: 0 24px;
}
.message {
  display: flex;
  gap: 12px;
  margin-bottom: 24px;
}
.message-avatar {
  flex-shrink: 0;
  padding-top: 2px;
}
.avatar {
  width: 30px;
  height: 30px;
  border-radius: 8px;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 13px;
  font-weight: 600;
}
.user-avatar {
  background: var(--accent-soft);
  color: var(--accent);
}
.ai-avatar {
  background: var(--bg-tertiary);
  color: var(--text-secondary);
}
.message-body {
  flex: 1;
  min-width: 0;
}
.message-role {
  font-size: 12px;
  font-weight: 600;
  color: var(--text-secondary);
  margin-bottom: 4px;
  text-transform: uppercase;
  letter-spacing: 0.5px;
}
.message-content {
  line-height: 1.7;
  word-break: break-word;
}

/* Typing indicator */
.typing-indicator {
  display: flex;
  gap: 4px;
  padding: 8px 0;
}
.typing-indicator span {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: var(--text-muted);
  animation: typing 1.4s infinite;
}
.typing-indicator span:nth-child(2) { animation-delay: 0.2s; }
.typing-indicator span:nth-child(3) { animation-delay: 0.4s; }
@keyframes typing {
  0%, 60%, 100% { opacity: 0.3; transform: scale(0.8); }
  30% { opacity: 1; transform: scale(1); }
}

/* Input area */
.input-area {
  padding: 16px 24px 24px;
}
.input-wrapper {
  max-width: 768px;
  margin: 0 auto;
  background: var(--bg-secondary);
  border: 1px solid var(--border);
  border-radius: var(--radius-lg);
  display: flex;
  align-items: flex-end;
  padding: 8px 8px 8px 16px;
  transition: border-color var(--transition);
}
.input-wrapper:focus-within {
  border-color: var(--accent);
}
.input-wrapper textarea {
  flex: 1;
  resize: none;
  font-size: 14px;
  line-height: 1.5;
  padding: 6px 0;
  max-height: 200px;
  color: var(--text-primary);
}
.input-wrapper textarea::placeholder {
  color: var(--text-muted);
}
.input-actions {
  flex-shrink: 0;
  margin-left: 8px;
}
.btn-send, .btn-stop {
  width: 34px;
  height: 34px;
  border-radius: var(--radius);
  display: flex;
  align-items: center;
  justify-content: center;
  transition: all var(--transition);
}
.btn-send {
  background: var(--accent);
  color: white;
}
.btn-send:hover:not(:disabled) {
  background: var(--accent-hover);
}
.btn-send:disabled {
  opacity: 0.3;
  cursor: not-allowed;
}
.btn-stop {
  background: var(--danger);
  color: white;
}
.btn-stop:hover {
  background: var(--danger);
  opacity: 0.9;
}
</style>
