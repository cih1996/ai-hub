<script setup lang="ts">
import { computed, ref, onMounted } from 'vue'
import type { Session } from '../types'
import { useChatStore } from '../stores/chat'
import { useRouter, useRoute } from 'vue-router'
import * as api from '../composables/api'
import { useTheme, type ThemeMode } from '../composables/theme'

const { mode: themeMode, setMode } = useTheme()

const themeModeLabel: Record<string, string> = { system: '跟随系统', light: '亮色', dark: '暗色' }

function toggleTheme() {
  const order: ThemeMode[] = ['system', 'light', 'dark']
  const next = order[(order.indexOf(themeMode.value) + 1) % 3] ?? 'system'
  setMode(next)
}

const store = useChatStore()
const router = useRouter()
const route = useRoute()

const deleteTarget = ref<Session | null>(null)
const version = ref('')

onMounted(async () => {
  try {
    const res = await api.getVersion()
    version.value = res.version
  } catch {}
})

function confirmDelete() {
  if (deleteTarget.value) {
    store.deleteSessionById(deleteTarget.value.id)
    deleteTarget.value = null
  }
}

const isManage = computed(() => route.path.startsWith('/manage'))
const isSkills = computed(() => route.path.startsWith('/skills'))
const isMcp = computed(() => route.path.startsWith('/mcp'))
const isTriggers = computed(() => route.path.startsWith('/triggers'))
const isChannels = computed(() => route.path.startsWith('/channels'))
const isTokenUsage = computed(() => route.path.startsWith('/token-usage'))

interface SessionGroup {
  key: string
  label: string
  sessions: Session[]
}

const groupedSessions = computed<SessionGroup[]>(() => {
  const groups = new Map<string, Session[]>()
  for (const s of store.sessions) {
    // Prefer group_name, fall back to work_dir
    const key = s.group_name || s.work_dir || ''
    if (!groups.has(key)) groups.set(key, [])
    groups.get(key)!.push(s)
  }
  const result: SessionGroup[] = []
  // Default group (no group_name and no work_dir) first
  const defaultGroup = groups.get('')
  if (defaultGroup) {
    result.push({ key: '', label: '默认', sessions: defaultGroup })
    groups.delete('')
  }
  // Other groups sorted alphabetically
  const sortedKeys = [...groups.keys()].sort()
  for (const key of sortedKeys) {
    // If it looks like a path, shorten it; otherwise use as-is
    const label = key.startsWith('/') ? key.replace(/^\/Users\/[^/]+/, '~') : key
    result.push({ key, label, sessions: groups.get(key)! })
  }
  return result
})

function newChat() {
  store.newChat()
  router.push('/chat')
}

function selectSession(id: number) {
  store.selectSession(id)
  router.push(`/chat/${id}`)
}

function formatTime(dateStr: string) {
  const d = new Date(dateStr)
  const now = new Date()
  const diff = now.getTime() - d.getTime()
  if (diff < 86400000) {
    return d.toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit' })
  }
  if (diff < 604800000) {
    const days = Math.floor(diff / 86400000)
    return `${days}d ago`
  }
  return d.toLocaleDateString('zh-CN', { month: 'short', day: 'numeric' })
}

function formatTokens(n: number): string {
  if (n >= 1000000) return (n / 1000000).toFixed(1) + 'M'
  if (n >= 1000) return (n / 1000).toFixed(1) + 'K'
  return String(n)
}

// Load session token totals on mount
onMounted(async () => {
  if (!store.sessionTokenTotals) store.sessionTokenTotals = {}
  for (const s of store.sessions) {
    try {
      const data = await api.getSessionTokenUsage(s.id)
      if (data.stats) {
        store.sessionTokenTotals[s.id] = data.stats.total_input_tokens + data.stats.total_output_tokens
      }
    } catch { /* ignore */ }
  }
})
</script>

<template>
  <aside class="sidebar">
    <div class="sidebar-header">
      <div class="logo">
        <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <path d="M12 2L2 7l10 5 10-5-10-5z"/>
          <path d="M2 17l10 5 10-5"/>
          <path d="M2 12l10 5 10-5"/>
        </svg>
        <span>AI Hub</span>
      </div>
    </div>

    <div class="sidebar-nav">
      <button class="nav-item" @click="newChat">
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <path d="M12 5v14M5 12h14"/>
        </svg>
        <span>新会话</span>
      </button>
      <button class="nav-item" :class="{ active: isManage }" @click="router.push('/manage')">
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <path d="M14 2H6a2 2 0 00-2 2v16a2 2 0 002 2h12a2 2 0 002-2V8z"/>
          <polyline points="14 2 14 8 20 8"/>
          <line x1="16" y1="13" x2="8" y2="13"/>
          <line x1="16" y1="17" x2="8" y2="17"/>
        </svg>
        <span>管理</span>
      </button>
      <button class="nav-item" :class="{ active: isSkills }" @click="router.push('/skills')">
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <path d="M22 11.08V12a10 10 0 11-5.93-9.14"/>
          <polyline points="22 4 12 14.01 9 11.01"/>
        </svg>
        <span>技能</span>
      </button>
      <button class="nav-item" :class="{ active: isMcp }" @click="router.push('/mcp')">
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <rect x="2" y="2" width="20" height="8" rx="2" ry="2"/>
          <rect x="2" y="14" width="20" height="8" rx="2" ry="2"/>
          <line x1="6" y1="6" x2="6.01" y2="6"/>
          <line x1="6" y1="18" x2="6.01" y2="18"/>
        </svg>
        <span>MCP</span>
      </button>
      <button class="nav-item" :class="{ active: isTriggers }" @click="router.push('/triggers')">
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <circle cx="12" cy="12" r="10"/>
          <polyline points="12 6 12 12 16 14"/>
        </svg>
        <span>定时</span>
      </button>
      <button class="nav-item" :class="{ active: isChannels }" @click="router.push('/channels')">
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <path d="M21 15a2 2 0 01-2 2H7l-4 4V5a2 2 0 012-2h14a2 2 0 012 2z"/>
        </svg>
        <span>通讯</span>
      </button>
      <button class="nav-item" :class="{ active: isTokenUsage }" @click="router.push('/token-usage')">
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <rect x="4" y="4" width="16" height="16" rx="2"/>
          <circle cx="9" cy="9" r="1.5"/><circle cx="15" cy="9" r="1.5"/>
          <circle cx="9" cy="15" r="1.5"/><circle cx="15" cy="15" r="1.5"/>
        </svg>
        <span>用量</span>
      </button>
    </div>

    <div class="session-list">
      <template v-for="group in groupedSessions" :key="group.key">
        <div v-if="groupedSessions.length > 1" class="group-label">{{ group.label }}</div>
        <div
          v-for="s in group.sessions"
          :key="s.id"
          class="session-item"
          :class="{ active: s.id === store.currentSessionId }"
          @click="selectSession(s.id)"
        >
          <div class="session-info">
            <div class="session-title-row">
              <span
                v-if="s.process_alive"
                class="process-dot"
                :class="s.process_state === 'busy' ? 'busy' : 'idle'"
                :title="s.process_state === 'busy' ? '运行中' : '空闲'"
              ></span>
              <svg v-if="s.streaming" class="streaming-icon" width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <path d="M21 12a9 9 0 11-6.219-8.56"/>
              </svg>
              <svg v-if="s.has_triggers" class="trigger-icon" width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <circle cx="12" cy="12" r="10"/><polyline points="12 6 12 12 16 14"/>
              </svg>
              <span class="session-id">#{{ s.id }}</span>
              <div class="session-title">{{ s.title }}</div>
            </div>
            <div class="session-time">
              {{ formatTime(s.updated_at) }}
              <span v-if="store.sessionTokenTotals?.[s.id]" class="session-token-tag" :title="(store.sessionTokenTotals![s.id] ?? 0).toLocaleString() + ' tokens'">
                {{ formatTokens(store.sessionTokenTotals![s.id] ?? 0) }}
              </span>
            </div>
          </div>
          <button class="btn-delete" @click.stop="deleteTarget = s" title="Delete">
            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <path d="M18 6L6 18M6 6l12 12"/>
            </svg>
          </button>
        </div>
      </template>
      <div v-if="store.sessions.length === 0" class="no-sessions">
        暂无会话
      </div>
    </div>

    <div class="sidebar-footer">
      <div class="footer-row">
        <button class="footer-btn" @click="router.push('/settings')">
          <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <circle cx="12" cy="12" r="3"/>
            <path d="M12 1v2M12 21v2M4.22 4.22l1.42 1.42M18.36 18.36l1.42 1.42M1 12h2M21 12h2M4.22 19.78l1.42-1.42M18.36 5.64l1.42-1.42"/>
          </svg>
          <span>设置</span>
        </button>
        <button class="theme-btn" @click="toggleTheme" :title="'主题: ' + themeModeLabel[themeMode]">
          <svg v-if="themeMode === 'dark'" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M21 12.79A9 9 0 1111.21 3 7 7 0 0021 12.79z"/></svg>
          <svg v-else-if="themeMode === 'light'" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="5"/><path d="M12 1v2M12 21v2M4.22 4.22l1.42 1.42M18.36 18.36l1.42 1.42M1 12h2M21 12h2M4.22 19.78l1.42-1.42M18.36 5.64l1.42-1.42"/></svg>
          <svg v-else width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="2" y="3" width="20" height="14" rx="2"/><line x1="8" y1="21" x2="16" y2="21"/><line x1="12" y1="17" x2="12" y2="21"/></svg>
        </button>
      </div>
      <div v-if="version" class="version-text">{{ version }}</div>
    </div>

    <!-- Delete confirmation modal -->
    <Teleport to="body">
      <div v-if="deleteTarget" class="modal-overlay" @click="deleteTarget = null">
        <div class="modal-box" @click.stop>
          <p class="modal-title">确认删除</p>
          <p class="modal-desc">删除会话「{{ deleteTarget.title }}」？此操作不可撤销。</p>
          <div class="modal-actions">
            <button class="modal-btn cancel" @click="deleteTarget = null">取消</button>
            <button class="modal-btn confirm" @click="confirmDelete">删除</button>
          </div>
        </div>
      </div>
    </Teleport>
  </aside>
</template>

<style scoped>
.sidebar {
  width: 260px;
  min-width: 260px;
  height: 100vh;
  background: var(--bg-secondary);
  border-right: 1px solid var(--border);
  display: flex;
  flex-direction: column;
}
.sidebar-header {
  padding: 16px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  border-bottom: 1px solid var(--border);
}
.logo {
  display: flex;
  align-items: center;
  gap: 8px;
  font-weight: 600;
  font-size: 15px;
  color: var(--text-primary);
}
.sidebar-nav {
  padding: 8px;
  display: flex;
  flex-direction: column;
  gap: 2px;
  border-bottom: 1px solid var(--border);
}
.nav-item {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 12px;
  border-radius: var(--radius);
  font-size: 13px;
  color: var(--text-secondary);
  transition: all var(--transition);
  width: 100%;
}
.nav-item:hover {
  background: var(--bg-hover);
  color: var(--text-primary);
}
.nav-item.active {
  background: var(--bg-active);
  color: var(--text-primary);
}
.session-list {
  flex: 1;
  overflow-y: auto;
  padding: 8px;
}
.group-label {
  font-size: 11px;
  color: var(--text-muted);
  padding: 8px 12px 4px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  border-bottom: 1px solid var(--border);
  margin-bottom: 4px;
}
.session-item {
  display: flex;
  align-items: center;
  padding: 10px 12px;
  border-radius: var(--radius);
  cursor: pointer;
  transition: background var(--transition);
  margin-bottom: 2px;
}
.session-item:hover { background: var(--bg-hover); }
.session-item.active { background: var(--bg-active); }
.session-info { flex: 1; min-width: 0; }
.session-id {
  flex-shrink: 0;
  font-size: 10px;
  color: var(--text-muted);
  background: var(--bg-hover);
  padding: 1px 5px;
  border-radius: 3px;
  font-family: monospace;
}
.session-title {
  font-size: 13px;
  font-weight: 500;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  min-width: 0;
}
.session-title-row {
  display: flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
}
.streaming-icon {
  flex-shrink: 0;
  color: var(--accent);
  animation: spin 1s linear infinite;
}
.trigger-icon {
  flex-shrink: 0;
  color: var(--text-muted);
}
.process-dot {
  flex-shrink: 0;
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: var(--text-muted);
}
.process-dot.busy {
  background: var(--success);
}
.process-dot.idle {
  background: var(--warning);
}
@keyframes spin { to { transform: rotate(360deg); } }
.session-time {
  font-size: 11px;
  color: var(--text-muted);
  margin-top: 2px;
  display: flex;
  align-items: center;
  gap: 6px;
}
.session-token-tag {
  font-size: 10px;
  color: var(--accent);
  background: var(--accent-soft);
  padding: 0 5px;
  border-radius: 3px;
  white-space: nowrap;
}
.btn-delete {
  opacity: 0;
  width: 24px;
  height: 24px;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: var(--radius-sm);
  color: var(--text-muted);
  transition: all var(--transition);
  flex-shrink: 0;
}
.session-item:hover .btn-delete { opacity: 1; }
.btn-delete:hover {
  color: var(--danger);
  background: rgba(239, 68, 68, 0.1);
}
.no-sessions {
  text-align: center;
  color: var(--text-muted);
  padding: 32px 16px;
  font-size: 13px;
}
.sidebar-footer {
  padding: 8px;
  border-top: 1px solid var(--border);
}
.footer-row {
  display: flex;
  align-items: center;
  gap: 4px;
}
.footer-btn {
  flex: 1;
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 10px 12px;
  border-radius: var(--radius);
  font-size: 13px;
  color: var(--text-secondary);
  transition: all var(--transition);
}
.footer-btn:hover {
  background: var(--bg-hover);
  color: var(--text-primary);
}
.theme-btn {
  width: 36px;
  height: 36px;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: var(--radius);
  color: var(--text-secondary);
  transition: all var(--transition);
  flex-shrink: 0;
}
.theme-btn:hover {
  background: var(--bg-hover);
  color: var(--text-primary);
}
.version-text {
  text-align: center;
  font-size: 11px;
  color: var(--text-muted);
  padding: 4px 0 2px;
}
.modal-overlay {
  position: fixed;
  inset: 0;
  background: var(--overlay);
  display: flex;
  align-items: center;
  justify-content: center;
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
</style>
