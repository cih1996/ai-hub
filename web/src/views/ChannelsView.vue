<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { listChannels, createChannel, updateChannel, deleteChannel, listSessions } from '../composables/api'
import type { Channel, Session } from '../types'

const channels = ref<Channel[]>([])
const sessions = ref<Session[]>([])
const loading = ref(false)
const showCreate = ref(false)
const editTarget = ref<Channel | null>(null)
const deleteTarget = ref<Channel | null>(null)

const form = ref({ name: '', platform: 'feishu', session_id: 0, config: '{}' })

const platformOptions = [
  { value: 'feishu', label: '飞书' },
  { value: 'telegram', label: 'Telegram' },
  { value: 'qq', label: 'QQ' },
]

function platformLabel(p: string) {
  return platformOptions.find(o => o.value === p)?.label || p
}

function sessionLabel(sid: number) {
  if (!sid) return '未绑定'
  const s = sessions.value.find(s => s.id === sid)
  return s ? `#${s.id} ${s.title}` : `#${sid}`
}

function configSummary(config: string) {
  try {
    const obj = JSON.parse(config)
    if (obj.app_id) return `App: ${obj.app_id}`
    return Object.keys(obj).length ? `${Object.keys(obj).length} 项配置` : '未配置'
  } catch { return '未配置' }
}

function webhookUrl(ch: Channel) {
  return `${location.origin}/api/v1/webhook/${ch.platform}`
}

async function load() {
  loading.value = true
  try {
    const [c, s] = await Promise.all([listChannels(), listSessions()])
    channels.value = c
    sessions.value = s
  } catch { channels.value = []; sessions.value = [] }
  loading.value = false
}

function openCreate() {
  form.value = { name: '', platform: 'feishu', session_id: 0, config: '{}' }
  showCreate.value = true
}

async function onCreate() {
  if (!form.value.name || !form.value.platform) return
  await createChannel(form.value)
  showCreate.value = false
  load()
}

function openEdit(ch: Channel) {
  editTarget.value = ch
  form.value = { name: ch.name, platform: ch.platform, session_id: ch.session_id, config: ch.config }
}

async function onEdit() {
  if (!editTarget.value) return
  await updateChannel(editTarget.value.id, form.value)
  editTarget.value = null
  load()
}

async function onToggle(ch: Channel) {
  const newEnabled = !ch.enabled
  ch.enabled = newEnabled
  try { await updateChannel(ch.id, { enabled: newEnabled }) } catch { ch.enabled = !newEnabled }
  load()
}

async function onDelete() {
  if (!deleteTarget.value) return
  await deleteChannel(deleteTarget.value.id)
  deleteTarget.value = null
  load()
}

onMounted(load)
</script>

<template>
  <div class="channels-page">
    <div class="page-header">
      <div>
        <h2 class="page-title">通讯频道</h2>
        <span class="page-desc">对接外部 IM 平台，接收消息并转发到绑定的会话</span>
      </div>
      <button class="btn-create" @click="openCreate">
        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M12 5v14M5 12h14"/></svg>
        新建
      </button>
    </div>

    <div v-if="loading" class="empty-state">加载中...</div>
    <div v-else-if="channels.length === 0" class="empty-state">暂无频道，点击「新建」添加</div>

    <div class="card-list">
      <div v-for="ch in channels" :key="ch.id" class="card">
        <div class="card-body">
          <div class="card-top">
            <span class="platform-tag" :class="'platform-' + ch.platform">{{ platformLabel(ch.platform) }}</span>
            <span class="card-name">{{ ch.name }}</span>
            <span v-if="!ch.enabled" class="status-tag status-disabled">已禁用</span>
          </div>
          <div class="card-meta">
            <span class="meta-item">绑定: {{ sessionLabel(ch.session_id) }}</span>
            <span class="meta-item">{{ configSummary(ch.config) }}</span>
            <span class="meta-item webhook-url" :title="webhookUrl(ch)">Webhook: {{ webhookUrl(ch) }}</span>
          </div>
        </div>
        <div class="card-actions">
          <label class="toggle">
            <input type="checkbox" :checked="ch.enabled" @change="onToggle(ch)" />
            <span class="toggle-slider"></span>
          </label>
          <button class="btn-edit" @click="openEdit(ch)" title="编辑">
            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M11 4H4a2 2 0 00-2 2v14a2 2 0 002 2h14a2 2 0 002-2v-7"/><path d="M18.5 2.5a2.121 2.121 0 013 3L12 15l-4 1 1-4 9.5-9.5z"/></svg>
          </button>
          <button class="btn-del" @click="deleteTarget = ch" title="删除">
            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M18 6L6 18M6 6l12 12"/></svg>
          </button>
        </div>
      </div>
    </div>

    <!-- Create modal -->
    <Teleport to="body">
      <div v-if="showCreate" class="modal-overlay" @click="showCreate = false">
        <div class="modal-box" @click.stop>
          <p class="modal-title">新建频道</p>
          <div class="form-group">
            <label>名称</label>
            <input v-model="form.name" placeholder="如：飞书客服机器人" />
          </div>
          <div class="form-group">
            <label>平台</label>
            <select v-model="form.platform">
              <option v-for="p in platformOptions" :key="p.value" :value="p.value">{{ p.label }}</option>
            </select>
          </div>
          <div class="form-group">
            <label>绑定会话</label>
            <select v-model.number="form.session_id">
              <option :value="0">不绑定</option>
              <option v-for="s in sessions" :key="s.id" :value="s.id">#{{ s.id }} {{ s.title }}</option>
            </select>
          </div>
          <div class="form-group">
            <label>平台配置 (JSON)</label>
            <textarea v-model="form.config" rows="5" placeholder='{"app_id":"","app_secret":"","verification_token":""}'></textarea>
            <span class="form-hint">飞书: app_id, app_secret, verification_token</span>
          </div>
          <div class="modal-actions">
            <button class="modal-btn cancel" @click="showCreate = false">取消</button>
            <button class="modal-btn confirm" @click="onCreate">创建</button>
          </div>
        </div>
      </div>
    </Teleport>

    <!-- Edit modal -->
    <Teleport to="body">
      <div v-if="editTarget" class="modal-overlay" @click="editTarget = null">
        <div class="modal-box" @click.stop>
          <p class="modal-title">编辑频道</p>
          <div class="form-group">
            <label>名称</label>
            <input v-model="form.name" />
          </div>
          <div class="form-group">
            <label>平台</label>
            <select v-model="form.platform">
              <option v-for="p in platformOptions" :key="p.value" :value="p.value">{{ p.label }}</option>
            </select>
          </div>
          <div class="form-group">
            <label>绑定会话</label>
            <select v-model.number="form.session_id">
              <option :value="0">不绑定</option>
              <option v-for="s in sessions" :key="s.id" :value="s.id">#{{ s.id }} {{ s.title }}</option>
            </select>
          </div>
          <div class="form-group">
            <label>平台配置 (JSON)</label>
            <textarea v-model="form.config" rows="5"></textarea>
          </div>
          <div class="modal-actions">
            <button class="modal-btn cancel" @click="editTarget = null">取消</button>
            <button class="modal-btn confirm" @click="onEdit">保存</button>
          </div>
        </div>
      </div>
    </Teleport>

    <!-- Delete confirm -->
    <Teleport to="body">
      <div v-if="deleteTarget" class="modal-overlay" @click="deleteTarget = null">
        <div class="modal-box" @click.stop>
          <p class="modal-title">确认删除</p>
          <p class="modal-desc">删除频道「{{ deleteTarget.name }}」？</p>
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
.channels-page { padding: 24px; overflow-y: auto; height: 100%; }
.page-header { display: flex; align-items: flex-start; justify-content: space-between; margin-bottom: 20px; }
.page-title { font-size: 18px; font-weight: 600; color: var(--text-primary); margin: 0; }
.page-desc { font-size: 12px; color: var(--text-muted); margin-top: 4px; display: block; }
.btn-create {
  display: flex; align-items: center; gap: 4px; padding: 6px 14px;
  border-radius: var(--radius); font-size: 13px; font-weight: 500;
  background: var(--accent); color: #fff; transition: opacity var(--transition); flex-shrink: 0;
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
.card-body { flex: 1; min-width: 0; }
.card-top { display: flex; align-items: center; gap: 8px; }
.card-name { font-size: 14px; font-weight: 500; color: var(--text-primary); }
.platform-tag { font-size: 11px; padding: 2px 8px; border-radius: 9999px; flex-shrink: 0; }
.platform-feishu { background: rgba(59,130,246,0.15); color: #3b82f6; }
.platform-telegram { background: rgba(34,197,94,0.15); color: #22c55e; }
.platform-qq { background: rgba(168,85,247,0.15); color: #a855f7; }
.status-tag { font-size: 11px; padding: 2px 8px; border-radius: 9999px; flex-shrink: 0; }
.status-disabled { background: var(--bg-tertiary); color: var(--text-muted); }
.card-meta { display: flex; flex-wrap: wrap; gap: 12px; margin-top: 6px; }
.meta-item { font-size: 11px; color: var(--text-muted); }
.webhook-url { max-width: 300px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
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
.toggle input:checked + .toggle-slider::before { transform: translateX(16px); background: white; }
.modal-overlay {
  position: fixed; inset: 0; background: rgba(0,0,0,0.5);
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
.form-group textarea { resize: vertical; font-family: monospace; font-size: 12px; }
.form-hint { font-size: 11px; color: var(--text-muted); margin-top: 2px; display: block; }
.modal-actions { display: flex; justify-content: flex-end; gap: 8px; margin-top: 16px; }
.modal-btn { padding: 6px 16px; border-radius: var(--radius); font-size: 13px; font-weight: 500; cursor: pointer; transition: all var(--transition); }
.modal-btn.cancel { color: var(--text-secondary); background: var(--bg-hover); }
.modal-btn.cancel:hover { color: var(--text-primary); }
.modal-btn.confirm { color: #fff; background: var(--accent); }
.modal-btn.confirm:hover { opacity: 0.9; }
</style>
