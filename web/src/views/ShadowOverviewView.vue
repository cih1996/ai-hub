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
            <svg width="28" height="28" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <path d="M20 21v-2a4 4 0 0 0-4-4H8a4 4 0 0 0-4 4v2"></path>
              <circle cx="12" cy="7" r="4"></circle>
            </svg>
          </div>
          <div class="metric-value">{{ metrics?.memory_count || 0 }}</div>
          <div class="metric-label">结构化记忆</div>
        </div>

        <div class="metric-card">
          <div class="metric-icon">
            <svg width="28" height="28" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <polyline points="9 18 15 12 9 6"></polyline>
            </svg>
          </div>
          <div class="metric-value">{{ metrics?.router_count || 0 }}</div>
          <div class="metric-label">注入路由</div>
        </div>

        <div class="metric-card">
          <div class="metric-icon">
            <svg width="28" height="28" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <path d="M22 11.08V12a10.06 10.06 0 1 1-5.93-9.14"></path>
              <polyline points="22 4 12 14.01 9 11.01"></polyline>
            </svg>
          </div>
          <div class="metric-content">
            <div class="health-stats">
              <div class="health-item">
                <span class="health-badge healthy">
                  <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="20 6 9 17 4 12"></polyline></svg>
                  {{ metrics?.session_health?.healthy || 0 }}
                </span>
                <span class="health-label">健康</span>
              </div>
              <div class="health-item">
                <span class="health-badge warning">
                  <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M10.29 3.86L1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z"></path><line x1="12" y1="9" x2="12" y2="13"></line><line x1="12" y1="17" x2="12.01" y2="17"></line></svg>
                  {{ metrics?.session_health?.warning || 0 }}
                </span>
                <span class="health-label">警告</span>
              </div>
              <div class="health-item">
                <span class="health-badge error">
                  <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"></circle><line x1="15" y1="9" x2="9" y2="15"></line><line x1="9" y1="9" x2="15" y2="15"></line></svg>
                  {{ metrics?.session_health?.error || 0 }}
                </span>
                <span class="health-label">错误</span>
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
  padding: 16px;
  max-width: 1400px;
  margin: 0 auto;
}

@media (min-width: 768px) {
  .shadow-overview {
    padding: 24px;
  }
}

.loading {
  text-align: center;
  padding: 40px;
  color: var(--text-secondary);
}

.overview-container {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

@media (min-width: 768px) {
  .overview-container {
    gap: 24px;
  }
}

/* 状态卡片 */
.status-card {
  background: var(--bg-secondary);
  border: 1px solid var(--border);
  border-radius: 8px;
  padding: 16px;
}

@media (min-width: 768px) {
  .status-card {
    padding: 24px;
  }
}

.card-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 16px;
}

@media (min-width: 768px) {
  .card-header {
    margin-bottom: 20px;
  }
}

.card-header h3 {
  margin: 0;
  font-size: 18px;
  font-weight: 600;
  color: var(--text-primary);
}

.toggle-btn {
  padding: 8px 16px;
  border: 1px solid var(--border);
  border-radius: 6px;
  cursor: pointer;
  font-size: 13px;
  font-weight: 500;
  transition: all 0.2s;
  background: var(--bg-primary);
  color: var(--text-primary);
}

@media (min-width: 768px) {
  .toggle-btn {
    padding: 8px 20px;
    font-size: 14px;
  }
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
  grid-template-columns: 1fr;
  gap: 16px;
}

@media (min-width: 640px) {
  .card-body {
    grid-template-columns: repeat(2, 1fr);
  }
}

@media (min-width: 1024px) {
  .card-body {
    grid-template-columns: repeat(4, 1fr);
  }
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

[data-theme="light"] .status-badge.enabled {
  background: #ecfdf5;
  color: #059669;
}

.status-badge.disabled {
  background: rgba(107, 114, 128, 0.1);
  color: #6b7280;
}

[data-theme="light"] .status-badge.disabled {
  background: #f3f4f6;
  color: #4b5563;
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
  color: var(--text-secondary);
  margin-bottom: 12px;
  display: flex;
  align-items: center;
  justify-content: center;
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
  flex-direction: column;
  align-items: center;
  gap: 4px;
}

.health-badge {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 2px;
  width: 56px;
  height: 56px;
  border-radius: 50%;
  font-size: 16px;
  font-weight: 600;
}

.health-badge svg {
  width: 20px;
  height: 20px;
}

.health-badge.healthy {
  background: rgba(16, 185, 129, 0.1);
  color: #10b981;
}

[data-theme="light"] .health-badge.healthy {
  background: #ecfdf5;
  color: #059669;
}

.health-badge.warning {
  background: rgba(245, 158, 11, 0.1);
  color: #f59e0b;
}

[data-theme="light"] .health-badge.warning {
  background: #fffbeb;
  color: #d97706;
}

.health-badge.error {
  background: rgba(239, 68, 68, 0.1);
  color: #ef4444;
}

[data-theme="light"] .health-badge.error {
  background: #fef2f2;
  color: #dc2626;
}

.health-label {
  font-size: 12px;
  color: var(--text-secondary);
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
  padding: 16px;
}

@media (min-width: 768px) {
  .activities-section {
    padding: 24px;
  }
}

.section-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 16px;
}

@media (min-width: 768px) {
  .section-header {
    margin-bottom: 20px;
  }
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
  flex-direction: column;
  align-items: flex-start;
  gap: 12px;
  padding: 12px 16px;
  cursor: pointer;
  background: var(--bg-primary);
  transition: all 0.2s;
}

@media (min-width: 640px) {
  .activity-header {
    flex-direction: row;
    align-items: center;
    justify-content: space-between;
    padding: 16px;
  }
}

.activity-header:hover {
  background: var(--bg-hover);
}

.activity-left {
  display: flex;
  align-items: center;
  gap: 12px;
  width: 100%;
}

@media (min-width: 640px) {
  .activity-left {
    width: auto;
  }
}

.activity-type {
  padding: 4px 12px;
  border-radius: 12px;
  font-size: 12px;
  font-weight: 500;
  white-space: nowrap;
}

.activity-time {
  font-size: 12px;
  color: var(--text-secondary);
}

@media (min-width: 640px) {
  .activity-time {
    font-size: 13px;
  }
}

.activity-right {
  display: flex;
  align-items: center;
  justify-content: space-between;
  width: 100%;
  gap: 12px;
}

@media (min-width: 640px) {
  .activity-right {
    width: auto;
    justify-content: flex-end;
  }
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
