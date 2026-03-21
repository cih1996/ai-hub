<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { listHooks, createHook, updateHook, deleteHook, enableHook, disableHook, listSessions } from '../composables/api'
import type { Hook } from '../composables/api'
import type { Session } from '../types'

const hooks = ref<Hook[]>([])
const sessions = ref<Session[]>([])
const loading = ref(false)
const showForm = ref(false)
const editingHook = ref<Hook | null>(null)
const deleteTarget = ref<Hook | null>(null)

const eventOptions = [
  'message.received',
  'message.sent',
  'session.created',
  'context.compressed',
  'context.reset',
  'error.detected',
]

const form = ref({
  event: 'message.received',
  condition: '',
  target_session: 0,
  payload: '',
  enabled: true,
})

function resetForm() {
  form.value = { event: 'message.received', condition: '', target_session: 0, payload: '', enabled: true }
  editingHook.value = null
}

function sessionTitle(id: number): string {
  const s = sessions.value.find(s => s.id === id)
  return s ? `#${id} ${s.title}` : `#${id}`
}

function eventLabel(e: string): string {
  const map: Record<string, string> = {
    'message.received': '收到消息',
    'message.sent': '发送消息',
    'session.created': '会话创建',
    'context.compressed': '上下文压缩',
    'context.reset': '上下文重置',
    'error.detected': '检测到错误',
  }
  return map[e] || e
}

async function load() {
  loading.value = true
  try {
    const [h, s] = await Promise.all([listHooks(), listSessions()])
    hooks.value = h
    sessions.value = s
  } catch {
    hooks.value = []
    sessions.value = []
  }
  loading.value = false
}

function openCreate() {
  resetForm()
  showForm.value = true
}

function openEdit(h: Hook) {
  editingHook.value = h
  form.value = {
    event: h.event,
    condition: h.condition,
    target_session: h.target_session,
    payload: h.payload,
    enabled: h.enabled,
  }
  showForm.value = true
}

async function onSubmit() {
  if (!form.value.event || !form.value.target_session) return
  if (editingHook.value) {
    await updateHook(editingHook.value.id, form.value)
  } else {
    await createHook(form.value)
  }
  showForm.value = false
  resetForm()
  load()
}

async function onToggle(h: Hook) {
  try {
    if (h.enabled) {
      await disableHook(h.id)
    } else {
      await enableHook(h.id)
    }
    h.enabled = !h.enabled
  } catch { /* revert visually on next load */ }
  load()
}

async function onDelete() {
  if (!deleteTarget.value) return
  await deleteHook(deleteTarget.value.id)
  deleteTarget.value = null
  load()
}

onMounted(load)
</script>

<template>
  <div class="hooks-page">
    <div class="page-header">
      <div>
        <h2 class="page-title">事件钩子</h2>
        <span class="page-desc">当系统事件发生时，自动向指定会话发送消息</span>
      </div>
      <button class="btn-create" @click="openCreate">
        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M12 5v14M5 12h14"/></svg>
        新建
      </button>
    </div>

    <div v-if="loading" class="empty-state">加载中...</div>
    <div v-else-if="hooks.length === 0" class="empty-state">暂无事件钩子</div>

    <div v-else class="card-list">
      <div v-for="h in hooks" :key="h.id" class="card" :class="{ disabled: !h.enabled }">
        <div class="card-body">
          <div class="card-top">
            <span class="event-tag">{{ eventLabel(h.event) }}</span>
            <span v-if="h.condition" class="condition-tag" :title="h.condition">{{ h.condition }}</span>
            <span class="fire-count" title="触发次数">{{ h.fired_count }}次</span>
          </div>
          <div class="card-meta">
            <span class="meta-item">目标: {{ sessionTitle(h.target_session) }}</span>
            <span v-if="h.payload" class="meta-item payload-preview" :title="h.payload">模板: {{ h.payload.length > 40 ? h.payload.slice(0, 40) + '...' : h.payload }}</span>
          </div>
        </div>
        <div class="card-actions">
          <button class="btn-edit" @click="openEdit(h)" title="编辑">
            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M11 4H4a2 2 0 00-2 2v14a2 2 0 002 2h14a2 2 0 002-2v-7"/><path d="M18.5 2.5a2.121 2.121 0 013 3L12 15l-4 1 1-4 9.5-9.5z"/></svg>
          </button>
          <label class="toggle">
            <input type="checkbox" :checked="h.enabled" @change="onToggle(h)" />
            <span class="toggle-slider"></span>
          </label>
          <button class="btn-del" @click="deleteTarget = h" title="删除">
            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M18 6L6 18M6 6l12 12"/></svg>
          </button>
        </div>
      </div>
    </div>

    <!-- Create/Edit modal -->
    <Teleport to="body">
      <div v-if="showForm" class="modal-overlay" @click="showForm = false">
        <div class="modal-box" @click.stop>
          <p class="modal-title">{{ editingHook ? '编辑钩子' : '新建钩子' }}</p>
          <div class="form-group">
            <label>事件类型</label>
            <select v-model="form.event">
              <option v-for="e in eventOptions" :key="e" :value="e">{{ eventLabel(e) }} ({{ e }})</option>
            </select>
          </div>
          <div class="form-group">
            <label>触发条件</label>
            <input v-model="form.condition" placeholder="如 content_match:关键词（留空=无条件触发）" />
            <span class="form-hint">支持: content_match:关键词, count_gt:100, session_id:5</span>
          </div>
          <div class="form-group">
            <label>目标会话</label>
            <select v-model.number="form.target_session">
              <option :value="0" disabled>选择目标会话...</option>
              <option v-for="s in sessions" :key="s.id" :value="s.id">#{{ s.id }} {{ s.title }}</option>
            </select>
          </div>
          <div class="form-group">
            <label>消息模板</label>
            <textarea v-model="form.payload" rows="3" placeholder="支持占位符: {content}, {source_session_id}, {event}"></textarea>
            <span class="form-hint">留空则发送默认通知消息</span>
          </div>
          <div class="form-group">
            <label class="toggle-label">
              <input type="checkbox" v-model="form.enabled" />
              <span>启用</span>
            </label>
          </div>
          <div class="modal-actions">
            <button class="modal-btn cancel" @click="showForm = false">取消</button>
            <button class="modal-btn confirm" @click="onSubmit">{{ editingHook ? '保存' : '创建' }}</button>
          </div>
        </div>
      </div>
    </Teleport>

    <!-- Delete confirm -->
    <Teleport to="body">
      <div v-if="deleteTarget" class="modal-overlay" @click="deleteTarget = null">
        <div class="modal-box" @click.stop>
          <p class="modal-title">确认删除</p>
          <p class="modal-desc">删除钩子「{{ eventLabel(deleteTarget.event) }}」→ {{ sessionTitle(deleteTarget.target_session) }}？</p>
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
.hooks-page { padding: 24px; overflow-y: auto; height: 100%; }
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
.card-list { display: flex; flex-direction: column; gap: 6px; }
.card {
  display: flex; align-items: center; justify-content: space-between;
  padding: 12px 16px; background: var(--bg-secondary); border: 1px solid var(--border);
  border-radius: var(--radius); transition: background var(--transition);
}
.card:hover { background: var(--bg-hover); }
.card.disabled { opacity: 0.55; }
.card-body { flex: 1; min-width: 0; }
.card-top { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; }
.event-tag {
  font-size: 12px; font-weight: 600; padding: 2px 8px; border-radius: 9999px;
  background: var(--accent-soft); color: var(--accent);
}
.condition-tag {
  font-size: 11px; padding: 2px 8px; border-radius: 9999px;
  background: var(--bg-tertiary); color: var(--text-secondary);
  max-width: 200px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis;
}
.fire-count { font-size: 11px; color: var(--text-muted); }
.card-meta { display: flex; flex-wrap: wrap; gap: 12px; margin-top: 6px; }
.meta-item { font-size: 11px; color: var(--text-muted); }
.payload-preview { max-width: 300px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.card-actions { display: flex; align-items: center; gap: 8px; flex-shrink: 0; margin-left: 12px; }
.btn-edit, .btn-del {
  width: 24px; height: 24px; display: flex; align-items: center; justify-content: center;
  border-radius: var(--radius-sm); color: var(--text-muted); transition: all var(--transition);
}
.btn-edit:hover { color: var(--accent); background: var(--accent-soft); }
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
  border-radius: 12px; padding: 24px; width: 460px; max-width: 90vw;
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
.toggle-label { display: flex; align-items: center; gap: 8px; cursor: pointer; font-size: 13px; color: var(--text-primary); }
.toggle-label input { width: 14px; height: 14px; }
.modal-actions { display: flex; justify-content: flex-end; gap: 8px; margin-top: 16px; }
.modal-btn { padding: 6px 16px; border-radius: var(--radius); font-size: 13px; font-weight: 500; cursor: pointer; transition: all var(--transition); }
.modal-btn.cancel { color: var(--text-secondary); background: var(--bg-hover); }
.modal-btn.cancel:hover { color: var(--text-primary); }
.modal-btn.confirm { color: var(--btn-text); background: var(--accent); }
.modal-btn.confirm:hover { opacity: 0.9; }
@media (max-width: 768px) {
  .hooks-page { padding: 12px; }
  .card { flex-direction: column; align-items: flex-start; gap: 8px; }
  .card-actions { width: 100%; justify-content: flex-end; }
}
</style>
