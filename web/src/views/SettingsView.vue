<script setup lang="ts">
import { computed, ref, onMounted, watch } from 'vue'
import { useRouter } from 'vue-router'
import { useChatStore } from '../stores/chat'
import type { Provider } from '../types'
import * as api from '../composables/api'
import type { ClaudeAuthStatus } from '../composables/api'
import { useTheme, type ThemeMode } from '../composables/theme'

const { mode: themeMode, setMode } = useTheme()
const themeModeLabel: Record<string, string> = { system: '跟随系统', light: '亮色', dark: '暗色' }
function toggleTheme() {
  const order: ThemeMode[] = ['system', 'light', 'dark']
  const next = order[(order.indexOf(themeMode.value) + 1) % 3] ?? 'system'
  setMode(next)
}

const router = useRouter()
const store = useChatStore()
const showForm = ref(false)
const editing = ref<Provider | null>(null)

const form = ref({
  name: '',
  auth_mode: 'api_key',
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
  form.value = { name: '', auth_mode: 'api_key', base_url: '', api_key: '', model_id: '', is_default: false }
  editing.value = null
  showForm.value = false
}

function editProvider(p: Provider) {
  editing.value = p
  form.value = {
    name: p.name,
    auth_mode: p.auth_mode || 'api_key',
    base_url: p.base_url,
    api_key: p.api_key,
    model_id: p.model_id,
    is_default: p.is_default,
  }
  showForm.value = true
  if (p.auth_mode === 'oauth') loadAuthStatus()
}

watch(() => form.value.auth_mode, (mode) => {
  if (mode === 'oauth') loadAuthStatus()
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

onMounted(() => store.loadProviders())
</script>

<template>
  <div class="settings-page">
    <button class="floating-theme-btn" @click="toggleTheme" :title="'主题: ' + themeModeLabel[themeMode]">
      <svg v-if="themeMode === 'dark'" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M21 12.79A9 9 0 1111.21 3 7 7 0 0021 12.79z"/></svg>
      <svg v-else-if="themeMode === 'light'" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="5"/><path d="M12 1v2M12 21v2M4.22 4.22l1.42 1.42M18.36 18.36l1.42 1.42M1 12h2M21 12h2M4.22 19.78l1.42-1.42M18.36 5.64l1.42-1.42"/></svg>
      <svg v-else width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="2" y="3" width="20" height="14" rx="2"/><line x1="8" y1="21" x2="16" y2="21"/><line x1="12" y1="17" x2="12" y2="21"/></svg>
    </button>
    <div class="settings-container">
      <div class="settings-header">
        <button class="btn-back" @click="router.push('/chat')">
          <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M19 12H5M12 19l-7-7 7-7"/>
          </svg>
          返回
        </button>
        <h1>设置</h1>
      </div>

      <section class="section">
        <div class="section-header">
          <div>
            <h2>模型供应商</h2>
            <p class="section-desc">配置 LLM API 端点。Claude 模型自动通过 Claude Code CLI 路由。</p>
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
                <span class="badge mode">{{ p.mode === 'claude-code' ? 'Claude Code' : '直连 API' }}</span>
                <span v-if="p.auth_mode === 'oauth'" class="badge oauth">OAuth</span>
              </div>
              <div class="provider-meta">
                {{ p.model_id }}
                <span v-if="p.base_url" class="sep">·</span>
                <span v-if="p.base_url" class="url">{{ p.base_url }}</span>
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
              <label>模型 ID</label>
              <input v-model="form.model_id" placeholder="claude-sonnet-4-20250514 / qwen3-coder / glm-4.7" />
              <span class="hint">包含 `claude` 或使用 Ollama 端点时会自动通过 Claude Code CLI 路由。</span>
            </div>

            <div class="form-group checkbox">
              <label>
                <input type="checkbox" v-model="form.is_default" />
                设为默认供应商
              </label>
            </div>

            <div class="form-actions">
              <button class="btn-cancel" @click="resetForm">取消</button>
              <button class="btn-save" @click="saveProvider" :disabled="!form.name || !form.model_id || (needsApiKey && !form.api_key)">
                保存
              </button>
            </div>
          </div>
        </div>
      </section>
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
.settings-header { margin-bottom: 32px; }
.settings-header h1 { font-size: 24px; font-weight: 600; margin-top: 16px; }
.btn-back {
  display: flex; align-items: center; gap: 6px;
  color: var(--text-secondary); font-size: 13px; padding: 6px 0;
  transition: color var(--transition);
}
.btn-back:hover { color: var(--text-primary); }

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
.floating-theme-btn {
  position: fixed;
  top: 16px;
  right: 16px;
  width: 36px;
  height: 36px;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: var(--radius);
  background: var(--bg-secondary);
  border: 1px solid var(--border);
  color: var(--text-secondary);
  transition: all var(--transition);
  z-index: 50;
  cursor: pointer;
}
.floating-theme-btn:hover {
  background: var(--bg-hover);
  color: var(--text-primary);
}
@media (max-width: 768px) {
  .settings-container { padding: 16px 12px; }
  .form-modal { width: 100vw; max-width: 100vw; height: 100vh; height: 100dvh; max-height: 100vh; max-height: 100dvh; border-radius: 0; display: flex; flex-direction: column; }
  .form-modal h3 { margin-bottom: 12px; }
  .provider-card { flex-direction: column; align-items: flex-start; gap: 10px; }
  .provider-actions { margin-left: 0; width: 100%; justify-content: flex-end; }
}
</style>
