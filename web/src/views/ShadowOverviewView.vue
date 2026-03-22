<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'

interface ShadowAIStatus {
  enabled: boolean
  session_id: number | null
  status: string
  config: any
  triggers: any[]
  created_at: string | null
  last_activity: string | null
  uptime_seconds: number
}

interface ShadowAIMetrics {
  memory_count: number
  router_count: number
  session_health: {
    healthy: number
    warning: number
    error: number
  }
  last_patrol: string
}

interface Activity {
  id?: number
  timestamp: string
  type: string
  summary: string
  details?: string
  expanded?: boolean
}

const status = ref<ShadowAIStatus | null>(null)
const metrics = ref<ShadowAIMetrics | null>(null)
const activities = ref<Activity[]>([])
const loading = ref(false)
const toggling = ref(false)

const uptimeDisplay = computed(() => {
  if (!status.value?.uptime_seconds) return '-'
  const hours = Math.floor(status.value.uptime_seconds / 3600)
  const days = Math.floor(hours / 24)
  if (days > 0) return `${days} 天 ${hours % 24} 小时`
  return `${hours} 小时`
})

const typeLabels: Record<string, { label: string; color: string }> = {
  patrol: { label: '巡检', color: '#3b82f6' },
  extract: { label: '提取', color: '#10b981' },
  deep_scan: { label: '深度扫描', color: '#f59e0b' },
  self_clean: { label: '自清理', color: '#8b5cf6' }
}

async function loadStatus() {
  try {
    const res = await fetch('/api/v1/shadow-ai/status')
    status.value = await res.json()
  } catch (err) {
    console.error('Failed to load status:', err)
  }
}

async function loadMetrics() {
  try {
    const res = await fetch('/api/v1/shadow-ai/metrics')
    metrics.value = await res.json()
  } catch (err) {
    console.error('Failed to load metrics:', err)
  }
}

async function loadActivities() {
  try {
    const res = await fetch('/api/v1/shadow-ai/activities?limit=10')
    const data = await res.json()
    activities.value = (data.activities || []).map((a: Activity) => ({
      ...a,
      expanded: false
    }))
  } catch (err) {
    console.error('Failed to load activities:', err)
  }
}

async function loadAll() {
  loading.value = true
  try {
    await Promise.all([loadStatus(), loadMetrics(), loadActivities()])
  } finally {
    loading.value = false
  }
}

async function toggleEnable() {
  if (!status.value) return

  toggling.value = true
  try {
    const endpoint = status.value.enabled
      ? '/api/v1/shadow-ai/disable'
      : '/api/v1/shadow-ai/enable'
    await fetch(endpoint, { method: 'POST' })
    await loadAll()
  } catch (err) {
    console.error('Failed to toggle enable:', err)
    alert('操作失败')
  } finally {
    toggling.value = false
  }
}

function toggleActivity(activity: Activity) {
  activity.expanded = !activity.expanded
}

onMounted(() => {
  loadAll()
})
</script>

<template>
  <div class="shadow-overview">
    <div v-if="loading && !status" class="loading">
      加载中...
    </div>
    <div v-else class="overview-container">
      <!-- 状态卡片 -->
      <div class="status-card">
        <div class="card-header">
          <h3>影子AI状态</h3>
          <button
            class="toggle-btn"
            :class="{ enabled: status?.enabled }"
            :disabled="toggling"
            @click="toggleEnable"
          >
            {{ toggling ? '操作中...' : (status?.enabled ? '禁用' : '启用') }}
          </button>
        </div>
        <div class="card-body">
          <div class="status-item">
            <span class="status-label">运行状态</span>
            <span :class="['status-badge', status?.enabled ? 'enabled' : 'disabled']">
              {{ status?.enabled ? '已启用' : '已禁用' }}
            </span>
          </div>
          <div class="status-item">
            <span class="status-label">运行时长</span>
            <span class="status-value">{{ uptimeDisplay }}</span>
          </div>
          <div class="status-item">
            <span class="status-label">会话ID</span>
            <span class="status-value">{{ status?.session_id || '-' }}</span>
          </div>
          <div class="status-item">
            <span class="status-label">最后活动</span>
            <span class="status-value">
              {{ status?.last_activity ? new Date(status.last_activity).toLocaleString('zh-CN') : '-' }}
            </span>
          </div>
        </div>
      </div>

      <!-- 关键指标卡片 -->
      <div class="metrics-grid">
        <div class="metric-card">
          <div class="metric-icon">
            <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/>
              <polyline points="14 2 14 8 20 8"/>
              <line x1="16" y1="13" x2="8" y2="13"/>
              <line x1="16" y1="17" x2="8" y2="17"/>
              <polyline points="10 9 9 9 8 9"/>
            </svg>
          </div>
          <div class="metric-value">{{ metrics?.memory_count || 0 }}</div>
          <div class="metric-label">结构化记忆</div>
        </div>

        <div class="metric-card">
          <div class="metric-icon">🔀</div>
          <div class="metric-value">{{ metrics?.router_count || 0 }}</div>
          <div class="metric-label">注入路由</div>
        </div>

        <div class="metric-card">
          <div class="metric-icon">💚</div>
          <div class="metric-content">
            <div class="health-stats">
              <div class="health-item">
                <span class="health-dot healthy"></span>
                <span class="health-count">{{ metrics?.session_health?.healthy || 0 }}</span>
              </div>
              <div class="health-item">
                <span class="health-dot warning"></span>
                <span class="health-count">{{ metrics?.session_health?.warning || 0 }}</span>
              </div>
              <div class="health-item">
                <span class="health-dot error"></span>
                <span class="health-count">{{ metrics?.session_health?.error || 0 }}</span>
              </div>
            </div>
          </div>
          <div class="metric-label">会话健康度</div>
        </div>

        <div class="metric-card">
          <div class="metric-icon">
            <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <circle cx="11" cy="11" r="8"/>
              <path d="m21 21-4.35-4.35"/>
            </svg>
          </div>
          <div class="metric-value small">
            {{ metrics?.last_patrol ? new Date(metrics.last_patrol).toLocaleString('zh-CN', { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' }) : '-' }}
          </div>
          <div class="metric-label">最后巡检</div>
        </div>
      </div>

      <!-- 最近活动列表 -->
      <div class="activities-section">
        <div class="section-header">
          <h3>最近活动</h3>
          <button class="refresh-btn" @click="loadActivities">
            <svg class="btn-icon" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <polyline points="23 4 23 10 17 10"/>
              <polyline points="1 20 1 14 7 14"/>
              <path d="M3.51 9a9 9 0 0 1 14.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0 0 20.49 15"/>
            </svg>
            刷新
          </button>
        </div>

        <div v-if="activities.length === 0" class="empty-state">
          <p>暂无活动记录</p>
        </div>
        <div v-else class="activities-list">
          <div
            v-for="(activity, index) in activities"
            :key="index"
            class="activity-item"
          >
            <div class="activity-header" @click="toggleActivity(activity)">
              <div class="activity-left">
                <span
                  class="activity-type"
                  :style="{ background: typeLabels[activity.type]?.color + '20', color: typeLabels[activity.type]?.color }"
                >
                  {{ typeLabels[activity.type]?.label || activity.type }}
                </span>
                <span class="activity-time">
                  {{ new Date(activity.timestamp).toLocaleString('zh-CN') }}
                </span>
              </div>
              <div class="activity-right">
                <span class="activity-summary">{{ activity.summary }}</span>
                <svg
                  v-if="activity.details"
                  class="expand-icon"
                  :class="{ expanded: activity.expanded }"
                  width="16"
                  height="16"
                  viewBox="0 0 24 24"
                  fill="none"
                  stroke="currentColor"
                  stroke-width="2"
                >
                  <polyline points="9 18 15 12 9 6"/>
                </svg>
              </div>
            </div>
            <div v-if="activity.expanded && activity.details" class="activity-details">
              {{ activity.details }}
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.shadow-overview {
  padding: 24px;
  max-width: 1400px;
  margin: 0 auto;
}

.loading {
  text-align: center;
  padding: 40px;
  color: var(--text-secondary);
}

.overview-container {
  display: flex;
  flex-direction: column;
  gap: 24px;
}

/* 状态卡片 */
.status-card {
  background: var(--bg-secondary);
  border: 1px solid var(--border);
  border-radius: 8px;
  padding: 24px;
}

.card-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 20px;
}

.card-header h3 {
  margin: 0;
  font-size: 18px;
  font-weight: 600;
  color: var(--text-primary);
}

.toggle-btn {
  padding: 8px 20px;
  border: 1px solid var(--border);
  border-radius: 6px;
  cursor: pointer;
  font-size: 14px;
  font-weight: 500;
  transition: all 0.2s;
  background: var(--bg-primary);
  color: var(--text-primary);
}

.toggle-btn.enabled {
  background: var(--primary);
  color: white;
  border-color: var(--primary);
}

.toggle-btn:hover:not(:disabled) {
  opacity: 0.9;
}

.toggle-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.card-body {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
  gap: 20px;
}

.status-item {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.status-label {
  font-size: 13px;
  color: var(--text-secondary);
}

.status-badge {
  padding: 4px 12px;
  border-radius: 12px;
  font-size: 13px;
  font-weight: 500;
  width: fit-content;
}

.status-badge.enabled {
  background: rgba(16, 185, 129, 0.1);
  color: #10b981;
}

.status-badge.disabled {
  background: rgba(107, 114, 128, 0.1);
  color: #6b7280;
}

.status-value {
  font-size: 16px;
  font-weight: 500;
  color: var(--text-primary);
}

/* 指标卡片 */
.metrics-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
  gap: 16px;
}

.metric-card {
  background: var(--bg-secondary);
  border: 1px solid var(--border);
  border-radius: 8px;
  padding: 24px;
  display: flex;
  flex-direction: column;
  align-items: center;
  text-align: center;
  transition: all 0.2s;
}

.metric-card:hover {
  border-color: var(--primary);
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.05);
}

.metric-icon {
  font-size: 32px;
  margin-bottom: 12px;
}

.metric-value {
  font-size: 36px;
  font-weight: 700;
  color: var(--text-primary);
  margin-bottom: 8px;
}

.metric-value.small {
  font-size: 16px;
  font-weight: 500;
}

.metric-content {
  margin-bottom: 8px;
}

.health-stats {
  display: flex;
  gap: 16px;
  justify-content: center;
}

.health-item {
  display: flex;
  align-items: center;
  gap: 6px;
}

.health-dot {
  width: 12px;
  height: 12px;
  border-radius: 50%;
}

.health-dot.healthy {
  background: #10b981;
}

.health-dot.warning {
  background: #f59e0b;
}

.health-dot.error {
  background: #ef4444;
}

.health-count {
  font-size: 20px;
  font-weight: 600;
  color: var(--text-primary);
}

.metric-label {
  font-size: 13px;
  color: var(--text-secondary);
  font-weight: 500;
}

/* 活动列表 */
.activities-section {
  background: var(--bg-secondary);
  border: 1px solid var(--border);
  border-radius: 8px;
  padding: 24px;
}

.section-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 20px;
}

.section-header h3 {
  margin: 0;
  font-size: 18px;
  font-weight: 600;
  color: var(--text-primary);
}

.refresh-btn {
  padding: 6px 12px;
  border: 1px solid var(--border);
  border-radius: 6px;
  cursor: pointer;
  font-size: 13px;
  background: var(--bg-primary);
  color: var(--text-primary);
  transition: all 0.2s;
}

.refresh-btn:hover {
  background: var(--bg-hover);
}

.empty-state {
  text-align: center;
  padding: 40px;
  color: var(--text-secondary);
}

.activities-list {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.activity-item {
  border: 1px solid var(--border);
  border-radius: 6px;
  overflow: hidden;
  transition: all 0.2s;
}

.activity-item:hover {
  border-color: var(--primary);
}

.activity-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 16px;
  cursor: pointer;
  background: var(--bg-primary);
  transition: all 0.2s;
}

.activity-header:hover {
  background: var(--bg-hover);
}

.activity-left {
  display: flex;
  align-items: center;
  gap: 12px;
}

.activity-type {
  padding: 4px 12px;
  border-radius: 12px;
  font-size: 12px;
  font-weight: 500;
}

.activity-time {
  font-size: 13px;
  color: var(--text-secondary);
}

.activity-right {
  display: flex;
  align-items: center;
  gap: 12px;
}

.activity-summary {
  font-size: 14px;
  color: var(--text-primary);
}

.expand-icon {
  color: var(--text-secondary);
  transition: transform 0.2s;
}

.expand-icon.expanded {
  transform: rotate(90deg);
}

.activity-details {
  padding: 16px;
  background: var(--bg-hover);
  border-top: 1px solid var(--border);
  font-size: 13px;
  color: var(--text-secondary);
  line-height: 1.6;
  white-space: pre-wrap;
}
</style>
