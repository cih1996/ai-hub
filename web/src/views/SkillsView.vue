<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { listSkills, toggleSkill, getSkillContent, previewSkillImport, importSkills, skillExportUrl } from '../composables/api'
import type { SkillItem, SkillImportPreview } from '../composables/api'

const skills = ref<SkillItem[]>([])
const loading = ref(false)
const fileInput = ref<HTMLInputElement | null>(null)
const importFile = ref<File | null>(null)
const importPreview = ref<SkillImportPreview | null>(null)
const showImportModal = ref(false)
const importLoading = ref(false)
const importing = ref(false)
const importError = ref('')
const selectedImportIds = ref<string[]>([])
const overwriteExisting = ref(false)

// Modal state
const showModal = ref(false)
const modalTitle = ref('')
const modalContent = ref('')
const modalLoading = ref(false)

const sourceLabels: Record<string, string> = {
  user: '用户技能',
  plugin: '插件技能',
  command: '命令',
}

const groups = computed(() => {
  const m: Record<string, SkillItem[]> = {}
  for (const s of skills.value) {
    ;(m[s.source] ??= []).push(s)
  }
  const order = ['user', 'plugin', 'command']
  return order
    .filter(k => m[k]?.length)
    .map(k => ({ key: k, label: sourceLabels[k] || k, items: m[k] }))
})

async function load() {
  loading.value = true
  try { skills.value = await listSkills() } catch { skills.value = [] }
  loading.value = false
}

async function onToggle(e: Event, s: SkillItem) {
  e.stopPropagation()
  const newState = !s.enabled
  s.enabled = newState
  try {
    await toggleSkill(s.name, s.source, newState)
  } catch {
    s.enabled = !newState
  }
}

async function openSkill(s: SkillItem) {
  // Only user skills have readable content via API
  if (s.source !== 'user') return
  showModal.value = true
  modalTitle.value = s.name
  modalContent.value = ''
  modalLoading.value = true
  try {
    const resp = await getSkillContent(s.name)
    modalContent.value = resp.content
  } catch {
    modalContent.value = '无法加载技能内容'
  }
  modalLoading.value = false
}

function closeModal() {
  showModal.value = false
  modalContent.value = ''
}

function triggerImport() {
  fileInput.value?.click()
}

async function onImportFileChange(e: Event) {
  const file = (e.target as HTMLInputElement).files?.[0]
  if (!file) return
  importFile.value = file
  importPreview.value = null
  importError.value = ''
  selectedImportIds.value = []
  overwriteExisting.value = false
  showImportModal.value = true
  importLoading.value = true
  try {
    const preview = await previewSkillImport(file)
    importPreview.value = preview
    selectedImportIds.value = preview.candidates.map(c => c.id)
  } catch (err: any) {
    importError.value = err.message || String(err)
  } finally {
    importLoading.value = false
    if (fileInput.value) fileInput.value.value = ''
  }
}

function closeImportModal() {
  showImportModal.value = false
  importFile.value = null
  importPreview.value = null
  importError.value = ''
  selectedImportIds.value = []
}

function toggleImportCandidate(id: string) {
  if (selectedImportIds.value.includes(id)) {
    selectedImportIds.value = selectedImportIds.value.filter(x => x !== id)
  } else {
    selectedImportIds.value.push(id)
  }
}

async function confirmImport() {
  if (!importFile.value || selectedImportIds.value.length === 0) return
  importing.value = true
  importError.value = ''
  try {
    const res = await importSkills(importFile.value, selectedImportIds.value, overwriteExisting.value)
    if (res.warnings?.length) alert('导入完成，但有提示：\n' + res.warnings.join('\n'))
    closeImportModal()
    await load()
  } catch (err: any) {
    importError.value = err.message || String(err)
  } finally {
    importing.value = false
  }
}

function exportSkill(s: SkillItem) {
  if (s.source !== 'user') return
  window.open(skillExportUrl(s.name), '_blank')
}

function importModeLabel(mode?: string) {
  if (mode === 'single-root-file') return '单技能包：根目录直接包含 SKILL.md，将使用压缩包名称作为技能目录'
  if (mode === 'single-folder') return '单技能包：压缩包内包含一个技能目录'
  if (mode === 'multi-skill') return '多技能包：检测到多个技能目录，请勾选后确认导入'
  return '未知结构'
}

onMounted(load)
</script>

<template>
  <div class="skills-page">
    <div class="page-header">
      <div>
        <h2 class="page-title">技能</h2>
        <span class="page-desc">管理 Claude Code 的技能和命令</span>
      </div>
      <div class="header-actions">
        <button class="btn-secondary" @click="triggerImport">导入技能包</button>
        <input ref="fileInput" type="file" accept=".zip,application/zip" class="hidden-input" @change="onImportFileChange" />
      </div>
    </div>
    <div v-if="loading" class="empty-state">加载中...</div>
    <div v-else-if="groups.length === 0" class="empty-state">暂无技能</div>
    <div v-for="g in groups" :key="g.key" class="skill-group">
      <div class="group-label">{{ g.label }}</div>
      <div class="card-list">
        <div
          v-for="s in g.items"
          :key="s.name + s.source"
          class="card"
          :class="{ clickable: s.source === 'user' }"
          @click="openSkill(s)"
        >
          <div class="card-body">
            <div class="card-name">{{ s.name }}</div>
            <div class="card-desc">{{ s.description || '—' }}</div>
            <div v-if="s.when_to_use" class="card-when">触发：{{ s.when_to_use }}</div>
            <div class="card-meta">
              <span class="tag" :class="'tag-' + s.source">{{ sourceLabels[s.source] || s.source }}</span>
            </div>
          </div>
          <div class="card-actions">
            <button v-if="s.source === 'user'" class="btn-export" title="导出技能包" @click.stop="exportSkill(s)">导出</button>
            <label class="toggle" @click.stop>
              <input type="checkbox" :checked="s.enabled" @change="onToggle($event, s)" />
              <span class="toggle-slider"></span>
            </label>
          </div>
        </div>
      </div>
    </div>

    <!-- Skill Content Modal -->
    <Teleport to="body">
      <div v-if="showModal" class="modal-overlay" @click.self="closeModal">
        <div class="modal-container">
          <div class="modal-header">
            <span class="modal-title">{{ modalTitle }}</span>
            <button class="modal-close" @click="closeModal">&times;</button>
          </div>
          <div class="modal-body">
            <div v-if="modalLoading" class="modal-loading">加载中...</div>
            <pre v-else class="modal-content">{{ modalContent }}</pre>
          </div>
        </div>
      </div>
    </Teleport>

    <!-- Skill Import Modal -->
    <Teleport to="body">
      <div v-if="showImportModal" class="modal-overlay" @click.self="closeImportModal">
        <div class="modal-container import-modal">
          <div class="modal-header">
            <span class="modal-title">导入技能包</span>
            <button class="modal-close" @click="closeImportModal">&times;</button>
          </div>
          <div class="modal-body">
            <div v-if="importLoading" class="modal-loading">正在分析压缩包...</div>
            <div v-else-if="importError" class="import-error">{{ importError }}</div>
            <template v-else-if="importPreview">
              <div class="import-summary">
                <div><strong>{{ importPreview.archive_name }}</strong></div>
                <div>{{ importModeLabel(importPreview.mode) }}</div>
              </div>
              <div v-if="importPreview.warnings?.length" class="import-warnings">
                <div v-for="w in importPreview.warnings" :key="w">{{ w }}</div>
              </div>
              <div class="import-list">
                <label v-for="c in importPreview.candidates" :key="c.id" class="import-item" :class="{ exists: c.exists }">
                  <input type="checkbox" :checked="selectedImportIds.includes(c.id)" @change="toggleImportCandidate(c.id)" />
                  <div class="import-item-body">
                    <div class="import-item-title">
                      <span>{{ c.name }}</span>
                      <span class="import-dir">目录：{{ c.dir_name }}</span>
                      <span v-if="c.exists" class="exists-badge">已存在</span>
                    </div>
                    <div class="import-desc">{{ c.description || '无描述' }}</div>
                    <div v-if="c.when_to_use" class="import-when">触发：{{ c.when_to_use }}</div>
                    <div class="import-files">{{ c.file_count }} 个文件：{{ c.files.join('、') }}</div>
                  </div>
                </label>
              </div>
              <label class="overwrite-row">
                <input type="checkbox" v-model="overwriteExisting" />
                覆盖已存在的同名技能
              </label>
            </template>
          </div>
          <div class="modal-footer">
            <button class="btn-secondary" @click="closeImportModal">取消</button>
            <button class="btn-primary" :disabled="importLoading || importing || !!importError || selectedImportIds.length === 0" @click="confirmImport">
              {{ importing ? '导入中...' : `确认导入 ${selectedImportIds.length} 个` }}
            </button>
          </div>
        </div>
      </div>
    </Teleport>
  </div>
</template>

<style scoped>
.skills-page { padding: 24px; overflow-y: auto; height: 100%; }
.page-header { margin-bottom: 20px; display: flex; align-items: center; justify-content: space-between; gap: 12px; }
.header-actions { display: flex; gap: 8px; }
.hidden-input { display: none; }
.btn-secondary, .btn-primary, .btn-export { border: 1px solid var(--border); border-radius: 8px; padding: 7px 12px; font-size: 12px; cursor: pointer; background: var(--bg-secondary); color: var(--text-primary); }
.btn-secondary:hover, .btn-export:hover { background: var(--bg-hover); }
.btn-primary { background: var(--accent); color: var(--btn-text); border-color: var(--accent); }
.btn-primary:disabled { opacity: 0.55; cursor: not-allowed; }
.page-title { font-size: 18px; font-weight: 600; color: var(--text-primary); margin: 0; }
.page-desc { font-size: 12px; color: var(--text-muted); margin-top: 4px; display: block; }
.empty-state { text-align: center; color: var(--text-muted); padding: 48px 16px; font-size: 14px; }
.skill-group { margin-bottom: 24px; }
.group-label { font-size: 12px; font-weight: 600; color: var(--text-muted); text-transform: uppercase; margin-bottom: 8px; }
.card-list { display: flex; flex-direction: column; gap: 6px; }
.card {
  display: flex; align-items: center; justify-content: space-between;
  padding: 12px 16px; background: var(--bg-secondary); border: 1px solid var(--border);
  border-radius: var(--radius); transition: background var(--transition);
}
.card:hover { background: var(--bg-hover); }
.card.clickable { cursor: pointer; }
.card-body { flex: 1; min-width: 0; }
.card-name { font-size: 14px; font-weight: 500; color: var(--text-primary); }
.card-desc { font-size: 12px; color: var(--text-secondary); margin-top: 2px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.card-when { font-size: 11px; color: var(--accent); margin-top: 3px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; opacity: 0.8; }
.card-meta { margin-top: 6px; display: flex; gap: 6px; }
.tag { font-size: 11px; padding: 2px 8px; border-radius: 9999px; }
.tag-user { background: var(--accent-soft); color: var(--accent); }
.tag-plugin { background: rgba(168,85,247,0.15); color: #a855f7; }
.tag-command { background: rgba(34,197,94,0.15); color: var(--success); }
.card-actions { display: flex; align-items: center; gap: 10px; flex-shrink: 0; }
.btn-export { padding: 5px 9px; color: var(--accent); }
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

/* Modal */
.modal-overlay {
  position: fixed; inset: 0; background: rgba(0,0,0,0.5); z-index: 9999;
  display: flex; align-items: center; justify-content: center;
  backdrop-filter: blur(2px);
}
.modal-container {
  background: var(--bg-primary); border: 1px solid var(--border);
  border-radius: 12px; width: 90%; max-width: 720px; max-height: 80vh;
  display: flex; flex-direction: column; box-shadow: 0 8px 32px rgba(0,0,0,0.3);
}
.modal-header {
  display: flex; align-items: center; justify-content: space-between;
  padding: 16px 20px; border-bottom: 1px solid var(--border); flex-shrink: 0;
}
.modal-title { font-size: 16px; font-weight: 600; color: var(--text-primary); }
.modal-close {
  background: none; border: none; font-size: 22px; color: var(--text-muted);
  cursor: pointer; padding: 0 4px; line-height: 1;
}
.modal-close:hover { color: var(--text-primary); }
.modal-body { padding: 20px; overflow-y: auto; flex: 1; }
.modal-loading { text-align: center; color: var(--text-muted); padding: 24px; }
.modal-content {
  font-size: 13px; line-height: 1.6; color: var(--text-secondary);
  white-space: pre-wrap; word-break: break-word; margin: 0;
  font-family: 'SF Mono', 'Fira Code', 'Cascadia Code', monospace;
}
.import-modal { max-width: 820px; }
.import-summary { padding: 12px 14px; border: 1px solid var(--border); border-radius: 10px; background: var(--bg-secondary); color: var(--text-secondary); font-size: 13px; margin-bottom: 12px; }
.import-warnings, .import-error { padding: 10px 12px; border-radius: 8px; background: rgba(245, 158, 11, 0.12); color: #f59e0b; font-size: 12px; margin-bottom: 12px; }
.import-error { background: rgba(239, 68, 68, 0.12); color: #ef4444; }
.import-list { display: flex; flex-direction: column; gap: 8px; }
.import-item { display: flex; gap: 10px; padding: 12px; border: 1px solid var(--border); border-radius: 10px; background: var(--bg-secondary); cursor: pointer; }
.import-item.exists { border-color: rgba(245, 158, 11, 0.45); }
.import-item input { margin-top: 3px; }
.import-item-body { min-width: 0; flex: 1; }
.import-item-title { display: flex; align-items: center; flex-wrap: wrap; gap: 8px; font-size: 14px; font-weight: 600; color: var(--text-primary); }
.import-dir, .exists-badge { font-size: 11px; font-weight: 400; color: var(--text-muted); background: var(--bg-tertiary); padding: 2px 6px; border-radius: 999px; }
.exists-badge { color: #f59e0b; background: rgba(245, 158, 11, 0.12); }
.import-desc, .import-when, .import-files { margin-top: 4px; font-size: 12px; color: var(--text-secondary); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.import-when { color: var(--accent); }
.import-files { color: var(--text-muted); }
.overwrite-row { display: flex; align-items: center; gap: 8px; margin-top: 14px; font-size: 13px; color: var(--text-secondary); }
.modal-footer { padding: 14px 20px; border-top: 1px solid var(--border); display: flex; justify-content: flex-end; gap: 10px; }

@media (max-width: 768px) {
  .skills-page { padding: 12px; }
  .page-header { align-items: flex-start; flex-direction: column; }
  .modal-container { width: 95%; max-height: 90vh; }
  .card { align-items: flex-start; gap: 10px; }
  .card-actions { flex-direction: column; align-items: flex-end; }
  .import-files { white-space: normal; }
}
</style>
