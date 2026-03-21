<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { listInjectionRoutes, createInjectionRoute, updateInjectionRoute, deleteInjectionRoute } from '../composables/api'
import type { InjectionRoute } from '../composables/api'

const routes = ref<InjectionRoute[]>([])
const conditionalCategories = ref<string[]>([])
const fixedCategories = ref<string[]>([])
const loading = ref(false)
const showForm = ref(false)
const editingRoute = ref<InjectionRoute | null>(null)
const deleteTarget = ref<InjectionRoute | null>(null)

const form = ref({
  keywords: '',
  selectedCategories: [] as string[],
})

const categoryLabels: Record<string, string> = {
  'identity': '用户身份画像',
  'preferences': '用户偏好习惯',
  'error-genome': 'AI错误模式库',
  'domain': '领域知识',
  'lessons': '踩坑教训',
  'active': '进行中事项',
  'decisions': '决策记录',
}

function getCategoryLabel(cat: string): string {
  return categoryLabels[cat] || cat
}

function formatKeywords(kw: string): string {
  return kw.split('|').map(k => k.trim()).join(' | ')
}

function formatCategories(cats: string): string {
  return cats.split(',').map(c => getCategoryLabel(c.trim())).join(', ')
}

async function load() {
  loading.value = true
  try {
    const res = await listInjectionRoutes()
    routes.value = res.routes || []
    conditionalCategories.value = res.conditional || []
    fixedCategories.value = res.fixed || []
  } catch {
    routes.value = []
  }
  loading.value = false
}

function resetForm() {
  form.value = { keywords: '', selectedCategories: [] }
  editingRoute.value = null
}

function openCreate() {
  resetForm()
  showForm.value = true
}

function openEdit(r: InjectionRoute) {
  editingRoute.value = r
  form.value = {
    keywords: r.keywords,
    selectedCategories: r.inject_categories.split(',').map(c => c.trim()),
  }
  showForm.value = true
}

async function onSubmit() {
  if (!form.value.keywords.trim() || form.value.selectedCategories.length === 0) return
  const categories = form.value.selectedCategories.join(',')
  if (editingRoute.value) {
    await updateInjectionRoute(editingRoute.value.id, {
      keywords: form.value.keywords.trim(),
      inject_categories: categories,
    })
  } else {
    await createInjectionRoute(form.value.keywords.trim(), categories)
  }
  showForm.value = false
  resetForm()
  load()
}

async function onDelete() {
  if (!deleteTarget.value) return
  await deleteInjectionRoute(deleteTarget.value.id)
  deleteTarget.value = null
  load()
}

function toggleCategory(cat: string) {
  const idx = form.value.selectedCategories.indexOf(cat)
  if (idx >= 0) {
    form.value.selectedCategories.splice(idx, 1)
  } else {
    form.value.selectedCategories.push(cat)
  }
}

onMounted(load)
</script>

<template>
  <div class="injection-page">
    <div class="page-header">
      <div>
        <h2 class="page-title">注入路由</h2>
        <span class="page-desc">配置关键词→记忆分类的映射规则。当用户消息匹配关键词时，对应分类的结构化记忆将注入到 AI 上下文中。</span>
      </div>
      <button class="btn-create" @click="openCreate">
        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M12 5v14M5 12h14"/></svg>
        新建规则
      </button>
    </div>

    <div v-if="fixedCategories.length > 0" class="fixed-note">
      <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10"/><line x1="12" y1="16" x2="12" y2="12"/><line x1="12" y1="8" x2="12.01" y2="8"/></svg>
      固定分类（{{ fixedCategories.map(c => getCategoryLabel(c)).join('、') }}）始终注入，无需配置
    </div>

    <div v-if="loading" class="empty-state">加载中...</div>
    <div v-else-if="routes.length === 0" class="empty-state">暂无注入路由规则</div>

    <div v-else class="route-table">
      <div class="table-header">
        <div class="col-keywords">关键词</div>
        <div class="col-categories">注入分类</div>
        <div class="col-actions">操作</div>
      </div>
      <div v-for="r in routes" :key="r.id" class="table-row">
        <div class="col-keywords">
          <span v-for="(kw, i) in r.keywords.split('|')" :key="i" class="keyword-tag">{{ kw.trim() }}</span>
        </div>
        <div class="col-categories">
          <span v-for="(cat, i) in r.inject_categories.split(',')" :key="i" class="category-tag">{{ getCategoryLabel(cat.trim()) }}</span>
        </div>
        <div class="col-actions">
          <button class="btn-edit" @click="openEdit(r)" title="编辑">
            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M11 4H4a2 2 0 00-2 2v14a2 2 0 002 2h14a2 2 0 002-2v-7"/><path d="M18.5 2.5a2.121 2.121 0 013 3L12 15l-4 1 1-4 9.5-9.5z"/></svg>
          </button>
          <button class="btn-del" @click="deleteTarget = r" title="删除">
            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M18 6L6 18M6 6l12 12"/></svg>
          </button>
        </div>
      </div>
    </div>

    <!-- Create/Edit modal -->
    <Teleport to="body">
      <div v-if="showForm" class="modal-overlay" @click="showForm = false">
        <div class="modal-box" @click.stop>
          <p class="modal-title">{{ editingRoute ? '编辑规则' : '新建规则' }}</p>
          <div class="form-group">
            <label>关键词</label>
            <input v-model="form.keywords" placeholder="用 | 分隔多个关键词，如：开发|编程|代码" />
            <span class="form-hint">多个关键词用 | 分隔，匹配任意一个即触发注入</span>
          </div>
          <div class="form-group">
            <label>注入分类</label>
            <div class="category-checkboxes">
              <label
                v-for="cat in conditionalCategories"
                :key="cat"
                class="checkbox-item"
                :class="{ checked: form.selectedCategories.includes(cat) }"
                @click="toggleCategory(cat)"
              >
                <span class="checkbox-box">
                  <svg v-if="form.selectedCategories.includes(cat)" width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3"><polyline points="20 6 9 17 4 12"/></svg>
                </span>
                <span class="checkbox-label">{{ getCategoryLabel(cat) }}</span>
                <span class="checkbox-key">{{ cat }}</span>
              </label>
            </div>
          </div>
          <div class="modal-actions">
            <button class="modal-btn cancel" @click="showForm = false">取消</button>
            <button class="modal-btn confirm" @click="onSubmit" :disabled="!form.keywords.trim() || form.selectedCategories.length === 0">{{ editingRoute ? '保存' : '创建' }}</button>
          </div>
        </div>
      </div>
    </Teleport>

    <!-- Delete confirm -->
    <Teleport to="body">
      <div v-if="deleteTarget" class="modal-overlay" @click="deleteTarget = null">
        <div class="modal-box" @click.stop>
          <p class="modal-title">确认删除</p>
          <p class="modal-desc">删除规则「{{ formatKeywords(deleteTarget.keywords) }}」→ {{ formatCategories(deleteTarget.inject_categories) }}？</p>
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
.injection-page { padding: 24px; overflow-y: auto; height: 100%; }
.page-header { display: flex; align-items: flex-start; justify-content: space-between; margin-bottom: 16px; }
.page-title { font-size: 18px; font-weight: 600; color: var(--text-primary); margin: 0; }
.page-desc { font-size: 12px; color: var(--text-muted); margin-top: 4px; display: block; max-width: 500px; line-height: 1.5; }
.btn-create {
  display: flex; align-items: center; gap: 4px; padding: 6px 14px;
  border-radius: var(--radius); font-size: 13px; font-weight: 500;
  background: var(--accent); color: var(--btn-text); transition: opacity var(--transition); flex-shrink: 0;
}
.btn-create:hover { opacity: 0.9; }
.fixed-note {
  display: flex; align-items: center; gap: 6px; padding: 8px 12px;
  background: var(--accent-soft); border-radius: var(--radius);
  font-size: 12px; color: var(--accent); margin-bottom: 16px;
}
.empty-state { text-align: center; color: var(--text-muted); padding: 48px 16px; font-size: 14px; }

/* Table */
.route-table { border: 1px solid var(--border); border-radius: var(--radius); overflow: hidden; }
.table-header {
  display: flex; padding: 8px 16px; background: var(--bg-tertiary);
  font-size: 11px; font-weight: 600; color: var(--text-muted); text-transform: uppercase;
}
.table-row {
  display: flex; padding: 10px 16px; border-top: 1px solid var(--border);
  align-items: center; transition: background var(--transition);
}
.table-row:hover { background: var(--bg-hover); }
.col-keywords { flex: 2; display: flex; flex-wrap: wrap; gap: 4px; }
.col-categories { flex: 2; display: flex; flex-wrap: wrap; gap: 4px; }
.col-actions { flex: 0 0 80px; display: flex; justify-content: flex-end; gap: 6px; }
.keyword-tag {
  font-size: 11px; padding: 2px 8px; border-radius: 9999px;
  background: var(--bg-tertiary); color: var(--text-secondary); border: 1px solid var(--border);
}
.category-tag {
  font-size: 11px; padding: 2px 8px; border-radius: 9999px;
  background: var(--accent-soft); color: var(--accent);
}
.btn-edit, .btn-del {
  width: 24px; height: 24px; display: flex; align-items: center; justify-content: center;
  border-radius: var(--radius-sm); color: var(--text-muted); transition: all var(--transition);
}
.btn-edit:hover { color: var(--accent); background: var(--accent-soft); }
.btn-del:hover { color: var(--danger); background: rgba(239,68,68,0.1); }

/* Modal */
.modal-overlay {
  position: fixed; inset: 0; background: var(--overlay);
  display: flex; align-items: center; justify-content: center; z-index: 1000;
}
.modal-box {
  background: var(--bg-secondary); border: 1px solid var(--border);
  border-radius: 12px; padding: 24px; width: 460px; max-width: 90vw;
}
.modal-title { font-size: 15px; font-weight: 600; color: var(--text-primary); margin-bottom: 16px; }
.modal-desc { font-size: 13px; color: var(--text-secondary); margin-bottom: 20px; line-height: 1.5; }
.form-group { margin-bottom: 14px; }
.form-group > label { display: block; font-size: 12px; font-weight: 600; color: var(--text-secondary); margin-bottom: 4px; }
.form-group input {
  width: 100%; padding: 8px 10px; font-size: 13px; border-radius: var(--radius);
  border: 1px solid var(--border); background: var(--bg-primary); color: var(--text-primary);
}
.form-hint { font-size: 11px; color: var(--text-muted); margin-top: 2px; display: block; }

/* Category checkboxes */
.category-checkboxes { display: flex; flex-direction: column; gap: 6px; margin-top: 4px; }
.checkbox-item {
  display: flex; align-items: center; gap: 8px; padding: 6px 10px;
  border-radius: var(--radius-sm); cursor: pointer; transition: background var(--transition);
  border: 1px solid var(--border);
}
.checkbox-item:hover { background: var(--bg-hover); }
.checkbox-item.checked { background: var(--accent-soft); border-color: var(--accent); }
.checkbox-box {
  width: 18px; height: 18px; display: flex; align-items: center; justify-content: center;
  border-radius: 3px; border: 2px solid var(--border); flex-shrink: 0;
  transition: all 0.2s;
}
.checkbox-item.checked .checkbox-box { background: var(--accent); border-color: var(--accent); color: var(--btn-text); }
.checkbox-label { font-size: 13px; color: var(--text-primary); }
.checkbox-key { font-size: 11px; color: var(--text-muted); margin-left: auto; }

.modal-actions { display: flex; justify-content: flex-end; gap: 8px; margin-top: 16px; }
.modal-btn { padding: 6px 16px; border-radius: var(--radius); font-size: 13px; font-weight: 500; cursor: pointer; transition: all var(--transition); }
.modal-btn.cancel { color: var(--text-secondary); background: var(--bg-hover); }
.modal-btn.cancel:hover { color: var(--text-primary); }
.modal-btn.confirm { color: var(--btn-text); background: var(--accent); }
.modal-btn.confirm:hover { opacity: 0.9; }
.modal-btn.confirm:disabled { opacity: 0.5; cursor: not-allowed; }

@media (max-width: 768px) {
  .injection-page { padding: 12px; }
  .table-header { display: none; }
  .table-row { flex-direction: column; align-items: flex-start; gap: 6px; }
  .col-actions { width: 100%; justify-content: flex-end; }
}
</style>
