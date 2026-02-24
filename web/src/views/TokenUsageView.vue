<script setup lang="ts">
import { ref, computed, onMounted, watch, nextTick, onUnmounted } from 'vue'
import * as api from '../composables/api'
import type { DailyTokenUsage, SessionTokenRanking, HourlyTokenUsage } from '../composables/api'
import type { TokenUsageStats } from '../types'
import { Chart, registerables } from 'chart.js'

Chart.register(...registerables)

const stats = ref<TokenUsageStats>({ total_input_tokens: 0, total_output_tokens: 0, total_cache_creation_tokens: 0, total_cache_read_tokens: 0, count: 0 })
const daily = ref<DailyTokenUsage[]>([])
const ranking = ref<SessionTokenRanking[]>([])
const hourly = ref<HourlyTokenUsage[]>([])
const sessions = ref<{ id: number; title: string }[]>([])
const selectedSessionId = ref(0)
const loading = ref(true)

// Time range
type RangeKey = 'today' | '7d' | '30d' | 'custom'
const rangeKey = ref<RangeKey>('30d')
const customStart = ref('')
const customEnd = ref('')

function getRange(): { start: string; end: string } {
  const now = new Date()
  const fmt = (d: Date) => d.toISOString().slice(0, 10)
  if (rangeKey.value === 'today') return { start: fmt(now), end: fmt(new Date(now.getTime() + 86400000)) }
  if (rangeKey.value === '7d') return { start: fmt(new Date(now.getTime() - 6 * 86400000)), end: fmt(new Date(now.getTime() + 86400000)) }
  if (rangeKey.value === '30d') return { start: fmt(new Date(now.getTime() - 29 * 86400000)), end: fmt(new Date(now.getTime() + 86400000)) }
  return { start: customStart.value, end: customEnd.value }
}

const totalTokens = computed(() => stats.value.total_input_tokens + stats.value.total_output_tokens + (stats.value.total_cache_creation_tokens || 0) + (stats.value.total_cache_read_tokens || 0))

// Chart refs
const areaCanvas = ref<HTMLCanvasElement>()
const barCanvas = ref<HTMLCanvasElement>()
const pieCanvas = ref<HTMLCanvasElement>()
const hourlyCanvas = ref<HTMLCanvasElement>()
let areaChart: Chart | null = null
let barChart: Chart | null = null
let pieChart: Chart | null = null
let hourlyChart: Chart | null = null

// Detail table pagination
const page = ref(1)
const pageSize = 20
const pagedRecords = computed(() => {
  const start = (page.value - 1) * pageSize
  return daily.value.slice(start, start + pageSize)
})
const totalPages = computed(() => Math.max(1, Math.ceil(daily.value.length / pageSize)))

async function loadData() {
  loading.value = true
  const { start, end } = getRange()
  try {
    const [s, d, r, h, sess] = await Promise.all([
      api.getSystemTokenUsage(start, end),
      api.getDailyTokenUsage(start, end),
      api.getTokenUsageRanking(start, end, 10),
      api.getHourlyTokenUsage(start, end, selectedSessionId.value),
      api.listSessions(),
    ])
    stats.value = s
    daily.value = d
    ranking.value = r
    hourly.value = h
    sessions.value = sess.map(x => ({ id: x.id, title: x.title || `#${x.id}` }))
    page.value = 1
  } catch (e) {
    console.error('Failed to load token usage data', e)
  } finally {
    loading.value = false
  }
  await nextTick()
  renderCharts()
}

function renderCharts() {
  renderAreaChart()
  renderBarChart()
  renderPieChart()
  renderHourlyChart()
}

function renderHourlyChart() {
  if (hourlyChart) hourlyChart.destroy()
  if (!hourlyCanvas.value) return
  const labels = hourly.value.map(h => h.hour.slice(5))
  hourlyChart = new Chart(hourlyCanvas.value, {
    type: 'bar',
    data: {
      labels,
      datasets: [
        { label: 'Input', data: hourly.value.map(h => h.input_tokens), backgroundColor: 'rgba(124,106,239,0.7)' },
        { label: 'Output', data: hourly.value.map(h => h.output_tokens), backgroundColor: 'rgba(34,197,94,0.7)' },
        { label: 'Cache Write', data: hourly.value.map(h => h.cache_creation_input_tokens), backgroundColor: 'rgba(245,158,11,0.7)' },
        { label: 'Cache Read', data: hourly.value.map(h => h.cache_read_input_tokens), backgroundColor: 'rgba(6,182,212,0.7)' },
      ],
    },
    options: { responsive: true, maintainAspectRatio: false, plugins: { legend: { position: 'top' } }, scales: { x: { stacked: true }, y: { stacked: true, beginAtZero: true } } },
  })
}

async function onSessionChange() {
  const { start, end } = getRange()
  hourly.value = await api.getHourlyTokenUsage(start, end, selectedSessionId.value)
  await nextTick()
  renderHourlyChart()
}

function renderAreaChart() {
  if (areaChart) areaChart.destroy()
  if (!areaCanvas.value) return
  const labels = daily.value.map(d => d.date)
  areaChart = new Chart(areaCanvas.value, {
    type: 'line',
    data: {
      labels,
      datasets: [
        { label: 'Input', data: daily.value.map(d => d.input_tokens), borderColor: '#7c6aef', backgroundColor: 'rgba(124,106,239,0.15)', fill: true, tension: 0.3 },
        { label: 'Output', data: daily.value.map(d => d.output_tokens), borderColor: '#22c55e', backgroundColor: 'rgba(34,197,94,0.15)', fill: true, tension: 0.3 },
        { label: 'Cache Write', data: daily.value.map(d => d.cache_creation_input_tokens), borderColor: '#f59e0b', backgroundColor: 'rgba(245,158,11,0.15)', fill: true, tension: 0.3 },
        { label: 'Cache Read', data: daily.value.map(d => d.cache_read_input_tokens), borderColor: '#06b6d4', backgroundColor: 'rgba(6,182,212,0.15)', fill: true, tension: 0.3 },
      ],
    },
    options: { responsive: true, maintainAspectRatio: false, plugins: { legend: { position: 'top' } }, scales: { y: { beginAtZero: true } } },
  })
}

function renderBarChart() {
  if (barChart) barChart.destroy()
  if (!barCanvas.value) return
  barChart = new Chart(barCanvas.value, {
    type: 'bar',
    data: {
      labels: ranking.value.map(r => r.title || `#${r.session_id}`),
      datasets: [
        { label: 'Input', data: ranking.value.map(r => r.input_tokens), backgroundColor: 'rgba(124,106,239,0.7)' },
        { label: 'Output', data: ranking.value.map(r => r.output_tokens), backgroundColor: 'rgba(34,197,94,0.7)' },
      ],
    },
    options: { responsive: true, maintainAspectRatio: false, indexAxis: 'y', plugins: { legend: { position: 'top' } }, scales: { x: { stacked: true, beginAtZero: true }, y: { stacked: true } } },
  })
}

function renderPieChart() {
  if (pieChart) pieChart.destroy()
  if (!pieCanvas.value) return
  pieChart = new Chart(pieCanvas.value, {
    type: 'doughnut',
    data: {
      labels: ['Input', 'Output', 'Cache Write', 'Cache Read'],
      datasets: [{ data: [stats.value.total_input_tokens, stats.value.total_output_tokens, stats.value.total_cache_creation_tokens || 0, stats.value.total_cache_read_tokens || 0], backgroundColor: ['rgba(124,106,239,0.8)', 'rgba(34,197,94,0.8)', 'rgba(245,158,11,0.8)', 'rgba(6,182,212,0.8)'] }],
    },
    options: { responsive: true, maintainAspectRatio: false, plugins: { legend: { position: 'bottom' } } },
  })
}

function formatNum(n: number): string {
  if (n >= 1000000) return (n / 1000000).toFixed(1) + 'M'
  if (n >= 1000) return (n / 1000).toFixed(1) + 'K'
  return String(n)
}

onMounted(loadData)
watch(rangeKey, loadData)
onUnmounted(() => { areaChart?.destroy(); barChart?.destroy(); pieChart?.destroy(); hourlyChart?.destroy() })

// __CONTINUE_HERE__
</script>

<template>
  <div class="token-usage-page">
    <div class="page-header">
      <h2 class="page-title">用量统计</h2>
      <div class="range-selector">
        <button v-for="r in (['today','7d','30d'] as RangeKey[])" :key="r" class="range-btn" :class="{ active: rangeKey === r }" @click="rangeKey = r">
          {{ r === 'today' ? '今天' : r === '7d' ? '7天' : '30天' }}
        </button>
        <button class="range-btn" :class="{ active: rangeKey === 'custom' }" @click="rangeKey = 'custom'">自定义</button>
        <template v-if="rangeKey === 'custom'">
          <input type="date" v-model="customStart" class="date-input" @change="loadData" />
          <span class="date-sep">~</span>
          <input type="date" v-model="customEnd" class="date-input" @change="loadData" />
        </template>
      </div>
    </div>

    <div v-if="loading" class="loading-text">加载中...</div>
    <template v-else>
      <div class="overview-cards">
        <div class="stat-card">
          <div class="stat-label">Input Tokens</div>
          <div class="stat-value input">{{ formatNum(stats.total_input_tokens) }}</div>
        </div>
        <div class="stat-card">
          <div class="stat-label">Output Tokens</div>
          <div class="stat-value output">{{ formatNum(stats.total_output_tokens) }}</div>
        </div>
        <div class="stat-card">
          <div class="stat-label">Cache Write</div>
          <div class="stat-value cache-w">{{ formatNum(stats.total_cache_creation_tokens || 0) }}</div>
        </div>
        <div class="stat-card">
          <div class="stat-label">Cache Read</div>
          <div class="stat-value cache-r">{{ formatNum(stats.total_cache_read_tokens || 0) }}</div>
        </div>
        <div class="stat-card">
          <div class="stat-label">总计</div>
          <div class="stat-value">{{ formatNum(totalTokens) }}</div>
        </div>
        <div class="stat-card">
          <div class="stat-label">对话次数</div>
          <div class="stat-value">{{ stats.count }}</div>
        </div>
      </div>

      <div class="charts-row">
        <div class="chart-box wide">
          <div class="chart-title">每日用量趋势</div>
          <div class="chart-wrap"><canvas ref="areaCanvas"></canvas></div>
        </div>
        <div class="chart-box narrow">
          <div class="chart-title">Input / Output 占比</div>
          <div class="chart-wrap pie-wrap"><canvas ref="pieCanvas"></canvas></div>
        </div>
      </div>

      <div class="chart-box full">
        <div class="chart-title">会话消耗排行 Top 10</div>
        <div class="chart-wrap bar-wrap"><canvas ref="barCanvas"></canvas></div>
      </div>

      <div class="chart-box full">
        <div class="chart-header">
          <div class="chart-title">小时用量趋势</div>
          <select class="session-select" v-model="selectedSessionId" @change="onSessionChange">
            <option :value="0">全部会话</option>
            <option v-for="s in sessions" :key="s.id" :value="s.id">{{ s.title }}</option>
          </select>
        </div>
        <div class="chart-wrap bar-wrap"><canvas ref="hourlyCanvas"></canvas></div>
      </div>

      <div class="detail-section">
        <div class="chart-title">每日明细</div>
        <table class="detail-table">
          <thead>
            <tr><th>日期</th><th>Input</th><th>Output</th><th>Cache Write</th><th>Cache Read</th><th>合计</th></tr>
          </thead>
          <tbody>
            <tr v-for="d in pagedRecords" :key="d.date">
              <td>{{ d.date }}</td>
              <td>{{ d.input_tokens.toLocaleString() }}</td>
              <td>{{ d.output_tokens.toLocaleString() }}</td>
              <td>{{ (d.cache_creation_input_tokens || 0).toLocaleString() }}</td>
              <td>{{ (d.cache_read_input_tokens || 0).toLocaleString() }}</td>
              <td>{{ (d.input_tokens + d.output_tokens + (d.cache_creation_input_tokens || 0) + (d.cache_read_input_tokens || 0)).toLocaleString() }}</td>
            </tr>
            <tr v-if="daily.length === 0"><td colspan="6" class="empty-row">暂无数据</td></tr>
          </tbody>
        </table>
        <div v-if="totalPages > 1" class="pagination">
          <button :disabled="page <= 1" @click="page--">&lt;</button>
          <span>{{ page }} / {{ totalPages }}</span>
          <button :disabled="page >= totalPages" @click="page++">&gt;</button>
        </div>
      </div>

      <!-- __CONTINUE_TEMPLATE__ -->
    </template>
  </div>
</template>

<style scoped>
.token-usage-page { padding: 24px; overflow-y: auto; height: 100vh; }
.page-header { display: flex; align-items: center; justify-content: space-between; margin-bottom: 20px; flex-wrap: wrap; gap: 12px; }
.page-title { font-size: 18px; font-weight: 600; color: var(--text-primary); margin: 0; }
.range-selector { display: flex; align-items: center; gap: 6px; flex-wrap: wrap; }
.range-btn {
  padding: 5px 14px; border-radius: var(--radius-sm); font-size: 12px;
  color: var(--text-secondary); background: var(--bg-tertiary); transition: all var(--transition); cursor: pointer; border: none;
}
.range-btn:hover { background: var(--bg-hover); color: var(--text-primary); }
.range-btn.active { background: var(--accent); color: var(--btn-text); }
.date-input {
  padding: 4px 8px; border-radius: var(--radius-sm); border: 1px solid var(--border);
  background: var(--bg-secondary); color: var(--text-primary); font-size: 12px;
}
.date-sep { color: var(--text-muted); font-size: 12px; }
.loading-text { text-align: center; color: var(--text-muted); padding: 60px 0; font-size: 14px; }
.overview-cards { display: grid; grid-template-columns: repeat(3, 1fr); gap: 12px; margin-bottom: 20px; }
.stat-card {
  background: var(--bg-secondary); border: 1px solid var(--border); border-radius: var(--radius);
  padding: 16px; text-align: center;
}
.stat-label { font-size: 12px; color: var(--text-muted); margin-bottom: 6px; }
.stat-value { font-size: 22px; font-weight: 700; color: var(--text-primary); }
.stat-value.input { color: #7c6aef; }
.stat-value.output { color: #22c55e; }
.stat-value.cache-w { color: #f59e0b; }
.stat-value.cache-r { color: #06b6d4; }
/* __CONTINUE_STYLE__ */
.charts-row { display: flex; gap: 12px; margin-bottom: 16px; }
.chart-box {
  background: var(--bg-secondary); border: 1px solid var(--border); border-radius: var(--radius);
  padding: 16px; display: flex; flex-direction: column;
}
.chart-box.wide { flex: 2; }
.chart-box.narrow { flex: 1; }
.chart-box.full { margin-bottom: 16px; }
.chart-title { font-size: 13px; font-weight: 600; color: var(--text-secondary); margin-bottom: 12px; }
.chart-header { display: flex; align-items: center; justify-content: space-between; margin-bottom: 12px; }
.chart-header .chart-title { margin-bottom: 0; }
.session-select {
  padding: 4px 8px; border-radius: var(--radius-sm); border: 1px solid var(--border);
  background: var(--bg-tertiary); color: var(--text-primary); font-size: 12px; max-width: 200px;
}
.chart-wrap { position: relative; height: 220px; }
.pie-wrap { height: 200px; }
.bar-wrap { height: 260px; }
.detail-section {
  background: var(--bg-secondary); border: 1px solid var(--border); border-radius: var(--radius);
  padding: 16px; margin-bottom: 24px;
}
.detail-table { width: 100%; border-collapse: collapse; font-size: 13px; }
.detail-table th {
  text-align: left; padding: 8px 12px; color: var(--text-muted); font-weight: 500;
  border-bottom: 1px solid var(--border); font-size: 12px;
}
.detail-table td { padding: 8px 12px; color: var(--text-primary); border-bottom: 1px solid var(--border); }
.detail-table tr:hover td { background: var(--bg-hover); }
.empty-row { text-align: center; color: var(--text-muted); padding: 24px 0 !important; }
.pagination {
  display: flex; align-items: center; justify-content: center; gap: 12px; margin-top: 12px;
}
.pagination button {
  padding: 4px 12px; border-radius: var(--radius-sm); font-size: 12px;
  background: var(--bg-tertiary); color: var(--text-secondary); border: none; cursor: pointer;
}
.pagination button:hover:not(:disabled) { background: var(--bg-hover); color: var(--text-primary); }
.pagination button:disabled { opacity: 0.4; cursor: not-allowed; }
.pagination span { font-size: 12px; color: var(--text-muted); }
@media (max-width: 640px) {
  .overview-cards { grid-template-columns: repeat(2, 1fr); }
  .charts-row { flex-direction: column; }
}
</style>

