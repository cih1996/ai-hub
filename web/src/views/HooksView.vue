<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
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
  { value: 'message.received', label: '收到消息' },
  { value: 'message.sent', label: 'AI 回复完成' },
  { value: 'message.count', label: '消息计数' },
  { value: 'session.created', label: '会话创建' },
  { value: 'session.error', label: '会话错误' },
  { value: 'session.compressed', label: '上下文压缩' },
]

// 每种事件类型支持的条件和模板变量
const eventMeta: Record<string, { conditions: string[]; desc: string }> = {
  'message.received':   { conditions: ['content_match'], desc: '任意会话收到用户消息时触发' },
  'message.sent':       { conditions: ['content_match'], desc: 'AI 完成回复后触发，可用于监控输出或触发后续流程' },
  'message.count':      { conditions: ['count_gt'],      desc: '与「收到消息」同时触发，配合消息数阈值使用' },
  'session.created':    { conditions: [],                desc: '新会话首次发消息时触发' },
  'session.error':      { conditions: ['content_match'], desc: 'AI 流式输出中检测到错误时触发' },
  'session.compressed': { conditions: [],                desc: '上下文自动压缩完成后触发，可用于通知或串联工作流' },
}

const conditionPresets: Record<string, { label: string; value: string; desc: string }[]> = {
  content_match: [
    { label: '包含关键词', value: 'content_match:关键词', desc: '内容包含任一关键词即触发（支持 | 分隔多个）' },
    { label: '正则匹配', value: 'content_match:^TODO', desc: '支持正则表达式，如 ^TODO 表示以 TODO 开头' },
  ],
  count_gt: [
    { label: '超过 50 条', value: 'count_gt:50', desc: '消息总数超过 50 条时触发' },
    { label: '超过 100 条', value: 'count_gt:100', desc: '消息总数超过 100 条时触发' },
    { label: '超过 200 条', value: 'count_gt:200', desc: '消息总数超过 200 条时触发' },
  ],
}

const templateVars = [
  { key: '{source_session_id}', desc: '触发源会话 ID' },
  { key: '{event_type}', desc: '事件类型（如 message.received）' },
  { key: '{content}', desc: '消息内容或错误摘要（截断 500 字）' },
  { key: '{message_count}', desc: '当前消息计数（仅 message.count 事件有值）' },
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
  const opt = eventOptions.find(o => o.value === e)
  return opt ? opt.label : e
}

const currentEventConditions = computed(() => {
  const meta = eventMeta[form.value.event]
  return meta ? meta.conditions : []
})

const currentEventDesc = computed(() => {
  const meta = eventMeta[form.value.event]
  return meta ? meta.desc : ''
})

function applyConditionPreset(val: string) {
  form.value.condition = val
}

function insertTemplateVar(v: string) {
  form.value.payload += v
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
              <option v-for="e in eventOptions" :key="e.value" :value="e.value">{{ e.label }}</option>
            </select>
            <span class="form-hint" v-if="currentEventDesc">{{ currentEventDesc }}</span>
          </div>

          <div class="form-group" v-if="currentEventConditions.length > 0">
            <label>触发条件</label>
            <input v-model="form.condition" placeholder="留空 = 无条件触发" />
            <div class="condition-presets" v-if="currentEventConditions.length">
              <span v-for="condType in currentEventConditions" :key="condType" class="preset-group">
                <button v-for="p in conditionPresets[condType]" :key="p.value"
                  class="preset-btn" :title="p.desc" @click="applyConditionPreset(p.value)">
                  {{ p.label }}
                </button>
              </span>
            </div>
            <span class="form-hint" v-if="!form.condition">留空表示该事件发生时无条件触发</span>
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
            <textarea v-model="form.payload" rows="3" placeholder="钩子触发时发送给目标会话的消息内容"></textarea>
            <div class="template-vars">
              <span class="tv-label">插入变量：</span>
              <button v-for="v in templateVars" :key="v.key" class="tv-btn" :title="v.desc"
                @click="insertTemplateVar(v.key)">{{ v.key }}</button>
            </div>
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
.condition-presets { display: flex; flex-wrap: wrap; gap: 4px; margin-top: 6px; }
.preset-group { display: flex; flex-wrap: wrap; gap: 4px; }
.preset-btn {
  padding: 2px 8px; font-size: 11px; border-radius: 9999px; cursor: pointer;
  background: var(--bg-tertiary); color: var(--text-secondary); border: 1px solid var(--border);
  transition: all var(--transition);
}
.preset-btn:hover { background: var(--accent-soft); color: var(--accent); border-color: var(--accent); }
.template-vars {
  display: flex; align-items: center; flex-wrap: wrap; gap: 4px; margin-top: 6px;
}
.tv-label { font-size: 11px; color: var(--text-muted); flex-shrink: 0; }
.tv-btn {
  padding: 2px 6px; font-size: 10px; font-family: 'SF Mono', 'Menlo', monospace;
  border-radius: 4px; cursor: pointer;
  background: var(--bg-tertiary); color: var(--accent); border: 1px solid var(--border);
  transition: all var(--transition);
}
.tv-btn:hover { background: var(--accent-soft); border-color: var(--accent); }
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
