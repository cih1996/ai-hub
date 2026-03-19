<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { listSkills, toggleSkill, getSkillContent } from '../composables/api'
import type { SkillItem } from '../composables/api'

const skills = ref<SkillItem[]>([])
const loading = ref(false)

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

onMounted(load)
</script>

<template>
  <div class="skills-page">
    <div class="page-header">
      <h2 class="page-title">技能</h2>
      <span class="page-desc">管理 Claude Code 的技能和命令</span>
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
            <div class="card-meta">
              <span class="tag" :class="'tag-' + s.source">{{ sourceLabels[s.source] || s.source }}</span>
            </div>
          </div>
          <label class="toggle" @click.stop>
            <input type="checkbox" :checked="s.enabled" @change="onToggle($event, s)" />
            <span class="toggle-slider"></span>
          </label>
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
  </div>
</template>

<style scoped>
.skills-page { padding: 24px; overflow-y: auto; height: 100%; }
.page-header { margin-bottom: 20px; }
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
.card-meta { margin-top: 6px; display: flex; gap: 6px; }
.tag { font-size: 11px; padding: 2px 8px; border-radius: 9999px; }
.tag-user { background: var(--accent-soft); color: var(--accent); }
.tag-plugin { background: rgba(168,85,247,0.15); color: #a855f7; }
.tag-command { background: rgba(34,197,94,0.15); color: var(--success); }
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

@media (max-width: 768px) {
  .skills-page { padding: 12px; }
  .modal-container { width: 95%; max-height: 90vh; }
}
</style>
