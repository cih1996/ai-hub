<script setup lang="ts">
import { ref, computed, inject, watch } from 'vue'
import type { Ref } from 'vue'
import { useRouter } from 'vue-router'
import { useChatStore } from '../stores/chat'
import * as api from '../composables/api'

const router = useRouter()
const store = useChatStore()
const isMobile = inject<Ref<boolean>>('isMobile', ref(false))

// Worker state
interface Worker {
  id: string
  sessionId: number
  name: string
  role: string
  team: string
  avatar: string
  state: 'idle' | 'working' | 'completed'
  message: string
}

const workers = ref<Map<number, Worker>>(new Map())
const activeWorkerIds = ref<Set<number>>(new Set())
const queue = ref<number[]>([])

// Max concurrent workers
const maxConcurrent = computed(() => {
  if (isMobile.value) return 2
  return Math.max(2, Math.floor((window.innerWidth - 200) / 140))
})

// Get session info for worker display
function getWorkerInfo(sessionId: number): Partial<Worker> {
  const session = store.sessions.find(s => s.id === sessionId)
  if (!session) {
    return {
      name: `会话 #${sessionId}`,
      role: 'AI 助手',
      team: '',
      avatar: `/avatars/avatar${(sessionId % 50) + 1}.svg`
    }
  }
  return {
    name: session.title || `会话 #${sessionId}`,
    role: session.group_name ? '团队成员' : 'AI 助手',
    team: session.group_name || '',
    avatar: session.icon ? `/avatars/${session.icon}` : `/avatars/avatar${(sessionId % 50) + 1}.svg`
  }
}

// Start worker (working state)
function startWorker(sessionId: number) {
  if (workers.value.has(sessionId)) {
    const worker = workers.value.get(sessionId)!
    if (worker.state === 'working') return
    worker.state = 'working'
    worker.message = ''
    return
  }

  const info = getWorkerInfo(sessionId)
  const worker: Worker = {
    id: `worker-${sessionId}`,
    sessionId,
    name: info.name || `会话 #${sessionId}`,
    role: info.role || 'AI 助手',
    team: info.team || '',
    avatar: info.avatar || `/avatars/avatar1.svg`,
    state: 'idle',
    message: ''
  }
  workers.value.set(sessionId, worker)

  // Check if can activate immediately
  if (activeWorkerIds.value.size < maxConcurrent.value) {
    activeWorkerIds.value.add(sessionId)
    worker.state = 'working'
  } else {
    queue.value.push(sessionId)
  }
}

// Complete worker
async function completeWorker(sessionId: number, fallbackMessage?: string) {
  const worker = workers.value.get(sessionId)
  if (!worker) return

  worker.state = 'completed'

  // Try to get last assistant message
  let message = fallbackMessage || '任务完成'
  try {
    const result = await api.getMessagesPaginated(sessionId, 5)
    const lastAssistant = result.messages.find(m => m.role === 'assistant')
    if (lastAssistant && lastAssistant.content) {
      // Truncate and clean message
      let content = lastAssistant.content
        .replace(/!\[.*?\]\(data:image\/[^)]+\)/g, '[图片]') // Remove base64 images
        .replace(/```[\s\S]*?```/g, '[代码]') // Replace code blocks
        .replace(/\n+/g, ' ') // Replace newlines
        .trim()
      if (content.length > 60) {
        content = content.slice(0, 57) + '...'
      }
      if (content) {
        message = content
      }
    }
  } catch {
    // Use fallback message
  }

  worker.message = message

  // Play ding sound
  playDingSound()
}

// Hide worker
function hideWorker(sessionId: number) {
  const worker = workers.value.get(sessionId)
  if (!worker) return

  worker.state = 'idle'
  activeWorkerIds.value.delete(sessionId)

  // Process queue
  if (queue.value.length > 0 && activeWorkerIds.value.size < maxConcurrent.value) {
    const nextId = queue.value.shift()!
    const nextWorker = workers.value.get(nextId)
    if (nextWorker) {
      activeWorkerIds.value.add(nextId)
      nextWorker.state = 'working'
    }
  }

  // Remove from map after animation
  setTimeout(() => {
    if (worker.state === 'idle') {
      workers.value.delete(sessionId)
    }
  }, 500)
}

// Click to navigate
function navigateToSession(sessionId: number) {
  const worker = workers.value.get(sessionId)
  // Only hide if completed, keep working state visible
  if (worker && worker.state === 'completed') {
    hideWorker(sessionId)
  }
  router.push(`/chat/${sessionId}`)
}

// Close without navigate
function closeWorker(sessionId: number, e: Event) {
  e.stopPropagation()
  hideWorker(sessionId)
}

// Play completion sound
function playDingSound() {
  try {
    const ctx = new (window.AudioContext || (window as any).webkitAudioContext)()
    const osc = ctx.createOscillator()
    const gain = ctx.createGain()
    osc.type = 'sine'
    osc.frequency.setValueAtTime(880, ctx.currentTime)
    gain.gain.setValueAtTime(0.08, ctx.currentTime)
    gain.gain.exponentialRampToValueAtTime(0.001, ctx.currentTime + 0.4)
    osc.connect(gain)
    gain.connect(ctx.destination)
    osc.start()
    osc.stop(ctx.currentTime + 0.4)
  } catch {
    // Audio not supported
  }
}

// Watch store streaming state
watch(() => store.streaming, (streaming, wasStreaming) => {
  const sessionId = store.currentSessionId
  if (!sessionId || sessionId === 0) return

  if (streaming && !wasStreaming) {
    startWorker(sessionId)
  } else if (!streaming && wasStreaming) {
    completeWorker(sessionId, '回复完成')
  }
})

// Watch for other sessions streaming (via WebSocket events)
// This would be triggered by the store when receiving ws messages for other sessions
watch(() => store.otherSessionsStreaming, (streamingSessions) => {
  for (const [sessionId, isStreaming] of Object.entries(streamingSessions)) {
    const sid = Number(sessionId)
    if (sid === store.currentSessionId) continue

    if (isStreaming) {
      startWorker(sid)
    } else {
      const worker = workers.value.get(sid)
      if (worker && worker.state === 'working') {
        completeWorker(sid, '任务完成')
      }
    }
  }
}, { deep: true })

// Visible workers (not idle)
const visibleWorkers = computed(() => {
  return Array.from(workers.value.values()).filter(w => w.state !== 'idle')
})

// Expose for external use
defineExpose({
  startWorker,
  completeWorker,
  hideWorker
})
</script>

<template>
  <Teleport to="body">
    <!-- Desktop: Top center -->
    <div v-if="!isMobile" class="ai-worker-container desktop">
      <div
        v-for="worker in visibleWorkers"
        :key="worker.id"
        class="ai-worker"
        :class="worker.state"
        @click="navigateToSession(worker.sessionId)"
      >
        <div class="avatar-wrapper">
          <div class="spinner"></div>
          <img class="avatar-img" :src="worker.avatar" :alt="worker.name" />
        </div>
        <div class="worker-info">
          <div class="worker-header">
            <p class="worker-name">{{ worker.name }}</p>
            <span class="session-id">#{{ worker.sessionId }}</span>
          </div>
          <div class="worker-meta">
            <span v-if="worker.team" class="team-name">{{ worker.team }}</span>
            <p class="worker-role">{{ worker.role }}</p>
          </div>
        </div>
        <button class="btn-close" @click="closeWorker(worker.sessionId, $event)" title="关闭">
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M18 6L6 18M6 6l12 12"/>
          </svg>
        </button>
        <div v-if="worker.state === 'completed'" class="speech-bubble">
          {{ worker.message }}
        </div>
      </div>
    </div>

    <!-- Mobile: Right side -->
    <div v-else class="ai-worker-container mobile">
      <div
        v-for="worker in visibleWorkers"
        :key="worker.id"
        class="ai-worker-mobile"
        :class="worker.state"
        @click="navigateToSession(worker.sessionId)"
      >
        <div class="avatar-wrapper">
          <div class="spinner"></div>
          <img class="avatar-img" :src="worker.avatar" :alt="worker.name" />
        </div>
        <div class="worker-expand">
          <div class="worker-info">
            <p class="worker-name">{{ worker.name }}</p>
            <span v-if="worker.team" class="team-name">{{ worker.team }}</span>
          </div>
          <p v-if="worker.state === 'completed'" class="worker-message">{{ worker.message }}</p>
          <button class="btn-close" @click="closeWorker(worker.sessionId, $event)">
            <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <path d="M18 6L6 18M6 6l12 12"/>
            </svg>
          </button>
        </div>
      </div>
    </div>
  </Teleport>
</template>

<style scoped>
/* Desktop container - top center */
.ai-worker-container.desktop {
  position: fixed;
  top: 0;
  left: 0;
  width: 100%;
  display: flex;
  justify-content: center;
  pointer-events: none;
  z-index: 9999;
}

/* Desktop worker card */
.ai-worker {
  pointer-events: auto;
  position: relative;
  height: 64px;
  background: var(--bg-secondary);
  border: 0 solid var(--border);
  border-top: none;
  border-radius: 0 0 24px 24px;
  box-shadow: 0 4px 20px rgba(0, 0, 0, 0.1);
  transition: all 0.5s cubic-bezier(0.34, 1.56, 0.64, 1);
  transform: translateY(-100%);
  width: 0;
  margin: 0;
  padding: 0;
  opacity: 0;
  overflow: hidden;
  display: flex;
  align-items: center;
  box-sizing: border-box;
  cursor: pointer;
}

.ai-worker.working {
  width: 72px;
  margin: 0 8px;
  padding: 0;
  opacity: 1;
  border-width: 1px;
  transform: translateY(-24px);
  justify-content: center;
  border-radius: 0 0 36px 36px;
}

.ai-worker.completed {
  width: 280px;
  height: 72px;
  margin: 0 8px;
  padding: 0 16px;
  opacity: 1;
  border-width: 1px;
  transform: translateY(0);
  overflow: visible;
  z-index: 100;
}

/* Avatar */
.avatar-wrapper {
  position: relative;
  width: 44px;
  height: 44px;
  border-radius: 50%;
  transition: all 0.5s cubic-bezier(0.34, 1.56, 0.64, 1);
  flex-shrink: 0;
}

.ai-worker.working .avatar-wrapper {
  width: 32px;
  height: 32px;
  transform: translateY(12px);
}

.avatar-img {
  width: 100%;
  height: 100%;
  border-radius: 50%;
  object-fit: cover;
  background: var(--bg-tertiary);
}

/* Spinner */
.spinner {
  position: absolute;
  top: -4px;
  left: -4px;
  right: -4px;
  bottom: -4px;
  border: 2px solid transparent;
  border-top-color: var(--accent);
  border-right-color: var(--accent);
  border-radius: 50%;
  animation: spin 1s linear infinite;
  opacity: 0;
  transition: opacity 0.3s;
}

.ai-worker.working .spinner {
  opacity: 1;
}

@keyframes spin {
  0% { transform: rotate(0deg); }
  100% { transform: rotate(360deg); }
}

/* Worker info */
.worker-info {
  margin-left: 12px;
  overflow: hidden;
  white-space: nowrap;
  opacity: 1;
  transition: opacity 0.3s;
  flex-grow: 1;
  display: flex;
  flex-direction: column;
  justify-content: center;
}

.ai-worker.working .worker-info {
  opacity: 0;
  width: 0;
  margin: 0;
  flex-grow: 0;
}

.worker-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 2px;
}

.worker-name {
  font-size: 14px;
  font-weight: 600;
  color: var(--text-primary);
  margin: 0;
  overflow: hidden;
  text-overflow: ellipsis;
}

.session-id {
  font-size: 10px;
  color: var(--text-muted);
  background: var(--bg-tertiary);
  padding: 2px 6px;
  border-radius: 4px;
  display: none;
}

.ai-worker.completed .session-id {
  display: inline-block;
}

.worker-meta {
  display: flex;
  align-items: center;
}

.team-name {
  font-size: 10px;
  color: var(--accent);
  background: var(--accent-soft);
  padding: 1px 4px;
  border-radius: 3px;
  margin-right: 6px;
  display: none;
}

.ai-worker.completed .team-name {
  display: inline-block;
}

.worker-role {
  font-size: 12px;
  color: var(--text-secondary);
  margin: 0;
  overflow: hidden;
  text-overflow: ellipsis;
}

/* Close button */
.btn-close {
  position: absolute;
  top: 50%;
  right: 8px;
  transform: translateY(-50%);
  width: 20px;
  height: 20px;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 50%;
  background: var(--bg-tertiary);
  color: var(--text-muted);
  opacity: 0;
  transition: all 0.2s;
  cursor: pointer;
}

.ai-worker.completed .btn-close {
  opacity: 1;
}

.btn-close:hover {
  background: rgba(239, 68, 68, 0.1);
  color: var(--danger);
}

/* Speech bubble */
.speech-bubble {
  position: absolute;
  top: 100%;
  left: 50%;
  margin-top: 12px;
  transform: translateX(-50%) translateY(-10px) scale(0.9);
  background: var(--bg-secondary);
  color: var(--text-primary);
  border: 1px solid var(--border);
  padding: 12px 16px;
  border-radius: 12px;
  font-size: 13px;
  line-height: 1.5;
  width: max-content;
  max-width: 260px;
  min-width: 120px;
  white-space: normal;
  word-break: break-word;
  opacity: 0;
  pointer-events: none;
  transition: all 0.4s cubic-bezier(0.34, 1.56, 0.64, 1);
  box-shadow: 0 4px 15px rgba(0, 0, 0, 0.1);
  z-index: 200;
}

.speech-bubble::before {
  content: '';
  position: absolute;
  top: -6px;
  left: 50%;
  width: 10px;
  height: 10px;
  background: var(--bg-secondary);
  border-top: 1px solid var(--border);
  border-left: 1px solid var(--border);
  transform: translateX(-50%) rotate(45deg);
}

.ai-worker.completed .speech-bubble {
  opacity: 1;
  transform: translateX(-50%) translateY(0) scale(1);
  pointer-events: auto;
}

/* Mobile container - right side */
.ai-worker-container.mobile {
  position: fixed;
  top: 80px;
  right: 0;
  display: flex;
  flex-direction: column;
  gap: 8px;
  pointer-events: none;
  z-index: 9999;
}

/* Mobile worker */
.ai-worker-mobile {
  pointer-events: auto;
  display: flex;
  align-items: center;
  background: var(--bg-secondary);
  border: 1px solid var(--border);
  border-right: none;
  border-radius: 24px 0 0 24px;
  box-shadow: -2px 2px 10px rgba(0, 0, 0, 0.1);
  transition: all 0.4s cubic-bezier(0.34, 1.56, 0.64, 1);
  transform: translateX(100%);
  opacity: 0;
  cursor: pointer;
  padding: 6px;
  padding-right: 0;
}

.ai-worker-mobile.working,
.ai-worker-mobile.completed {
  transform: translateX(0);
  opacity: 1;
}

.ai-worker-mobile .avatar-wrapper {
  width: 36px;
  height: 36px;
  flex-shrink: 0;
}

.ai-worker-mobile.working .avatar-wrapper {
  width: 36px;
  height: 36px;
  transform: none;
}

.ai-worker-mobile .spinner {
  top: -3px;
  left: -3px;
  right: -3px;
  bottom: -3px;
}

/* Mobile expand area */
.worker-expand {
  display: flex;
  flex-direction: column;
  overflow: hidden;
  max-width: 0;
  opacity: 0;
  transition: all 0.4s cubic-bezier(0.34, 1.56, 0.64, 1);
  padding: 0;
}

.ai-worker-mobile.completed .worker-expand {
  max-width: 180px;
  opacity: 1;
  padding: 4px 8px 4px 8px;
}

.ai-worker-mobile .worker-info {
  margin: 0;
  display: flex;
  align-items: center;
  gap: 6px;
}

.ai-worker-mobile .worker-name {
  font-size: 12px;
  font-weight: 600;
  max-width: 100px;
  overflow: hidden;
  text-overflow: ellipsis;
}

.ai-worker-mobile .team-name {
  display: inline-block;
  font-size: 9px;
}

.ai-worker-mobile .worker-message {
  font-size: 11px;
  color: var(--text-secondary);
  margin: 2px 0 0;
  max-width: 160px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.ai-worker-mobile .btn-close {
  position: static;
  width: 18px;
  height: 18px;
  margin-left: auto;
  flex-shrink: 0;
  opacity: 1;
}
</style>
