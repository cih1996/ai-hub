<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import * as api from '../composables/api'

const props = defineProps<{
  groupName: string
  visible: boolean
}>()

const emit = defineEmits<{ (e: 'close'): void }>()

type TabKey = 'knowledge' | 'memory' | 'rules'

interface TabDef {
  key: TabKey
  label: string
}

const tabs: TabDef[] = [
  { key: 'knowledge', label: '知识库' },
  { key: 'memory', label: '记忆库' },
  { key: 'rules', label: '团队规则' },
]

const activeTab = ref<TabKey>('knowledge')

interface FileItem {
  name: string
  content: string | null   // null = not yet loaded
  expanded: boolean
  loading: boolean
}

const files = ref<FileItem[]>([])
const listLoading = ref(false)
const listError = ref('')

const scope = computed(() => `${props.groupName}/${activeTab.value}`)

async function loadFiles() {
  if (!props.visible || !props.groupName) return
  listLoading.value = true
  listError.value = ''
  files.value = []
  try {
    const names = await api.listVectorFiles(scope.value)
    files.value = names.map((name) => ({ name, content: null, expanded: false, loading: false }))
  } catch (e: any) {
    listError.value = e.message || '加载失败'
  } finally {
    listLoading.value = false
  }
}

async function toggleFile(item: FileItem) {
  if (item.expanded) {
    item.expanded = false
    return
  }
  if (item.content !== null) {
    item.expanded = true
    return
  }
  item.loading = true
  try {
    const res = await api.readVectorFile(scope.value, item.name)
    item.content = res.content
    item.expanded = true
  } catch {
    item.content = '读取文件内容失败'
    item.expanded = true
  } finally {
    item.loading = false
  }
}

function switchTab(tab: TabKey) {
  activeTab.value = tab
  files.value = []
}

// Re-load whenever modal becomes visible or tab changes
watch(
  () => [props.visible, props.groupName, activeTab.value] as const,
  ([visible]) => {
    if (visible) loadFiles()
  },
  { immediate: true }
)
</script>

<template>
  <Teleport to="body">
    <div v-if="visible" class="tdm-overlay" @click.self="emit('close')">
      <div class="tdm-box" role="dialog" :aria-label="`${groupName} 团队详情`">
        <!-- Header -->
        <div class="tdm-header">
          <div class="tdm-title">
            <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <path d="M17 21v-2a4 4 0 00-4-4H5a4 4 0 00-4 4v2"/>
              <circle cx="9" cy="7" r="4"/>
              <path d="M23 21v-2a4 4 0 00-3-3.87"/>
              <path d="M16 3.13a4 4 0 010 7.75"/>
            </svg>
            <span>{{ groupName }}</span>
          </div>
          <button class="tdm-close" @click="emit('close')" title="关闭">
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <path d="M18 6L6 18M6 6l12 12"/>
            </svg>
          </button>
        </div>

        <!-- Tabs -->
        <div class="tdm-tabs">
          <button
            v-for="tab in tabs"
            :key="tab.key"
            class="tdm-tab"
            :class="{ active: activeTab === tab.key }"
            @click="switchTab(tab.key)"
          >{{ tab.label }}</button>
        </div>

        <!-- Content -->
        <div class="tdm-content">
          <!-- Loading -->
          <div v-if="listLoading" class="tdm-state">
            <svg class="tdm-spin" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <path d="M21 12a9 9 0 11-6.219-8.56"/>
            </svg>
            <span>加载中…</span>
          </div>

          <!-- Error -->
          <div v-else-if="listError" class="tdm-state tdm-error">{{ listError }}</div>

          <!-- Empty -->
          <div v-else-if="files.length === 0" class="tdm-state tdm-empty">
            <svg width="32" height="32" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
              <path d="M14 2H6a2 2 0 00-2 2v16a2 2 0 002 2h12a2 2 0 002-2V8z"/>
              <polyline points="14 2 14 8 20 8"/>
            </svg>
            <span>暂无{{ tabs.find(t => t.key === activeTab)?.label }}文件</span>
          </div>

          <!-- File list -->
          <div v-else class="tdm-file-list">
            <div
              v-for="item in files"
              :key="item.name"
              class="tdm-file"
            >
              <!-- File header row -->
              <button class="tdm-file-header" @click="toggleFile(item)">
                <svg
                  class="tdm-chevron"
                  :class="{ expanded: item.expanded }"
                  width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"
                >
                  <polyline points="6 9 12 15 18 9"/>
                </svg>
                <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                  <path d="M14 2H6a2 2 0 00-2 2v16a2 2 0 002 2h12a2 2 0 002-2V8z"/>
                  <polyline points="14 2 14 8 20 8"/>
                </svg>
                <span class="tdm-file-name">{{ item.name }}</span>
                <svg v-if="item.loading" class="tdm-spin tdm-inline-spin" width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                  <path d="M21 12a9 9 0 11-6.219-8.56"/>
                </svg>
              </button>

              <!-- File content -->
              <div v-if="item.expanded && item.content !== null" class="tdm-file-content">
                <pre>{{ item.content }}</pre>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  </Teleport>
</template>

<style scoped>
.tdm-overlay {
  position: fixed;
  inset: 0;
  background: var(--overlay);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 1000;
  padding: 16px;
}
.tdm-box {
  background: var(--bg-secondary);
  border: 1px solid var(--border);
  border-radius: 12px;
  width: 560px;
  max-width: 100%;
  max-height: 80vh;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}
.tdm-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 16px 20px 12px;
  border-bottom: 1px solid var(--border);
  flex-shrink: 0;
}
.tdm-title {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 15px;
  font-weight: 600;
  color: var(--text-primary);
  min-width: 0;
}
.tdm-title span {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.tdm-close {
  width: 28px;
  height: 28px;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: var(--radius-sm);
  color: var(--text-muted);
  flex-shrink: 0;
  transition: all var(--transition);
}
.tdm-close:hover {
  background: var(--bg-hover);
  color: var(--text-primary);
}
.tdm-tabs {
  display: flex;
  gap: 0;
  padding: 0 20px;
  border-bottom: 1px solid var(--border);
  flex-shrink: 0;
}
.tdm-tab {
  padding: 10px 16px;
  font-size: 13px;
  font-weight: 500;
  color: var(--text-muted);
  border-bottom: 2px solid transparent;
  margin-bottom: -1px;
  transition: all var(--transition);
  cursor: pointer;
}
.tdm-tab:hover {
  color: var(--text-primary);
}
.tdm-tab.active {
  color: var(--accent);
  border-bottom-color: var(--accent);
}
.tdm-content {
  flex: 1;
  overflow-y: auto;
  padding: 12px 0;
}
.tdm-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 10px;
  padding: 40px 20px;
  color: var(--text-muted);
  font-size: 13px;
}
.tdm-error {
  color: var(--danger, #ef4444);
}
.tdm-empty {
  opacity: 0.6;
}
.tdm-spin {
  animation: tdm-spin 1s linear infinite;
}
.tdm-inline-spin {
  margin-left: auto;
  flex-shrink: 0;
  color: var(--text-muted);
}
@keyframes tdm-spin { to { transform: rotate(360deg); } }
.tdm-file-list {
  padding: 0 12px;
}
.tdm-file {
  border-radius: var(--radius);
  margin-bottom: 4px;
  overflow: hidden;
  border: 1px solid var(--border);
}
.tdm-file-header {
  width: 100%;
  display: flex;
  align-items: center;
  gap: 7px;
  padding: 9px 12px;
  text-align: left;
  font-size: 13px;
  color: var(--text-primary);
  background: var(--bg-tertiary);
  cursor: pointer;
  transition: background var(--transition);
}
.tdm-file-header:hover {
  background: var(--bg-hover);
}
.tdm-chevron {
  flex-shrink: 0;
  color: var(--text-muted);
  transition: transform var(--transition);
}
.tdm-chevron.expanded {
  transform: rotate(180deg);
}
.tdm-file-name {
  flex: 1;
  font-weight: 500;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.tdm-file-content {
  background: var(--bg-primary);
  border-top: 1px solid var(--border);
  max-height: 300px;
  overflow-y: auto;
}
.tdm-file-content pre {
  margin: 0;
  padding: 12px 14px;
  font-size: 12px;
  line-height: 1.6;
  color: var(--text-secondary);
  white-space: pre-wrap;
  word-break: break-word;
  font-family: 'SF Mono', 'Fira Code', 'Consolas', monospace;
}
</style>
