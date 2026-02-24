<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { listMcpServers, toggleMcpServer } from '../composables/api'
import type { McpServerItem } from '../composables/api'

const servers = ref<McpServerItem[]>([])
const loading = ref(false)

async function load() {
  loading.value = true
  try { servers.value = await listMcpServers() } catch { servers.value = [] }
  loading.value = false
}

async function onToggle(s: McpServerItem) {
  const newState = !s.enabled
  s.enabled = newState
  try {
    await toggleMcpServer(s.name, newState)
  } catch {
    s.enabled = !newState
  }
}

onMounted(load)
</script>

<template>
  <div class="mcp-page">
    <div class="page-header">
      <h2 class="page-title">MCP 服务器</h2>
      <span class="page-desc">管理 Claude Code 的 MCP 服务器连接</span>
    </div>
    <div v-if="loading" class="empty-state">加载中...</div>
    <div v-else-if="servers.length === 0" class="empty-state">暂无 MCP 服务器</div>
    <div class="card-list">
      <div v-for="s in servers" :key="s.name" class="card">
        <div class="card-body">
          <div class="card-name">{{ s.name }}</div>
          <div class="card-detail">{{ s.url || s.command || '—' }}</div>
          <div class="card-meta">
            <span class="tag" :class="'tag-' + s.type">{{ s.type }}</span>
          </div>
        </div>
        <label class="toggle">
          <input type="checkbox" :checked="s.enabled" @change="onToggle(s)" />
          <span class="toggle-slider"></span>
        </label>
      </div>
    </div>
  </div>
</template>

<style scoped>
.mcp-page { padding: 24px; overflow-y: auto; height: 100%; }
.page-header { margin-bottom: 20px; }
.page-title { font-size: 18px; font-weight: 600; color: var(--text-primary); margin: 0; }
.page-desc { font-size: 12px; color: var(--text-muted); margin-top: 4px; display: block; }
.empty-state { text-align: center; color: var(--text-muted); padding: 48px 16px; font-size: 14px; }
.card-list { display: flex; flex-direction: column; gap: 6px; }
.card {
  display: flex; align-items: center; justify-content: space-between;
  padding: 12px 16px; background: var(--bg-secondary); border: 1px solid var(--border);
  border-radius: var(--radius); transition: background var(--transition);
}
.card:hover { background: var(--bg-hover); }
.card-body { flex: 1; min-width: 0; }
.card-name { font-size: 14px; font-weight: 500; color: var(--text-primary); }
.card-detail {
  font-size: 12px; color: var(--text-secondary); margin-top: 2px;
  font-family: 'SF Mono', 'Fira Code', monospace;
  white-space: nowrap; overflow: hidden; text-overflow: ellipsis;
}
.card-meta { margin-top: 6px; display: flex; gap: 6px; }
.tag { font-size: 11px; padding: 2px 8px; border-radius: 9999px; }
.tag-http { background: var(--accent-soft); color: var(--accent); }
.tag-stdio { background: rgba(251,146,60,0.15); color: #fb923c; }
.toggle { position: relative; display: inline-block; width: 36px; height: 20px; flex-shrink: 0; cursor: pointer; }
.toggle input { opacity: 0; width: 0; height: 0; }
.toggle-slider {
  position: absolute; inset: 0; background: var(--bg-tertiary); border-radius: 10px;
  transition: background 0.2s; border: 1px solid var(--border);
}
.toggle-slider::before {
  content: ''; position: absolute; width: 14px; height: 14px; left: 2px; top: 2px;
  background: var(--text-muted); border-radius: 50%; transition: transform 0.2s, background 0.2s;
}
.toggle input:checked + .toggle-slider { background: var(--accent); border-color: var(--accent); }
.toggle input:checked + .toggle-slider::before { transform: translateX(16px); background: var(--btn-text); }
</style>
