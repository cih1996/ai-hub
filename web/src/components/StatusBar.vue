<script setup lang="ts">
import { ref, onMounted, onUnmounted, computed } from 'vue'
import { getStatus, retryInstall, type DepsStatus } from '../composables/api'

const status = ref<DepsStatus | null>(null)
const polling = ref<number>()

function needsPolling(s: DepsStatus): boolean {
  return !s.node_installed || !s.npm_installed || !s.claude_installed || s.installing || !!s.install_error
}

function startPolling() {
  if (polling.value) return
  polling.value = window.setInterval(fetchStatus, 3000)
}

function stopPolling() {
  if (polling.value) {
    clearInterval(polling.value)
    polling.value = undefined
  }
}

async function fetchStatus() {
  try {
    status.value = await getStatus()
    if (status.value && !needsPolling(status.value)) {
      stopPolling()
    } else if (!polling.value && status.value && needsPolling(status.value)) {
      startPolling()
    }
  } catch {
    // server not ready yet
  }
}

async function handleRetry() {
  await retryInstall()
  startPolling()
  await fetchStatus()
}

const visible = computed(() => {
  if (!status.value) return false
  return needsPolling(status.value)
})

const message = computed(() => {
  const s = status.value
  if (!s) return ''
  if (!s.node_installed) return 'Node.js is not installed. Claude Code CLI requires Node.js.'
  if (!s.npm_installed) return 'npm is not available. Required to install Claude Code CLI.'
  if (s.installing) return 'Installing Claude Code CLI...'
  if (s.install_error) return `Install failed: ${s.install_error}`
  if (!s.claude_installed) return 'Claude Code CLI not found. Click to install.'
  return ''
})

const level = computed(() => {
  const s = status.value
  if (!s) return 'info'
  if (!s.node_installed || !s.npm_installed) return 'error'
  if (s.install_error) return 'error'
  if (s.installing) return 'info'
  if (!s.claude_installed) return 'warn'
  return 'info'
})

onMounted(() => {
  fetchStatus()
})

onUnmounted(() => {
  stopPolling()
})
</script>

<template>
  <Transition name="slide">
    <div v-if="visible" class="status-bar" :class="level">
      <div class="status-content">
        <svg v-if="status?.installing" class="spin" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <path d="M21 12a9 9 0 11-6.219-8.56"/>
        </svg>
        <div class="status-text">
          <span>{{ message }}</span>
          <span v-if="status?.install_hint" class="hint">Fix: {{ status.install_hint }}</span>
        </div>
      </div>
      <div class="status-actions">
        <button
          v-if="status?.install_error || (!status?.claude_installed && !status?.installing && status?.npm_installed)"
          class="retry-btn"
          @click="handleRetry"
        >
          {{ status?.install_error ? 'Retry' : 'Install' }}
        </button>
        <a v-if="!status?.node_installed" href="https://nodejs.org" target="_blank" class="link-btn">
          Get Node.js
        </a>
      </div>
    </div>
  </Transition>
</template>

<style scoped>
.status-bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 8px 16px;
  font-size: 12px;
  font-weight: 500;
  border-bottom: 1px solid transparent;
  flex-shrink: 0;
}
.status-bar.info {
  background: rgba(124, 106, 239, 0.08);
  color: var(--accent);
  border-color: rgba(124, 106, 239, 0.15);
}
.status-bar.warn {
  background: rgba(234, 179, 8, 0.08);
  color: var(--warning);
  border-color: rgba(234, 179, 8, 0.15);
}
.status-bar.error {
  background: rgba(239, 68, 68, 0.08);
  color: var(--danger);
  border-color: rgba(239, 68, 68, 0.15);
}
.status-content {
  display: flex; align-items: center; gap: 8px; min-width: 0; flex: 1;
}
.status-text {
  display: flex; flex-direction: column; gap: 2px; min-width: 0;
}
.status-text > span {
  white-space: nowrap; overflow: hidden; text-overflow: ellipsis;
}
.hint {
  font-size: 11px; opacity: 0.75;
  font-family: 'SF Mono', 'Fira Code', monospace;
}
.status-actions { display: flex; gap: 8px; flex-shrink: 0; margin-left: 12px; }
.retry-btn, .link-btn {
  padding: 3px 10px; border-radius: 4px; font-size: 11px; font-weight: 600;
  background: var(--glass); color: inherit; transition: background 0.15s;
}
.retry-btn:hover, .link-btn:hover { background: var(--bg-hover); }
.spin { animation: spin 1s linear infinite; }
@keyframes spin { to { transform: rotate(360deg); } }
.slide-enter-active, .slide-leave-active { transition: all 0.3s ease; }
.slide-enter-from, .slide-leave-to { opacity: 0; transform: translateY(-100%); }
</style>
