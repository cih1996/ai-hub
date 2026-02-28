<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { listTriggers, createTrigger, updateTrigger, deleteTrigger, listSessions } from '../composables/api'
import type { Trigger, Session } from '../types'

const triggers = ref<Trigger[]>([])
const sessions = ref<Session[]>([])
const loading = ref(false)
const showCreate = ref(false)
const deleteTarget = ref<Trigger | null>(null)

const form = ref({ session_id: 0, content: '', trigger_time: '', max_fires: -1 })

const grouped = computed(() => {
  const m = new Map<number, Trigger[]>()
  for (const t of triggers.value) {
    if (!m.has(t.session_id)) m.set(t.session_id, [])
    m.get(t.session_id)!.push(t)
  }
  const result: { sessionId: number; title: string; items: Trigger[] }[] = []
  for (const [sid, items] of m) {
    const s = sessions.value.find(s => s.id === sid)
    result.push({ sessionId: sid, title: s?.title || `会话 #${sid}`, items })
  }
  result.sort((a, b) => a.sessionId - b.sessionId)
  return result
})

function statusLabel(s: string) {
  const map: Record<string, string> = { active: '等待中', fired: '已触发', failed: '失败', completed: '已完成', disabled: '已禁用' }
  return map[s] || s
}

function statusClass(s: string) {
  if (s === 'active' || s === 'fired') return 'status-active'
  if (s === 'failed') return 'status-failed'
  if (s === 'completed') return 'status-completed'
  return 'status-disabled'
}

function fireLabel(t: Trigger) {
  return t.max_fires === -1 ? `${t.fired_count} / ∞` : `${t.fired_count} / ${t.max_fires}`
}

async function load() {
  loading.value = true
  try {
    const [t, s] = await Promise.all([listTriggers(), listSessions()])
    triggers.value = t
    sessions.value = s
  } catch { triggers.value = []; sessions.value = [] }
  loading.value = false
}

async function onCreate() {
  if (!form.value.session_id || !form.value.content || !form.value.trigger_time) return
  await createTrigger(form.value)
  showCreate.value = false
  form.value = { session_id: 0, content: '', trigger_time: '', max_fires: -1 }
  load()
}

async function onToggle(t: Trigger) {
  const newEnabled = !t.enabled
  t.enabled = newEnabled
  try { await updateTrigger(t.id, { ...t, enabled: newEnabled }) } catch { t.enabled = !newEnabled }
  load()
}

async function onDelete() {
  if (!deleteTarget.value) return
  await deleteTrigger(deleteTarget.value.id)
  deleteTarget.value = null
  load()
}

onMounted(load)
</script>

<template>
  <div class="triggers-page">
    <div class="page-header">
      <div>
        <h2 class="page-title">定时触发器</h2>
        <span class="page-desc">管理自动定时任务，到时间自动向指定会话发送指令</span>
      </div>
      <button class="btn-create" @click="showCreate = true">
        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M12 5v14M5 12h14"/></svg>
        新建
      </button>
    </div>

    <div v-if="loading" class="empty-state">加载中...</div>
    <div v-else-if="grouped.length === 0" class="empty-state">暂无触发器</div>

    <div v-for="g in grouped" :key="g.sessionId" class="trigger-group">
      <div class="group-label">
        <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10"/><polyline points="12 6 12 12 16 14"/></svg>
        {{ g.title }}
        <span class="group-id">#{{ g.sessionId }}</span>
      </div>
      <div class="card-list">
        <div v-for="t in g.items" :key="t.id" class="card">
          <div class="card-body">
            <div class="card-top">
              <span class="card-content">{{ t.content }}</span>
              <span class="status-tag" :class="statusClass(t.status)">{{ statusLabel(t.status) }}</span>
            </div>
            <div class="card-meta">
              <span class="meta-item">触发: {{ t.trigger_time }}</span>
              <span class="meta-item">次数: {{ fireLabel(t) }}</span>
              <span v-if="t.next_fire_at" class="meta-item">下次: {{ t.next_fire_at }}</span>
              <span v-if="t.last_fired_at" class="meta-item">上次: {{ t.last_fired_at }}</span>
            </div>
          </div>
          <div class="card-actions">
            <label class="toggle">
              <input type="checkbox" :checked="t.enabled" @change="onToggle(t)" />
              <span class="toggle-slider"></span>
            </label>
            <button class="btn-del" @click="deleteTarget = t" title="删除">
              <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M18 6L6 18M6 6l12 12"/></svg>
            </button>
          </div>
        </div>
      </div>
    </div>
    <!-- Create modal -->
    <Teleport to="body">
      <div v-if="showCreate" class="modal-overlay" @click="showCreate = false">
        <div class="modal-box" @click.stop>
          <p class="modal-title">新建触发器</p>
          <div class="form-group">
            <label>会话</label>
            <select v-model.number="form.session_id">
              <option :value="0" disabled>选择会话...</option>
              <option v-for="s in sessions" :key="s.id" :value="s.id">#{{ s.id }} {{ s.title }}</option>
            </select>
          </div>
          <div class="form-group">
            <label>指令内容</label>
            <textarea v-model="form.content" rows="3" placeholder="发送给会话的自然语言指令"></textarea>
          </div>
          <div class="form-group">
            <label>触发时间</label>
            <input v-model="form.trigger_time" placeholder="10:30:00 / 2026-02-17 10:30:00 / 1h30m" />
            <span class="form-hint">支持: 固定时间(10:30:00)、具体日期(2026-02-17 10:30:00)、间隔(1h30m)</span>
          </div>
          <div class="form-group">
            <label>最大触发次数</label>
            <input v-model.number="form.max_fires" type="number" placeholder="-1 = 无限" />
          </div>
          <div class="modal-actions">
            <button class="modal-btn cancel" @click="showCreate = false">取消</button>
            <button class="modal-btn confirm" @click="onCreate">创建</button>
          </div>
        </div>
      </div>
    </Teleport>

    <!-- Delete confirm -->
    <Teleport to="body">
      <div v-if="deleteTarget" class="modal-overlay" @click="deleteTarget = null">
        <div class="modal-box" @click.stop>
          <p class="modal-title">确认删除</p>
          <p class="modal-desc">删除触发器「{{ deleteTarget.content }}」？</p>
          <div class="modal-actions">
            <button class="modal-btn cancel" @click="deleteTarget = null">取消</button>
            <button class="modal-btn confirm" @click="onDelete">删除</button>
          </div>
        </div>
      </div>
    </Teleport>
  </div>
</template>

<style scoped>
.triggers-page { padding: 24px; overflow-y: auto; height: 100%; }
.page-header { display: flex; align-items: flex-start; justify-content: space-between; margin-bottom: 20px; }
.page-title { font-size: 18px; font-weight: 600; color: var(--text-primary); margin: 0; }
.page-desc { font-size: 12px; color: var(--text-muted); margin-top: 4px; display: block; }
.btn-create {
  display: flex; align-items: center; gap: 4px; padding: 6px 14px;
  border-radius: var(--radius); font-size: 13px; font-weight: 500;
  background: var(--accent); color: var(--btn-text); transition: opacity var(--transition); flex-shrink: 0;
}
.btn-create:hover { opacity: 0.9; }
.empty-state { text-align: center; color: var(--text-muted); padding: 48px 16px; font-size: 14px; }
.trigger-group { margin-bottom: 24px; }
.group-label {
  display: flex; align-items: center; gap: 6px;
  font-size: 12px; font-weight: 600; color: var(--text-muted); margin-bottom: 8px;
}
.group-id { font-weight: 400; }
.card-list { display: flex; flex-direction: column; gap: 6px; }
.card {
  display: flex; align-items: center; justify-content: space-between;
  padding: 12px 16px; background: var(--bg-secondary); border: 1px solid var(--border);
  border-radius: var(--radius); transition: background var(--transition);
}
.card:hover { background: var(--bg-hover); }
.card-body { flex: 1; min-width: 0; }
.card-top { display: flex; align-items: center; gap: 8px; }
.card-content { font-size: 14px; font-weight: 500; color: var(--text-primary); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.status-tag { font-size: 11px; padding: 2px 8px; border-radius: 9999px; flex-shrink: 0; }
.status-active { background: var(--accent-soft); color: var(--accent); }
.status-failed { background: rgba(239,68,68,0.15); color: var(--danger); }
.status-completed { background: rgba(34,197,94,0.15); color: var(--success); }
.status-disabled { background: var(--bg-tertiary); color: var(--text-muted); }
.card-meta { display: flex; flex-wrap: wrap; gap: 12px; margin-top: 6px; }
.meta-item { font-size: 11px; color: var(--text-muted); }
.card-actions { display: flex; align-items: center; gap: 8px; flex-shrink: 0; margin-left: 12px; }
.btn-del {
  width: 24px; height: 24px; display: flex; align-items: center; justify-content: center;
  border-radius: var(--radius-sm); color: var(--text-muted); transition: all var(--transition);
}
.btn-del:hover { color: var(--danger); background: rgba(239,68,68,0.1); }
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
  position: fixed; inset: 0; background: var(--overlay);
  display: flex; align-items: center; justify-content: center; z-index: 1000;
}
.modal-box {
  background: var(--bg-secondary); border: 1px solid var(--border);
  border-radius: 12px; padding: 24px; width: 420px; max-width: 90vw;
}
.modal-title { font-size: 15px; font-weight: 600; color: var(--text-primary); margin-bottom: 16px; }
.modal-desc { font-size: 13px; color: var(--text-secondary); margin-bottom: 20px; line-height: 1.5; }
.form-group { margin-bottom: 14px; }
.form-group label { display: block; font-size: 12px; font-weight: 600; color: var(--text-secondary); margin-bottom: 4px; }
.form-group input, .form-group select, .form-group textarea {
  width: 100%; padding: 8px 10px; font-size: 13px; border-radius: var(--radius);
  border: 1px solid var(--border); background: var(--bg-primary); color: var(--text-primary);
}
.form-group textarea { resize: vertical; font-family: inherit; }
.form-hint { font-size: 11px; color: var(--text-muted); margin-top: 2px; display: block; }
.modal-actions { display: flex; justify-content: flex-end; gap: 8px; margin-top: 16px; }
.modal-btn { padding: 6px 16px; border-radius: var(--radius); font-size: 13px; font-weight: 500; cursor: pointer; transition: all var(--transition); }
.modal-btn.cancel { color: var(--text-secondary); background: var(--bg-hover); }
.modal-btn.cancel:hover { color: var(--text-primary); }
.modal-btn.confirm { color: var(--btn-text); background: var(--accent); }
.modal-btn.confirm:hover { opacity: 0.9; }
@media (max-width: 768px) {
  .triggers-page { padding: 12px; }
  .trigger-card { flex-direction: column; align-items: flex-start; gap: 8px; }
  .trigger-actions { width: 100%; justify-content: flex-end; }
  .form-overlay .form-modal { width: 100vw; max-width: 100vw; max-height: 100vh; max-height: 100dvh; border-radius: 0; }
}
</style>
