<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import * as api from '../composables/api'

function exportTeam(groupName: string) {
  const a = document.createElement('a')
  a.href = api.exportTeamUrl(groupName)
  a.download = ''
  document.body.appendChild(a)
  a.click()
  document.body.removeChild(a)
}

const props = defineProps<{
  groupName: string
  visible: boolean
}>()

const emit = defineEmits<{ (e: 'close'): void }>()

type TabKey = 'memory' | 'rules'

interface TabDef {
  key: TabKey
  label: string
}

const tabs: TabDef[] = [
  { key: 'memory', label: '记忆库' },
  { key: 'rules', label: '团队规则' },
]

const activeTab = ref<TabKey>('memory')

interface FileItem {
  name: string
  content: string | null   // null = not yet loaded
  expanded: boolean
  loading: boolean
  editing: boolean
  draft: string
  saving: boolean
  deleting: boolean
  sourceSessionId: number  // 0 = unknown
  updatedAt: string        // RFC3339 mod time
}

function formatTime(rfc3339: string): string {
  if (!rfc3339) return ''
  try {
    const d = new Date(rfc3339)
    const now = new Date()
    const isToday =
      d.getFullYear() === now.getFullYear() &&
      d.getMonth() === now.getMonth() &&
      d.getDate() === now.getDate()
    const hh = String(d.getHours()).padStart(2, '0')
    const mm = String(d.getMinutes()).padStart(2, '0')
    if (isToday) return `今天 ${hh}:${mm}`
    const mo = String(d.getMonth() + 1).padStart(2, '0')
    const dd = String(d.getDate()).padStart(2, '0')
    // Same year: MM-DD HH:MM, otherwise full date
    if (d.getFullYear() === now.getFullYear()) return `${mo}-${dd} ${hh}:${mm}`
    return `${d.getFullYear()}-${mo}-${dd}`
  } catch {
    return ''
  }
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
    const res = await api.listVectorFilesRich(scope.value)
    files.value = res.files.map((f) => ({
      name: f.file_name,
      content: null,
      expanded: false,
      loading: false,
      editing: false,
      draft: '',
      saving: false,
      deleting: false,
      sourceSessionId: f.source_session_id,
      updatedAt: f.updated_at,
    }))
  } catch (e: any) {
    listError.value = e.message || '加载失败'
  } finally {
    listLoading.value = false
  }
}

async function loadContent(item: FileItem) {
  if (item.content !== null) return
  item.loading = true
  try {
    const res = await api.readVectorFile(scope.value, item.name)
    item.content = res.content
    item.draft = res.content
  } catch {
    item.content = '读取文件内容失败'
    item.draft = item.content
  } finally {
    item.loading = false
  }
}

async function toggleFile(item: FileItem) {
  if (item.expanded && !item.editing) {
    item.expanded = false
    return
  }
  await loadContent(item)
  item.expanded = true
}

async function startEdit(item: FileItem) {
  await loadContent(item)
  item.editing = true
  item.expanded = true
}

function cancelEdit(item: FileItem) {
  item.editing = false
  item.draft = item.content ?? ''
}

async function saveRule(item: FileItem) {
  item.saving = true
  try {
    await api.writeVectorFile(scope.value, item.name, item.draft)
    item.content = item.draft
    item.editing = false
  } catch (e: any) {
    alert('保存失败: ' + (e.message || '未知错误'))
  } finally {
    item.saving = false
  }
}

async function deleteFile(item: FileItem) {
  if (activeTab.value === 'rules') return
  if (!confirm(`确定删除 ${item.name}？`)) return
  item.deleting = true
  try {
    await api.deleteVectorFile(scope.value, item.name)
    files.value = files.value.filter((f) => f.name !== item.name)
  } catch (e: any) {
    alert('删除失败: ' + (e.message || '未知错误'))
  } finally {
    item.deleting = false
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
          <button class="tdm-export" @click="exportTeam(groupName)" title="导出团队">
            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <path d="M21 15v4a2 2 0 01-2 2H5a2 2 0 01-2-2v-4"/>
              <polyline points="17 8 12 3 7 8"/>
              <line x1="12" y1="3" x2="12" y2="15"/>
            </svg>
          </button>
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
              <div class="tdm-file-header" @click="toggleFile(item)">
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
                <span v-if="item.sourceSessionId > 0" class="tdm-meta-badge">会话 #{{ item.sourceSessionId }}</span>
                <span v-if="item.updatedAt" class="tdm-meta-time" :title="item.updatedAt">{{ formatTime(item.updatedAt) }}</span>
                <button
                  v-if="activeTab === 'rules'"
                  class="tdm-action-btn"
                  :disabled="item.loading || item.saving"
                  @click.stop="startEdit(item)"
                >编辑</button>
                <button
                  v-else
                  class="tdm-action-btn tdm-action-danger"
                  :disabled="item.deleting || item.loading"
                  @click.stop="deleteFile(item)"
                >{{ item.deleting ? '删除中…' : '删除' }}</button>
              </div>

              <!-- File content -->
              <div v-if="item.expanded && item.content !== null" class="tdm-file-content">
                <template v-if="activeTab === 'rules' && item.editing">
                  <textarea v-model="item.draft" class="tdm-editor" spellcheck="false" />
                  <div class="tdm-editor-actions">
                    <button class="tdm-action-btn" :disabled="item.saving" @click="cancelEdit(item)">取消</button>
                    <button class="tdm-action-btn tdm-action-primary" :disabled="item.saving" @click="saveRule(item)">
                      {{ item.saving ? '保存中…' : '保存' }}
                    </button>
                  </div>
                </template>
                <pre v-else>{{ item.content }}</pre>
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
.tdm-export {
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
.tdm-export:hover {
  background: var(--bg-hover);
  color: var(--accent);
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
.tdm-meta-badge {
  font-size: 10px;
  padding: 1px 5px;
  border-radius: 3px;
  background: color-mix(in srgb, var(--accent) 12%, transparent);
  color: var(--accent);
  white-space: nowrap;
  flex-shrink: 0;
}
.tdm-meta-time {
  font-size: 10px;
  color: var(--text-muted);
  white-space: nowrap;
  flex-shrink: 0;
  opacity: 0.8;
}
.tdm-file-content {
  background: var(--bg-primary);
  border-top: 1px solid var(--border);
  max-height: 300px;
  overflow-y: auto;
}
.tdm-action-btn {
  padding: 3px 8px;
  font-size: 11px;
  border-radius: var(--radius-sm);
  color: var(--text-secondary);
  background: var(--bg-primary);
  border: 1px solid var(--border);
  flex-shrink: 0;
}
.tdm-action-btn:hover { color: var(--text-primary); }
.tdm-action-btn:disabled { opacity: 0.6; cursor: not-allowed; }
.tdm-action-primary {
  color: var(--accent);
  border-color: color-mix(in srgb, var(--accent) 35%, var(--border));
}
.tdm-action-danger {
  color: var(--danger, #ef4444);
  border-color: color-mix(in srgb, var(--danger, #ef4444) 30%, var(--border));
}
.tdm-editor {
  width: 100%;
  min-height: 180px;
  padding: 12px 14px;
  border: none;
  resize: vertical;
  outline: none;
  font-size: 12px;
  line-height: 1.6;
  color: var(--text-primary);
  background: var(--bg-primary);
  font-family: 'SF Mono', 'Fira Code', 'Consolas', monospace;
}
.tdm-editor-actions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  padding: 10px 12px 12px;
  border-top: 1px solid var(--border);
  background: var(--bg-secondary);
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
