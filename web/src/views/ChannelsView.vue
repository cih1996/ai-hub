<script setup lang="ts">
import { ref, reactive, onMounted } from 'vue'
import { listChannels, createChannel, updateChannel, deleteChannel, listSessions, sendChat } from '../composables/api'
import type { Channel, Session } from '../types'

const channels = ref<Channel[]>([])
const sessions = ref<Session[]>([])
const loading = ref(false)
const showCreate = ref(false)
const editTarget = ref<Channel | null>(null)
const deleteTarget = ref<Channel | null>(null)
const deploying = ref(false)
const deployResult = ref('')
const showSmartCreate = ref(false)
const smartDesc = ref('')
const smartCreating = ref(false)
const smartResult = ref('')

const form = ref({ name: '', platform: 'feishu', session_id: 0, config: '{}' })

const configFields = reactive({
  feishu: { app_id: '', app_secret: '', verification_token: '' },
  telegram: { bot_token: '' },
  qq: { napcat_http_url: '', napcat_ws_url: '', token: '' },
})

function parseConfigToFields(platform: string, config: string) {
  try {
    const obj = JSON.parse(config)
    if (platform === 'feishu') {
      configFields.feishu.app_id = obj.app_id || ''
      configFields.feishu.app_secret = obj.app_secret || ''
      configFields.feishu.verification_token = obj.verification_token || ''
    } else if (platform === 'telegram') {
      configFields.telegram.bot_token = obj.bot_token || ''
    } else if (platform === 'qq') {
      configFields.qq.napcat_http_url = obj.napcat_http_url || ''
      configFields.qq.napcat_ws_url = obj.napcat_ws_url || ''
      configFields.qq.token = obj.token || ''
    }
  } catch {
    // reset on parse error
    resetConfigFields(platform)
  }
}

function resetConfigFields(platform?: string) {
  if (!platform || platform === 'feishu') {
    configFields.feishu = { app_id: '', app_secret: '', verification_token: '' }
  }
  if (!platform || platform === 'telegram') {
    configFields.telegram = { bot_token: '' }
  }
  if (!platform || platform === 'qq') {
    configFields.qq = { napcat_http_url: '', napcat_ws_url: '', token: '' }
  }
}

function serializeFieldsToConfig(platform: string): string {
  if (platform === 'feishu') return JSON.stringify(configFields.feishu)
  if (platform === 'telegram') return JSON.stringify(configFields.telegram)
  if (platform === 'qq') return JSON.stringify(configFields.qq)
  return '{}'
}

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
  resetConfigFields()
  showCreate.value = true
}

async function onCreate() {
  if (!form.value.name || !form.value.platform) return
  form.value.config = serializeFieldsToConfig(form.value.platform)
  await createChannel(form.value)
  showCreate.value = false
  load()
}

function openEdit(ch: Channel) {
  editTarget.value = ch
  form.value = { name: ch.name, platform: ch.platform, session_id: ch.session_id, config: ch.config }
  resetConfigFields()
  parseConfigToFields(ch.platform, ch.config)
}

async function onEdit() {
  if (!editTarget.value) return
  form.value.config = serializeFieldsToConfig(form.value.platform)
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

async function onDeploy() {
  if (!form.value.name) { deployResult.value = '请先填写频道名称'; return }
  deploying.value = true
  deployResult.value = ''
  try {
    const content = `请读取 ~/.ai-hub/skills/feishu-deploy/SKILL.md 获取飞书应用部署流程，然后按流程执行。

部署参数：
- 应用名称：${form.value.name}
- 应用描述：AI Hub 智能助手

部署完成后请输出 App ID 和 App Secret。`
    const res = await sendChat(0, content)
    deployResult.value = `已创建开通会话 #${res.session_id}，AI 正在自动开通飞书应用`
  } catch (e: any) {
    deployResult.value = `创建失败: ${e.message}`
  }
  deploying.value = false
}

async function onDeployQQ() {
  deploying.value = true
  deployResult.value = ''
  try {
    const rules = `你是 QQ 机器人部署助手。请阅读 ~/.ai-hub/skills/qq-deploy/SKILL.md 获取完整部署手册。

关键要求：
- 用户是非技术人员，全程用简单易懂的语言引导
- 每一步只给一个操作，等用户确认后再继续
- 遇到下载慢或失败，自动切换国内镜像源
- 主动检测用户本地代理环境
- 部署完成后输出 NapCat HTTP 地址、WebSocket 地址和 Token`
    const res = await sendChat(0, '请引导我部署 QQ 机器人（NapCat），我不太懂技术，请一步步告诉我该怎么做。', '', rules)
    deployResult.value = `已创建部署会话 #${res.session_id}，AI 正在引导部署 QQ 机器人`
  } catch (e: any) {
    deployResult.value = `创建失败: ${e.message}`
  }
  deploying.value = false
}

async function onSmartCreate() {
  if (!smartDesc.value.trim()) return
  smartCreating.value = true
  smartResult.value = ''
  try {
    // 1. Build session rules based on user description
    const port = location.port || (location.protocol === 'https:' ? '443' : '80')
    const rules = `# 飞书消息处理助手

## 角色
${smartDesc.value.trim()}

## 消息处理流程
1. 收到【飞书消息】后，解析发送者、会话ID、消息内容
2. 根据角色定位理解意图并生成回复
3. 通过飞书 API 回复（参考 ~/.ai-hub/skills/feishu-message/SKILL.md）
4. 飞书凭证（app_id、app_secret）从消息中的「频道凭证」部分获取

## 会话间通信协议
- 需要其他会话协助时：POST http://localhost:${port}/api/v1/chat/send
- 派发消息中必须注明自己的会话ID，要求对方处理完后通过同一 API 回复
- 严禁轮询或读取其他会话的消息记录
- 派发后等待对方主动回复，收到后再通过飞书 API 回复用户

## 回复规范
- 内容简洁友好，适合 IM 阅读
- 每条消息必须回复，不能丢失`

    // 2. Create session with rules in one atomic call
    const res = await sendChat(0, '你已就绪。之后收到的每条消息都是飞书用户消息，请按会话规则处理并通过飞书 API 回复。', '', rules)

    // 3. Bind session to form
    form.value.session_id = res.session_id
    smartResult.value = `已创建会话 #${res.session_id}`
    // Refresh sessions so the new one appears in dropdown
    const s = await listSessions()
    sessions.value = s
    showSmartCreate.value = false
    smartDesc.value = ''
  } catch (e: any) {
    smartResult.value = `创建失败: ${e.message}`
  }
  smartCreating.value = false
}
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
            <div class="session-row">
              <select v-model.number="form.session_id">
                <option :value="0">不绑定</option>
                <option v-for="s in sessions" :key="s.id" :value="s.id">#{{ s.id }} {{ s.title }}</option>
              </select>
              <button class="btn-smart" @click="showSmartCreate = !showSmartCreate" type="button">
                <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M12 2l3.09 6.26L22 9.27l-5 4.87 1.18 6.88L12 17.77l-6.18 3.25L7 14.14 2 9.27l6.91-1.01L12 2z"/></svg>
                智能创建
              </button>
            </div>
            <div v-if="showSmartCreate" class="smart-create-area">
              <textarea v-model="smartDesc" rows="3" placeholder="描述一下你希望 AI 怎么处理飞书消息，比如：自动回答产品相关问题、充当客服角色等"></textarea>
              <div class="smart-create-actions">
                <button class="btn-smart-go" :disabled="smartCreating || !smartDesc.trim()" @click="onSmartCreate" type="button">
                  <svg v-if="smartCreating" class="spinning" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M21 12a9 9 0 11-6.219-8.56"/></svg>
                  {{ smartCreating ? '正在创建...' : '创建会话' }}
                </button>
                <span v-if="smartResult" class="smart-result">{{ smartResult }}</span>
              </div>
            </div>
          </div>
          <div class="form-group">
            <label>平台配置</label>
            <div v-if="form.platform === 'feishu'" class="config-fields">
              <input v-model="configFields.feishu.app_id" placeholder="App ID" />
              <input v-model="configFields.feishu.app_secret" placeholder="App Secret" />
              <input v-model="configFields.feishu.verification_token" placeholder="Verification Token" />
            </div>
            <div v-else-if="form.platform === 'telegram'" class="config-fields">
              <input v-model="configFields.telegram.bot_token" placeholder="Bot Token" />
            </div>
            <div v-else-if="form.platform === 'qq'" class="config-fields">
              <input v-model="configFields.qq.napcat_http_url" placeholder="NapCat HTTP 地址（发消息用，如 http://129.204.22.176:3055）" />
              <input v-model="configFields.qq.napcat_ws_url" placeholder="NapCat WebSocket 地址（收消息用，如 ws://129.204.22.176:3056）" />
              <input v-model="configFields.qq.token" placeholder="Token（可选，HTTP 和 WS 共用）" />
            </div>
          </div>
          <div v-if="form.platform === 'feishu'" class="deploy-section">
            <button class="btn-deploy" :disabled="deploying" @click="onDeploy">
              <svg v-if="!deploying" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="3" y="11" width="18" height="11" rx="2"/><path d="M7 11V7a5 5 0 0110 0v4"/><circle cx="12" cy="16" r="1"/></svg>
              <svg v-else class="spinning" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M21 12a9 9 0 11-6.219-8.56"/></svg>
              {{ deploying ? '正在创建开通任务...' : '一键开通飞书应用' }}
            </button>
            <span v-if="deployResult" class="deploy-result">{{ deployResult }}</span>
          </div>
          <div v-if="form.platform === 'qq'" class="deploy-section">
            <button class="btn-deploy" :disabled="deploying" @click="onDeployQQ">
              <svg v-if="!deploying" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M12 2a7 7 0 017 7c0 3-2 5-3 7l1 4H7l1-4c-1-2-3-4-3-7a7 7 0 017-7z"/><path d="M9 22h6"/></svg>
              <svg v-else class="spinning" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M21 12a9 9 0 11-6.219-8.56"/></svg>
              {{ deploying ? '正在创建部署任务...' : '一键部署 QQ 机器人' }}
            </button>
            <span v-if="deployResult" class="deploy-result">{{ deployResult }}</span>
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
            <div class="session-row">
              <select v-model.number="form.session_id">
                <option :value="0">不绑定</option>
                <option v-for="s in sessions" :key="s.id" :value="s.id">#{{ s.id }} {{ s.title }}</option>
              </select>
              <button class="btn-smart" @click="showSmartCreate = !showSmartCreate" type="button">
                <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M12 2l3.09 6.26L22 9.27l-5 4.87 1.18 6.88L12 17.77l-6.18 3.25L7 14.14 2 9.27l6.91-1.01L12 2z"/></svg>
                智能创建
              </button>
            </div>
            <div v-if="showSmartCreate" class="smart-create-area">
              <textarea v-model="smartDesc" rows="3" placeholder="描述一下你希望 AI 怎么处理飞书消息，比如：自动回答产品相关问题、充当客服角色等"></textarea>
              <div class="smart-create-actions">
                <button class="btn-smart-go" :disabled="smartCreating || !smartDesc.trim()" @click="onSmartCreate" type="button">
                  <svg v-if="smartCreating" class="spinning" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M21 12a9 9 0 11-6.219-8.56"/></svg>
                  {{ smartCreating ? '正在创建...' : '创建会话' }}
                </button>
                <span v-if="smartResult" class="smart-result">{{ smartResult }}</span>
              </div>
            </div>
          </div>
          <div class="form-group">
            <label>平台配置</label>
            <div v-if="form.platform === 'feishu'" class="config-fields">
              <input v-model="configFields.feishu.app_id" placeholder="App ID" />
              <input v-model="configFields.feishu.app_secret" placeholder="App Secret" />
              <input v-model="configFields.feishu.verification_token" placeholder="Verification Token" />
            </div>
            <div v-else-if="form.platform === 'telegram'" class="config-fields">
              <input v-model="configFields.telegram.bot_token" placeholder="Bot Token" />
            </div>
            <div v-else-if="form.platform === 'qq'" class="config-fields">
              <input v-model="configFields.qq.napcat_http_url" placeholder="NapCat HTTP 地址（发消息用，如 http://129.204.22.176:3055）" />
              <input v-model="configFields.qq.napcat_ws_url" placeholder="NapCat WebSocket 地址（收消息用，如 ws://129.204.22.176:3056）" />
              <input v-model="configFields.qq.token" placeholder="Token（可选，HTTP 和 WS 共用）" />
            </div>
          </div>
          <div v-if="form.platform === 'feishu'" class="deploy-section">
            <button class="btn-deploy" :disabled="deploying" @click="onDeploy">
              <svg v-if="!deploying" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="3" y="11" width="18" height="11" rx="2"/><path d="M7 11V7a5 5 0 0110 0v4"/><circle cx="12" cy="16" r="1"/></svg>
              <svg v-else class="spinning" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M21 12a9 9 0 11-6.219-8.56"/></svg>
              {{ deploying ? '正在创建开通任务...' : '一键开通飞书应用' }}
            </button>
            <span v-if="deployResult" class="deploy-result">{{ deployResult }}</span>
          </div>
          <div v-if="form.platform === 'qq'" class="deploy-section">
            <button class="btn-deploy" :disabled="deploying" @click="onDeployQQ">
              <svg v-if="!deploying" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M12 2a7 7 0 017 7c0 3-2 5-3 7l1 4H7l1-4c-1-2-3-4-3-7a7 7 0 017-7z"/><path d="M9 22h6"/></svg>
              <svg v-else class="spinning" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M21 12a9 9 0 11-6.219-8.56"/></svg>
              {{ deploying ? '正在创建部署任务...' : '一键部署 QQ 机器人' }}
            </button>
            <span v-if="deployResult" class="deploy-result">{{ deployResult }}</span>
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
.deploy-section { margin-bottom: 14px; display: flex; flex-direction: column; gap: 6px; }
.btn-deploy {
  display: flex; align-items: center; gap: 6px; padding: 6px 14px;
  border-radius: var(--radius); font-size: 13px; font-weight: 500;
  background: var(--bg-hover); color: var(--text-secondary); border: 1px solid var(--border);
  transition: all var(--transition); cursor: pointer; width: fit-content;
}
.btn-deploy:hover:not(:disabled) { background: var(--accent-soft); color: var(--accent); border-color: var(--accent); }
.btn-deploy:disabled { opacity: 0.6; cursor: not-allowed; }
.btn-deploy .spinning { animation: spin 1s linear infinite; }
@keyframes spin { to { transform: rotate(360deg); } }
.deploy-result { font-size: 12px; color: var(--accent); }
.session-row { display: flex; gap: 8px; align-items: center; }
.session-row select { flex: 1; }
.btn-smart {
  display: flex; align-items: center; gap: 4px; padding: 6px 12px;
  border-radius: var(--radius); font-size: 12px; font-weight: 500;
  background: var(--bg-hover); color: var(--accent); border: 1px solid var(--border);
  transition: all var(--transition); cursor: pointer; white-space: nowrap; flex-shrink: 0;
}
.btn-smart:hover { background: var(--accent-soft); border-color: var(--accent); }
.smart-create-area { margin-top: 8px; display: flex; flex-direction: column; gap: 8px; }
.smart-create-area textarea {
  width: 100%; padding: 8px 10px; font-size: 13px; border-radius: var(--radius);
  border: 1px solid var(--border); background: var(--bg-primary); color: var(--text-primary);
  resize: vertical;
}
.smart-create-actions { display: flex; align-items: center; gap: 8px; }
.btn-smart-go {
  display: flex; align-items: center; gap: 4px; padding: 5px 12px;
  border-radius: var(--radius); font-size: 12px; font-weight: 500;
  background: var(--accent); color: #fff; transition: opacity var(--transition); cursor: pointer;
}
.btn-smart-go:hover:not(:disabled) { opacity: 0.9; }
.btn-smart-go:disabled { opacity: 0.5; cursor: not-allowed; }
.btn-smart-go .spinning { animation: spin 1s linear infinite; }
.smart-result { font-size: 12px; color: var(--accent); }
.config-fields { display: flex; flex-direction: column; gap: 6px; }
</style>
