<script setup lang="ts">
import { computed, ref, onMounted, watch, reactive } from 'vue'
import { useChatStore } from '../stores/chat'
import type { Provider, CompressSettings } from '../types'
import * as api from '../composables/api'
import type { ClaudeAuthStatus } from '../composables/api'

const store = useChatStore()
const showForm = ref(false)
const editing = ref<Provider | null>(null)

type UsageMode = 'upstream' | 'middleware'
type ProviderForm = {
  name: string
  auth_mode: string
  usage_mode: UsageMode
  proxy_url: string
  base_url: string
  api_key: string
  model_id: string
  is_default: boolean
}

const form = ref<ProviderForm>({
  name: '',
  auth_mode: 'api_key',
  usage_mode: 'upstream',
  proxy_url: '',
  base_url: '',
  api_key: '',
  model_id: '',
  is_default: false,
})

const authStatus = ref<ClaudeAuthStatus | null>(null)
const authLoading = ref(false)

async function loadAuthStatus() {
  authLoading.value = true
  try {
    authStatus.value = await api.getClaudeAuthStatus()
  } catch { authStatus.value = null }
  finally { authLoading.value = false }
}

function resetForm() {
  form.value = { name: '', auth_mode: 'api_key', usage_mode: 'upstream', proxy_url: '', base_url: '', api_key: '', model_id: '', is_default: false }
  editing.value = null
  showForm.value = false
}

function editProvider(p: Provider) {
  editing.value = p
  form.value = {
    name: p.name,
    auth_mode: p.auth_mode || 'api_key',
    usage_mode: p.usage_mode || 'upstream',
    proxy_url: p.proxy_url || '',
    base_url: p.base_url,
    api_key: p.api_key,
    model_id: p.model_id,
    is_default: p.is_default,
  }
  showForm.value = true
  if (p.auth_mode === 'oauth') loadAuthStatus()
}

watch(() => form.value.auth_mode, (mode) => {
  if (mode === 'oauth') {
    form.value.model_id = ''
    loadAuthStatus()
  }
})

function isLikelyOllamaBaseURL(baseURL: string): boolean {
  try {
    const u = new URL(baseURL.trim())
    const host = u.hostname.toLowerCase()
    const port = u.port
    if (host.includes('ollama')) return true
    return (host === 'localhost' || host === '127.0.0.1') && port === '11434'
  } catch {
    return false
  }
}

const needsApiKey = computed(() => {
  if (form.value.auth_mode === 'oauth') return false
  return !isLikelyOllamaBaseURL(form.value.base_url)
})

async function saveProvider() {
  if (editing.value) {
    await api.updateProvider(editing.value.id, form.value)
  } else {
    await api.createProvider(form.value)
  }
  await store.loadProviders()
  resetForm()
}

async function removeProvider(id: string) {
  await api.deleteProvider(id)
  await store.loadProviders()
}

async function setDefaultProvider(id: string) {
  await api.setProviderDefault(id)
  await store.loadProviders()
}

function maskKey(key: string): string {
  if (!key || key.length < 8) return '••••••••'
  return key.slice(0, 4) + '••••' + key.slice(-4)
}


// ---- Data Management ----
const importFileInput = ref<HTMLInputElement | null>(null)

// 导入功能
function handleImport() {
  importFileInput.value?.click()
}

async function onImportFileSelected(event: Event) {
  const target = event.target as HTMLInputElement
  const file = target.files?.[0]
  if (!file) return

  try {
    const formData = new FormData()
    formData.append('file', file)

    const res = await fetch('/api/v1/import', {
      method: 'POST',
      body: formData,
    })

    if (!res.ok) {
      const error = await res.json()
      alert('导入失败: ' + (error.error || '未知错误'))
      return
    }

    alert('导入成功！页面将刷新以加载新数据。')
    window.location.reload()
  } catch (err) {
    console.error('Import failed:', err)
    alert('导入失败: ' + err)
  } finally {
    // Reset file input
    if (target) target.value = ''
  }
}

// 导出功能
async function handleExport() {
  try {
    const res = await fetch('/api/v1/export')
    if (!res.ok) {
      const error = await res.json()
      alert('导出失败: ' + (error.error || '未知错误'))
      return
    }

    const blob = await res.blob()
    const url = window.URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = `ai-hub-backup-${new Date().toISOString().split('T')[0]}.json`
    document.body.appendChild(a)
    a.click()
    document.body.removeChild(a)
    window.URL.revokeObjectURL(url)
  } catch (err) {
    console.error('Export failed:', err)
    alert('导出失败: ' + err)
  }
}
onMounted(() => {
  store.loadProviders()
  loadCompressSettings()
})

// ---- Auto Compress Settings ----
const compressForm = reactive<CompressSettings>({
  auto_enabled: false,
  threshold: 80000,
  mode: 'auto',
  min_turns: 10,
})
const compressSaveOk = ref(false)
const compressSaveErr = ref('')

async function loadCompressSettings() {
  try {
    const cfg = await api.getCompressSettings()
    compressForm.auto_enabled = cfg.auto_enabled
    compressForm.threshold = cfg.threshold
    compressForm.mode = cfg.mode
    compressForm.min_turns = cfg.min_turns ?? 10
  } catch { /* ignore */ }
}

async function saveCompressSettings() {
  compressSaveOk.value = false
  compressSaveErr.value = ''
  try {
    await api.updateCompressSettings({ ...compressForm })
    compressSaveOk.value = true
    setTimeout(() => { compressSaveOk.value = false }, 3000)
  } catch (e: unknown) {
    compressSaveErr.value = e instanceof Error ? e.message : '保存失败'
  }
}

</script>

<template>
  <div class="settings-page">
    <div class="settings-container">

      <section class="section">
        <div class="section-header">
          <div>
            <h2>模型供应商</h2>
            <p class="section-desc">配置 LLM API 端点。所有供应商统一通过 Claude Code CLI 路由。</p>
          </div>
          <button class="btn-add" @click="showForm = true">
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <path d="M12 5v14M5 12h14"/>
            </svg>
            添加
          </button>
        </div>

        <div class="provider-list">
          <div v-for="p in store.providers" :key="p.id" class="provider-card">
            <div class="provider-info">
              <div class="provider-name">
                {{ p.name }}
                <span v-if="p.is_default" class="badge default">默认</span>
                <span class="badge mode">Claude Code</span>
                <span v-if="p.auth_mode === 'oauth'" class="badge oauth">OAuth</span>
                <span v-if="p.usage_mode === 'middleware'" class="badge meter">Middleware Metering</span>
              </div>
              <div class="provider-meta">
                {{ p.model_id }}
                <span v-if="p.base_url" class="sep">·</span>
                <span v-if="p.base_url" class="url">{{ p.base_url }}</span>
                <span v-if="p.proxy_url" class="sep">·</span>
                <span v-if="p.proxy_url" class="url">Proxy {{ p.proxy_url }}</span>
                <span class="sep">·</span>
                <span class="key">{{ maskKey(p.api_key) }}</span>
              </div>
            </div>
            <div class="provider-actions">
              <button v-if="!p.is_default" class="btn-sm btn-default" @click="setDefaultProvider(p.id)" title="设为默认运营商">设为默认</button>
              <button class="btn-sm" @click="editProvider(p)">编辑</button>
              <button class="btn-sm btn-danger" @click="removeProvider(p.id)">删除</button>
            </div>
          </div>
          <div v-if="store.providers.length === 0" class="empty">
            暂无供应商，请添加一个开始使用。
          </div>
        </div>

        <!-- Form Modal -->
        <div v-if="showForm" class="form-overlay" @click.self="resetForm">
          <div class="form-modal">
            <h3>{{ editing ? '编辑' : '添加' }}供应商</h3>

            <div class="form-group">
              <label>名称</label>
              <input v-model="form.name" placeholder="如：Claude Pro、GPT-4o" />
            </div>

            <div class="form-group">
              <label>认证模式</label>
              <select v-model="form.auth_mode">
                <option value="api_key">API Key</option>
                <option value="oauth">订阅账号 (OAuth)</option>
              </select>
              <span class="hint">OAuth 模式使用本机已登录的 Claude 订阅账号，无需 API Key。</span>
            </div>

            <template v-if="form.auth_mode === 'oauth'">
              <div class="form-group">
                <label>登录状态</label>
                <div v-if="authLoading" class="auth-status loading">检测中...</div>
                <div v-else-if="authStatus?.logged_in" class="auth-status ok">
                  ✓ 已登录 ({{ authStatus.auth_method }}<span v-if="authStatus.email">, {{ authStatus.email }}</span>)
                </div>
                <div v-else class="auth-status fail">
                  ✗ 未登录，请在终端执行 <code>claude auth login</code>
                </div>
              </div>
            </template>

            <template v-if="form.auth_mode !== 'oauth'">
              <div class="form-group">
                <label>API 地址</label>
                <input v-model="form.base_url" placeholder="https://api.example.com" />
                <span class="hint">API 端点地址。Ollama 示例：`http://localhost:11434`。</span>
              </div>

              <div class="form-group">
                <label>API 密钥</label>
                <input v-model="form.api_key" type="password" placeholder="sk-..." />
                <span class="hint">Ollama 可留空；其他供应商通常必填。</span>
              </div>
            </template>

            <div class="form-group">
              <label>代理地址（可选）</label>
              <input v-model="form.proxy_url" placeholder="http://127.0.0.1:7890" />
              <span class="hint">为该供应商的 Claude 子进程单独设置代理。留空则不覆盖系统代理。</span>
            </div>

            <div class="form-group">
              <label>模型 ID</label>
              <input
                v-model="form.model_id"
                :disabled="form.auth_mode === 'oauth'"
                placeholder="留空使用默认模型；可填 qwen3-coder / glm-4.7 / llama3.1"
              />
              <span class="hint" v-if="form.auth_mode === 'oauth'">订阅账号模式不支持手动指定模型，将使用 Claude 默认模型。</span>
              <span class="hint" v-else>可留空使用默认模型；按需填写具体模型 ID。</span>
            </div>

            <div class="form-group">
              <label>Token 统计模式</label>
              <select v-model="form.usage_mode">
                <option value="upstream">Upstream（默认）</option>
                <option value="middleware">Middleware（中转修正）</option>
              </select>
              <span class="hint">默认使用上游返回。仅在需要本地中转修正统计时开启，便于后续接入不同 LLM API。</span>
            </div>

            <div class="form-group checkbox">
              <label>
                <input type="checkbox" v-model="form.is_default" />
                设为默认供应商
              </label>
            </div>

            <div class="form-actions">
              <button class="btn-cancel" @click="resetForm">取消</button>
              <button class="btn-save" @click="saveProvider" :disabled="!form.name || (needsApiKey && !form.api_key)">
                保存
              </button>
            </div>
          </div>
        </div>
      </section>

      <!-- Auto Compress Settings -->
      <section class="section">
        <div class="section-header">
          <div>
            <h2>自动压缩</h2>
            <p class="section-desc">Token 使用量达到阈值时自动压缩会话上下文，延长可用会话长度。</p>
          </div>
        </div>

        <div class="compress-settings">
          <div class="form-group checkbox">
            <label>
              <input type="checkbox" v-model="compressForm.auto_enabled" />
              启用自动压缩
            </label>
            <span class="hint">开启后，每轮对话结束时检测 token 总量，超过阈值则自动触发压缩并重置会话上下文。</span>
          </div>

          <template v-if="compressForm.auto_enabled">
            <div class="form-group">
              <label>触发阈值（input tokens）</label>
              <div class="threshold-row">
                <input
                  type="number"
                  v-model.number="compressForm.threshold"
                  min="10000"
                  max="500000"
                  step="5000"
                />
                <span class="threshold-label">{{ (compressForm.threshold / 1000).toFixed(0) }}k tokens</span>
              </div>
              <span class="hint">单会话累计 input token 数超过此值时触发压缩。建议：80000（约 80k tokens，对应 200k 上下文窗口的 40%）。</span>
            </div>

            <div class="form-group">
              <label>最少对话轮数</label>
              <div class="threshold-row">
                <input
                  type="number"
                  v-model.number="compressForm.min_turns"
                  min="0"
                  max="500"
                  step="1"
                />
                <span class="threshold-label">轮</span>
              </div>
              <span class="hint">token 数超过阈值且对话轮数（用户消息数）达到此值，才触发压缩。设为 0 则仅按 token 阈值判断。默认 10 轮，避免会话过短时频繁压缩。</span>
            </div>

            <div class="form-group">
              <label>压缩模式</label>
              <select v-model="compressForm.mode">
                <option value="auto">智能优先（推荐）：先用 Claude 生成摘要，失败自动降级为简单截取</option>
                <option value="intelligent">仅智能：Claude 生成摘要，失败则跳过压缩</option>
                <option value="simple">仅简单截取：取最近 10 条消息，无需额外 API 调用</option>
              </select>
              <span class="hint">智能模式使用 Claude 生成高质量上下文摘要（消耗少量 token）；简单模式不消耗额外 token。</span>
            </div>
          </template>

          <div class="form-actions">
            <button class="btn-save" @click="saveCompressSettings">保存配置</button>
            <span v-if="compressSaveOk" class="save-ok">✓ 已保存</span>
            <span v-if="compressSaveErr" class="save-err">{{ compressSaveErr }}</span>
          </div>
        </div>
      </section>


      <!-- 数据管理 -->
      <section class="section">
        <div class="section-header">
          <div>
            <h2>数据管理</h2>
            <p class="section-desc">导入和导出会话数据</p>
          </div>
        </div>
        
        <div class="data-management">
          <!-- 导入功能 -->
          <div class="management-item">
            <div class="item-info">
              <h4>导入数据</h4>
              <p>从备份文件导入会话、设置和记忆数据</p>
            </div>
            <button class="action-btn" @click="handleImport">
              <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/>
                <polyline points="7 10 12 15 17 10"/>
                <line x1="12" y1="15" x2="12" y2="3"/>
              </svg>
              导入
            </button>
          </div>
          
          <!-- 导出功能 -->
          <div class="management-item">
            <div class="item-info">
              <h4>导出数据</h4>
              <p>导出所有会话、设置和记忆数据到备份文件</p>
            </div>
            <button class="action-btn" @click="handleExport">
              <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/>
                <polyline points="17 8 12 3 7 8"/>
                <line x1="12" y1="3" x2="12" y2="15"/>
              </svg>
              导出
            </button>
          </div>
        </div>
      </section>

      <!-- 隐藏的文件选择器 -->
      <input
        ref="importFileInput"
        type="file"
        accept=".json"
        style="display: none"
        @change="onImportFileSelected"
      />
    </div>
  </div>
</template>

<style scoped>
.settings-page {
  height: 100vh;
  height: 100dvh;
  overflow-y: auto;
  background: var(--bg-primary);
}
.settings-container {
  max-width: 680px;
  margin: 0 auto;
  padding: 32px 24px;
}

.section { margin-bottom: 32px; }
.section-header {
  display: flex; align-items: flex-start; justify-content: space-between;
  margin-bottom: 16px;
}
.section-header h2 { font-size: 16px; font-weight: 600; }
.section-desc { font-size: 12px; color: var(--text-muted); margin-top: 4px; }
.btn-add {
  display: flex; align-items: center; gap: 6px;
  padding: 8px 14px; background: var(--accent); color: var(--btn-text);
  border-radius: var(--radius); font-size: 13px; font-weight: 500;
  transition: background var(--transition); flex-shrink: 0;
}
.btn-add:hover { background: var(--accent-hover); }

.provider-list { display: flex; flex-direction: column; gap: 8px; }
.provider-card {
  display: flex; align-items: center; justify-content: space-between;
  padding: 14px 16px; background: var(--bg-secondary);
  border: 1px solid var(--border); border-radius: var(--radius);
}
.provider-info { min-width: 0; flex: 1; }
.provider-name {
  font-weight: 500; font-size: 14px;
  display: flex; align-items: center; gap: 8px;
}
.provider-meta {
  font-size: 12px; color: var(--text-muted); margin-top: 4px;
  white-space: nowrap; overflow: hidden; text-overflow: ellipsis;
}
.sep { margin: 0 2px; }
.badge {
  font-size: 10px; padding: 2px 8px; border-radius: 99px;
  font-weight: 600; text-transform: uppercase; letter-spacing: 0.5px;
}
.badge.default { background: var(--accent-soft); color: var(--accent); }
.badge.mode { background: var(--bg-tertiary); color: var(--text-secondary); }
.badge.oauth { background: rgba(34,197,94,0.15); color: #22c55e; }
.badge.meter { background: rgba(59,130,246,0.14); color: #3b82f6; }
.auth-status { font-size: 13px; padding: 8px 12px; border-radius: var(--radius); }
.auth-status.loading { color: var(--text-muted); background: var(--bg-tertiary); }
.auth-status.ok { color: #22c55e; background: rgba(34,197,94,0.1); }
.auth-status.fail { color: var(--danger); background: rgba(239,68,68,0.1); }
.auth-status code { font-size: 12px; background: var(--bg-tertiary); padding: 2px 6px; border-radius: 3px; }
.provider-actions { display: flex; gap: 6px; flex-shrink: 0; margin-left: 12px; }
.btn-sm {
  padding: 6px 12px; font-size: 12px; border-radius: var(--radius-sm);
  background: var(--bg-tertiary); color: var(--text-secondary);
  transition: all var(--transition);
}
.btn-sm:hover { background: var(--bg-hover); color: var(--text-primary); }
.btn-danger:hover { background: rgba(239,68,68,0.15); color: var(--danger); }
.btn-default { color: var(--accent); }
.btn-default:hover { background: var(--accent-soft); color: var(--accent); }
.empty { text-align: center; color: var(--text-muted); padding: 32px; font-size: 13px; }

/* Modal */
.form-overlay {
  position: fixed; inset: 0; background: var(--overlay);
  display: flex; align-items: center; justify-content: center;
  z-index: 100; backdrop-filter: blur(4px);
}
.form-modal {
  background: var(--bg-secondary); border: 1px solid var(--border);
  border-radius: var(--radius-lg); padding: 24px;
  width: 440px; max-width: 90vw;
}
.form-modal h3 { font-size: 16px; font-weight: 600; margin-bottom: 20px; }
.form-group { margin-bottom: 14px; }
.form-group label {
  display: block; font-size: 12px; font-weight: 500;
  color: var(--text-secondary); margin-bottom: 6px;
  text-transform: uppercase; letter-spacing: 0.5px;
}
.form-group input, .form-group select {
  width: 100%; padding: 10px 12px;
  background: var(--bg-tertiary); border: 1px solid var(--border);
  border-radius: var(--radius); font-size: 14px; color: var(--text-primary);
  transition: border-color var(--transition);
}
.form-group input:focus { border-color: var(--accent); }
.hint { display: block; font-size: 11px; color: var(--text-muted); margin-top: 4px; }
.form-group.checkbox label {
  display: flex; align-items: center; gap: 8px;
  text-transform: none; letter-spacing: 0; font-size: 14px; cursor: pointer;
}
.form-group.checkbox input[type="checkbox"] {
  width: 16px; height: 16px; accent-color: var(--accent);
}
.form-actions { display: flex; justify-content: flex-end; gap: 8px; margin-top: 20px; }
.btn-cancel {
  padding: 8px 16px; border-radius: var(--radius); font-size: 13px;
  color: var(--text-secondary); background: var(--bg-tertiary);
  transition: all var(--transition);
}
.btn-cancel:hover { background: var(--bg-hover); }
.btn-save {
  padding: 8px 20px; border-radius: var(--radius); font-size: 13px; font-weight: 500;
  background: var(--accent); color: var(--btn-text); transition: background var(--transition);
}
.btn-save:hover:not(:disabled) { background: var(--accent-hover); }
.btn-save:disabled { opacity: 0.4; cursor: not-allowed; }
/* ---- Auto Compress Settings ---- */
.compress-settings { display: flex; flex-direction: column; gap: 16px; }
.threshold-row { display: flex; align-items: center; gap: 10px; }
.threshold-row input[type="number"] { width: 120px; }
.threshold-label { font-size: 13px; color: var(--text-secondary); }
.save-ok { font-size: 13px; color: var(--accent); margin-left: 10px; }
.save-err { font-size: 13px; color: #e74c3c; margin-left: 10px; }

/* ---- Shadow AI Settings ---- */
.shadow-settings { display: flex; flex-direction: column; gap: 16px; }
.shadow-loading { text-align: center; color: var(--text-muted); padding: 20px; font-size: 13px; }

.shadow-toggle-row {
  display: flex; align-items: center; justify-content: space-between;
  padding: 14px 16px; background: var(--bg-secondary);
  border: 1px solid var(--border); border-radius: var(--radius);
}
.shadow-info { display: flex; align-items: center; gap: 12px; }
.shadow-status-badge {
  font-size: 11px; font-weight: 600; padding: 3px 10px;
  border-radius: 99px; text-transform: uppercase;
}
.shadow-status-badge.running { background: rgba(34,197,94,0.15); color: #22c55e; }
.shadow-status-badge.paused { background: rgba(251,191,36,0.15); color: #f59e0b; }
.shadow-status-badge.uninitialized { background: var(--bg-tertiary); color: var(--text-muted); }
.shadow-session { font-size: 12px; color: var(--text-muted); }

.btn-shadow-toggle {
  padding: 6px 16px; border-radius: var(--radius); font-size: 13px; font-weight: 500;
  background: var(--accent); color: var(--btn-text); transition: all var(--transition);
}
.btn-shadow-toggle:hover:not(:disabled) { background: var(--accent-hover); }
.btn-shadow-toggle.enabled { background: rgba(239,68,68,0.12); color: #ef4444; }
.btn-shadow-toggle.enabled:hover:not(:disabled) { background: rgba(239,68,68,0.2); }
.btn-shadow-toggle:disabled { opacity: 0.5; cursor: not-allowed; }

.shadow-config {
  display: grid; grid-template-columns: repeat(auto-fill, minmax(180px, 1fr)); gap: 8px;
  padding: 12px 16px; background: var(--bg-secondary);
  border: 1px solid var(--border); border-radius: var(--radius);
}
.config-item { display: flex; justify-content: space-between; align-items: center; }
.config-label { font-size: 12px; color: var(--text-secondary); }
.config-value { font-size: 12px; font-weight: 600; color: var(--text-primary); }
.config-edit-item { display: flex; justify-content: space-between; align-items: center; gap: 8px; }
.config-input {
  width: 100px; padding: 4px 8px; font-size: 12px; font-weight: 600;
  background: var(--bg-tertiary); border: 1px solid var(--border);
  border-radius: var(--radius-sm); color: var(--text-primary); text-align: right;
}
.config-input:focus { border-color: var(--accent); outline: none; }

.shadow-config-actions {
  display: flex; align-items: center; gap: 8px; justify-content: flex-end;
}

.shadow-triggers h4 { font-size: 13px; font-weight: 600; margin-bottom: 8px; color: var(--text-secondary); }
.trigger-item {
  display: flex; align-items: center; gap: 8px;
  padding: 6px 0; font-size: 12px; border-bottom: 1px solid var(--border);
}
.trigger-item:last-child { border-bottom: none; }
.trigger-status.active { color: #22c55e; }
.trigger-status.disabled { color: var(--text-muted); }
.trigger-content { flex: 1; color: var(--text-primary); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.trigger-meta { color: var(--text-muted); white-space: nowrap; }

.shadow-logs-section h4 { font-size: 13px; font-weight: 600; margin-bottom: 8px; color: var(--text-secondary); }
.shadow-logs-content {
  font-size: 11px; line-height: 1.5; color: var(--text-secondary);
  background: var(--bg-tertiary); border: 1px solid var(--border);
  border-radius: var(--radius); padding: 12px; margin: 0;
  max-height: 160px; overflow-y: auto; white-space: pre-wrap; word-break: break-word;
  font-family: 'SF Mono', 'Cascadia Code', 'Consolas', monospace;
}
.shadow-logs-loading { font-size: 12px; color: var(--text-muted); padding: 12px; }
.shadow-logs-empty { font-size: 12px; color: var(--text-muted); padding: 12px; text-align: center; }

@media (max-width: 768px) {
  .settings-container { padding: 16px 12px; }
  .form-modal { width: 100vw; max-width: 100vw; height: 100vh; height: 100dvh; max-height: 100vh; max-height: 100dvh; border-radius: 0; display: flex; flex-direction: column; }
  .form-modal h3 { margin-bottom: 12px; }
  .provider-card { flex-direction: column; align-items: flex-start; gap: 10px; }
  .provider-actions { margin-left: 0; width: 100%; justify-content: flex-end; }
}
</style>

/* ---- Data Management ---- */
.data-management {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.management-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 16px;
  background: var(--bg-secondary);
  border: 1px solid var(--border);
  border-radius: 8px;
}

.item-info h4 {
  margin: 0 0 4px 0;
  font-size: 14px;
  font-weight: 600;
  color: var(--text-primary);
}

.item-info p {
  margin: 0;
  font-size: 13px;
  color: var(--text-secondary);
}

.action-btn {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 16px;
  background: var(--primary);
  color: white;
  border: none;
  border-radius: 6px;
  cursor: pointer;
  font-size: 14px;
  transition: all 0.2s;
}

.action-btn:hover {
  opacity: 0.9;
}

.action-btn svg {
  flex-shrink: 0;
}
