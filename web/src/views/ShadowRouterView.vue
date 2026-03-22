<script setup lang="ts">
import { ref, onMounted } from 'vue'

interface InjectionRoute {
  id: number
  keywords: string
  inject_categories: string
  created_at: string
  updated_at: string
}

interface RouterData {
  routes: InjectionRoute[]
  categories: string[]
  fixed: string[]
  conditional: string[]
}

const routerData = ref<RouterData>({
  routes: [],
  categories: [],
  fixed: [],
  conditional: []
})
const loading = ref(false)
const showDialog = ref(false)
const editingRoute = ref<InjectionRoute | null>(null)
const formData = ref({
  keywords: '',
  inject_categories: ''
})

async function loadRoutes() {
  loading.value = true
  try {
    const res = await fetch('/api/v1/injection-router')
    routerData.value = await res.json()
  } catch (err) {
    console.error('Failed to load routes:', err)
  } finally {
    loading.value = false
  }
}

function openCreateDialog() {
  editingRoute.value = null
  formData.value = {
    keywords: '',
    inject_categories: ''
  }
  showDialog.value = true
}

function openEditDialog(route: InjectionRoute) {
  editingRoute.value = route
  formData.value = {
    keywords: route.keywords,
    inject_categories: route.inject_categories
  }
  showDialog.value = true
}

function closeDialog() {
  showDialog.value = false
  editingRoute.value = null
  formData.value = {
    keywords: '',
    inject_categories: ''
  }
}

async function saveRoute() {
  if (!formData.value.keywords.trim() || !formData.value.inject_categories.trim()) {
    alert('关键词和注入分类不能为空')
    return
  }

  try {
    if (editingRoute.value) {
      // 更新路由
      await fetch(`/api/v1/injection-router/${editingRoute.value.id}`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(formData.value)
      })
    } else {
      // 创建路由
      await fetch('/api/v1/injection-router', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(formData.value)
      })
    }
    await loadRoutes()
    closeDialog()
    alert(editingRoute.value ? '更新成功' : '创建成功')
  } catch (err) {
    console.error('Failed to save route:', err)
    alert('保存失败')
  }
}

async function deleteRoute(route: InjectionRoute) {
  if (!confirm(`确定要删除路由 "${route.keywords}" 吗？`)) {
    return
  }

  try {
    await fetch(`/api/v1/injection-router/${route.id}`, {
      method: 'DELETE'
    })
    await loadRoutes()
    alert('删除成功')
  } catch (err) {
    console.error('Failed to delete route:', err)
    alert('删除失败')
  }
}

onMounted(() => {
  loadRoutes()
})
</script>

<template>
  <div class="shadow-router">
    <!-- 使用指南 -->
    <div class="guide-section">
      <h3>注入路由配置</h3>
      <p class="guide-desc">配置关键词触发规则，当用户消息包含关键词时，自动注入对应的结构化记忆分类。</p>

      <div class="category-info">
        <div class="info-card">
          <div class="info-header">
            <span class="info-icon">📌</span>
            <span class="info-title">固定分类（始终注入）</span>
          </div>
          <div class="info-content">
            <span v-for="cat in routerData.fixed" :key="cat" class="category-tag fixed">{{ cat }}</span>
          </div>
        </div>

        <div class="info-card">
          <div class="info-header">
            <span class="info-icon">🔀</span>
            <span class="info-title">条件分类（按关键词注入）</span>
          </div>
          <div class="info-content">
            <span v-for="cat in routerData.conditional" :key="cat" class="category-tag conditional">{{ cat }}</span>
          </div>
        </div>
      </div>
    </div>

    <!-- 操作栏 -->
    <div class="action-bar">
      <button class="create-btn" @click="openCreateDialog">
        ➕ 新增路由
      </button>
    </div>

    <!-- 路由列表 -->
    <div class="routes-section">
      <div v-if="loading" class="loading">
        加载中...
      </div>
      <div v-else-if="routerData.routes.length === 0" class="empty-state">
        <p>暂无路由配置</p>
        <button class="create-btn" @click="openCreateDialog">
          创建第一个路由
        </button>
      </div>
      <table v-else class="routes-table">
        <thead>
          <tr>
            <th>ID</th>
            <th>关键词</th>
            <th>注入分类</th>
            <th>创建时间</th>
            <th>操作</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="route in routerData.routes" :key="route.id">
            <td>{{ route.id }}</td>
            <td class="keywords-cell">{{ route.keywords }}</td>
            <td class="categories-cell">
              <span v-for="cat in route.inject_categories.split(',')" :key="cat" class="category-tag">
                {{ cat.trim() }}
              </span>
            </td>
            <td class="time-cell">{{ new Date(route.created_at).toLocaleString('zh-CN') }}</td>
            <td class="actions-cell">
              <button class="action-btn edit" @click="openEditDialog(route)">
                ✏️ 编辑
              </button>
              <button class="action-btn delete" @click="deleteRoute(route)">
                🗑️ 删除
              </button>
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <!-- 新增/编辑对话框 -->
    <div v-if="showDialog" class="dialog-overlay" @click.self="closeDialog">
      <div class="dialog">
        <div class="dialog-header">
          <h3>{{ editingRoute ? '编辑路由' : '新增路由' }}</h3>
          <button class="close-btn" @click="closeDialog">✕</button>
        </div>

        <div class="dialog-body">
          <div class="form-item">
            <label>关键词 <span class="required">*</span></label>
            <input
              v-model="formData.keywords"
              type="text"
              class="form-input"
              placeholder="例如：项目开发、bug修复"
            />
            <span class="form-hint">多个关键词用逗号分隔</span>
          </div>

          <div class="form-item">
            <label>注入分类 <span class="required">*</span></label>
            <input
              v-model="formData.inject_categories"
              type="text"
              class="form-input"
              placeholder="例如：domain,lessons"
            />
            <span class="form-hint">多个分类用逗号分隔，可选：{{ routerData.conditional.join(', ') }}</span>
          </div>
        </div>

        <div class="dialog-footer">
          <button class="cancel-btn" @click="closeDialog">取消</button>
          <button class="save-btn" @click="saveRoute">保存</button>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.shadow-router {
  padding: 24px;
  max-width: 1200px;
  margin: 0 auto;
}

/* 使用指南 */
.guide-section {
  background: var(--bg-secondary);
  border: 1px solid var(--border);
  border-radius: 8px;
  padding: 20px;
  margin-bottom: 24px;
}

.guide-section h3 {
  margin: 0 0 8px 0;
  font-size: 18px;
  font-weight: 600;
  color: var(--text-primary);
}

.guide-desc {
  margin: 0 0 16px 0;
  font-size: 14px;
  color: var(--text-secondary);
  line-height: 1.6;
}

.category-info {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(300px, 1fr));
  gap: 16px;
}

.info-card {
  background: var(--bg-primary);
  border: 1px solid var(--border);
  border-radius: 6px;
  padding: 16px;
}

.info-header {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 12px;
}

.info-icon {
  font-size: 18px;
}

.info-title {
  font-size: 14px;
  font-weight: 600;
  color: var(--text-primary);
}

.info-content {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.category-tag {
  padding: 4px 12px;
  border-radius: 12px;
  font-size: 12px;
  font-weight: 500;
  background: var(--bg-hover);
  color: var(--text-primary);
}

.category-tag.fixed {
  background: rgba(59, 130, 246, 0.1);
  color: #3b82f6;
}

.category-tag.conditional {
  background: rgba(16, 185, 129, 0.1);
  color: #10b981;
}

/* 操作栏 */
.action-bar {
  margin-bottom: 16px;
}

.create-btn {
  padding: 10px 20px;
  background: var(--primary);
  color: white;
  border: none;
  border-radius: 6px;
  cursor: pointer;
  font-size: 14px;
  font-weight: 500;
  transition: all 0.2s;
}

.create-btn:hover {
  opacity: 0.9;
}

/* 路由列表 */
.routes-section {
  background: var(--bg-secondary);
  border: 1px solid var(--border);
  border-radius: 8px;
  overflow: hidden;
}

.loading {
  text-align: center;
  padding: 40px;
  color: var(--text-secondary);
}

.empty-state {
  text-align: center;
  padding: 60px 20px;
}

.empty-state p {
  margin: 0 0 20px 0;
  color: var(--text-secondary);
  font-size: 16px;
}

.routes-table {
  width: 100%;
  border-collapse: collapse;
  table-layout: fixed;
}

.routes-table thead {
  background: var(--bg-hover);
}

.routes-table th {
  padding: 12px 16px;
  text-align: left;
  font-size: 13px;
  font-weight: 600;
  color: var(--text-secondary);
  text-transform: uppercase;
  border-bottom: 1px solid var(--border);
}

/* 列宽优化 */
.routes-table th:nth-child(1) { width: 80px; }   /* ID */
.routes-table th:nth-child(2) { width: 30%; }    /* 关键词 */
.routes-table th:nth-child(3) { width: 30%; }    /* 注入分类 */
.routes-table th:nth-child(4) { width: 150px; }  /* 创建时间 */
.routes-table th:nth-child(5) { width: 200px; }  /* 操作 */

.routes-table td {
  padding: 16px;
  border-bottom: 1px solid var(--border);
  color: var(--text-primary);
  overflow: hidden;
  text-overflow: ellipsis;
}

.routes-table tbody tr:hover {
  background: var(--bg-hover);
}

.keywords-cell {
  font-weight: 500;
}

.categories-cell {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}

.time-cell {
  font-size: 13px;
  color: var(--text-secondary);
}

.actions-cell {
  display: flex;
  gap: 8px;
}

.action-btn {
  padding: 6px 12px;
  border: 1px solid var(--border);
  border-radius: 4px;
  cursor: pointer;
  font-size: 13px;
  transition: all 0.2s;
  background: var(--bg-primary);
  color: var(--text-primary);
}

.action-btn:hover {
  background: var(--bg-hover);
}

.action-btn.edit {
  border-color: #3b82f6;
  color: #3b82f6;
}

.action-btn.delete {
  border-color: #ef4444;
  color: #ef4444;
}

/* 对话框 */
.dialog-overlay {
  position: fixed;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  background: rgba(0, 0, 0, 0.5);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 1000;
}

.dialog {
  background: var(--bg-secondary);
  border-radius: 8px;
  width: 90%;
  max-width: 500px;
  box-shadow: 0 20px 25px -5px rgba(0, 0, 0, 0.1);
}

.dialog-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 20px;
  border-bottom: 1px solid var(--border);
}

.dialog-header h3 {
  margin: 0;
  font-size: 18px;
  font-weight: 600;
  color: var(--text-primary);
}

.close-btn {
  width: 32px;
  height: 32px;
  border: none;
  background: none;
  cursor: pointer;
  font-size: 20px;
  color: var(--text-secondary);
  border-radius: 4px;
  transition: all 0.2s;
}

.close-btn:hover {
  background: var(--bg-hover);
}

.dialog-body {
  padding: 20px;
  display: flex;
  flex-direction: column;
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

.required {
  color: #ef4444;
}

.form-input {
  padding: 10px 12px;
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

.dialog-footer {
  display: flex;
  justify-content: flex-end;
  gap: 12px;
  padding: 20px;
  border-top: 1px solid var(--border);
}

.cancel-btn,
.save-btn {
  padding: 10px 20px;
  border: none;
  border-radius: 6px;
  cursor: pointer;
  font-size: 14px;
  font-weight: 500;
  transition: all 0.2s;
}

.cancel-btn {
  background: var(--bg-hover);
  color: var(--text-primary);
}

.cancel-btn:hover {
  background: var(--bg-primary);
}

.save-btn {
  background: var(--primary);
  color: white;
}

.save-btn:hover {
  opacity: 0.9;
}
</style>
