<script setup lang="ts">
import { ref, watch, onMounted, onBeforeUnmount, nextTick } from 'vue'
import { listFiles, readFileContent, writeFileContent, createFileApi, deleteFileApi, getTemplateVars, getDefaultFile, searchMemory } from '../composables/api'
import type { TemplateVar, MemorySearchResult } from '../composables/api'

interface FileItem {
  name: string
  path: string
  exists: boolean
}

const fileLabels: Record<string, string> = {}

function getLabel(f: FileItem): string {
  return fileLabels[f.path] || fileLabels[f.name] || f.name.replace(/\.md$/, '')
}

const tabs: { key: string; label: string; desc: string }[] = [
  { key: 'rules', label: '全局', desc: '~/.ai-hub/rules/' },
  { key: 'memory', label: '记忆', desc: '~/.ai-hub/memory/' },
  { key: 'notes', label: '笔记', desc: '~/.ai-hub/notes/' },
]

type Scope = 'rules' | 'memory' | 'notes'

const activeTab = ref<Scope>('rules')
const activeTabDesc = ref('~/.ai-hub/rules/')
const files = ref<FileItem[]>([])
const selectedFile = ref<FileItem | null>(null)
const content = ref('')
const loading = ref(false)
const saving = ref(false)
const showNewDialog = ref(false)
const newFileName = ref('')
const templateVars = ref<TemplateVar[]>([])
const showVars = ref(false)
const restoringDefault = ref(false)

function isFileTab(): boolean {
  return true
}

// Validate JSON and update error message

// Vector search
const searchQuery = ref('')
const searching = ref(false)
const searchResults = ref<MemorySearchResult[]>([])
const searchDone = ref(false)

function similarityColor(s: number): string {
  if (s >= 0.7) return 'var(--success, #22c55e)'
  if (s >= 0.4) return 'var(--warning, #eab308)'
  return 'var(--text-muted)'
}

function levelLabel(level: string): string {
  switch (level) {
    case 'session': return '会话'
    case 'team': return '团队'
    case 'global': return '全局'
    default: return level
  }
}

function levelClass(level: string): string {
  switch (level) {
    case 'session': return 'level-session'
    case 'team': return 'level-team'
    case 'global': return 'level-global'
    default: return ''
  }
}

function formatTime(t: string): string {
  if (!t) return '-'
  try {
    const d = new Date(t)
    return d.toLocaleDateString('zh-CN', { month: '2-digit', day: '2-digit' }) + ' ' + d.toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit' })
  } catch { return t }
}

async function onSearch() {
  const q = searchQuery.value.trim()
  if (!q) return
  searching.value = true
  searchResults.value = []
  searchDone.value = false
  try {
    const res = await searchMemory(q, 10)
    searchResults.value = res.results || []
  } catch (e: any) {
    searchResults.value = []
  }
  searchDone.value = true
  searching.value = false
}

// 点击搜索结果项：关闭搜索框 + 定位到文件列表中对应文件
async function locateFile(r: MemorySearchResult) {
  // 搜索结果都是 memory 类型，切换到记忆 tab
  if (activeTab.value !== 'memory') {
    activeTab.value = 'memory'
    await nextTick()
    // watch 会触发 loadFiles，等文件列表加载完
    await loadFiles()
  }
  // 在文件列表中找到对应文件并选中
  const target = files.value.find(f => f.name === r.id)
  if (target) {
    selectFile(target)
  }
  // 关闭搜索结果
  closeSearch()
}

function closeSearch() {
  searchResults.value = []
  searchDone.value = false
}

// 点击外部关闭搜索结果
const searchBarRef = ref<HTMLElement | null>(null)

function onClickOutside(e: MouseEvent) {
  if (searchBarRef.value && !searchBarRef.value.contains(e.target as Node)) {
    closeSearch()
  }
}

onMounted(async () => {
  loadFiles()
  try { templateVars.value = await getTemplateVars() } catch {}
  document.addEventListener('click', onClickOutside)
})


async function restoreDefault() {
  if (!selectedFile.value) return
  if (!confirm('确定恢复为内置默认模板？当前编辑内容将被替换（需点"保存"才生效）')) return
  restoringDefault.value = true
  try {
    const res = await getDefaultFile(selectedFile.value.path)
    content.value = res.content
  } catch (e: any) {
    alert('获取默认模板失败: ' + e.message)
  }
  restoringDefault.value = false
}

async function loadFiles() {
  loading.value = true
  try {
    files.value = await listFiles(activeTab.value)
  } catch {
    files.value = []
  }
  loading.value = false
}

async function selectFile(f: FileItem) {
  selectedFile.value = f
  try {
    const res = await readFileContent(activeTab.value, f.path)
    content.value = res.content
  } catch {
    content.value = ''
  }
}

async function saveFile() {
  if (!selectedFile.value) return
  saving.value = true
  try {
    await writeFileContent(activeTab.value, selectedFile.value.path, content.value)
    await loadFiles()
    const updated = files.value.find(f => f.path === selectedFile.value!.path)
    if (updated) selectedFile.value = updated
  } catch (e: any) {
    alert('保存失败: ' + e.message)
  }
  saving.value = false
}

async function createNew() {
  let name = newFileName.value.trim()
  if (!name) return
  if (!name.endsWith('.md')) name += '.md'
  const scope = activeTab.value
  const path = scope === 'rules' && name === 'CLAUDE.md' ? 'CLAUDE.md' : scope + '/' + name
  try {
    await createFileApi(scope, path, '')
    showNewDialog.value = false
    newFileName.value = ''
    await loadFiles()
    const created = files.value.find(f => f.path === path)
    if (created) selectFile(created)
  } catch (e: any) {
    alert('创建失败: ' + e.message)
  }
}

async function deleteFile(f: FileItem) {
  if (!confirm(`确定删除 ${f.name}？`)) return
  try {
    await deleteFileApi(activeTab.value, f.path)
    if (selectedFile.value?.path === f.path) {
      selectedFile.value = null
      content.value = ''
    }
    await loadFiles()
  } catch (e: any) {
    alert('删除失败: ' + e.message)
  }
}

function varTag(name: string): string {
  return '\u007B\u007B' + name + '\u007D\u007D'
}

function insertVar(name: string) {
  content.value += `{{${name}}}`
}


watch(activeTab, () => {
  selectedFile.value = null
  content.value = ''
  activeTabDesc.value = tabs.find(t => t.key === activeTab.value)?.desc ?? ''
  loadFiles()
})

onBeforeUnmount(() => {
  document.removeEventListener('click', onClickOutside)
})
</script>

<template>
  <div class="manage">
    <div class="manage-header">
      <div class="tabs">
        <button
          v-for="tab in tabs"
          :key="tab.key"
          class="tab"
          :class="{ active: activeTab === tab.key }"
          @click="activeTab = tab.key as Scope"
        >{{ tab.label }}</button>
      </div>
      <div class="tab-desc">{{ activeTabDesc }}</div>
      <div v-if="isFileTab()" class="search-bar" ref="searchBarRef">
        <div class="search-inputs">
          <input
            v-model="searchQuery"
            class="search-input"
            placeholder="输入关键词进行语义搜索"
            @keyup.enter="onSearch"
          />
          <button class="btn-search" :disabled="searching || !searchQuery.trim()" @click="onSearch">
            <svg v-if="searching" class="spinning" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M21 12a9 9 0 11-6.219-8.56"/></svg>
            <svg v-else width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="11" cy="11" r="8"/><path d="M21 21l-4.35-4.35"/></svg>
            {{ searching ? '搜索中...' : '搜索' }}
          </button>
        </div>
        <div v-if="searchDone && searchResults.length === 0" class="search-empty">未找到相关结果</div>
        <div v-if="searchResults.length > 0" class="search-results">
          <div v-for="r in searchResults" :key="r.id + (r.origin || '')" class="search-result-item" @click="locateFile(r)">
            <div class="result-header">
              <span class="result-level" :class="levelClass(r.origin || r.level)">{{ levelLabel(r.origin || r.level) }}</span>
              <span class="result-filename">{{ r.id }}</span>
              <span class="result-score" :style="{ color: similarityColor(r.similarity) }">{{ (r.similarity * 100).toFixed(1) }}%</span>
            </div>
            <div class="result-meta">
              <span class="meta-item" title="命中次数">命中 {{ r.hit_count || 0 }}</span>
              <span class="meta-item" title="阅读次数">阅读 {{ r.read_count || 0 }}</span>
              <span class="meta-item" title="创建时间">创建 {{ formatTime(r.created_at) }}</span>
              <span class="meta-item" title="更新时间">更新 {{ formatTime(r.updated_at) }}</span>
            </div>
            <div class="result-preview">{{ r.document.slice(0, 200) + (r.document.length > 200 ? '...' : '') }}</div>
          </div>
        </div>
      </div>
    </div>
    <div class="manage-body">
      <!-- File mode (rules / memory / notes) -->
      <template v-if="isFileTab()">
        <div class="file-list">
          <div class="file-list-header">
            <span class="file-list-title">文件</span>
            <button class="btn-sm" @click="showNewDialog = true">+ 新建</button>
          </div>
          <div v-if="showNewDialog" class="new-file-dialog">
            <input v-model="newFileName" placeholder="文件名.md" class="input-sm" @keyup.enter="createNew" />
            <button class="btn-sm btn-accent" @click="createNew">创建</button>
            <button class="btn-sm" @click="showNewDialog = false; newFileName = ''">取消</button>
          </div>
          <div v-if="loading" class="file-list-empty">加载中...</div>
          <div v-else-if="files.length === 0" class="file-list-empty">暂无文件</div>
          <div
            v-for="f in files"
            :key="f.path"
            class="file-item"
            :class="{ active: selectedFile?.path === f.path, ghost: !f.exists }"
            @click="selectFile(f)"
          >
            <div class="file-info">
              <span class="file-label">{{ getLabel(f) }}</span>
              <span class="file-subpath">{{ f.name }}</span>
            </div>
            <button class="btn-delete-file" @click.stop="deleteFile(f)" title="删除">
              <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <path d="M18 6L6 18M6 6l12 12"/>
              </svg>
            </button>
          </div>
        </div>
        <div class="editor-panel">
          <div v-if="selectedFile" class="editor-content">
            <div class="editor-toolbar">
              <span class="editor-filename">{{ selectedFile.path }}</span>
              <div class="editor-actions">
                <button
                  v-if="activeTab === 'rules'"
                  class="btn-default"
                  :disabled="restoringDefault"
                  @click="restoreDefault"
                >{{ restoringDefault ? '获取中...' : '默认' }}</button>
                <button class="btn-vars" @click="showVars = !showVars">
                  {{ showVars ? '隐藏变量' : '插入变量' }}
                </button>
                <button class="btn-save" :disabled="saving" @click="saveFile">
                  {{ saving ? '保存中...' : '保存' }}
                </button>
              </div>
            </div>
            <div v-if="showVars" class="vars-panel">
              <div v-for="v in templateVars" :key="v.name" class="var-item" @click="insertVar(v.name)">
                <span class="var-tag">{{ varTag(v.name) }}</span>
                <span class="var-desc">{{ v.desc }}</span>
                <span class="var-value">{{ v.value }}</span>
              </div>
            </div>
            <textarea v-model="content" class="editor-textarea" spellcheck="false" />
          </div>
          <div v-else class="editor-empty">选择一个文件进行编辑</div>
        </div>
      </template>

    </div>
  </div>
</template>

<style scoped>
.manage { display: flex; flex-direction: column; height: 100%; }
.manage-header { padding: 12px 16px; border-bottom: 1px solid var(--border); }
.tabs { display: flex; gap: 4px; }
.tab {
  padding: 6px 16px; border-radius: var(--radius-sm); font-size: 13px;
  color: var(--text-secondary); transition: all var(--transition);
}
.tab:hover { background: var(--bg-hover); color: var(--text-primary); }
.tab.active { background: var(--accent-soft); color: var(--accent); }
.tab-desc { font-size: 11px; color: var(--text-muted); margin-top: 6px; font-family: 'SF Mono', 'Fira Code', monospace; }
.manage-body { flex: 1; display: flex; min-height: 0; }
.file-list {
  width: 260px; min-width: 260px; border-right: 1px solid var(--border);
  display: flex; flex-direction: column; overflow-y: auto;
}
.file-list-header {
  display: flex; align-items: center; justify-content: space-between;
  padding: 10px 12px; border-bottom: 1px solid var(--border);
}
.file-list-title { font-size: 12px; font-weight: 600; color: var(--text-muted); text-transform: uppercase; }
.btn-sm { padding: 4px 10px; font-size: 12px; border-radius: var(--radius-sm); color: var(--text-secondary); transition: all var(--transition); }
.btn-sm:hover { background: var(--bg-hover); color: var(--text-primary); }
.btn-accent { color: var(--accent); }
.btn-accent:hover { background: var(--accent-soft); }
.new-file-dialog { display: flex; gap: 6px; padding: 8px 12px; border-bottom: 1px solid var(--border); align-items: center; }
.input-sm {
  flex: 1; padding: 4px 8px; font-size: 12px; background: var(--bg-tertiary);
  border: 1px solid var(--border); border-radius: var(--radius-sm); color: var(--text-primary);
}
.file-list-empty { padding: 24px 12px; text-align: center; color: var(--text-muted); font-size: 13px; }
.file-item { display: flex; align-items: center; padding: 8px 12px; cursor: pointer; transition: background var(--transition); }
.file-item:hover { background: var(--bg-hover); }
.file-item.active { background: var(--bg-active); }
.file-item.ghost { opacity: 0.5; }
.file-item.ghost.active { opacity: 1; }
.file-info { flex: 1; min-width: 0; }
.file-label { display: block; font-size: 13px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.file-subpath { display: block; font-size: 11px; color: var(--text-muted); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; margin-top: 1px; }
.btn-delete-file {
  opacity: 0; width: 22px; height: 22px; display: flex; align-items: center; justify-content: center;
  border-radius: var(--radius-sm); color: var(--text-muted); transition: all var(--transition); flex-shrink: 0;
}
.file-item:hover .btn-delete-file { opacity: 1; }
.btn-delete-file:hover { color: var(--danger); background: rgba(239, 68, 68, 0.1); }
.editor-panel { flex: 1; display: flex; flex-direction: column; min-width: 0; }
.editor-content { display: flex; flex-direction: column; height: 100%; }
.editor-toolbar {
  display: flex; align-items: center; justify-content: space-between;
  padding: 8px 16px; border-bottom: 1px solid var(--border);
}
.editor-filename { font-size: 12px; color: var(--text-muted); font-family: 'SF Mono', 'Fira Code', monospace; }
.editor-actions { display: flex; gap: 8px; align-items: center; }
.btn-vars {
  padding: 4px 12px; font-size: 12px; border-radius: var(--radius-sm);
  color: var(--text-secondary); transition: all var(--transition);
}
.btn-vars:hover { background: var(--bg-hover); color: var(--text-primary); }
.btn-default {
  padding: 4px 12px; font-size: 12px; border-radius: var(--radius-sm);
  color: var(--text-secondary); transition: all var(--transition);
}
.btn-default:hover { background: var(--bg-hover); color: var(--text-primary); }
.btn-default:disabled { opacity: 0.5; cursor: not-allowed; }
.btn-save {
  padding: 6px 16px; font-size: 12px; border-radius: var(--radius-sm);
  background: var(--accent); color: var(--btn-text); transition: all var(--transition);
}
.btn-save:hover { background: var(--accent-hover); }
.btn-save:disabled { opacity: 0.5; cursor: not-allowed; }
.vars-panel {
  padding: 8px 16px; border-bottom: 1px solid var(--border);
  background: var(--bg-secondary); display: flex; flex-wrap: wrap; gap: 6px;
}
.var-item {
  display: flex; align-items: center; gap: 6px; padding: 4px 8px;
  border-radius: var(--radius-sm); background: var(--bg-tertiary);
  cursor: pointer; transition: all var(--transition); font-size: 12px;
}
.var-item:hover { background: var(--bg-hover); }
.var-tag { color: var(--accent); font-family: 'SF Mono', 'Fira Code', monospace; font-size: 11px; }
.var-desc { color: var(--text-secondary); }
.var-value { color: var(--text-muted); font-family: 'SF Mono', 'Fira Code', monospace; font-size: 11px; }
.editor-textarea {
  flex: 1; padding: 16px; font-family: 'SF Mono', 'Fira Code', monospace;
  font-size: 13px; line-height: 1.6; resize: none;
  background: var(--bg-primary); color: var(--text-primary); border: none;
}
/* Vector search */
.search-bar { margin-top: 10px; }
.search-inputs { display: flex; gap: 6px; align-items: center; }
.search-input {
  flex: 1; padding: 6px 10px; font-size: 13px; border-radius: var(--radius);
  border: 1px solid var(--border); background: var(--bg-primary); color: var(--text-primary);
}
.search-scope {
  padding: 6px 8px; font-size: 12px; border-radius: var(--radius);
  border: 1px solid var(--border); background: var(--bg-primary); color: var(--text-primary);
}
.btn-search {
  display: flex; align-items: center; gap: 4px; padding: 6px 12px;
  border-radius: var(--radius); font-size: 12px; font-weight: 500;
  background: var(--accent); color: var(--btn-text); transition: opacity var(--transition);
  cursor: pointer; white-space: nowrap; flex-shrink: 0;
}
.btn-search:hover:not(:disabled) { opacity: 0.9; }
.btn-search:disabled { opacity: 0.5; cursor: not-allowed; }
.btn-search .spinning { animation: spin 1s linear infinite; }
@keyframes spin { to { transform: rotate(360deg); } }
.search-empty { padding: 12px 0; text-align: center; color: var(--text-muted); font-size: 13px; }
.search-results { margin-top: 8px; max-height: 260px; overflow-y: auto; display: flex; flex-direction: column; gap: 4px; }
.search-result-item {
  padding: 8px 10px; background: var(--bg-secondary); border: 1px solid var(--border);
  border-radius: var(--radius); cursor: pointer; transition: background var(--transition);
}
.search-result-item:hover { background: var(--bg-hover); }
.result-header { display: flex; align-items: center; gap: 8px; margin-bottom: 4px; cursor: pointer; }
.result-level {
  font-size: 10px; padding: 1px 6px; border-radius: 9999px; flex-shrink: 0; font-weight: 500;
}
.level-session { background: rgba(59,130,246,0.15); color: #3b82f6; }
.level-team { background: rgba(168,85,247,0.15); color: #a855f7; }
.level-global { background: rgba(34,197,94,0.15); color: #22c55e; }
.result-filename {
  font-size: 12px; color: var(--text-primary); font-weight: 500;
  flex: 1; white-space: nowrap; overflow: hidden; text-overflow: ellipsis;
}
.result-score { font-size: 12px; font-weight: 600; flex-shrink: 0; font-family: 'SF Mono', 'Fira Code', monospace; }
.result-preview {
  font-size: 12px; color: var(--text-muted); line-height: 1.5;
  white-space: pre-wrap; word-break: break-all; cursor: pointer;
}
.result-meta {
  display: flex; gap: 10px; margin-bottom: 4px; flex-wrap: wrap;
}
.meta-item {
  font-size: 11px; color: var(--text-muted); white-space: nowrap;
}
.result-actions {
  margin-top: 6px; display: flex; gap: 6px;
}
.btn-view {
  padding: 3px 10px; font-size: 11px; border-radius: var(--radius-sm);
  background: var(--accent-soft); color: var(--accent); transition: all var(--transition);
  cursor: pointer;
}
.btn-view:hover { background: var(--accent); color: var(--btn-text); }
.btn-view:disabled { opacity: 0.5; cursor: not-allowed; }
/* Content viewer modal */
.content-modal-overlay {
  position: fixed; inset: 0; background: rgba(0,0,0,0.5); z-index: 1000;
  display: flex; align-items: center; justify-content: center; padding: 24px;
}
.content-modal {
  background: var(--bg-primary); border: 1px solid var(--border); border-radius: var(--radius);
  width: 100%; max-width: 720px; max-height: 80vh; display: flex; flex-direction: column;
  box-shadow: 0 8px 32px rgba(0,0,0,0.2);
}
.content-modal-header {
  display: flex; align-items: center; gap: 8px; padding: 12px 16px;
  border-bottom: 1px solid var(--border); flex-shrink: 0;
}
.content-modal-title {
  font-size: 13px; font-weight: 600; color: var(--text-primary);
  flex: 1; white-space: nowrap; overflow: hidden; text-overflow: ellipsis;
}
.content-modal-scope {
  font-size: 11px; color: var(--text-muted); font-family: 'SF Mono', 'Fira Code', monospace;
  flex-shrink: 0;
}
.content-modal-close {
  width: 28px; height: 28px; display: flex; align-items: center; justify-content: center;
  border-radius: var(--radius-sm); color: var(--text-muted); transition: all var(--transition);
  flex-shrink: 0; cursor: pointer;
}
.content-modal-close:hover { background: var(--bg-hover); color: var(--text-primary); }
.content-modal-body {
  flex: 1; overflow-y: auto; padding: 16px; font-size: 13px; line-height: 1.6;
  font-family: 'SF Mono', 'Fira Code', monospace; color: var(--text-primary);
  white-space: pre-wrap; word-break: break-word; margin: 0;
}
/* Mobile */
@media (max-width: 768px) {
  .manage-header { padding: 10px 12px; }
  .tabs { flex-wrap: wrap; }
  .search-inputs { flex-wrap: wrap; }
  .search-input { min-width: 0; }
  .manage-body { flex-direction: column; }
  .file-list { width: 100%; min-width: 0; border-right: none; border-bottom: 1px solid var(--border); max-height: 200px; }
  .editor-toolbar { flex-wrap: wrap; gap: 6px; padding: 8px 12px; }
  .editor-filename { font-size: 11px; width: 100%; }
  .editor-actions { width: 100%; justify-content: flex-end; }
  .editor-textarea { padding: 12px; font-size: 12px; }
  .vars-panel { padding: 6px 12px; }
}


.save-ok-inline { color: #22c55e; font-size: 13px; }

.history-panel {
  max-height: 240px; overflow-y: auto;
  border-bottom: 1px solid var(--border);
  background: var(--bg-secondary); padding: 12px 16px;
}
.history-loading, .history-empty {
  font-size: 12px; color: var(--text-muted); text-align: center; padding: 16px;
}
.history-timeline { display: flex; flex-direction: column; gap: 8px; }
.history-item {
  padding: 8px 12px; border-radius: var(--radius-sm);
  background: var(--bg-tertiary); border-left: 3px solid var(--accent);
}
.history-meta {
  display: flex; align-items: center; gap: 8px; flex-wrap: wrap;
  margin-bottom: 4px;
}
.history-version {
  font-size: 11px; font-weight: 700; color: var(--accent);
  font-family: 'SF Mono', 'Fira Code', monospace;
}
.history-type {
  font-size: 10px; padding: 1px 6px; border-radius: 3px;
  font-weight: 600; text-transform: uppercase;
}
.history-type.create { background: rgba(34,197,94,0.15); color: #22c55e; }
.history-type.update { background: rgba(59,130,246,0.15); color: #3b82f6; }
.history-type.delete { background: rgba(239,68,68,0.15); color: #ef4444; }
.history-time { font-size: 11px; color: var(--text-muted); }
.history-session { font-size: 11px; color: var(--text-muted); }
.btn-rollback {
  font-size: 10px; padding: 1px 8px; border-radius: 3px;
  background: var(--bg-hover); color: var(--text-secondary); cursor: pointer;
  transition: all var(--transition); margin-left: auto; border: 1px solid var(--border);
}
.btn-rollback:hover { background: var(--accent-soft); color: var(--accent); border-color: var(--accent); }
.btn-rollback:disabled { opacity: 0.5; cursor: not-allowed; }
.history-diff {
  font-size: 11px; color: var(--text-secondary); line-height: 1.4;
  white-space: pre-wrap; word-break: break-word;
}

</style>
