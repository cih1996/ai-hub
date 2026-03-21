<script setup lang="ts">
import { ref, onMounted } from 'vue'

interface ShadowAIStatus {
  enabled: boolean
  session_id: number | null
  trigger_ids: number[]
  config: {
    patrol_interval: number
    extract_interval: number
    deep_scan_interval: number
    self_clean_interval: number
    context_reset_threshold: number
  }
  created_at: string | null
  last_activity: string | null
  uptime_seconds: number
}

interface ShadowAIConfig {
  patrol_interval: number
  extract_interval: number
  deep_scan_interval: number
  self_clean_interval: number
  context_reset_threshold: number
}

const status = ref<ShadowAIStatus | null>(null)
const config = ref<ShadowAIConfig>({
  patrol_interval: 10,
  extract_interval: 60,
  deep_scan_interval: 360,
  self_clean_interval: 1440,
  context_reset_threshold: 50,
})
const loading = ref(false)
const saving = ref(false)
const showAdvanced = ref(false)

const rules = {
  patrol_interval: { min: 1, max: 1440, message: '巡检间隔必须在 1-1440 分钟之间' },
  extract_interval: { min: 10, max: 1440, message: '提取间隔必须在 10-1440 分钟之间' },
  deep_scan_interval: { min: 60, max: 1440, message: '深度扫描间隔必须在 60-1440 分钟之间' },
  self_clean_interval: { min: 60, max: 10080, message: '自清理间隔必须在 60-10080 分钟之间' },
  context_reset_threshold: { min: 10, max: 200, message: '上下文重置阈值必须在 10-200 之间' },
}

// 解析后端返回的时间字符串（如 "10m", "1h", "24h"）为分钟数
function parseDuration(str: string): number {
  if (!str) return 0
  if (str.endsWith('m')) return parseInt(str)
  if (str.endsWith('h')) return parseInt(str) * 60
  if (str.endsWith('d')) return parseInt(str) * 1440
  return parseInt(str)
}

// 将分钟数转换为后端期望的时间字符串格式
function toDurationString(minutes: number): string {
  if (minutes < 60) return `${minutes}m`
  if (minutes % 60 === 0) return `${minutes / 60}h`
  return `${minutes}m`
}

async function loadStatus() {
  loading.value = true
  try {
    const res = await fetch('/api/v1/shadow-ai/status')
    status.value = await res.json()
    if (status.value) {
      config.value = {
        patrol_interval: parseDuration(status.value.config.patrol_interval as any),
        extract_interval: parseDuration(status.value.config.extract_interval as any),
        deep_scan_interval: parseDuration(status.value.config.deep_scan_interval as any),
        self_clean_interval: parseDuration(status.value.config.self_clean_interval as any),
        context_reset_threshold: status.value.config.context_reset_threshold,
      }
    }
  } catch (err) {
    console.error('Failed to load status:', err)
  } finally {
    loading.value = false
  }
}

async function saveConfig() {
  // 验证表单
  for (const [key, rule] of Object.entries(rules)) {
    const value = config.value[key as keyof ShadowAIConfig]
    if (value < rule.min || value > rule.max) {
      alert(rule.message)
      return
    }
  }

  saving.value = true
  try {
    const payload = {
      patrol_interval: toDurationString(config.value.patrol_interval),
      extract_interval: toDurationString(config.value.extract_interval),
      deep_scan_interval: toDurationString(config.value.deep_scan_interval),
      self_clean_interval: toDurationString(config.value.self_clean_interval),
      context_reset_threshold: config.value.context_reset_threshold,
    }

    await fetch('/api/v1/shadow-ai/config', {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload),
    })
    await loadStatus()
    alert('配置已保存')
  } catch (err) {
    console.error('Failed to save config:', err)
    alert('保存失败')
  } finally {
    saving.value = false
  }
}

async function toggleEnable() {
  const endpoint = status.value?.enabled
    ? '/api/v1/shadow-ai/disable'
    : '/api/v1/shadow-ai/enable'
  try {
    await fetch(endpoint, { method: 'POST' })
    await loadStatus()
  } catch (err) {
    console.error('Failed to toggle enable:', err)
    alert('操作失败')
  }
}

function formatDuration(minutes: number): string {
  if (minutes < 60) return `${minutes} 分钟`
  const hours = Math.floor(minutes / 60)
  const mins = minutes % 60
  if (mins === 0) return `${hours} 小时`
  return `${hours} 小时 ${mins} 分钟`
}

onMounted(() => {
  loadStatus()
})
</script>

<template>
  <div class="shadow-config">
    <div v-if="loading" class="loading">加载中...</div>

    <div v-else class="config-container">
      <!-- 启用/禁用开关 -->
      <div class="config-section">
        <div class="section-header">
          <h3>影子AI状态</h3>
        </div>
        <div class="status-card">
          <div class="status-row">
            <div class="status-info">
              <span class="status-label">运行状态</span>
              <span :class="['status-badge', status?.enabled ? 'enabled' : 'disabled']">
                {{ status?.enabled ? '已启用' : '已禁用' }}
              </span>
            </div>
            <button
              class="toggle-btn"
              @click="toggleEnable"
              :disabled="loading"
            >
              {{ status?.enabled ? '禁用' : '启用' }}
            </button>
          </div>
          <div v-if="status?.enabled && status?.last_activity" class="status-detail">
            <span class="detail-label">最后活动:</span>
            <span class="detail-value">{{ new Date(status.last_activity).toLocaleString('zh-CN') }}</span>
          </div>
        </div>
      </div>

      <!-- 配置表单 -->
      <div class="config-section">
        <div class="section-header">
          <h3>定时任务配置</h3>
          <p class="section-desc">设置影子AI各项任务的执行间隔</p>
        </div>

        <div class="form-grid">
          <div class="form-item">
            <label>巡检间隔（分钟）</label>
            <input
              v-model.number="config.patrol_interval"
              type="number"
              :min="rules.patrol_interval.min"
              :max="rules.patrol_interval.max"
              class="form-input"
            />
            <span class="form-hint">{{ formatDuration(config.patrol_interval) }} - 检查会话健康状态</span>
          </div>

          <div class="form-item">
            <label>提取间隔（分钟）</label>
            <input
              v-model.number="config.extract_interval"
              type="number"
              :min="rules.extract_interval.min"
              :max="rules.extract_interval.max"
              class="form-input"
            />
            <span class="form-hint">{{ formatDuration(config.extract_interval) }} - 提取结构化记忆</span>
          </div>

          <div class="form-item">
            <label>深度扫描间隔（分钟）</label>
            <input
              v-model.number="config.deep_scan_interval"
              type="number"
              :min="rules.deep_scan_interval.min"
              :max="rules.deep_scan_interval.max"
              class="form-input"
            />
            <span class="form-hint">{{ formatDuration(config.deep_scan_interval) }} - 全面检查系统状态</span>
          </div>

          <div class="form-item">
            <label>自清理间隔（分钟）</label>
            <input
              v-model.number="config.self_clean_interval"
              type="number"
              :min="rules.self_clean_interval.min"
              :max="rules.self_clean_interval.max"
              class="form-input"
            />
            <span class="form-hint">{{ formatDuration(config.self_clean_interval) }} - 归档日志和清理数据</span>
          </div>

          <div class="form-item">
            <label>上下文重置阈值</label>
            <input
              v-model.number="config.context_reset_threshold"
              type="number"
              :min="rules.context_reset_threshold.min"
              :max="rules.context_reset_threshold.max"
              class="form-input"
            />
            <span class="form-hint">{{ config.context_reset_threshold }} 条消息 - 自动重置上下文</span>
          </div>
        </div>

        <div class="form-actions">
          <button
            class="save-btn"
            @click="saveConfig"
            :disabled="saving || loading"
          >
            {{ saving ? '保存中...' : '保存配置' }}
          </button>
        </div>
      </div>

      <!-- 高级配置 -->
      <div class="config-section">
        <div class="section-header clickable" @click="showAdvanced = !showAdvanced">
          <h3>高级信息</h3>
          <span class="toggle-icon">{{ showAdvanced ? '▼' : '▶' }}</span>
        </div>

        <div v-if="showAdvanced" class="advanced-info">
          <div class="info-row">
            <span class="info-label">会话ID:</span>
            <span class="info-value">{{ status?.session_id || '-' }}</span>
          </div>
          <div class="info-row">
            <span class="info-label">定时器ID:</span>
            <span class="info-value">{{ status?.trigger_ids?.join(', ') || '-' }}</span>
          </div>
          <div class="info-row">
            <span class="info-label">创建时间:</span>
            <span class="info-value">
              {{ status?.created_at ? new Date(status.created_at).toLocaleString('zh-CN') : '-' }}
            </span>
          </div>
          <div class="info-row">
            <span class="info-label">运行时长:</span>
            <span class="info-value">
              {{ status?.uptime_seconds ? Math.floor(status.uptime_seconds / 3600) + ' 小时' : '-' }}
            </span>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.shadow-config {
  padding: 24px;
  max-width: 900px;
  margin: 0 auto;
}

.loading {
  text-align: center;
  padding: 40px;
  color: var(--text-secondary);
}

.config-container {
  display: flex;
  flex-direction: column;
  gap: 24px;
}

.config-section {
  background: var(--bg-secondary);
  border: 1px solid var(--border);
  border-radius: 8px;
  padding: 20px;
}

.section-header {
  margin-bottom: 16px;
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.section-header.clickable {
  cursor: pointer;
  user-select: none;
}

.section-header h3 {
  margin: 0;
  font-size: 16px;
  font-weight: 600;
  color: var(--text-primary);
}

.section-desc {
  margin: 8px 0 0 0;
  font-size: 14px;
  color: var(--text-secondary);
}

.toggle-icon {
  font-size: 12px;
  color: var(--text-secondary);
}

.status-card {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.status-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.status-info {
  display: flex;
  align-items: center;
  gap: 12px;
}

.status-label {
  font-size: 14px;
  color: var(--text-secondary);
}

.status-badge {
  padding: 4px 12px;
  border-radius: 12px;
  font-size: 12px;
  font-weight: 500;
}

.status-badge.enabled {
  background: rgba(16, 185, 129, 0.1);
  color: #10b981;
}

.status-badge.disabled {
  background: rgba(107, 114, 128, 0.1);
  color: #6b7280;
}

.status-detail {
  display: flex;
  gap: 8px;
  font-size: 13px;
  color: var(--text-secondary);
}

.detail-label {
  font-weight: 500;
}

.toggle-btn {
  padding: 8px 16px;
  background: var(--primary);
  color: white;
  border: none;
  border-radius: 6px;
  cursor: pointer;
  font-size: 14px;
  transition: all 0.2s;
}

.toggle-btn:hover:not(:disabled) {
  opacity: 0.9;
}

.toggle-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.form-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(250px, 1fr));
  gap: 20px;
}

.form-item {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.form-item label {
  font-size: 14px;
  font-weight: 500;
  color: var(--text-primary);
}

.form-input {
  padding: 8px 12px;
  border: 1px solid var(--border);
  border-radius: 6px;
  background: var(--bg-primary);
  color: var(--text-primary);
  font-size: 14px;
  transition: all 0.2s;
}

.form-input:focus {
  outline: none;
  border-color: var(--primary);
}

.form-hint {
  font-size: 12px;
  color: var(--text-secondary);
}

.form-actions {
  margin-top: 20px;
  display: flex;
  justify-content: flex-end;
}

.save-btn {
  padding: 10px 24px;
  background: var(--primary);
  color: white;
  border: none;
  border-radius: 6px;
  cursor: pointer;
  font-size: 14px;
  font-weight: 500;
  transition: all 0.2s;
}

.save-btn:hover:not(:disabled) {
  opacity: 0.9;
}

.save-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.advanced-info {
  display: flex;
  flex-direction: column;
  gap: 12px;
  padding-top: 12px;
}

.info-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 8px 0;
  border-bottom: 1px solid var(--border);
}

.info-row:last-child {
  border-bottom: none;
}

.info-label {
  font-size: 14px;
  color: var(--text-secondary);
}

.info-value {
  font-size: 14px;
  color: var(--text-primary);
  font-family: monospace;
}
</style>
