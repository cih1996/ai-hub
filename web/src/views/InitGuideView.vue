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
  model: 'claude-sonnet-4-20250514',
  is_default: true
})

// Proxy settings
const proxyEnabled = ref(false)
const proxyUrl = ref('')

// Preset providers
const presetProviders = [
  { name: 'Anthropic', base_url: 'https://api.anthropic.com', model: 'claude-sonnet-4-20250514' },
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
      // Refresh status
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
    // Save proxy to localStorage for now
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
  if (confirm('确定跳过引导？您可以稍后在设置中配置。')) {
    localStorage.setItem('ai-hub-init-completed', 'true')
    router.push('/chat')
  }
}
</script>

<template>
  <div class="init-guide">
    <div class="guide-container">
      <!-- Progress bar -->
      <div class="progress-bar">
        <div class="progress-track">
          <div class="progress-fill" :style="{ width: `${(currentStep / totalSteps) * 100}%` }"></div>
        </div>
        <div class="step-indicators">
          <div
            v-for="step in totalSteps"
            :key="step"
            class="step-dot"
            :class="{ active: step <= currentStep, current: step === currentStep }"
          >
            {{ step }}
          </div>
        </div>
      </div>

      <!-- Step 1: Welcome -->
      <div v-if="currentStep === 1" class="step-content">
        <div class="welcome-icon">🚀</div>
        <h1>欢迎使用 AI Hub</h1>
        <p class="subtitle">智能体管理中心，让 AI 为你工作</p>

        <div class="env-check" v-if="initStatus">
          <h3>环境检测</h3>
          <div class="check-list">
            <div class="check-item" :class="{ ok: initStatus.deps_status.node_installed }">
              <span class="icon">{{ initStatus.deps_status.node_installed ? '✅' : '❌' }}</span>
              <span>Node.js {{ initStatus.deps_status.node_version || '未安装' }}</span>
            </div>
            <div class="check-item" :class="{ ok: initStatus.deps_status.npm_installed }">
              <span class="icon">{{ initStatus.deps_status.npm_installed ? '✅' : '❌' }}</span>
              <span>npm {{ initStatus.deps_status.npm_version || '未安装' }}</span>
            </div>
            <div class="check-item" :class="{ ok: initStatus.deps_status.claude_installed }">
              <span class="icon">{{ initStatus.deps_status.claude_installed ? '✅' : '❌' }}</span>
              <span>Claude CLI {{ initStatus.deps_status.claude_version || '未安装' }}</span>
            </div>
          </div>
        </div>

        <div class="actions">
          <button class="btn-primary" @click="nextStep">开始配置</button>
          <button class="btn-text" @click="skipGuide">跳过引导</button>
        </div>
      </div>

      <!-- Step 2: Install Dependencies -->
      <div v-if="currentStep === 2" class="step-content">
        <h2>安装依赖</h2>
        <p class="subtitle">检测到以下依赖需要安装</p>

        <div v-if="requiredDeps.length === 0 && optionalDeps.length === 0" class="all-good">
          <div class="icon">✅</div>
          <p>所有依赖已就绪！</p>
        </div>

        <div v-else class="deps-list">
          <div v-if="requiredDeps.length > 0" class="deps-section">
            <h4>必需依赖</h4>
            <div v-for="dep in requiredDeps" :key="dep.name" class="dep-item">
              <div class="dep-info">
                <span class="dep-name">{{ dep.name }}</span>
                <span class="dep-desc">{{ dep.description }}</span>
              </div>
              <div class="dep-action">
                <template v-if="installProgress[dep.name]">
                  <span v-if="installProgress[dep.name]?.status === 'installing'" class="status installing">安装中...</span>
                  <span v-else-if="installProgress[dep.name]?.status === 'success'" class="status success">✅ 已安装</span>
                  <span v-else class="status error">❌ 失败</span>
                </template>
                <button v-else class="btn-small" @click="installDep(dep)" :disabled="!dep.install_cmd">
                  安装
                </button>
              </div>
            </div>
          </div>

          <div v-if="optionalDeps.length > 0" class="deps-section">
            <h4>可选依赖</h4>
            <div v-for="dep in optionalDeps" :key="dep.name" class="dep-item">
              <div class="dep-info">
                <span class="dep-name">{{ dep.name }}</span>
                <span class="dep-desc">{{ dep.description }}</span>
              </div>
              <div class="dep-action">
                <template v-if="installProgress[dep.name]">
                  <span v-if="installProgress[dep.name]?.status === 'installing'" class="status installing">安装中...</span>
                  <span v-else-if="installProgress[dep.name]?.status === 'success'" class="status success">✅ 已安装</span>
                  <span v-else class="status error">❌ 失败</span>
                </template>
                <button v-else class="btn-small btn-secondary" @click="installDep(dep)" :disabled="!dep.install_cmd">
                  安装
                </button>
              </div>
            </div>
          </div>

          <button class="btn-outline" @click="installAllDeps">一键安装全部</button>
        </div>

        <div class="actions">
          <button class="btn-secondary" @click="prevStep">上一步</button>
          <button class="btn-primary" @click="nextStep">
            {{ allRequiredInstalled ? '下一步' : '跳过' }}
          </button>
        </div>
      </div>

      <!-- Step 3: Proxy Settings -->
      <div v-if="currentStep === 3" class="step-content">
        <h2>代理设置</h2>
        <p class="subtitle">如果您在中国大陆，可能需要配置代理访问 API</p>

        <div class="form-group">
          <label class="checkbox-label">
            <input type="checkbox" v-model="proxyEnabled" />
            <span>启用代理</span>
          </label>
        </div>

        <div v-if="proxyEnabled" class="form-group">
          <label>代理地址</label>
          <input
            type="text"
            v-model="proxyUrl"
            placeholder="http://127.0.0.1:7890"
            class="input"
          />
          <p class="hint">支持 HTTP/SOCKS5 代理，如 Clash、V2Ray 等</p>
        </div>

        <div class="actions">
          <button class="btn-secondary" @click="prevStep">上一步</button>
          <button class="btn-primary" @click="saveProxy">下一步</button>
        </div>
      </div>

      <!-- Step 4: Configure Provider -->
      <div v-if="currentStep === 4" class="step-content">
        <h2>配置 API 供应商</h2>
        <p class="subtitle">选择您的 AI 服务提供商并填写 API Key</p>

        <div class="preset-list">
          <button
            v-for="preset in presetProviders"
            :key="preset.name"
            class="preset-btn"
            :class="{ active: providerForm.name === preset.name }"
            @click="selectPreset(preset)"
          >
            {{ preset.name }}
          </button>
        </div>

        <div class="form-group">
          <label>名称</label>
          <input type="text" v-model="providerForm.name" class="input" />
        </div>

        <div class="form-group">
          <label>API Base URL</label>
          <input type="text" v-model="providerForm.base_url" class="input" placeholder="https://api.anthropic.com" />
        </div>

        <div class="form-group">
          <label>API Key <span class="required">*</span></label>
          <input type="password" v-model="providerForm.api_key" class="input" placeholder="sk-..." />
        </div>

        <div class="form-group">
          <label>模型</label>
          <input type="text" v-model="providerForm.model" class="input" placeholder="claude-sonnet-4-20250514" />
        </div>

        <div class="actions">
          <button class="btn-secondary" @click="prevStep">上一步</button>
          <button class="btn-primary" @click="saveProvider" :disabled="loading">
            {{ loading ? '保存中...' : '保存并继续' }}
          </button>
        </div>
      </div>

      <!-- Step 5: Complete -->
      <div v-if="currentStep === 5" class="step-content">
        <div class="complete-icon">🎉</div>
        <h1>配置完成！</h1>
        <p class="subtitle">AI Hub 已准备就绪，开始您的智能体之旅吧</p>

        <div class="tips">
          <h4>快速上手</h4>
          <ul>
            <li>💬 在聊天界面与 AI 对话</li>
            <li>⚙️ 在设置中管理供应商和规则</li>
            <li>🔧 在扩展中启用 Skills 和 MCP</li>
            <li>📊 在自动化中配置定时任务</li>
          </ul>
        </div>

        <div class="actions">
          <button class="btn-primary btn-large" @click="finish">进入 AI Hub</button>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.init-guide {
  min-height: 100vh;
  background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 20px;
}

.guide-container {
  background: white;
  border-radius: 16px;
  padding: 40px;
  max-width: 600px;
  width: 100%;
  box-shadow: 0 20px 60px rgba(0, 0, 0, 0.3);
}

.progress-bar {
  margin-bottom: 40px;
}

.progress-track {
  height: 4px;
  background: #e0e0e0;
  border-radius: 2px;
  overflow: hidden;
}

.progress-fill {
  height: 100%;
  background: linear-gradient(90deg, #667eea, #764ba2);
  transition: width 0.3s ease;
}

.step-indicators {
  display: flex;
  justify-content: space-between;
  margin-top: 12px;
}

.step-dot {
  width: 28px;
  height: 28px;
  border-radius: 50%;
  background: #e0e0e0;
  color: #999;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 12px;
  font-weight: 600;
  transition: all 0.3s ease;
}

.step-dot.active {
  background: #667eea;
  color: white;
}

.step-dot.current {
  transform: scale(1.2);
  box-shadow: 0 0 0 4px rgba(102, 126, 234, 0.3);
}

.step-content {
  text-align: center;
}

.welcome-icon, .complete-icon {
  font-size: 64px;
  margin-bottom: 20px;
}

h1 {
  font-size: 28px;
  color: #333;
  margin-bottom: 8px;
}

h2 {
  font-size: 24px;
  color: #333;
  margin-bottom: 8px;
}

.subtitle {
  color: #666;
  margin-bottom: 30px;
}

.env-check {
  background: #f8f9fa;
  border-radius: 12px;
  padding: 20px;
  margin-bottom: 30px;
  text-align: left;
}

.env-check h3 {
  font-size: 16px;
  margin-bottom: 16px;
  color: #333;
}

.check-list {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.check-item {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 8px 12px;
  background: white;
  border-radius: 8px;
}

.check-item .icon {
  font-size: 18px;
}

.actions {
  display: flex;
  gap: 12px;
  justify-content: center;
  margin-top: 30px;
}

.btn-primary {
  background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
  color: white;
  border: none;
  padding: 12px 32px;
  border-radius: 8px;
  font-size: 16px;
  font-weight: 600;
  cursor: pointer;
  transition: transform 0.2s, box-shadow 0.2s;
}

.btn-primary:hover {
  transform: translateY(-2px);
  box-shadow: 0 4px 12px rgba(102, 126, 234, 0.4);
}

.btn-primary:disabled {
  opacity: 0.6;
  cursor: not-allowed;
  transform: none;
}

.btn-secondary {
  background: #f0f0f0;
  color: #333;
  border: none;
  padding: 12px 24px;
  border-radius: 8px;
  font-size: 14px;
  cursor: pointer;
}

.btn-text {
  background: none;
  border: none;
  color: #666;
  cursor: pointer;
  font-size: 14px;
}

.btn-text:hover {
  color: #333;
}

.btn-outline {
  background: none;
  border: 2px solid #667eea;
  color: #667eea;
  padding: 10px 20px;
  border-radius: 8px;
  cursor: pointer;
  margin-top: 20px;
}

.btn-small {
  padding: 6px 16px;
  font-size: 13px;
  border-radius: 6px;
  border: none;
  background: #667eea;
  color: white;
  cursor: pointer;
}

.btn-small.btn-secondary {
  background: #e0e0e0;
  color: #333;
}

.btn-large {
  padding: 16px 48px;
  font-size: 18px;
}

/* Dependencies */
.all-good {
  padding: 40px;
}

.all-good .icon {
  font-size: 48px;
  margin-bottom: 16px;
}

.deps-list {
  text-align: left;
}

.deps-section {
  margin-bottom: 24px;
}

.deps-section h4 {
  font-size: 14px;
  color: #666;
  margin-bottom: 12px;
}

.dep-item {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 12px 16px;
  background: #f8f9fa;
  border-radius: 8px;
  margin-bottom: 8px;
}

.dep-info {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.dep-name {
  font-weight: 600;
  color: #333;
}

.dep-desc {
  font-size: 12px;
  color: #666;
}

.status {
  font-size: 13px;
}

.status.installing {
  color: #667eea;
}

.status.success {
  color: #28a745;
}

.status.error {
  color: #dc3545;
}

/* Form */
.form-group {
  margin-bottom: 20px;
  text-align: left;
}

.form-group label {
  display: block;
  font-size: 14px;
  font-weight: 600;
  color: #333;
  margin-bottom: 8px;
}

.input {
  width: 100%;
  padding: 12px 16px;
  border: 2px solid #e0e0e0;
  border-radius: 8px;
  font-size: 14px;
  transition: border-color 0.2s;
  box-sizing: border-box;
}

.input:focus {
  outline: none;
  border-color: #667eea;
}

.hint {
  font-size: 12px;
  color: #666;
  margin-top: 6px;
}

.required {
  color: #dc3545;
}

.checkbox-label {
  display: flex;
  align-items: center;
  gap: 8px;
  cursor: pointer;
}

.checkbox-label input {
  width: 18px;
  height: 18px;
}

/* Presets */
.preset-list {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
  margin-bottom: 24px;
  justify-content: center;
}

.preset-btn {
  padding: 8px 16px;
  border: 2px solid #e0e0e0;
  border-radius: 20px;
  background: white;
  cursor: pointer;
  transition: all 0.2s;
}

.preset-btn:hover {
  border-color: #667eea;
}

.preset-btn.active {
  background: #667eea;
  border-color: #667eea;
  color: white;
}

/* Tips */
.tips {
  background: #f8f9fa;
  border-radius: 12px;
  padding: 20px;
  margin: 30px 0;
  text-align: left;
}

.tips h4 {
  margin-bottom: 12px;
  color: #333;
}

.tips ul {
  list-style: none;
  padding: 0;
  margin: 0;
}

.tips li {
  padding: 8px 0;
  color: #555;
}
</style>
