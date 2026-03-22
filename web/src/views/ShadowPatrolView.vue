<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { marked } from 'marked'

interface PatrolResult {
  content: string
  exists: boolean
}

interface PatrolActivity {
  timestamp: string
  type: string
  summary: string
  details?: string
  expanded?: boolean
}

const patrolResult = ref<PatrolResult>({ content: '', exists: false })
const allActivities = ref<PatrolActivity[]>([])
const loading = ref(false)
const currentPage = ref(1)
const pageSize = 20

const patrolActivities = computed(() => {
  return allActivities.value.filter(a => a.type === 'patrol')
})

const totalPages = computed(() => {
  return Math.ceil(patrolActivities.value.length / pageSize)
})

const paginatedActivities = computed(() => {
  const start = (currentPage.value - 1) * pageSize
  const end = start + pageSize
  return patrolActivities.value.slice(start, end)
})

const totalPatrols = computed(() => patrolActivities.value.length)

const lastPatrolTime = computed(() => {
  const firstPatrol = patrolActivities.value[0]
  if (!firstPatrol) return '-'
  return new Date(firstPatrol.timestamp).toLocaleString('zh-CN')
})

const renderedContent = computed(() => {
  if (!patrolResult.value.content) return '<p class="empty-hint">暂无巡检结果</p>'
  try {
    return marked(patrolResult.value.content)
  } catch (err) {
    console.error('Markdown render error:', err)
    return '<p class="error-hint">Markdown 渲染失败</p>'
  }
})

async function loadPatrolResult() {
  try {
    const res = await fetch('/api/v1/files/content?scope=session&path=memory/shadow/patrol-result.md')
    patrolResult.value = await res.json()
  } catch (err) {
    console.error('Failed to load patrol result:', err)
  }
}

async function loadActivities() {
  try {
    const res = await fetch('/api/v1/shadow-ai/activities?limit=100')
    const data = await res.json()
    allActivities.value = (data.activities || []).map((a: PatrolActivity) => ({
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
    await Promise.all([loadPatrolResult(), loadActivities()])
  } finally {
    loading.value = false
  }
}

function toggleActivity(activity: PatrolActivity) {
  activity.expanded = !activity.expanded
}

function goToPage(page: number) {
  if (page < 1 || page > totalPages.value) return
  currentPage.value = page
}

onMounted(() => {
  loadAll()
})
</script>

<template>
  <div class="shadow-patrol">
    <div v-if="loading && !patrolResult.exists" class="loading">
      加载中...
    </div>
    <div v-else class="patrol-container">
      <!-- 统计卡片 -->
      <div class="stats-grid">
        <div class="stat-card">
          <div class="stat-icon">
            <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <circle cx="11" cy="11" r="8"/>
              <path d="m21 21-4.35-4.35"/>
            </svg>
          </div>
          <div class="stat-value">{{ totalPatrols }}</div>
          <div class="stat-label">总巡检次数</div>
        </div>

        <div class="stat-card">
          <div class="stat-icon">⏰</div>
          <div class="stat-value small">{{ lastPatrolTime }}</div>
          <div class="stat-label">最近巡检时间</div>
        </div>
      </div>

      <!-- 最近巡检结果 -->
      <div class="result-section">
        <div class="section-header">
          <h3>最近巡检结果</h3>
          <button class="refresh-btn" @click="loadPatrolResult">
            <svg class="btn-icon" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <polyline points="23 4 23 10 17 10"/>
              <polyline points="1 20 1 14 7 14"/>
              <path d="M3.51 9a9 9 0 0 1 14.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0 0 20.49 15"/>
            </svg>
            刷新
          </button>
        </div>

        <div class="result-content">
          <div v-if="loading" class="loading-state">
            <div class="spinner"></div>
            <p>加载中...</p>
          </div>
          <div v-else class="markdown-content" v-html="renderedContent"></div>
        </div>
      </div>

      <!-- 巡检历史 -->
      <div class="history-section">
        <div class="section-header">
          <h3>巡检历史</h3>
          <span class="history-count">共 {{ totalPatrols }} 条记录</span>
        </div>

        <div v-if="patrolActivities.length === 0" class="empty-state">
          <p>暂无巡检历史</p>
        </div>
        <div v-else>
          <div class="history-list">
            <div
              v-for="(activity, index) in paginatedActivities"
              :key="index"
              class="history-item"
            >
              <div class="history-timeline">
                <div class="timeline-dot"></div>
                <div v-if="index < paginatedActivities.length - 1" class="timeline-line"></div>
              </div>
              <div class="history-content">
                <div class="history-header" @click="toggleActivity(activity)">
                  <div class="history-time">
                    {{ new Date(activity.timestamp).toLocaleString('zh-CN') }}
                  </div>
                  <div class="history-summary">
                    {{ activity.summary }}
                  </div>
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
                <div v-if="activity.expanded && activity.details" class="history-details">
                  {{ activity.details }}
                </div>
              </div>
            </div>
          </div>

          <!-- 分页 -->
          <div v-if="totalPages > 1" class="pagination">
            <button
              class="page-btn"
              :disabled="currentPage === 1"
              @click="goToPage(currentPage - 1)"
            >
              上一页
            </button>
            <span class="page-info">
              第 {{ currentPage }} / {{ totalPages }} 页
            </span>
            <button
              class="page-btn"
              :disabled="currentPage === totalPages"
              @click="goToPage(currentPage + 1)"
            >
              下一页
            </button>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.shadow-patrol {
  padding: 24px;
  max-width: 1200px;
  margin: 0 auto;
}

.loading {
  text-align: center;
  padding: 40px;
  color: var(--text-secondary);
}

.patrol-container {
  display: flex;
  flex-direction: column;
  gap: 24px;
}

/* 统计卡片 */
.stats-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(250px, 1fr));
  gap: 16px;
}

.stat-card {
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

.stat-card:hover {
  border-color: var(--primary);
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.05);
}

.stat-icon {
  font-size: 32px;
  margin-bottom: 12px;
}

.stat-value {
  font-size: 36px;
  font-weight: 700;
  color: var(--text-primary);
  margin-bottom: 8px;
}

.stat-value.small {
  font-size: 16px;
  font-weight: 500;
}

.stat-label {
  font-size: 13px;
  color: var(--text-secondary);
  font-weight: 500;
}

/* 巡检结果 */
.result-section {
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

.history-count {
  font-size: 13px;
  color: var(--text-secondary);
}

.result-content {
  background: var(--bg-primary);
  border: 1px solid var(--border);
  border-radius: 6px;
  padding: 20px;
  min-height: 200px;
}

.loading-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 12px;
  padding: 40px;
}

.spinner {
  width: 32px;
  height: 32px;
  border: 3px solid var(--border);
  border-top-color: var(--primary);
  border-radius: 50%;
  animation: spin 0.8s linear infinite;
}

@keyframes spin {
  to { transform: rotate(360deg); }
}

.markdown-content {
  line-height: 1.8;
  color: var(--text-primary);
}

.markdown-content :deep(h1),
.markdown-content :deep(h2),
.markdown-content :deep(h3),
.markdown-content :deep(h4),
.markdown-content :deep(h5),
.markdown-content :deep(h6) {
  margin-top: 24px;
  margin-bottom: 16px;
  font-weight: 600;
  line-height: 1.25;
}

.markdown-content :deep(h1) { font-size: 2em; }
.markdown-content :deep(h2) { font-size: 1.5em; }
.markdown-content :deep(h3) { font-size: 1.25em; }

.markdown-content :deep(p) {
  margin-bottom: 16px;
}

.markdown-content :deep(ul),
.markdown-content :deep(ol) {
  margin-bottom: 16px;
  padding-left: 2em;
}

.markdown-content :deep(li) {
  margin-bottom: 8px;
}

.markdown-content :deep(code) {
  padding: 2px 6px;
  background: var(--bg-hover);
  border-radius: 3px;
  font-family: 'Monaco', 'Menlo', 'Courier New', monospace;
  font-size: 0.9em;
}

.markdown-content :deep(pre) {
  padding: 16px;
  background: var(--bg-hover);
  border-radius: 6px;
  overflow-x: auto;
  margin-bottom: 16px;
}

.markdown-content :deep(pre code) {
  padding: 0;
  background: none;
}

.empty-hint,
.error-hint {
  color: var(--text-secondary);
  font-style: italic;
}

.error-hint {
  color: #ef4444;
}

/* 巡检历史 */
.history-section {
  background: var(--bg-secondary);
  border: 1px solid var(--border);
  border-radius: 8px;
  padding: 24px;
}

.empty-state {
  text-align: center;
  padding: 40px;
  color: var(--text-secondary);
}

.history-list {
  display: flex;
  flex-direction: column;
}

.history-item {
  display: flex;
  gap: 16px;
  position: relative;
}

.history-timeline {
  display: flex;
  flex-direction: column;
  align-items: center;
  padding-top: 4px;
}

.timeline-dot {
  width: 12px;
  height: 12px;
  border-radius: 50%;
  background: var(--primary);
  border: 2px solid var(--bg-secondary);
  flex-shrink: 0;
}

.timeline-line {
  width: 2px;
  flex: 1;
  background: var(--border);
  margin-top: 4px;
}

.history-content {
  flex: 1;
  margin-bottom: 20px;
}

.history-header {
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 16px;
  background: var(--bg-primary);
  border: 1px solid var(--border);
  border-radius: 6px;
  cursor: pointer;
  transition: all 0.2s;
}

.history-header:hover {
  background: var(--bg-hover);
  border-color: var(--primary);
}

.history-time {
  font-size: 13px;
  color: var(--text-secondary);
  font-weight: 500;
}

.history-summary {
  font-size: 14px;
  color: var(--text-primary);
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.expand-icon {
  color: var(--text-secondary);
  transition: transform 0.2s;
  flex-shrink: 0;
}

.expand-icon.expanded {
  transform: rotate(90deg);
}

.history-details {
  margin-top: 12px;
  padding: 16px;
  background: var(--bg-hover);
  border: 1px solid var(--border);
  border-radius: 6px;
  font-size: 13px;
  color: var(--text-secondary);
  line-height: 1.6;
  white-space: pre-wrap;
}

/* 分页 */
.pagination {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 16px;
  margin-top: 24px;
  padding-top: 24px;
  border-top: 1px solid var(--border);
}

.page-btn {
  padding: 8px 16px;
  border: 1px solid var(--border);
  border-radius: 6px;
  cursor: pointer;
  font-size: 14px;
  background: var(--bg-primary);
  color: var(--text-primary);
  transition: all 0.2s;
}

.page-btn:hover:not(:disabled) {
  background: var(--bg-hover);
  border-color: var(--primary);
}

.page-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.page-info {
  font-size: 14px;
  color: var(--text-secondary);
}
</style>
