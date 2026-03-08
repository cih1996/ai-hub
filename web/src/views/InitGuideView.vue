<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { useRouter } from 'vue-router'

interface MissingDep {
  name: string
  description: string
  install_cmd?: string
  required: boolean
}

interface DepsStatus {
  node_installed: boolean
  node_version: string
  npm_installed: boolean
  npm_version: string
  claude_installed: boolean
  claude_version: string
  installing: boolean
  install_error: string
}

interface InitStatus {
  is_first_run: boolean
  has_provider: boolean
  has_session: boolean
  missing_deps: MissingDep[] | null
  deps_status: DepsStatus
}

interface Provider {
  id: string
  name: string
  base_url: string
  api_key: string
  model: string
  is_default: boolean
}

const router = useRouter()
const currentStep = ref(1)
const totalSteps = 5
const loading = ref(false)
const initStatus = ref<InitStatus | null>(null)
const installProgress = ref<{ [key: string]: { status: string; output: string } }>({})

// Provider form
const providerForm = ref<Provider>({
  id: '',
  name: 'Anthropic',
  base_url: 'https://api.anthropic.com',
  api_key: '',
  model: 'claude-3-5-sonnet-20240620',
  is_default: true
})

// Proxy settings
const proxyEnabled = ref(false)
const proxyUrl = ref('')

// Preset providers
const presetProviders = [
  { name: 'Anthropic', base_url: 'https://api.anthropic.com', model: 'claude-3-5-sonnet-20240620' },
  { name: 'OpenAI', base_url: 'https://api.openai.com/v1', model: 'gpt-4o' },
  { name: 'DeepSeek', base_url: 'https://api.deepseek.com', model: 'deepseek-chat' },
  { name: '自定义', base_url: '', model: '' }
]

const requiredDeps = computed(() =>
  initStatus.value?.missing_deps?.filter(d => d.required) || []
)

const optionalDeps = computed(() =>
  initStatus.value?.missing_deps?.filter(d => !d.required) || []
)

const allRequiredInstalled = computed(() => requiredDeps.value.length === 0)

onMounted(async () => {
  await checkInitStatus()
})

async function checkInitStatus() {
  loading.value = true
  try {
    const res = await fetch('/api/v1/system/init-status')
    initStatus.value = await res.json()
  } catch (e) {
    console.error('Failed to check init status:', e)
  } finally {
    loading.value = false
  }
}

async function installDep(dep: MissingDep) {
  if (!dep.install_cmd) return

  installProgress.value[dep.name] = { status: 'installing', output: '' }

  try {
    const res = await fetch('/api/v1/system/install-dep', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ name: dep.name, install_cmd: dep.install_cmd })
    })
    const result = await res.json()

    if (result.success) {
      installProgress.value[dep.name] = { status: 'success', output: result.output }
      await checkInitStatus()
    } else {
      installProgress.value[dep.name] = { status: 'error', output: result.error || result.output }
    }
  } catch (e) {
    installProgress.value[dep.name] = { status: 'error', output: String(e) }
  }
}

async function installAllDeps() {
  const deps = [...requiredDeps.value, ...optionalDeps.value]
  for (const dep of deps) {
    if (dep.install_cmd && !installProgress.value[dep.name]?.status) {
      await installDep(dep)
    }
  }
}

function selectPreset(preset: typeof presetProviders[0]) {
  providerForm.value.name = preset.name
  providerForm.value.base_url = preset.base_url
  providerForm.value.model = preset.model
}

async function saveProvider() {
  if (!providerForm.value.api_key) {
    alert('请输入 API Key')
    return
  }

  loading.value = true
  try {
    const res = await fetch('/api/v1/providers', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(providerForm.value)
    })

    if (res.ok) {
      nextStep()
    } else {
      const err = await res.json()
      alert('保存失败: ' + (err.error || '未知错误'))
    }
  } catch (e) {
    alert('保存失败: ' + e)
  } finally {
    loading.value = false
  }
}

async function saveProxy() {
  if (proxyEnabled.value && proxyUrl.value) {
    localStorage.setItem('ai-hub-proxy', proxyUrl.value)
  }
  nextStep()
}

function nextStep() {
  if (currentStep.value < totalSteps) {
    currentStep.value++
  }
}

function prevStep() {
  if (currentStep.value > 1) {
    currentStep.value--
  }
}

function finish() {
  localStorage.setItem('ai-hub-init-completed', 'true')
  router.push('/chat')
}

function skipGuide() {
  if (confirm('确定要跳过引导吗？您稍后可以在设置中进行配置。')) {
    localStorage.setItem('ai-hub-init-completed', 'true')
    router.push('/chat')
  }
}
</script>

<template>
  <div class="init-guide">
    <div class="guide-wrapper">
      <!-- Header / Progress -->
      <header class="guide-header">
        <div class="logo">
          <svg width="24" height="24" viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
            <path d="M12 2L2 7L12 12L22 7L12 2Z" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/>
            <path d="M2 17L12 22L22 17" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/>
            <path d="M2 12L12 17L22 12" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/>
          </svg>
          <span>AI Hub</span>
        </div>
        <div class="steps-count">
          <span class="current">{{ currentStep }}</span>
          <span class="divider">/</span>
          <span class="total">{{ totalSteps }}</span>
        </div>
      </header>

      <div class="guide-content">
        <!-- Step 1: Welcome -->
        <transition name="fade" mode="out-in">
          <div v-if="currentStep === 1" class="step-pane welcome-pane">
            <div class="hero-text">
              <h1>初始化您的<br/>智能中心</h1>
              <p class="subtitle">配置环境以释放 AI 智能体的潜力。</p>
            </div>

            <div class="env-status-card" v-if="initStatus">
              <div class="card-header">
                <h3>系统状态</h3>
                <span class="status-badge" :class="{ 'all-ok': initStatus.deps_status.node_installed && initStatus.deps_status.npm_installed }">
                  {{ initStatus.deps_status.node_installed && initStatus.deps_status.npm_installed ? '就绪' : '检测到问题' }}
                </span>
              </div>
              <div class="status-grid">
                <div class="status-item">
                  <span class="label">Node.js</span>
                  <span class="value">{{ initStatus.deps_status.node_version || '未安装' }}</span>
                  <div class="indicator" :class="{ active: initStatus.deps_status.node_installed }"></div>
                </div>
                <div class="status-item">
                  <span class="label">NPM</span>
                  <span class="value">{{ initStatus.deps_status.npm_version || '未安装' }}</span>
                  <div class="indicator" :class="{ active: initStatus.deps_status.npm_installed }"></div>
                </div>
                <div class="status-item">
                  <span class="label">Claude CLI</span>
                  <span class="value">{{ initStatus.deps_status.claude_version || '未安装' }}</span>
                  <div class="indicator" :class="{ active: initStatus.deps_status.claude_installed }"></div>
                </div>
              </div>
            </div>

            <div class="step-actions">
              <button class="btn-primary" @click="nextStep">开始配置</button>
              <button class="btn-text" @click="skipGuide">跳过设置</button>
            </div>
          </div>

          <!-- Step 2: Dependencies -->
          <div v-else-if="currentStep === 2" class="step-pane">
            <div class="pane-header">
              <h2>依赖项</h2>
              <p>安装所需工具以获得最佳性能。</p>
            </div>

            <div v-if="requiredDeps.length === 0 && optionalDeps.length === 0" class="empty-state">
              <div class="icon-circle">
                <svg width="24" height="24" viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
                  <path d="M20 6L9 17L4 12" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/>
                </svg>
              </div>
              <p>所有依赖项已安装。</p>
            </div>

            <div v-else class="deps-container">
              <div v-for="dep in [...requiredDeps, ...optionalDeps]" :key="dep.name" class="dep-card">
                <div class="dep-content">
                  <div class="dep-header">
                    <span class="dep-name">{{ dep.name }}</span>
                    <span v-if="dep.required" class="tag-required">必需</span>
                    <span v-else class="tag-optional">可选</span>
                  </div>
                  <p class="dep-desc">{{ dep.description }}</p>
                </div>
                <div class="dep-action">
                   <template v-if="installProgress[dep.name]">
                    <span v-if="installProgress[dep.name]?.status === 'installing'" class="status-text installing">安装中...</span>
                    <span v-else-if="installProgress[dep.name]?.status === 'success'" class="status-text success">已安装</span>
                    <span v-else class="status-text error">失败</span>
                  </template>
                  <button v-else class="btn-icon" @click="installDep(dep)" :disabled="!dep.install_cmd" title="安装">
                    <svg width="20" height="20" viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
                      <path d="M12 5V19M12 19L5 12M12 19L19 12" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/>
                    </svg>
                  </button>
                </div>
              </div>
              
              <button class="btn-secondary full-width" @click="installAllDeps">一键安装缺失项</button>
            </div>

            <div class="step-actions">
              <button class="btn-secondary" @click="prevStep">上一步</button>
              <button class="btn-primary" @click="nextStep">
                {{ allRequiredInstalled ? '下一步' : '跳过' }}
              </button>
            </div>
          </div>

          <!-- Step 3: Proxy -->
          <div v-else-if="currentStep === 3" class="step-pane">
            <div class="pane-header">
              <h2>网络代理</h2>
              <p>如果您在受限区域，请配置网络访问。</p>
            </div>

            <div class="proxy-card">
              <label class="toggle-row">
                <span>启用代理</span>
                <input type="checkbox" v-model="proxyEnabled" class="toggle-input" />
                <div class="toggle-switch"></div>
              </label>

              <div class="input-group" :class="{ disabled: !proxyEnabled }">
                <label>代理地址</label>
                <input
                  type="text"
                  v-model="proxyUrl"
                  placeholder="http://127.0.0.1:7890"
                  class="modern-input"
                  :disabled="!proxyEnabled"
                />
                <p class="input-hint">支持 HTTP 和 SOCKS5</p>
              </div>
            </div>

            <div class="step-actions">
              <button class="btn-secondary" @click="prevStep">上一步</button>
              <button class="btn-primary" @click="saveProxy">下一步</button>
            </div>
          </div>

          <!-- Step 4: Provider -->
          <div v-else-if="currentStep === 4" class="step-pane">
            <div class="pane-header">
              <h2>AI 服务商</h2>
              <p>选择并配置您的主要 AI 模型服务商。</p>
            </div>

            <div class="provider-grid">
              <button
                v-for="preset in presetProviders"
                :key="preset.name"
                class="provider-card"
                :class="{ active: providerForm.name === preset.name }"
                @click="selectPreset(preset)"
              >
                <span class="provider-name">{{ preset.name }}</span>
              </button>
            </div>

            <div class="form-stack">
              <div class="input-group">
                <label>API 基础地址</label>
                <input type="text" v-model="providerForm.base_url" class="modern-input" placeholder="https://api..." />
              </div>

              <div class="input-group">
                <label>API 密钥</label>
                <input type="password" v-model="providerForm.api_key" class="modern-input" placeholder="sk-..." />
              </div>

              <div class="input-group">
                <label>模型</label>
                <input type="text" v-model="providerForm.model" class="modern-input" placeholder="model-name" />
              </div>
            </div>

            <div class="step-actions">
              <button class="btn-secondary" @click="prevStep">上一步</button>
              <button class="btn-primary" @click="saveProvider" :disabled="loading">
                {{ loading ? '保存中...' : '保存并继续' }}
              </button>
            </div>
          </div>

          <!-- Step 5: Finish -->
          <div v-else-if="currentStep === 5" class="step-pane finish-pane">
            <div class="success-ring">
              <svg width="40" height="40" viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
                <path d="M20 6L9 17L4 12" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/>
              </svg>
            </div>
            <h2>设置完成</h2>
            <p class="subtitle">您的 AI Hub 已准备就绪。</p>

            <div class="quick-tips">
              <div class="tip-item">
                <span class="tip-icon">💬</span>
                <span>与 AI 智能体对话</span>
              </div>
              <div class="tip-item">
                <span class="tip-icon">🧩</span>
                <span>安装扩展</span>
              </div>
              <div class="tip-item">
                <span class="tip-icon">⚡</span>
                <span>自动化任务</span>
              </div>
            </div>

            <div class="step-actions">
              <button class="btn-primary full-width" @click="finish">进入 AI Hub</button>
            </div>
          </div>
        </transition>
      </div>
    </div>
  </div>
</template>

<style scoped>
.init-guide {
  min-height: 100vh;
  background-color: var(--bg-primary);
  color: var(--text-primary);
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 20px;
  font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif;
}

.guide-wrapper {
  width: 100%;
  max-width: 480px;
  display: flex;
  flex-direction: column;
  gap: 32px;
}

/* Header */
.guide-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 0 4px;
}

.logo {
  display: flex;
  align-items: center;
  gap: 8px;
  font-weight: 600;
  font-size: 16px;
  color: var(--text-primary);
}

.steps-count {
  font-family: monospace;
  font-size: 14px;
  color: var(--text-muted);
}

.steps-count .current {
  color: var(--text-primary);
  font-weight: 600;
}

/* Content Area */
.guide-content {
  position: relative;
  min-height: 400px;
}

.step-pane {
  display: flex;
  flex-direction: column;
  gap: 24px;
}

.welcome-pane {
  text-align: center;
  padding-top: 20px;
}

.finish-pane {
  text-align: center;
  padding-top: 40px;
  align-items: center;
}

.hero-text h1 {
  font-size: 32px;
  line-height: 1.2;
  font-weight: 700;
  margin-bottom: 12px;
  letter-spacing: -0.5px;
}

.pane-header h2 {
  font-size: 24px;
  font-weight: 600;
  margin-bottom: 8px;
}

.pane-header p, .subtitle {
  color: var(--text-secondary);
  font-size: 15px;
  line-height: 1.5;
}

/* Status Card */
.env-status-card {
  background: var(--bg-secondary);
  border: 1px solid var(--border);
  border-radius: 16px;
  padding: 20px;
  text-align: left;
}

.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 16px;
}

.card-header h3 {
  font-size: 14px;
  font-weight: 600;
  color: var(--text-secondary);
  text-transform: uppercase;
  letter-spacing: 0.5px;
}

.status-badge {
  font-size: 12px;
  padding: 4px 8px;
  border-radius: 100px;
  background: var(--bg-tertiary);
  color: var(--text-muted);
}

.status-badge.all-ok {
  background: var(--success);
  color: #fff;
}

.status-grid {
  display: grid;
  gap: 12px;
}

.status-item {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 12px;
  background: var(--bg-primary);
  border-radius: 8px;
}

.status-item .label {
  font-size: 14px;
  font-weight: 500;
}

.status-item .value {
  font-size: 13px;
  color: var(--text-secondary);
  font-family: monospace;
}

.indicator {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: var(--danger);
}

.indicator.active {
  background: var(--success);
}

/* Dependencies */
.deps-container {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.dep-card {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 16px;
  background: var(--bg-secondary);
  border: 1px solid var(--border);
  border-radius: 12px;
  transition: border-color 0.2s;
}

.dep-card:hover {
  border-color: var(--text-muted);
}

.dep-content {
  flex: 1;
  min-width: 0;
  margin-right: 16px;
}

.dep-header {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 4px;
}

.dep-name {
  font-weight: 600;
  font-size: 15px;
}

.tag-required, .tag-optional {
  font-size: 10px;
  padding: 2px 6px;
  border-radius: 4px;
  text-transform: uppercase;
  font-weight: 600;
}

.tag-required {
  background: var(--bg-tertiary);
  color: var(--text-primary);
}

.tag-optional {
  background: transparent;
  border: 1px solid var(--border);
  color: var(--text-muted);
}

.dep-desc {
  font-size: 13px;
  color: var(--text-secondary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.btn-icon {
  width: 36px;
  height: 36px;
  border-radius: 50%;
  border: 1px solid var(--border);
  background: var(--bg-tertiary);
  color: var(--text-primary);
  display: flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  transition: all 0.2s;
}

.btn-icon:hover:not(:disabled) {
  background: var(--text-primary);
  color: var(--bg-primary);
  border-color: var(--text-primary);
}

.btn-icon:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.status-text {
  font-size: 12px;
  font-weight: 500;
}

.status-text.installing { color: var(--accent); }
.status-text.success { color: var(--success); }
.status-text.error { color: var(--danger); }

/* Proxy & Forms */
.proxy-card {
  background: var(--bg-secondary);
  border: 1px solid var(--border);
  border-radius: 16px;
  padding: 20px;
}

.toggle-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  cursor: pointer;
  margin-bottom: 20px;
}

.toggle-input {
  display: none;
}

.toggle-switch {
  width: 44px;
  height: 24px;
  background: var(--bg-tertiary);
  border-radius: 12px;
  position: relative;
  transition: background 0.2s;
}

.toggle-switch::after {
  content: '';
  position: absolute;
  top: 2px;
  left: 2px;
  width: 20px;
  height: 20px;
  background: white;
  border-radius: 50%;
  transition: transform 0.2s;
  box-shadow: 0 1px 2px rgba(0,0,0,0.1);
}

.toggle-input:checked + .toggle-switch {
  background: var(--text-primary);
}

.toggle-input:checked + .toggle-switch::after {
  transform: translateX(20px);
}

.input-group {
  display: flex;
  flex-direction: column;
  gap: 8px;
  transition: opacity 0.2s;
}

.input-group.disabled {
  opacity: 0.5;
  pointer-events: none;
}

.input-group label {
  font-size: 13px;
  font-weight: 500;
  color: var(--text-secondary);
}

.modern-input {
  width: 100%;
  padding: 12px 16px;
  background: var(--bg-primary);
  border: 1px solid var(--border);
  border-radius: 8px;
  font-size: 14px;
  color: var(--text-primary);
  transition: all 0.2s;
}

.modern-input:focus {
  border-color: var(--text-primary);
  outline: none;
  box-shadow: 0 0 0 2px var(--bg-tertiary);
}

.input-hint {
  font-size: 12px;
  color: var(--text-muted);
}

/* Provider Grid */
.provider-grid {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 12px;
}

.provider-card {
  padding: 16px;
  background: var(--bg-secondary);
  border: 1px solid var(--border);
  border-radius: 12px;
  cursor: pointer;
  text-align: center;
  transition: all 0.2s;
}

.provider-card:hover {
  border-color: var(--text-muted);
}

.provider-card.active {
  background: var(--text-primary);
  color: var(--bg-primary);
  border-color: var(--text-primary);
}

.form-stack {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

/* Finish Pane */
.success-ring {
  width: 80px;
  height: 80px;
  border-radius: 50%;
  border: 2px solid var(--border);
  display: flex;
  align-items: center;
  justify-content: center;
  margin-bottom: 24px;
  color: var(--success);
}

.quick-tips {
  display: flex;
  flex-direction: column;
  gap: 12px;
  width: 100%;
  margin: 24px 0;
}

.tip-item {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 12px;
  background: var(--bg-secondary);
  border-radius: 8px;
  font-size: 14px;
}

.tip-icon {
  font-size: 18px;
}

/* Actions */
.step-actions {
  display: flex;
  gap: 12px;
  margin-top: auto;
  padding-top: 20px;
}

.btn-primary, .btn-secondary, .btn-text {
  font-size: 14px;
  font-weight: 500;
  padding: 12px 24px;
  border-radius: 100px;
  cursor: pointer;
  transition: all 0.2s;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  height: 48px;
}

.btn-primary {
  background: var(--text-primary);
  color: var(--bg-primary);
  border: 1px solid transparent;
  flex: 2;
}

.btn-primary:hover {
  opacity: 0.9;
  transform: translateY(-1px);
}

.btn-primary:disabled {
  opacity: 0.5;
  cursor: not-allowed;
  transform: none;
}

.btn-secondary {
  background: transparent;
  border: 1px solid var(--border);
  color: var(--text-primary);
  flex: 1;
}

.btn-secondary:hover {
  background: var(--bg-tertiary);
}

.btn-text {
  background: transparent;
  color: var(--text-secondary);
  padding: 12px;
}

.btn-text:hover {
  color: var(--text-primary);
}

.full-width {
  width: 100%;
  flex: 1;
}

.empty-state {
  text-align: center;
  padding: 40px 0;
  color: var(--text-secondary);
}

.icon-circle {
  width: 48px;
  height: 48px;
  background: var(--bg-secondary);
  border-radius: 50%;
  display: flex;
  align-items: center;
  justify-content: center;
  margin: 0 auto 16px;
}

/* Animations */
.fade-enter-active,
.fade-leave-active {
  transition: opacity 0.3s ease, transform 0.3s ease;
}

.fade-enter-from {
  opacity: 0;
  transform: translateY(10px);
}

.fade-leave-to {
  opacity: 0;
  transform: translateY(-10px);
}

/* Mobile Responsiveness */
@media (max-width: 600px) {
  .init-guide {
    padding: 16px;
    align-items: flex-start;
  }
  
  .guide-wrapper {
    gap: 24px;
    padding-top: 20px;
  }

  .hero-text h1 {
    font-size: 28px;
  }

  .step-actions {
    position: fixed;
    bottom: 0;
    left: 0;
    width: 100%;
    padding: 16px;
    padding-bottom: calc(16px + env(safe-area-inset-bottom));
    background: var(--bg-primary);
    border-top: 1px solid var(--border);
    z-index: 100;
    flex-direction: row-reverse; /* Put primary action on right */
    box-shadow: 0 -4px 20px rgba(0,0,0,0.1);
  }

  .guide-content {
    padding-bottom: 120px; /* Space for fixed actions */
  }
  
  .form-stack {
    margin-bottom: 32px;
  }
  
  .provider-grid {
    grid-template-columns: 1fr;
  }
}
</style>
