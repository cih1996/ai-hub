<script setup lang="ts">
import { ref, onMounted, onBeforeUnmount, computed } from 'vue'
import { marked } from 'marked'

interface LogFile {
  key: string
  label: string
  path: string
}

const logFiles: LogFile[] = [
  { key: 'work-log', label: '工作日志', path: 'memory/shadow/work-log.md' },
  { key: 'patrol-result', label: '巡检结果', path: 'memory/shadow/patrol-result.md' },
  { key: 'status', label: '状态记录', path: 'memory/shadow/status.md' },
  { key: 'config', label: '配置记录', path: 'memory/shadow/config.md' },
]

const selectedLog = ref('work-log')
const content = ref('')
const loading = ref(false)
const autoRefresh = ref(true)
let refreshTimer: number | null = null

const renderedContent = computed(() => {
  if (!content.value) return '<p class="empty-hint">暂无日志内容</p>'
  try {
    return marked(content.value)
  } catch (err) {
    console.error('Markdown render error:', err)
    return '<p class="error-hint">Markdown 渲染失败</p>'
  }
})

async function loadLog(key: string) {
  loading.value = true
  try {
    const file = logFiles.find(f => f.key === key)
    if (!file) return
    const res = await fetch(`/api/v1/files/content?scope=session&path=${file.path}`)
    const data = await res.json()
    content.value = data.content || ''
  } catch (err) {
    console.error('Failed to load log:', err)
    content.value = ''
  } finally {
    loading.value = false
  }
}

async function clearLog() {
  if (!confirm('确定要清空当前日志吗？此操作不可恢复。')) return
  const file = logFiles.find(f => f.key === selectedLog.value)
  if (!file) return

  loading.value = true
  try {
    await fetch('/api/v1/files/content', {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ path: file.path, content: '' }),
    })
    await loadLog(selectedLog.value)
  } catch (err) {
    console.error('Failed to clear log:', err)
    alert('清空失败')
  } finally {
    loading.value = false
  }
}

function downloadLog() {
  const file = logFiles.find(f => f.key === selectedLog.value)
  if (!file) return

  const blob = new Blob([content.value], { type: 'text/markdown' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = `shadow-${selectedLog.value}-${Date.now()}.md`
  a.click()
  URL.revokeObjectURL(url)
}

function selectLog(key: string) {
  selectedLog.value = key
  loadLog(key)
}

function startAutoRefresh() {
  if (refreshTimer) clearInterval(refreshTimer)
  if (!autoRefresh.value) return

  refreshTimer = window.setInterval(() => {
    if (autoRefresh.value) {
      loadLog(selectedLog.value)
    }
  }, 30000) // 30秒
}

function toggleAutoRefresh() {
  autoRefresh.value = !autoRefresh.value
  if (autoRefresh.value) {
    startAutoRefresh()
  } else if (refreshTimer) {
    clearInterval(refreshTimer)
    refreshTimer = null
  }
}

onMounted(() => {
  loadLog(selectedLog.value)
  startAutoRefresh()
})

onBeforeUnmount(() => {
  if (refreshTimer) {
    clearInterval(refreshTimer)
  }
})
</script>

<template>
  <div class="shadow-logs">
    <div class="logs-container">
      <!-- 左侧文件列表 -->
      <div class="logs-sidebar">
        <div class="sidebar-header">
          <h3>日志文件</h3>
        </div>
        <div class="log-list">
          <div
            v-for="file in logFiles"
            :key="file.key"
            :class="['log-item', { active: selectedLog === file.key }]"
            @click="selectLog(file.key)"
          >
            <span class="log-icon">📄</span>
            <span class="log-label">{{ file.label }}</span>
          </div>
        </div>
      </div>

      <!-- 右侧内容展示 -->
      <div class="logs-content">
        <div class="content-header">
          <div class="header-left">
            <h3>{{ logFiles.find(f => f.key === selectedLog)?.label }}</h3>
            <span v-if="loading" class="loading-indicator">加载中...</span>
          </div>
          <div class="header-actions">
            <button
              :class="['action-btn', { active: autoRefresh }]"
              @click="toggleAutoRefresh"
              title="自动刷新（30秒）"
            >
              <span class="btn-icon">🔄</span>
              {{ autoRefresh ? '自动刷新' : '手动刷新' }}
            </button>
            <button
              class="action-btn"
              @click="loadLog(selectedLog)"
              :disabled="loading"
              title="立即刷新"
            >
              <span class="btn-icon">↻</span>
              刷新
            </button>
            <button
              class="action-btn"
              @click="downloadLog"
              :disabled="!content"
              title="导出日志"
            >
              <span class="btn-icon">⬇</span>
              导出
            </button>
            <button
              class="action-btn danger"
              @click="clearLog"
              :disabled="!content || loading"
              title="清空日志"
            >
              <span class="btn-icon">🗑</span>
              清空
            </button>
          </div>
        </div>

        <div class="content-body">
          <div
            v-if="loading"
            class="loading-state"
          >
            <div class="spinner"></div>
            <p>加载中...</p>
          </div>
          <div
            v-else
            class="markdown-content"
            v-html="renderedContent"
          ></div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.shadow-logs {
  height: 100%;
  display: flex;
  flex-direction: column;
}

.logs-container {
  display: flex;
  height: 100%;
  gap: 0;
}

.logs-sidebar {
  width: 220px;
  background: var(--bg-secondary);
  border-right: 1px solid var(--border);
  display: flex;
  flex-direction: column;
}

.sidebar-header {
  padding: 16px;
  border-bottom: 1px solid var(--border);
}

.sidebar-header h3 {
  margin: 0;
  font-size: 14px;
  font-weight: 600;
  color: var(--text-primary);
}

.log-list {
  flex: 1;
  overflow-y: auto;
  padding: 8px;
}

.log-item {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 10px 12px;
  border-radius: 6px;
  cursor: pointer;
  transition: all 0.2s;
  margin-bottom: 4px;
}

.log-item:hover {
  background: var(--bg-hover);
}

.log-item.active {
  background: var(--primary);
  color: white;
}

.log-icon {
  font-size: 16px;
}

.log-label {
  font-size: 14px;
  font-weight: 500;
}

.logs-content {
  flex: 1;
  display: flex;
  flex-direction: column;
  background: var(--bg-primary);
}

.content-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 16px 20px;
  border-bottom: 1px solid var(--border);
  background: var(--bg-secondary);
}

.header-left {
  display: flex;
  align-items: center;
  gap: 12px;
}

.header-left h3 {
  margin: 0;
  font-size: 16px;
  font-weight: 600;
  color: var(--text-primary);
}

.loading-indicator {
  font-size: 12px;
  color: var(--text-secondary);
}

.header-actions {
  display: flex;
  gap: 8px;
}

.action-btn {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 6px 12px;
  background: var(--bg-primary);
  border: 1px solid var(--border);
  border-radius: 6px;
  cursor: pointer;
  font-size: 13px;
  color: var(--text-primary);
  transition: all 0.2s;
}

.action-btn:hover:not(:disabled) {
  background: var(--bg-hover);
  border-color: var(--primary);
}

.action-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.action-btn.active {
  background: var(--primary);
  color: white;
  border-color: var(--primary);
}

.action-btn.danger:hover:not(:disabled) {
  background: #fee;
  border-color: #ef4444;
  color: #ef4444;
}

.btn-icon {
  font-size: 14px;
}

.content-body {
  flex: 1;
  overflow-y: auto;
  padding: 20px;
}

.loading-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  height: 200px;
  gap: 12px;
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
  max-width: 900px;
  margin: 0 auto;
  line-height: 1.6;
  color: var(--text-primary);
}

.markdown-content :deep(h1),
.markdown-content :deep(h2),
.markdown-content :deep(h3),
.markdown-content :deep(h4),
.markdown-content :deep(h5),
.markdown-content :deep(h6) {
  margin-top: 24px;
  margin-bottom: 12px;
  font-weight: 600;
  color: var(--text-primary);
}

.markdown-content :deep(h1) { font-size: 24px; }
.markdown-content :deep(h2) { font-size: 20px; }
.markdown-content :deep(h3) { font-size: 18px; }

.markdown-content :deep(p) {
  margin: 12px 0;
}

.markdown-content :deep(ul),
.markdown-content :deep(ol) {
  margin: 12px 0;
  padding-left: 24px;
}

.markdown-content :deep(li) {
  margin: 6px 0;
}

.markdown-content :deep(code) {
  background: var(--bg-secondary);
  padding: 2px 6px;
  border-radius: 3px;
  font-family: monospace;
  font-size: 13px;
}

.markdown-content :deep(pre) {
  background: var(--bg-secondary);
  padding: 12px;
  border-radius: 6px;
  overflow-x: auto;
  margin: 12px 0;
}

.markdown-content :deep(pre code) {
  background: none;
  padding: 0;
}

.markdown-content :deep(blockquote) {
  border-left: 3px solid var(--primary);
  padding-left: 12px;
  margin: 12px 0;
  color: var(--text-secondary);
}

.markdown-content :deep(table) {
  border-collapse: collapse;
  width: 100%;
  margin: 12px 0;
}

.markdown-content :deep(th),
.markdown-content :deep(td) {
  border: 1px solid var(--border);
  padding: 8px 12px;
  text-align: left;
}

.markdown-content :deep(th) {
  background: var(--bg-secondary);
  font-weight: 600;
}

.markdown-content :deep(a) {
  color: var(--primary);
  text-decoration: none;
}

.markdown-content :deep(a:hover) {
  text-decoration: underline;
}

.empty-hint,
.error-hint {
  text-align: center;
  padding: 40px;
  color: var(--text-secondary);
  font-size: 14px;
}

.error-hint {
  color: #ef4444;
}
</style>
