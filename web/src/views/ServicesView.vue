<script setup lang="ts">
import { ref, onMounted } from 'vue'
import {
  listServices, createService, updateService, deleteService,
  startService, stopService, restartService, getServiceLogs,
  type Service
} from '../composables/api'

const services = ref<Service[]>([])
const loading = ref(false)
const showCreate = ref(false)
const editTarget = ref<Service | null>(null)
const deleteTarget = ref<Service | null>(null)
const logTarget = ref<Service | null>(null)
const logContent = ref('')
const logLoading = ref(false)
const actionLoading = ref<Record<number, boolean>>({})

const form = ref({ name: '', command: '', work_dir: '', port: 0, auto_start: false })

function resetForm() {
  form.value = { name: '', command: '', work_dir: '', port: 0, auto_start: false }
}

async function load() {
  loading.value = true
  try { services.value = await listServices() } catch { services.value = [] }
  loading.value = false
}

async function handleCreate() {
  if (!form.value.name || !form.value.command) return
  await createService(form.value)
  showCreate.value = false
  resetForm()
  load()
}

function openEdit(svc: Service) {
  editTarget.value = svc
  form.value = { name: svc.name, command: svc.command, work_dir: svc.work_dir, port: svc.port, auto_start: svc.auto_start }
}

async function handleUpdate() {
  if (!editTarget.value) return
  await updateService(editTarget.value.id, form.value)
  editTarget.value = null
  resetForm()
  load()
}

async function handleDelete() {
  if (!deleteTarget.value) return
  await deleteService(deleteTarget.value.id)
  deleteTarget.value = null
  load()
}

async function handleStart(id: number) {
  actionLoading.value[id] = true
  try { await startService(id); await load() } finally { actionLoading.value[id] = false }
}

async function handleStop(id: number) {
  actionLoading.value[id] = true
  try { await stopService(id); await load() } finally { actionLoading.value[id] = false }
}

async function handleRestart(id: number) {
  actionLoading.value[id] = true
  try { await restartService(id); await load() } finally { actionLoading.value[id] = false }
}

async function viewLogs(svc: Service) {
  logTarget.value = svc
  logLoading.value = true
  logContent.value = ''
  try {
    const res = await getServiceLogs(svc.id, 200)
    logContent.value = res.logs || '(空日志)'
    if (res.error) logContent.value = `⚠ ${res.error}\n\n${logContent.value}`
  } catch (e: any) { logContent.value = '加载失败: ' + e.message }
  logLoading.value = false
}

async function refreshLogs() {
  if (!logTarget.value) return
  viewLogs(logTarget.value)
}

function statusColor(status: string) {
  if (status === 'running') return '#22c55e'
  if (status === 'dead') return '#ef4444'
  return 'var(--text-muted)'
}

function statusLabel(status: string) {
  if (status === 'running') return '运行中'
  if (status === 'dead') return '已崩溃'
  return '已停止'
}

onMounted(load)
</script>

<template>
  <div class="services-page">
    <div class="page-header">
      <div>
        <h2 class="page-title">作品</h2>
        <span class="page-desc">管理托管服务进程，支持启停控制和日志查看</span>
      </div>
      <button class="btn-create" @click="resetForm(); showCreate = true">
        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M12 5v14M5 12h14"/></svg>
        新建
      </button>
    </div>

    <div v-if="loading" class="empty-state">加载中...</div>
    <div v-else-if="services.length === 0" class="empty-state">
      <svg width="40" height="40" viewBox="0 0 24 24" fill="none" stroke="var(--text-muted)" stroke-width="1.5" style="margin-bottom:8px">
        <path d="M4.5 16.5c-1.5 1.26-2 5-2 5s3.74-.5 5-2c.71-.84.7-2.13-.09-2.91a2.18 2.18 0 00-2.91-.09z"/>
        <path d="M12 15l-3-3a22 22 0 012-3.95A12.88 12.88 0 0122 2c0 2.72-.78 7.5-6 11a22.35 22.35 0 01-4 2z"/>
      </svg>
      <div>暂无服务，点击「新建」添加你的第一个作品</div>
    </div>

    <div class="card-list">
      <div v-for="svc in services" :key="svc.id" class="card">
        <div class="card-body">
          <div class="card-top">
            <span class="status-dot" :style="{ background: statusColor(svc.status) }"></span>
            <span class="card-name">{{ svc.name }}</span>
            <span class="status-label" :style="{ color: statusColor(svc.status) }">{{ statusLabel(svc.status) }}</span>
          </div>
          <div class="card-meta">
            <span class="svc-meta-item" :title="svc.command">命令: {{ svc.command.length > 50 ? svc.command.slice(0, 50) + '...' : svc.command }}</span>
            <span v-if="svc.port" class="svc-meta-item">端口: {{ svc.port }}</span>
            <span v-if="svc.pid" class="svc-meta-item">PID: {{ svc.pid }}</span>
            <span v-if="svc.work_dir" class="svc-meta-item" :title="svc.work_dir">目录: {{ svc.work_dir.length > 30 ? '...' + svc.work_dir.slice(-30) : svc.work_dir }}</span>
            <span v-if="svc.auto_start" class="svc-meta-item auto-tag">自启</span>
          </div>
        </div>
        <div class="card-actions">
          <button
            v-if="svc.status !== 'running'"
            class="btn-action btn-start"
            :disabled="actionLoading[svc.id]"
            @click="handleStart(svc.id)"
            title="启动"
          >
            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polygon points="5 3 19 12 5 21 5 3"/></svg>
          </button>
          <button
            v-if="svc.status === 'running'"
            class="btn-action btn-stop"
            :disabled="actionLoading[svc.id]"
            @click="handleStop(svc.id)"
            title="停止"
          >
            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="6" y="4" width="4" height="16"/><rect x="14" y="4" width="4" height="16"/></svg>
          </button>
          <button
            class="btn-action btn-restart"
            :disabled="actionLoading[svc.id]"
            @click="handleRestart(svc.id)"
            title="重启"
          >
            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="23 4 23 10 17 10"/><path d="M20.49 15a9 9 0 11-2.12-9.36L23 10"/></svg>
          </button>
          <button class="btn-action btn-log" @click="viewLogs(svc)" title="日志">
            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M14 2H6a2 2 0 00-2 2v16a2 2 0 002 2h12a2 2 0 002-2V8z"/><polyline points="14 2 14 8 20 8"/></svg>
          </button>
          <button class="btn-action btn-edit" @click="openEdit(svc)" title="编辑">
            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M11 4H4a2 2 0 00-2 2v14a2 2 0 002 2h14a2 2 0 002-2v-7"/><path d="M18.5 2.5a2.121 2.121 0 013 3L12 15l-4 1 1-4 9.5-9.5z"/></svg>
          </button>
          <button class="btn-action btn-del" @click="deleteTarget = svc" title="删除">
            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M18 6L6 18M6 6l12 12"/></svg>
          </button>
        </div>
      </div>
    </div>

    <!-- Create modal -->
    <Teleport to="body">
      <div v-if="showCreate" class="modal-overlay" @click="showCreate = false">
        <div class="modal-box" @click.stop>
          <p class="modal-title">新建服务</p>
          <div class="form-group">
            <label>名称</label>
            <input v-model="form.name" placeholder="如：my-web-server" />
          </div>
          <div class="form-group">
            <label>启动命令</label>
            <input v-model="form.command" placeholder="如：node server.js" />
          </div>
          <div class="form-group">
            <label>工作目录</label>
            <input v-model="form.work_dir" placeholder="可选，留空使用默认目录" />
          </div>
          <div class="form-row">
            <div class="form-group form-half">
              <label>端口</label>
              <input v-model.number="form.port" type="number" placeholder="0" />
            </div>
            <div class="form-group form-half">
              <label>自动启动</label>
              <label class="toggle">
                <input type="checkbox" v-model="form.auto_start" />
                <span class="toggle-slider"></span>
              </label>
            </div>
          </div>
          <div class="modal-actions">
            <button class="modal-btn cancel" @click="showCreate = false">取消</button>
            <button class="modal-btn confirm" @click="handleCreate">创建</button>
          </div>
        </div>
      </div>
    </Teleport>

    <!-- Edit modal -->
    <Teleport to="body">
      <div v-if="editTarget" class="modal-overlay" @click="editTarget = null">
        <div class="modal-box" @click.stop>
          <p class="modal-title">编辑服务</p>
          <div class="form-group">
            <label>名称</label>
            <input v-model="form.name" />
          </div>
          <div class="form-group">
            <label>启动命令</label>
            <input v-model="form.command" />
          </div>
          <div class="form-group">
            <label>工作目录</label>
            <input v-model="form.work_dir" placeholder="可选" />
          </div>
          <div class="form-row">
            <div class="form-group form-half">
              <label>端口</label>
              <input v-model.number="form.port" type="number" />
            </div>
            <div class="form-group form-half">
              <label>自动启动</label>
              <label class="toggle">
                <input type="checkbox" v-model="form.auto_start" />
                <span class="toggle-slider"></span>
              </label>
            </div>
          </div>
          <div class="modal-actions">
            <button class="modal-btn cancel" @click="editTarget = null">取消</button>
            <button class="modal-btn confirm" @click="handleUpdate">保存</button>
          </div>
        </div>
      </div>
    </Teleport>

    <!-- Delete confirm -->
    <Teleport to="body">
      <div v-if="deleteTarget" class="modal-overlay" @click="deleteTarget = null">
        <div class="modal-box" @click.stop>
          <p class="modal-title">确认删除</p>
          <p class="modal-desc">删除服务「{{ deleteTarget.name }}」？运行中的服务将被停止。</p>
          <div class="modal-actions">
            <button class="modal-btn cancel" @click="deleteTarget = null">取消</button>
            <button class="modal-btn confirm-danger" @click="handleDelete">删除</button>
          </div>
        </div>
      </div>
    </Teleport>

    <!-- Log modal -->
    <Teleport to="body">
      <div v-if="logTarget" class="modal-overlay" @click="logTarget = null">
        <div class="log-modal" @click.stop>
          <div class="log-header">
            <span class="log-title">{{ logTarget.name }} — 日志</span>
            <div class="log-header-actions">
              <button class="btn-refresh" @click="refreshLogs" :disabled="logLoading">
                <svg :class="{ spinning: logLoading }" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="23 4 23 10 17 10"/><path d="M20.49 15a9 9 0 11-2.12-9.36L23 10"/></svg>
                刷新
              </button>
              <button class="btn-close-log" @click="logTarget = null">
                <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M18 6L6 18M6 6l12 12"/></svg>
              </button>
            </div>
          </div>
          <pre class="log-content">{{ logContent }}</pre>
        </div>
      </div>
    </Teleport>
  </div>
</template>

<style scoped>
.services-page { padding: 24px; overflow-y: auto; height: 100%; }
.page-header { display: flex; align-items: flex-start; justify-content: space-between; margin-bottom: 20px; }
.page-title { font-size: 18px; font-weight: 600; color: var(--text-primary); margin: 0; }
.page-desc { font-size: 12px; color: var(--text-muted); margin-top: 4px; display: block; }
.btn-create {
  display: flex; align-items: center; gap: 4px; padding: 6px 14px;
  border-radius: var(--radius); font-size: 13px; font-weight: 500;
  background: var(--accent); color: var(--btn-text); transition: opacity var(--transition); flex-shrink: 0;
}
.btn-create:hover { opacity: 0.9; }
.empty-state { text-align: center; color: var(--text-muted); padding: 48px 16px; font-size: 14px; display: flex; flex-direction: column; align-items: center; }
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
.status-dot { width: 8px; height: 8px; border-radius: 50%; flex-shrink: 0; }
.status-label { font-size: 11px; flex-shrink: 0; }
.card-meta { display: flex; flex-wrap: wrap; gap: 12px; margin-top: 6px; }
.svc-meta-item { font-size: 11px; color: var(--text-muted); max-width: 300px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.auto-tag { color: var(--accent); background: var(--accent-soft); padding: 0 6px; border-radius: 9999px; }
.card-actions { display: flex; align-items: center; gap: 4px; flex-shrink: 0; margin-left: 12px; }
.btn-action {
  width: 28px; height: 28px; display: flex; align-items: center; justify-content: center;
  border-radius: var(--radius-sm); color: var(--text-muted); transition: all var(--transition);
}
.btn-action:disabled { opacity: 0.4; cursor: not-allowed; }
.btn-start:hover:not(:disabled) { color: #22c55e; background: rgba(34,197,94,0.1); }
.btn-stop:hover:not(:disabled) { color: #f59e0b; background: rgba(245,158,11,0.1); }
.btn-restart:hover:not(:disabled) { color: var(--accent); background: var(--accent-soft); }
.btn-log:hover { color: var(--info); background: rgba(59,130,246,0.1); }
.btn-edit:hover { color: var(--accent); background: var(--accent-soft); }
.btn-del:hover { color: var(--danger); background: rgba(239,68,68,0.1); }
/* Modals */
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
.form-group input, .form-group select {
  width: 100%; padding: 8px 10px; font-size: 13px; border-radius: var(--radius);
  border: 1px solid var(--border); background: var(--bg-primary); color: var(--text-primary); box-sizing: border-box;
}
.form-row { display: flex; gap: 12px; }
.form-half { flex: 1; }
.modal-actions { display: flex; justify-content: flex-end; gap: 8px; margin-top: 16px; }
.modal-btn { padding: 6px 16px; border-radius: var(--radius); font-size: 13px; font-weight: 500; cursor: pointer; transition: all var(--transition); }
.modal-btn.cancel { color: var(--text-secondary); background: var(--bg-hover); }
.modal-btn.cancel:hover { color: var(--text-primary); }
.modal-btn.confirm { color: var(--btn-text); background: var(--accent); }
.modal-btn.confirm:hover { opacity: 0.9; }
.modal-btn.confirm-danger { color: var(--btn-text); background: var(--danger, #ef4444); }
.modal-btn.confirm-danger:hover { opacity: 0.9; }
/* Toggle */
.toggle { position: relative; display: inline-block; width: 36px; height: 20px; cursor: pointer; margin-top: 4px; }
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
/* Log modal */
.log-modal {
  background: #1a1a2e; border: 1px solid var(--border); border-radius: 12px;
  width: 700px; max-width: 95vw; max-height: 80vh; display: flex; flex-direction: column;
}
.log-header {
  display: flex; align-items: center; justify-content: space-between;
  padding: 12px 16px; border-bottom: 1px solid rgba(255,255,255,0.1);
}
.log-title { font-size: 14px; font-weight: 500; color: #e2e8f0; }
.log-header-actions { display: flex; align-items: center; gap: 8px; }
.btn-refresh {
  display: flex; align-items: center; gap: 4px; padding: 4px 10px;
  border-radius: var(--radius-sm); font-size: 12px; color: #94a3b8;
  background: rgba(255,255,255,0.06); border: 1px solid rgba(255,255,255,0.1);
  cursor: pointer; transition: all 0.2s;
}
.btn-refresh:hover:not(:disabled) { color: #e2e8f0; background: rgba(255,255,255,0.1); }
.btn-refresh:disabled { opacity: 0.5; cursor: not-allowed; }
.btn-close-log {
  width: 28px; height: 28px; display: flex; align-items: center; justify-content: center;
  border-radius: var(--radius-sm); color: #94a3b8; cursor: pointer; transition: all 0.2s;
}
.btn-close-log:hover { color: #e2e8f0; background: rgba(255,255,255,0.1); }
.log-content {
  flex: 1; overflow: auto; padding: 16px; margin: 0;
  font-family: 'SF Mono', 'Fira Code', monospace; font-size: 12px; line-height: 1.6;
  color: #a5f3fc; white-space: pre-wrap; word-break: break-all;
}
.spinning { animation: spin 1s linear infinite; }
@keyframes spin { to { transform: rotate(360deg); } }
@media (max-width: 768px) {
  .services-page { padding: 12px; }
  .card { flex-direction: column; align-items: flex-start; gap: 8px; }
  .card-actions { width: 100%; justify-content: flex-end; margin-left: 0; }
  .form-row { flex-direction: column; gap: 0; }
  .log-modal { width: 100vw; max-width: 100vw; border-radius: 0; max-height: 100vh; max-height: 100dvh; }
}
</style>
