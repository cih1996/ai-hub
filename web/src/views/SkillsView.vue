<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { listSkills, toggleSkill } from '../composables/api'
import type { SkillItem } from '../composables/api'

const skills = ref<SkillItem[]>([])
const loading = ref(false)

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

async function onToggle(s: SkillItem) {
  const newState = !s.enabled
  s.enabled = newState
  try {
    await toggleSkill(s.name, s.source, newState)
  } catch {
    s.enabled = !newState
  }
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
        <div v-for="s in g.items" :key="s.name + s.source" class="card">
          <div class="card-body">
            <div class="card-name">{{ s.name }}</div>
            <div class="card-desc">{{ s.description || '—' }}</div>
            <div class="card-meta">
              <span class="tag" :class="'tag-' + s.source">{{ sourceLabels[s.source] || s.source }}</span>
            </div>
          </div>
          <label class="toggle">
            <input type="checkbox" :checked="s.enabled" @change="onToggle(s)" />
            <span class="toggle-slider"></span>
          </label>
        </div>
      </div>
    </div>
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
.card-body { flex: 1; min-width: 0; }
.card-name { font-size: 14px; font-weight: 500; color: var(--text-primary); }
.card-desc { font-size: 12px; color: var(--text-secondary); margin-top: 2px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.card-meta { margin-top: 6px; display: flex; gap: 6px; }
.tag { font-size: 11px; padding: 2px 8px; border-radius: 9999px; }
.tag-user { background: var(--accent-soft); color: var(--accent); }
.tag-plugin { background: rgba(168,85,247,0.15); color: #a855f7; }
.tag-command { background: rgba(34,197,94,0.15); color: #22c55e; }
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
.toggle input:checked + .toggle-slider::before { transform: translateX(16px); background: white; }
</style>

