<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { marked } from 'marked'

interface CategoryInfo {
  category: string
  label: string
  has_data: boolean
  fixed: boolean
}

interface MemoryContent {
  category: string
  label: string
  content: string
}

const categories = ref<CategoryInfo[]>([])
const selectedCategory = ref<string | null>(null)
const content = ref('')
const isEditing = ref(false)
const loading = ref(false)
const saving = ref(false)

const fixedCategories = computed(() => categories.value.filter(c => c.fixed))
const conditionalCategories = computed(() => categories.value.filter(c => !c.fixed))

const renderedContent = computed(() => {
  if (!content.value) return '<p class="empty-hint">暂无内容</p>'
  try {
    return marked(content.value)
  } catch (err) {
    console.error('Markdown render error:', err)
    return '<p class="error-hint">Markdown 渲染失败</p>'
  }
})

async function loadCategories() {
  loading.value = true
  try {
    const res = await fetch('/api/v1/structured-memory')
    const data = await res.json()
    categories.value = data.categories || []
  } catch (err) {
    console.error('Failed to load categories:', err)
  } finally {
    loading.value = false
  }
}

async function loadContent(category: string) {
  loading.value = true
  try {
    const res = await fetch(`/api/v1/structured-memory/${category}`)
    const data: MemoryContent = await res.json()
    content.value = data.content || ''
    selectedCategory.value = category
    isEditing.value = false
  } catch (err) {
    console.error('Failed to load content:', err)
    content.value = ''
  } finally {
    loading.value = false
  }
}

async function saveContent() {
  if (!selectedCategory.value) return

  saving.value = true
  try {
    await fetch(`/api/v1/structured-memory/${selectedCategory.value}`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ content: content.value })
    })
    await loadCategories()
    isEditing.value = false
    alert('保存成功')
  } catch (err) {
    console.error('Failed to save content:', err)
    alert('保存失败')
  } finally {
    saving.value = false
  }
}

function selectCategory(category: string) {
  loadContent(category)
}

onMounted(() => {
  loadCategories()
})
</script>

<template>
  <div class="shadow-memory">
    <div v-if="loading && categories.length === 0" class="loading">
      加载中...
    </div>
    <div v-else class="memory-container">
      <!-- 左侧分类列表 -->
      <div class="categories-sidebar">
        <div class="sidebar-header">
          <h3>结构化记忆</h3>
        </div>

        <!-- 固定分类 -->
        <div class="category-group">
          <div class="group-label">固定分类（始终注入）</div>
          <div class="category-list">
            <div
              v-for="cat in fixedCategories"
              :key="cat.category"
              :class="['category-item', { active: selectedCategory === cat.category, 'has-data': cat.has_data }]"
              @click="selectCategory(cat.category)"
            >
              <svg class="category-icon" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                <path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/>
                <polyline points="14 2 14 8 20 8"/>
                <line x1="16" y1="13" x2="8" y2="13"/>
                <line x1="16" y1="17" x2="8" y2="17"/>
                <polyline points="10 9 9 9 8 9"/>
              </svg>
              <span class="category-label">{{ cat.label }}</span>
              <span v-if="cat.has_data" class="data-badge">●</span>
            </div>
          </div>
        </div>

        <!-- 条件分类 -->
        <div class="category-group">
          <div class="group-label">条件分类（按关键词注入）</div>
          <div class="category-list">
            <div
              v-for="cat in conditionalCategories"
              :key="cat.category"
              :class="['category-item', { active: selectedCategory === cat.category, 'has-data': cat.has_data }]"
              @click="selectCategory(cat.category)"
            >
              <svg class="category-icon" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                <path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/>
                <polyline points="14 2 14 8 20 8"/>
                <line x1="16" y1="13" x2="8" y2="13"/>
                <line x1="16" y1="17" x2="8" y2="17"/>
                <polyline points="10 9 9 9 8 9"/>
              </svg>
              <span class="category-label">{{ cat.label }}</span>
              <span v-if="cat.has_data" class="data-badge">●</span>
            </div>
          </div>
        </div>
      </div>

      <!-- 右侧内容展示 -->
      <div class="content-area">
        <div v-if="!selectedCategory" class="empty-state">
          <p>请从左侧选择一个分类查看内容</p>
        </div>
        <div v-else class="content-wrapper">
          <div class="content-header">
            <h3>{{ categories.find(c => c.category === selectedCategory)?.label }}</h3>
            <div class="header-actions">
              <button
                v-if="!isEditing"
                class="action-btn"
                @click="isEditing = true"
              >
                <svg class="btn-icon" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                  <path d="M11 4H4a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-7"/>
                  <path d="M18.5 2.5a2.121 2.121 0 0 1 3 3L12 15l-4 1 1-4 9.5-9.5z"/>
                </svg>
                编辑
              </button>
              <button
                v-if="isEditing"
                class="action-btn"
                @click="isEditing = false"
              >
                <svg class="btn-icon" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                  <path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z"/>
                  <circle cx="12" cy="12" r="3"/>
                </svg>
                预览
              </button>
              <button
                v-if="isEditing"
                class="action-btn primary"
                :disabled="saving"
                @click="saveContent"
              >
                <svg v-if="!saving" class="btn-icon" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                  <path d="M19 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h11l5 5z"/>
                  <polyline points="17 21 17 13 7 13 7 21"/>
                  <polyline points="7 3 7 8 15 8"/>
                </svg>
                <svg v-else class="btn-icon spinner-icon" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                  <line x1="12" y1="2" x2="12" y2="6"/>
                  <line x1="12" y1="18" x2="12" y2="22"/>
                  <line x1="4.93" y1="4.93" x2="7.76" y2="7.76"/>
                  <line x1="16.24" y1="16.24" x2="19.07" y2="19.07"/>
                  <line x1="2" y1="12" x2="6" y2="12"/>
                  <line x1="18" y1="12" x2="22" y2="12"/>
                  <line x1="4.93" y1="19.07" x2="7.76" y2="16.24"/>
                  <line x1="16.24" y1="7.76" x2="19.07" y2="4.93"/>
                </svg>
                {{ saving ? '保存中...' : '保存' }}
              </button>
            </div>
          </div>

          <div class="content-body">
            <div v-if="loading" class="loading-state">
              <div class="spinner"></div>
              <p>加载中...</p>
            </div>
            <div v-else-if="isEditing" class="edit-mode">
              <textarea
                v-model="content"
                class="content-editor"
                placeholder="输入 Markdown 格式的内容..."
              ></textarea>
            </div>
            <div v-else class="preview-mode">
              <div class="markdown-content" v-html="renderedContent"></div>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.shadow-memory {
  padding: 24px;
  height: calc(100vh - 120px);
}

.loading {
  text-align: center;
  padding: 40px;
  color: var(--text-secondary);
}

.memory-container {
  display: flex;
  gap: 24px;
  height: 100%;
}

/* 左侧分类列表 */
.categories-sidebar {
  width: 280px;
  background: var(--bg-secondary);
  border: 1px solid var(--border);
  border-radius: 8px;
  padding: 20px;
  overflow-y: auto;
}

.sidebar-header {
  margin-bottom: 20px;
}

.sidebar-header h3 {
  margin: 0;
  font-size: 18px;
  font-weight: 600;
  color: var(--text-primary);
}

.category-group {
  margin-bottom: 24px;
}

.category-group:last-child {
  margin-bottom: 0;
}

.group-label {
  font-size: 12px;
  font-weight: 600;
  color: var(--text-secondary);
  text-transform: uppercase;
  margin-bottom: 12px;
  padding: 0 8px;
}

.category-list {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.category-item {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 10px 12px;
  border-radius: 6px;
  cursor: pointer;
  transition: all 0.2s;
  position: relative;
}

.category-item:hover {
  background: var(--bg-hover);
}

.category-item.active {
  background: var(--primary);
  color: white !important;
}

.category-item.active .category-label {
  color: white !important;
}

.category-icon {
  font-size: 16px;
}

.category-label {
  flex: 1;
  font-size: 14px;
  color: var(--text-primary);
}

.data-badge {
  font-size: 8px;
  color: var(--primary);
}

.category-item.active .data-badge {
  color: white !important;
}

/* Light theme 适配：激活状态使用深色文字 */
[data-theme="light"] .category-item.active {
  color: var(--text-primary) !important;
}

[data-theme="light"] .category-item.active .category-label {
  color: var(--text-primary) !important;
}

[data-theme="light"] .category-item.active .category-icon {
  color: var(--text-primary) !important;
}

[data-theme="light"] .category-item.active .data-badge {
  color: var(--text-primary) !important;
}

/* 右侧内容区域 */
.content-area {
  flex: 1;
  background: var(--bg-secondary);
  border: 1px solid var(--border);
  border-radius: 8px;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.empty-state {
  display: flex;
  align-items: center;
  justify-content: center;
  height: 100%;
  color: var(--text-secondary);
}

.content-wrapper {
  display: flex;
  flex-direction: column;
  height: 100%;
}

.content-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 20px;
  border-bottom: 1px solid var(--border);
}

.content-header h3 {
  margin: 0;
  font-size: 18px;
  font-weight: 600;
  color: var(--text-primary);
}

.header-actions {
  display: flex;
  gap: 8px;
}

.action-btn {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 8px 16px;
  background: var(--bg-primary);
  border: 1px solid var(--border);
  border-radius: 6px;
  cursor: pointer;
  font-size: 14px;
  color: var(--text-primary);
  transition: all 0.2s;
}

.btn-icon {
  width: 16px;
  height: 16px;
}

.spinner-icon {
  animation: spin 1.2s linear infinite;
}

.action-btn:hover:not(:disabled) {
  background: var(--bg-hover);
}

.action-btn.primary {
  background: var(--primary);
  color: white;
  border-color: var(--primary);
}

.action-btn.primary:hover:not(:disabled) {
  opacity: 0.9;
}

.action-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
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
  height: 100%;
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

.edit-mode {
  height: 100%;
}

.content-editor {
  width: 100%;
  height: 100%;
  padding: 16px;
  border: 1px solid var(--border);
  border-radius: 6px;
  background: var(--bg-primary);
  color: var(--text-primary);
  font-size: 14px;
  font-family: 'Monaco', 'Menlo', 'Courier New', monospace;
  line-height: 1.6;
  resize: none;
}

.content-editor:focus {
  outline: none;
  border-color: var(--primary);
}

.preview-mode {
  min-height: 100%;
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

@media (max-width: 768px) {
  .shadow-memory {
    padding: 16px;
    height: auto;
    min-height: calc(100vh - 120px);
  }

  .memory-container {
    flex-direction: column;
  }

  .categories-sidebar {
    width: 100%;
    max-height: 300px;
  }

  .content-area {
    min-height: 400px;
  }
}
</style>
