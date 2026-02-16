<script setup lang="ts">
import { computed } from 'vue'
import { useChatStore } from '../stores/chat'
import { useRouter, useRoute } from 'vue-router'

const store = useChatStore()
const router = useRouter()
const route = useRoute()

const isManage = computed(() => route.path.startsWith('/manage'))

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
    </div>

    <div class="session-list">
      <div
        v-for="s in store.sessions"
        :key="s.id"
        class="session-item"
        :class="{ active: s.id === store.currentSessionId }"
        @click="selectSession(s.id)"
      >
        <div class="session-info">
          <div class="session-title-row">
            <svg v-if="s.streaming" class="streaming-icon" width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <path d="M21 12a9 9 0 11-6.219-8.56"/>
            </svg>
            <div class="session-title">{{ s.title }}</div>
          </div>
          <div class="session-time">{{ formatTime(s.updated_at) }}</div>
        </div>
        <button class="btn-delete" @click.stop="store.deleteSessionById(s.id)" title="Delete">
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M18 6L6 18M6 6l12 12"/>
          </svg>
        </button>
      </div>
      <div v-if="store.sessions.length === 0" class="no-sessions">
        No conversations yet
      </div>
    </div>

    <div class="sidebar-footer">
      <button class="footer-btn" @click="router.push('/settings')">
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <circle cx="12" cy="12" r="3"/>
          <path d="M12 1v2M12 21v2M4.22 4.22l1.42 1.42M18.36 18.36l1.42 1.42M1 12h2M21 12h2M4.22 19.78l1.42-1.42M18.36 5.64l1.42-1.42"/>
        </svg>
        <span>Settings</span>
      </button>
    </div>
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
@keyframes spin { to { transform: rotate(360deg); } }
.session-time {
  font-size: 11px;
  color: var(--text-muted);
  margin-top: 2px;
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
.footer-btn {
  width: 100%;
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
</style>
