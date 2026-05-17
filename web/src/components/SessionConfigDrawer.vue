<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { marked } from 'marked'
import { useChatStore } from '../stores/chat'
import * as api from '../composables/api'
import IconPicker from './IconPicker.vue'

const props = defineProps<{
  visible: boolean
}>()

const emit = defineEmits<{
  (e: 'update:visible', val: boolean): void
}>()

const store = useChatStore()

// Tabs
const activeTab = ref<'role' | 'memory'>('role')

// --- Role State ---
const sessionRulesContent = ref('')
const sessionRulesSaving = ref(false)
const sessionRulesLoading = ref(false)
const groupsList = ref<api.Group[]>([])
const selectedGroupName = ref('')
const groupSaving = ref(false)

// --- Team Rules State ---
const teamRulesContent = ref('')
const teamRulesLoading = ref(false)

// --- Memory State ---
const memoryLoading = ref(false)
const memoryFiles = ref<api.ScopedFileRich[]>([])
const memorySearchQuery = ref('')

// For tree structure
interface FileNode {
  name: string
  path: string
  isFile: boolean
  children?: FileNode[]
  origin?: string
  file?: api.ScopedFileRich
  isOpen?: boolean // for directories
}
const memoryTree = ref<FileNode[]>([])

const filteredMemoryTree = computed(() => {
  if (!memorySearchQuery.value.trim()) {
    return memoryTree.value
  }
  const query = memorySearchQuery.value.toLowerCase()
  
  const filterNode = (node: FileNode): FileNode | null => {
    if (node.isFile) {
      return node.name.toLowerCase().includes(query) ? node : null
    }
    if (node.children) {
      const filteredChildren = node.children.map(filterNode).filter(Boolean) as FileNode[]
      if (filteredChildren.length > 0) {
        return { ...node, children: filteredChildren, isOpen: true } // auto expand if matched
      }
    }
    return null
  }
  
  return memoryTree.value.map(filterNode).filter(Boolean) as FileNode[]
})

const flatMemoryTree = computed(() => {
  const result: { node: FileNode, depth: number }[] = []
  function traverse(nodes: FileNode[], depth: number) {
    for (const node of nodes) {
      result.push({ node, depth })
      if (!node.isFile && node.isOpen && node.children) {
        traverse(node.children, depth + 1)
      }
    }
  }
  traverse(filteredMemoryTree.value, 0)
  return result
})

const memorySelectedFile = ref<api.ScopedFileRich | null>(null)
const memoryFileContent = ref('')
const memoryFileLoading = ref(false)
const memoryFileSaving = ref(false)
const memoryEditing = ref(false)
const memoryCreating = ref(false)
const memoryNewFileName = ref('')

// Toast state
const toastMsg = ref('')
const toastType = ref<'success' | 'error'>('success')
const toastVisible = ref(false)
let toastTimer: ReturnType<typeof setTimeout>

function showToast(msg: string, type: 'success' | 'error' = 'success') {
  toastMsg.value = msg
  toastType.value = type
  toastVisible.value = true
  clearTimeout(toastTimer)
  toastTimer = setTimeout(() => { toastVisible.value = false }, 2500)
}

function closeDrawer() {
  emit('update:visible', false)
}

watch(() => props.visible, async (newVal) => {
  if (newVal) {
    activeTab.value = 'role'
    await loadRoleData()
    await loadMemoryData()
  }
})

watch(activeTab, async (tab) => {
  if (!props.visible) return
  if (tab === 'memory') {
    await loadMemoryData()
  }
})

// --- Role Methods ---
async function loadRoleData() {
  const sid = store.currentSession?.id
  if (!sid) return
  sessionRulesLoading.value = true
  selectedGroupName.value = store.currentSession?.group_name || ''
  try {
    const [rulesRes, groupsRes] = await Promise.all([
      api.getSessionRules(sid).catch(() => ({ content: '' })),
      api.listGroups().catch(() => [])
    ])
    sessionRulesContent.value = rulesRes.content || ''
    groupsList.value = groupsRes
  } catch {
    sessionRulesContent.value = ''
    groupsList.value = []
  } finally {
    sessionRulesLoading.value = false
  }
  // Load team rules if session belongs to a team
  await loadTeamRules()
}

async function loadTeamRules() {
  const groupName = store.currentSession?.group_name || ''
  if (!groupName) {
    teamRulesContent.value = ''
    return
  }
  teamRulesLoading.value = true
  try {
    const res = await api.listScopedFiles(`${groupName}/rules`, { type: 'rules' })
    const files = res.files || []
    if (files.length === 0) {
      teamRulesContent.value = ''
      return
    }
    // Concatenate all team rule files
    const parts: string[] = []
    for (const f of files) {
      try {
        const content = await api.readScopedFile(`${groupName}/rules`, f.file_name, undefined, 'rules')
        if (content.content?.trim()) {
          parts.push(content.content.trim())
        }
      } catch { /* skip unreadable files */ }
    }
    teamRulesContent.value = parts.join('\n\n---\n\n')
  } catch {
    teamRulesContent.value = ''
  } finally {
    teamRulesLoading.value = false
  }
}

async function updateSessionGroup() {
  const sid = store.currentSession?.id
  if (!sid) return
  groupSaving.value = true
  try {
    await api.updateSession(sid, { group_name: selectedGroupName.value })
    if (store.currentSession) {
      store.currentSession.group_name = selectedGroupName.value
    }
    showToast('团队已更新')
    await loadMemoryData() // Team change might affect memory scope
  } catch (e: any) {
    showToast('更新失败: ' + (e.message || '未知错误'), 'error')
  } finally {
    groupSaving.value = false
  }
}

async function saveSessionRules() {
  const sid = store.currentSession?.id
  if (!sid) return
  sessionRulesSaving.value = true
  try {
    await api.putSessionRules(sid, sessionRulesContent.value)
    showToast('保存成功')
  } catch (e: any) {
    showToast('保存失败: ' + (e.message || '未知错误'), 'error')
  } finally {
    sessionRulesSaving.value = false
  }
}

async function deleteSessionRules() {
  const sid = store.currentSession?.id
  if (!sid) return
  await api.deleteSessionRules(sid)
  sessionRulesContent.value = ''
  showToast('已清除')
}

async function updateSessionIcon(icon: string) {
  const session = store.currentSession
  if (!session) return
  try {
    await api.updateSession(session.id, { icon })
    session.icon = icon
    showToast('图标已更新')
  } catch (e: any) {
    showToast('更新失败: ' + (e.message || '未知错误'), 'error')
  }
}

// --- Memory Methods ---
async function loadMemoryData() {
  const sid = store.currentSession?.id
  if (!sid) return
  memoryLoading.value = true
  try {
    const res = await api.listScopedFiles('', { session_id: sid, level: 'all', type: 'memory' })
    memoryFiles.value = res.files || []
    buildMemoryTree()
  } catch {
    memoryFiles.value = []
    memoryTree.value = []
  } finally {
    memoryLoading.value = false
  }
}

function buildMemoryTree() {
  // We want to group by origin: session, team, global
  const rootNodes: FileNode[] = [
    { name: '会话记忆', path: 'session', isFile: false, children: [], isOpen: true },
    { name: '团队记忆', path: 'team', isFile: false, children: [], isOpen: true },
    { name: '全局记忆', path: 'global', isFile: false, children: [], isOpen: true },
  ]
  
  for (const f of memoryFiles.value) {
    let parentNode: FileNode | undefined
    if (f.origin === 'session') parentNode = rootNodes[0]
    else if (f.origin === 'team') parentNode = rootNodes[1]
    else if (f.origin === 'global') parentNode = rootNodes[2]
    
    if (parentNode) {
      const parts = f.file_name.split('/')
      let currentParent = parentNode
      for (let i = 0; i < parts.length - 1; i++) {
        const folderName = parts[i] || ''
        let folderNode = currentParent.children!.find(c => c.name === folderName && !c.isFile)
        if (!folderNode) {
          folderNode = {
            name: folderName,
            path: parts.slice(0, i + 1).join('/'),
            isFile: false,
            children: [],
            isOpen: true,
            origin: f.origin
          }
          currentParent.children!.push(folderNode)
        }
        currentParent = folderNode as FileNode
      }
      currentParent.children!.push({
        name: parts[parts.length - 1] || '',
        path: f.file_name,
        isFile: true,
        origin: f.origin,
        file: f
      })
    }
  }
  
  // Filter out empty roots
  memoryTree.value = rootNodes.filter(n => n.children && n.children.length > 0)
}

function toggleNode(node: FileNode) {
  if (!node.isFile) {
    node.isOpen = !node.isOpen
  } else if (node.file) {
    selectMemoryFile(node.file)
  }
}

async function selectMemoryFile(file: api.ScopedFileRich) {
  memorySelectedFile.value = file
  memoryEditing.value = false
  memoryCreating.value = false
  memoryFileLoading.value = true
  try {
    const res = await api.readScopedFile(file.scope, file.file_name, undefined, 'memory')
    memoryFileContent.value = res.content || ''
  } catch {
    memoryFileContent.value = ''
  } finally {
    memoryFileLoading.value = false
  }
}

async function saveMemoryFile() {
  if (!memorySelectedFile.value) return
  memoryFileSaving.value = true
  try {
    await api.writeScopedFile(memorySelectedFile.value.scope, memorySelectedFile.value.file_name, memoryFileContent.value, undefined, 'memory')
    showToast('保存成功')
    memoryEditing.value = false
    await loadMemoryData()
  } catch (e: any) {
    showToast('保存失败: ' + (e.message || '未知错误'), 'error')
  } finally {
    memoryFileSaving.value = false
  }
}

async function deleteMemoryFile() {
  if (!memorySelectedFile.value) return
  if (!confirm(`确定删除「${memorySelectedFile.value.file_name}」？`)) return
  try {
    await api.deleteScopedFile(memorySelectedFile.value.scope, memorySelectedFile.value.file_name, undefined, 'memory')
    showToast('已删除')
    memorySelectedFile.value = null
    memoryFileContent.value = ''
    await loadMemoryData()
  } catch (e: any) {
    showToast('删除失败: ' + (e.message || '未知错误'), 'error')
  }
}

async function createMemoryFile() {
  const sid = store.currentSession?.id
  if (!sid || !memoryNewFileName.value.trim()) return
  let fileName = memoryNewFileName.value.trim()
  if (!fileName.endsWith('.md')) fileName += '.md'
  memoryFileSaving.value = true
  try {
    const res = await api.writeScopedFile('', fileName, memoryFileContent.value, sid, 'memory')
    showToast('创建成功')
    memoryCreating.value = false
    await loadMemoryData()
    const newFile = memoryFiles.value.find(f => f.file_name === fileName && f.scope === res.scope)
    if (newFile) selectMemoryFile(newFile)
  } catch (e: any) {
    showToast('创建失败: ' + (e.message || '未知错误'), 'error')
  } finally {
    memoryFileSaving.value = false
  }
}

function formatFileSize(bytes: number | undefined): string {
  if (bytes === undefined || bytes === null) return ''
  const kb = bytes / 1024
  if (kb < 1) return '<1 KB'
  if (kb > 1024) return (kb / 1024).toFixed(1) + ' MB'
  return Math.round(kb) + ' KB'
}

function onResetContext() {
  if (!confirm('确定重置上下文？将清空所有消息，保留会话配置。')) return
  store.resetContext()
}

marked.setOptions({ breaks: true, gfm: true })
function renderMd(text: string): string {
  if (!text) return ''
  return marked.parse(text) as string
}

</script>

<template>
  <Teleport to="body">
    <div class="drawer-overlay" :class="{ 'is-visible': visible }" @click="closeDrawer">
      <div class="drawer-content" :class="{ 'is-visible': visible }" @click.stop>
        
        <div class="drawer-header">
          <div class="drawer-title">
            <IconPicker
              :model-value="store.currentSession?.icon || ''"
              :entity-id="store.currentSession?.id"
              @update:model-value="updateSessionIcon"
            />
            <span>会话配置</span>
          </div>
          <button class="btn-close" @click="closeDrawer">
            <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <path d="M18 6L6 18M6 6l12 12"/>
            </svg>
          </button>
        </div>

        <div class="drawer-tabs">
          <button 
            class="drawer-tab" 
            :class="{ active: activeTab === 'role' }"
            @click="activeTab = 'role'"
          >
            角色规则
          </button>
          <button 
            class="drawer-tab" 
            :class="{ active: activeTab === 'memory' }"
            @click="activeTab = 'memory'"
          >
            记忆库
          </button>
        </div>

        <div class="drawer-body">
          <!-- Role Tab -->
          <div v-show="activeTab === 'role'" class="tab-pane">
            <div v-if="sessionRulesLoading" class="drawer-loading">加载中...</div>
            <template v-else>
              <div class="form-group">
                <label class="form-label">所属团队</label>
                <div class="group-select-row">
                  <select v-model="selectedGroupName" class="form-select" :disabled="groupSaving">
                    <option value="">无团队</option>
                    <option v-for="g in groupsList" :key="g.name" :value="g.name">
                      {{ g.name }}
                    </option>
                  </select>
                  <button
                    class="btn-secondary"
                    @click="updateSessionGroup"
                    :disabled="groupSaving || selectedGroupName === (store.currentSession?.group_name || '')"
                  >
                    {{ groupSaving ? '...' : '保存' }}
                  </button>
                </div>
              </div>

              <!-- Team Rules (read-only) -->
              <div v-if="teamRulesLoading" class="form-group">
                <label class="form-label">团队规则</label>
                <div class="drawer-loading">加载中...</div>
              </div>
              <div v-else-if="teamRulesContent" class="form-group">
                <label class="form-label">团队规则</label>
                <div class="team-rules-display memory-markdown" v-html="renderMd(teamRulesContent)"></div>
              </div>

              <div class="form-group flex-1" style="display: flex; flex-direction: column; min-height: 0;">
                <label class="form-label">角色规则</label>
                <textarea
                  v-model="sessionRulesContent"
                  class="form-textarea flex-1"
                  placeholder="输入会话角色规则（Markdown 格式）...&#10;&#10;例如：&#10;你是一名测试工程师，负责..."
                ></textarea>
              </div>
              
              <div class="drawer-actions">
                <button
                  class="btn-danger-outline"
                  @click="deleteSessionRules"
                  :disabled="!sessionRulesContent"
                >
                  清除
                </button>
                <button
                  class="btn-primary"
                  :disabled="sessionRulesSaving"
                  @click="saveSessionRules"
                >
                  {{ sessionRulesSaving ? '保存中...' : '保存' }}
                </button>
              </div>

              <div class="danger-zone">
                <div class="danger-zone-title">危险操作</div>
                <button
                  class="btn-danger-outline btn-reset-context"
                  :disabled="store.streaming"
                  @click="onResetContext"
                >
                  <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                    <path d="M3 12a9 9 0 1 0 9-9 9.75 9.75 0 0 0-6.74 2.74L3 8"/>
                    <path d="M3 3v5h5"/>
                  </svg>
                  重置上下文
                </button>
                <div class="danger-zone-hint">清空所有消息，保留会话配置</div>
              </div>
            </template>
          </div>

          <!-- Memory Tab -->
          <div v-show="activeTab === 'memory'" class="tab-pane memory-pane">
            <div class="memory-layout">
              <!-- Left: Tree -->
              <div class="memory-tree-sidebar">
                <div class="memory-sidebar-header">
                  <div class="search-input-wrapper">
                    <svg class="search-icon" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                      <circle cx="11" cy="11" r="8"/><line x1="21" y1="21" x2="16.65" y2="16.65"/>
                    </svg>
                    <input
                      v-model="memorySearchQuery"
                      class="memory-search-input"
                      placeholder="搜索记忆..."
                    />
                  </div>
                </div>
                <div class="memory-tree">
                  <div v-if="memoryLoading" class="drawer-loading">加载中...</div>
                  <div v-else-if="filteredMemoryTree.length === 0" class="drawer-empty">暂无匹配记忆</div>
                  <template v-else>
                    <div v-for="item in flatMemoryTree" :key="item.node.path" class="tree-node">
                      <div 
                        class="tree-node-row" 
                        @click="toggleNode(item.node)"
                        :class="{ 'is-file': item.node.isFile, 'active': memorySelectedFile && memorySelectedFile.file_name === item.node.file?.file_name && memorySelectedFile.scope === item.node.file?.scope }"
                        :style="{ paddingLeft: `${16 + item.depth * 20}px` }"
                      >
                        <span class="tree-icon">
                          <svg v-if="!item.node.isFile" :class="{ 'is-open': item.node.isOpen }" width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                            <polyline points="9 18 15 12 9 6"/>
                          </svg>
                          <svg v-else width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                            <path d="M14 2H6a2 2 0 00-2 2v16a2 2 0 002 2h12a2 2 0 002-2V8z"/>
                            <polyline points="14 2 14 8 20 8"/>
                          </svg>
                        </span>
                        <span class="tree-name">{{ item.node.name }}</span>
                        <span class="tree-size" v-if="item.node.isFile && item.node.file?.size !== undefined">{{ formatFileSize(item.node.file?.size) }}</span>
                      </div>
                    </div>
                  </template>
                </div>
              </div>
              
              <!-- Right: Content -->
              <div class="memory-editor">
                <template v-if="memoryCreating">
                  <div class="editor-header">
                    <input
                      v-model="memoryNewFileName"
                      class="form-input"
                      placeholder="文件名（如：工作总结.md）"
                    />
                  </div>
                  <textarea
                    v-model="memoryFileContent"
                    class="form-textarea flex-1"
                    placeholder="输入记忆内容..."
                  ></textarea>
                  <div class="drawer-actions">
                    <button class="btn-secondary" @click="memoryCreating = false">取消</button>
                    <button
                      class="btn-primary"
                      :disabled="!memoryNewFileName.trim() || memoryFileSaving"
                      @click="createMemoryFile"
                    >{{ memoryFileSaving ? '创建中...' : '创建' }}</button>
                  </div>
                </template>
                <template v-else-if="memorySelectedFile">
                  <div v-if="memoryFileLoading" class="drawer-loading">加载中...</div>
                  <template v-else>
                    <template v-if="!memoryEditing">
                      <div class="memory-markdown flex-1" v-html="renderMd(memoryFileContent)"></div>
                    </template>
                    <template v-else>
                      <textarea
                        v-model="memoryFileContent"
                        class="form-textarea flex-1"
                        placeholder="编辑记忆内容..."
                      ></textarea>
                    </template>
                    <div class="drawer-actions">
                      <button class="btn-danger-outline" @click="deleteMemoryFile">删除</button>
                      <template v-if="memoryEditing">
                        <button class="btn-secondary" @click="memoryEditing = false; selectMemoryFile(memorySelectedFile!)">取消</button>
                        <button
                          class="btn-primary"
                          :disabled="memoryFileSaving"
                          @click="saveMemoryFile"
                        >{{ memoryFileSaving ? '保存中...' : '保存' }}</button>
                      </template>
                      <button v-else class="btn-primary" @click="memoryEditing = true">编辑</button>
                    </div>
                  </template>
                </template>
                <div v-else class="drawer-empty flex-1 flex-center">← 选择一个记忆文件查看</div>
              </div>
            </div>
          </div>

        </div>
      </div>
    </div>

    <!-- Toast -->
    <div v-if="toastVisible" class="toast" :class="toastType">{{ toastMsg }}</div>
  </Teleport>
</template>

<style scoped>
.drawer-overlay {
  position: fixed;
  inset: 0;
  background: var(--overlay);
  z-index: 2000;
  opacity: 0;
  pointer-events: none;
  transition: opacity 0.3s ease;
}
.drawer-overlay.is-visible {
  opacity: 1;
  pointer-events: auto;
}

.drawer-content {
  position: fixed;
  top: 0;
  right: 0;
  bottom: 0;
  width: 760px;
  max-width: 100vw;
  background: var(--bg-primary);
  box-shadow: -4px 0 24px rgba(0, 0, 0, 0.1);
  z-index: 2001;
  transform: translateX(100%);
  transition: transform 0.3s cubic-bezier(0.16, 1, 0.3, 1);
  display: flex;
  flex-direction: column;
}
.drawer-content.is-visible {
  transform: translateX(0);
}

.drawer-header {
  padding: 16px 20px;
  border-bottom: 1px solid var(--border);
  display: flex;
  align-items: center;
  justify-content: space-between;
  background: var(--bg-secondary);
}

.drawer-title {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 16px;
  font-weight: 600;
  color: var(--text-primary);
}

.btn-close {
  background: none;
  border: none;
  color: var(--text-secondary);
  cursor: pointer;
  padding: 4px;
  border-radius: 4px;
  display: flex;
  align-items: center;
  justify-content: center;
}
.btn-close:hover {
  background: var(--bg-hover);
  color: var(--text-primary);
}

.drawer-tabs {
  display: flex;
  border-bottom: 1px solid var(--border);
  background: var(--bg-secondary);
  padding: 0 16px;
}

.drawer-tab {
  padding: 12px 20px;
  background: none;
  border: none;
  font-size: 15px;
  font-weight: 500;
  color: var(--text-secondary);
  cursor: pointer;
  border-bottom: 2px solid transparent;
  transition: all 0.2s;
}
.drawer-tab:hover {
  color: var(--text-primary);
}
.drawer-tab.active {
  color: var(--accent);
  border-bottom-color: var(--accent);
}

.drawer-body {
  flex: 1;
  overflow: hidden;
  display: flex;
  flex-direction: column;
}

.tab-pane {
  flex: 1;
  display: flex;
  flex-direction: column;
  padding: 24px;
  overflow-y: auto;
}
.tab-pane.memory-pane {
  padding: 0; /* Layout handles its own padding */
}

.form-group {
  margin-bottom: 24px;
  display: flex;
  flex-direction: column;
}
.form-group.flex-1 {
  flex: 1;
  min-height: 0;
}

.form-label {
  font-size: 14px;
  font-weight: 600;
  color: var(--text-primary);
  margin-bottom: 10px;
}

.group-select-row {
  display: flex;
  gap: 12px;
}

.form-select, .form-input {
  flex: 1;
  padding: 10px 14px;
  border: 1px solid var(--border);
  border-radius: 8px;
  background: var(--bg-tertiary);
  color: var(--text-primary);
  font-size: 15px;
}
.form-select:focus, .form-input:focus {
  outline: none;
  border-color: var(--accent);
}

.memory-markdown {
  font-size: 13px;
  line-height: 1.6;
  color: var(--text-primary);
  overflow-y: auto;
  padding-right: 8px;
}

.team-rules-display {
  max-height: 200px;
  border: 1px solid var(--border-color);
  border-radius: 6px;
  padding: 10px 12px;
  background: var(--bg-secondary);
  margin-bottom: 4px;
}
.memory-markdown :deep(h1),
.memory-markdown :deep(h2),
.memory-markdown :deep(h3),
.memory-markdown :deep(h4),
.memory-markdown :deep(h5),
.memory-markdown :deep(h6) {
  margin-top: 1.2em;
  margin-bottom: 0.6em;
  font-weight: 600;
}
.memory-markdown :deep(p) {
  margin-bottom: 1em;
}
.memory-markdown :deep(ul),
.memory-markdown :deep(ol) {
  padding-left: 1.5em;
  margin-bottom: 1em;
}
.memory-markdown :deep(li) {
  margin-bottom: 0.25em;
}
.memory-markdown :deep(code) {
  font-family: 'SF Mono', 'Menlo', 'Monaco', 'Consolas', monospace;
  background: var(--bg-tertiary);
  padding: 0.2em 0.4em;
  border-radius: 4px;
  font-size: 0.9em;
}
.memory-markdown :deep(pre) {
  background: var(--bg-tertiary);
  padding: 12px;
  border-radius: 8px;
  overflow-x: auto;
  margin-bottom: 1em;
}
.memory-markdown :deep(pre code) {
  background: none;
  padding: 0;
  font-size: 12px;
}

.form-textarea {
  width: 100%;
  padding: 14px;
  border: 1px solid var(--border);
  border-radius: 8px;
  background: var(--bg-tertiary);
  color: var(--text-primary);
  font-size: 15px;
  font-family: inherit;
  resize: none;
  line-height: 1.6;
}
.form-textarea:focus {
  outline: none;
  border-color: var(--accent);
}
.form-textarea.flex-1 {
  flex: 1;
}

.drawer-actions {
  display: flex;
  justify-content: flex-end;
  gap: 12px;
  margin-top: 24px;
}

.btn-primary, .btn-secondary, .btn-danger-outline {
  padding: 10px 20px;
  border-radius: 8px;
  font-size: 14px;
  font-weight: 500;
  cursor: pointer;
  transition: all 0.2s;
}

.btn-primary {
  background: var(--accent);
  color: white;
  border: none;
}
.btn-primary:hover {
  opacity: 0.9;
}
.btn-primary:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.btn-secondary {
  background: var(--bg-hover);
  color: var(--text-primary);
  border: 1px solid var(--border);
}
.btn-secondary:hover {
  background: var(--bg-active);
}

.btn-danger-outline {
  background: transparent;
  color: var(--danger);
  border: 1px solid var(--danger);
}
.btn-danger-outline:hover {
  background: rgba(239, 68, 68, 0.1);
}
.btn-danger-outline:disabled {
  opacity: 0.5;
  cursor: not-allowed;
  border-color: var(--text-muted);
  color: var(--text-muted);
}

.danger-zone {
  margin-top: 32px;
  padding-top: 20px;
  border-top: 1px dashed var(--border);
}
.danger-zone-title {
  font-size: 12px;
  font-weight: 600;
  color: var(--text-muted);
  text-transform: uppercase;
  letter-spacing: 0.05em;
  margin-bottom: 12px;
}
.btn-reset-context {
  display: inline-flex;
  align-items: center;
  gap: 6px;
}
.danger-zone-hint {
  font-size: 12px;
  color: var(--text-muted);
  margin-top: 8px;
}

.drawer-loading, .drawer-empty {
  color: var(--text-muted);
  font-size: 14px;
  text-align: center;
  padding: 24px;
}
.flex-center {
  display: flex;
  align-items: center;
  justify-content: center;
}

/* Memory Layout */
.memory-layout {
  display: flex;
  height: 100%;
  width: 100%;
}

.memory-tree-sidebar {
  width: 220px;
  border-right: 1px solid var(--border);
  display: flex;
  flex-direction: column;
  background: var(--bg-secondary);
}

.memory-sidebar-header {
  padding: 12px 16px;
  border-bottom: 1px solid var(--border);
}

.search-input-wrapper {
  position: relative;
  display: flex;
  align-items: center;
}

.search-icon {
  position: absolute;
  left: 10px;
  color: var(--text-muted);
}

.memory-search-input {
  width: 100%;
  padding: 8px 12px 8px 32px;
  border: 1px solid var(--border);
  border-radius: 6px;
  background: var(--bg-tertiary);
  color: var(--text-primary);
  font-size: 13px;
  transition: all 0.2s;
}
.memory-search-input:focus {
  outline: none;
  border-color: var(--accent);
  background: var(--bg-primary);
}

.memory-tree {
  flex: 1;
  overflow-y: auto;
  padding: 12px 0;
}

.tree-node-row {
  display: flex;
  align-items: center;
  padding: 8px 16px;
  cursor: pointer;
  color: var(--text-secondary);
  font-size: 14px;
  transition: background 0.2s;
  user-select: none;
}
.tree-node-row:hover {
  background: var(--bg-hover);
  color: var(--text-primary);
}
.tree-node-row.active {
  background: var(--bg-active);
  color: var(--accent);
}

.tree-icon {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 18px;
  height: 18px;
  margin-right: 8px;
}
.tree-icon svg {
  transition: transform 0.2s;
}
.tree-icon svg.is-open {
  transform: rotate(90deg);
}

.tree-name {
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  flex: 1;
}

.tree-size {
  font-size: 11px;
  color: var(--text-muted);
  margin-left: 8px;
  flex-shrink: 0;
}

.memory-editor {
  flex: 1;
  display: flex;
  flex-direction: column;
  padding: 24px;
  min-width: 0;
}

.editor-header {
  margin-bottom: 16px;
}
.editor-filename {
  font-size: 16px;
  font-weight: 600;
  color: var(--text-primary);
}

.toast {
  position: fixed;
  bottom: 20px;
  left: 50%;
  transform: translateX(-50%);
  padding: 12px 24px;
  background: var(--bg-secondary);
  color: var(--text-primary);
  border: 1px solid var(--border);
  border-radius: 8px;
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.15);
  font-size: 15px;
  z-index: 3000;
  pointer-events: none;
}
.toast.success { border-left: 4px solid var(--success, #22c55e); }
.toast.error { border-left: 4px solid var(--danger, #ef4444); }

/* Mobile Responsiveness */
@media (max-width: 768px) {
  .drawer-content {
    width: 100vw;
  }
  
  .memory-layout {
    flex-direction: column;
  }
  
  .memory-tree-sidebar {
    width: 100%;
    flex: 0 0 35%; /* Fixed percentage height for mobile tree */
    border-right: none;
    border-bottom: 1px solid var(--border);
  }
  
  .memory-editor {
    flex: 1;
    padding: 16px;
  }

  .drawer-tabs {
    padding: 0 8px;
  }
  
  .drawer-tab {
    padding: 10px 12px;
    font-size: 14px;
  }

  .tab-pane {
    padding: 16px;
  }

  .form-select, .form-input, .form-textarea {
    font-size: 14px;
    padding: 8px 12px;
  }

  .btn-primary, .btn-secondary, .btn-danger-outline {
    padding: 8px 16px;
  }
}
</style>
