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
              <span class="category-icon">📝</span>
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
              <span class="category-icon">📋</span>
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
                ✏️ 编辑
              </button>
              <button
                v-if="isEditing"
                class="action-btn"
                @click="isEditing = false"
              >
                👁️ 预览
              </button>
              <button
                v-if="isEditing"
                class="action-btn primary"
                :disabled="saving"
                @click="saveContent"
              >
                {{ saving ? '保存中...' : '💾 保存' }}
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
  color: white;
}

.category-item.active .category-label {
  color: white;
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
  color: white;
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
  padding: 8px 16px;
  background: var(--bg-primary);
  border: 1px solid var(--border);
  border-radius: 6px;
  cursor: pointer;
  font-size: 14px;
  color: var(--text-primary);
  transition: all 0.2s;
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
</style>
