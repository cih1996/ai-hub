<script setup lang="ts">
import { computed } from 'vue'
import { useRouter, useRoute } from 'vue-router'

const router = useRouter()
const route = useRoute()

const tabs = [
  { path: '/shadow-ai/overview', label: '概览' },
  { path: '/shadow-ai/config', label: '配置' },
  { path: '/shadow-ai/logs', label: '日志' },
  { path: '/shadow-ai/memory', label: '记忆' },
  { path: '/shadow-ai/patrol', label: '巡检' },
  { path: '/shadow-ai/router', label: '路由' },
]

const activeTab = computed(() => {
  for (const tab of tabs) {
    if (route.path.startsWith(tab.path)) return tab.path
  }
  return '/shadow-ai/overview'
})
</script>

<template>
  <div class="shadow-ai-layout">
    <div class="shadow-ai-sidebar">
      <div class="sidebar-header">
        <button class="btn-back" @click="router.push('/chat')">
          <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M19 12H5M12 19l-7-7 7-7"/>
          </svg>
          返回
        </button>
        <h1>影子AI</h1>
      </div>
      <nav class="sidebar-nav">
        <router-link
          v-for="tab in tabs"
          :key="tab.path"
          :to="tab.path"
          class="nav-item"
          :class="{ active: activeTab === tab.path }"
        >
          {{ tab.label }}
        </router-link>
      </nav>
    </div>

    <div class="shadow-ai-content">
      <router-view />
    </div>
  </div>
</template>

<style scoped>
.shadow-ai-layout {
  display: flex;
  height: 100vh;
  height: 100dvh;
  background: var(--bg-primary);
}

.shadow-ai-sidebar {
  width: 200px;
  min-width: 200px;
  background: var(--bg-secondary);
  border-right: 1px solid var(--border);
  display: flex;
  flex-direction: column;
}

.sidebar-header {
  padding: 16px;
  border-bottom: 1px solid var(--border);
}

.sidebar-header h1 {
  font-size: 18px;
  font-weight: 600;
  margin-top: 12px;
}

.btn-back {
  display: flex;
  align-items: center;
  gap: 6px;
  color: var(--text-secondary);
  font-size: 13px;
  padding: 6px 0;
  transition: color var(--transition);
}

.btn-back:hover {
  color: var(--text-primary);
}

.sidebar-nav {
  flex: 1;
  padding: 8px;
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.nav-item {
  display: block;
  padding: 10px 12px;
  border-radius: var(--radius);
  font-size: 13px;
  color: var(--text-secondary);
  text-decoration: none;
  transition: all var(--transition);
}

.nav-item:hover {
  background: var(--bg-hover);
  color: var(--text-primary);
}

.nav-item.active {
  background: var(--bg-active);
  color: var(--text-primary);
  font-weight: 500;
}

.shadow-ai-content {
  flex: 1;
  overflow-y: auto;
}

@media (max-width: 768px) {
  .shadow-ai-layout {
    flex-direction: column;
  }

  .shadow-ai-sidebar {
    width: 100%;
    min-width: 100%;
    border-right: none;
    border-bottom: 1px solid var(--border);
  }

  .sidebar-nav {
    flex-direction: row;
    overflow-x: auto;
    gap: 4px;
    padding: 8px 12px;
  }

  .nav-item {
    white-space: nowrap;
    padding: 8px 12px;
  }
}
</style>
