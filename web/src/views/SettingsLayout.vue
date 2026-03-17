<script setup lang="ts">
import { computed } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { useTheme, type ThemeMode } from '../composables/theme'

const router = useRouter()
const route = useRoute()
const { mode: themeMode, setMode } = useTheme()

const themeModeLabel: Record<string, string> = { system: '跟随系统', light: '亮色', dark: '暗色' }
function toggleTheme() {
  const order: ThemeMode[] = ['system', 'light', 'dark']
  const next = order[(order.indexOf(themeMode.value) + 1) % 3] ?? 'system'
  setMode(next)
}

const tabs = [
  { path: '/settings', label: '模型与压缩', exact: true },
  { path: '/settings/rules', label: '规则与记忆' },
  { path: '/settings/extensions', label: '技能与MCP' },
  { path: '/settings/data', label: '数据中心' },
  { path: '/settings/im', label: 'IM通讯' },
  { path: '/settings/triggers', label: '定时器' },
]

const activeTab = computed(() => {
  for (const tab of tabs) {
    if (tab.exact && route.path === tab.path) return tab.path
    if (!tab.exact && route.path.startsWith(tab.path)) return tab.path
  }
  return '/settings'
})
</script>

<template>
  <div class="settings-layout">
    <button class="floating-theme-btn" @click="toggleTheme" :title="'主题: ' + themeModeLabel[themeMode]">
      <svg v-if="themeMode === 'dark'" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M21 12.79A9 9 0 1111.21 3 7 7 0 0021 12.79z"/></svg>
      <svg v-else-if="themeMode === 'light'" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="5"/><path d="M12 1v2M12 21v2M4.22 4.22l1.42 1.42M18.36 18.36l1.42 1.42M1 12h2M21 12h2M4.22 19.78l1.42-1.42M18.36 5.64l1.42-1.42"/></svg>
      <svg v-else width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="2" y="3" width="20" height="14" rx="2"/><line x1="8" y1="21" x2="16" y2="21"/><line x1="12" y1="17" x2="12" y2="21"/></svg>
    </button>

    <div class="settings-sidebar">
      <div class="sidebar-header">
        <button class="btn-back" @click="router.push('/chat')">
          <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M19 12H5M12 19l-7-7 7-7"/>
          </svg>
          返回
        </button>
        <h1>设置</h1>
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

    <div class="settings-content">
      <router-view />
    </div>
  </div>
</template>

<style scoped>
.settings-layout {
  display: flex;
  height: 100vh;
  height: 100dvh;
  background: var(--bg-primary);
}

.settings-sidebar {
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

.settings-content {
  flex: 1;
  overflow-y: auto;
}

.floating-theme-btn {
  position: fixed;
  top: 16px;
  right: 16px;
  width: 36px;
  height: 36px;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: var(--radius);
  background: var(--bg-secondary);
  border: 1px solid var(--border);
  color: var(--text-secondary);
  transition: all var(--transition);
  z-index: 50;
  cursor: pointer;
}

.floating-theme-btn:hover {
  background: var(--bg-hover);
  color: var(--text-primary);
}

@media (max-width: 768px) {
  .settings-layout {
    flex-direction: column;
  }

  .settings-sidebar {
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
