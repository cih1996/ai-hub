<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { useChatStore } from '../stores/chat'
import type { Provider } from '../types'
import * as api from '../composables/api'

const router = useRouter()
const store = useChatStore()
const showForm = ref(false)
const editing = ref<Provider | null>(null)

const form = ref({
  name: '',
  base_url: '',
  api_key: '',
  model_id: '',
  is_default: false,
})

function resetForm() {
  form.value = { name: '', base_url: '', api_key: '', model_id: '', is_default: false }
  editing.value = null
  showForm.value = false
}

function editProvider(p: Provider) {
  editing.value = p
  form.value = {
    name: p.name,
    base_url: p.base_url,
    api_key: p.api_key,
    model_id: p.model_id,
    is_default: p.is_default,
  }
  showForm.value = true
}

async function saveProvider() {
  if (editing.value) {
    await api.updateProvider(editing.value.id, form.value)
  } else {
    await api.createProvider(form.value)
  }
  await store.loadProviders()
  resetForm()
}

async function removeProvider(id: string) {
  await api.deleteProvider(id)
  await store.loadProviders()
}

function maskKey(key: string): string {
  if (!key || key.length < 8) return '••••••••'
  return key.slice(0, 4) + '••••' + key.slice(-4)
}

onMounted(() => store.loadProviders())
</script>

<template>
  <div class="settings-page">
    <div class="settings-container">
      <div class="settings-header">
        <button class="btn-back" @click="router.push('/chat')">
          <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M19 12H5M12 19l-7-7 7-7"/>
          </svg>
          Back
        </button>
        <h1>Settings</h1>
      </div>

      <section class="section">
        <div class="section-header">
          <div>
            <h2>Providers</h2>
            <p class="section-desc">Configure your LLM API endpoints. Claude models auto-route through Claude Code CLI.</p>
          </div>
          <button class="btn-add" @click="showForm = true">
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <path d="M12 5v14M5 12h14"/>
            </svg>
            Add
          </button>
        </div>

        <div class="provider-list">
          <div v-for="p in store.providers" :key="p.id" class="provider-card">
            <div class="provider-info">
              <div class="provider-name">
                {{ p.name }}
                <span v-if="p.is_default" class="badge default">Default</span>
                <span class="badge mode">{{ p.mode === 'claude-code' ? 'Claude Code' : 'Direct API' }}</span>
              </div>
              <div class="provider-meta">
                {{ p.model_id }}
                <span v-if="p.base_url" class="sep">·</span>
                <span v-if="p.base_url" class="url">{{ p.base_url }}</span>
                <span class="sep">·</span>
                <span class="key">{{ maskKey(p.api_key) }}</span>
              </div>
            </div>
            <div class="provider-actions">
              <button class="btn-sm" @click="editProvider(p)">Edit</button>
              <button class="btn-sm btn-danger" @click="removeProvider(p.id)">Delete</button>
            </div>
          </div>
          <div v-if="store.providers.length === 0" class="empty">
            No providers yet. Add one to start chatting.
          </div>
        </div>

        <!-- Form Modal -->
        <div v-if="showForm" class="form-overlay" @click.self="resetForm">
          <div class="form-modal">
            <h3>{{ editing ? 'Edit' : 'Add' }} Provider</h3>

            <div class="form-group">
              <label>Name</label>
              <input v-model="form.name" placeholder="e.g. Claude Pro, GPT-4o" />
            </div>

            <div class="form-group">
              <label>API Base URL</label>
              <input v-model="form.base_url" placeholder="https://api.example.com" />
              <span class="hint">The API endpoint. Leave empty for default Anthropic API.</span>
            </div>

            <div class="form-group">
              <label>API Key</label>
              <input v-model="form.api_key" type="password" placeholder="sk-..." />
            </div>

            <div class="form-group">
              <label>Model ID</label>
              <input v-model="form.model_id" placeholder="claude-sonnet-4-20250514 / gpt-4o" />
              <span class="hint">Models containing "claude" auto-route through Claude Code CLI.</span>
            </div>

            <div class="form-group checkbox">
              <label>
                <input type="checkbox" v-model="form.is_default" />
                Set as default provider
              </label>
            </div>

            <div class="form-actions">
              <button class="btn-cancel" @click="resetForm">Cancel</button>
              <button class="btn-save" @click="saveProvider" :disabled="!form.name || !form.api_key || !form.model_id">
                Save
              </button>
            </div>
          </div>
        </div>
      </section>
    </div>
  </div>
</template>

<style scoped>
.settings-page {
  height: 100vh;
  overflow-y: auto;
  background: var(--bg-primary);
}
.settings-container {
  max-width: 680px;
  margin: 0 auto;
  padding: 32px 24px;
}
.settings-header { margin-bottom: 32px; }
.settings-header h1 { font-size: 24px; font-weight: 600; margin-top: 16px; }
.btn-back {
  display: flex; align-items: center; gap: 6px;
  color: var(--text-secondary); font-size: 13px; padding: 6px 0;
  transition: color var(--transition);
}
.btn-back:hover { color: var(--text-primary); }

.section { margin-bottom: 32px; }
.section-header {
  display: flex; align-items: flex-start; justify-content: space-between;
  margin-bottom: 16px;
}
.section-header h2 { font-size: 16px; font-weight: 600; }
.section-desc { font-size: 12px; color: var(--text-muted); margin-top: 4px; }
.btn-add {
  display: flex; align-items: center; gap: 6px;
  padding: 8px 14px; background: var(--accent); color: white;
  border-radius: var(--radius); font-size: 13px; font-weight: 500;
  transition: background var(--transition); flex-shrink: 0;
}
.btn-add:hover { background: var(--accent-hover); }

.provider-list { display: flex; flex-direction: column; gap: 8px; }
.provider-card {
  display: flex; align-items: center; justify-content: space-between;
  padding: 14px 16px; background: var(--bg-secondary);
  border: 1px solid var(--border); border-radius: var(--radius);
}
.provider-info { min-width: 0; flex: 1; }
.provider-name {
  font-weight: 500; font-size: 14px;
  display: flex; align-items: center; gap: 8px;
}
.provider-meta {
  font-size: 12px; color: var(--text-muted); margin-top: 4px;
  white-space: nowrap; overflow: hidden; text-overflow: ellipsis;
}
.sep { margin: 0 2px; }
.badge {
  font-size: 10px; padding: 2px 8px; border-radius: 99px;
  font-weight: 600; text-transform: uppercase; letter-spacing: 0.5px;
}
.badge.default { background: var(--accent-soft); color: var(--accent); }
.badge.mode { background: var(--bg-tertiary); color: var(--text-secondary); }
.provider-actions { display: flex; gap: 6px; flex-shrink: 0; margin-left: 12px; }
.btn-sm {
  padding: 6px 12px; font-size: 12px; border-radius: var(--radius-sm);
  background: var(--bg-tertiary); color: var(--text-secondary);
  transition: all var(--transition);
}
.btn-sm:hover { background: var(--bg-hover); color: var(--text-primary); }
.btn-danger:hover { background: rgba(239,68,68,0.15); color: var(--danger); }
.empty { text-align: center; color: var(--text-muted); padding: 32px; font-size: 13px; }

/* Modal */
.form-overlay {
  position: fixed; inset: 0; background: rgba(0,0,0,0.6);
  display: flex; align-items: center; justify-content: center;
  z-index: 100; backdrop-filter: blur(4px);
}
.form-modal {
  background: var(--bg-secondary); border: 1px solid var(--border);
  border-radius: var(--radius-lg); padding: 24px;
  width: 440px; max-width: 90vw;
}
.form-modal h3 { font-size: 16px; font-weight: 600; margin-bottom: 20px; }
.form-group { margin-bottom: 14px; }
.form-group label {
  display: block; font-size: 12px; font-weight: 500;
  color: var(--text-secondary); margin-bottom: 6px;
  text-transform: uppercase; letter-spacing: 0.5px;
}
.form-group input, .form-group select {
  width: 100%; padding: 10px 12px;
  background: var(--bg-tertiary); border: 1px solid var(--border);
  border-radius: var(--radius); font-size: 14px; color: var(--text-primary);
  transition: border-color var(--transition);
}
.form-group input:focus { border-color: var(--accent); }
.hint { display: block; font-size: 11px; color: var(--text-muted); margin-top: 4px; }
.form-group.checkbox label {
  display: flex; align-items: center; gap: 8px;
  text-transform: none; letter-spacing: 0; font-size: 14px; cursor: pointer;
}
.form-group.checkbox input[type="checkbox"] {
  width: 16px; height: 16px; accent-color: var(--accent);
}
.form-actions { display: flex; justify-content: flex-end; gap: 8px; margin-top: 20px; }
.btn-cancel {
  padding: 8px 16px; border-radius: var(--radius); font-size: 13px;
  color: var(--text-secondary); background: var(--bg-tertiary);
  transition: all var(--transition);
}
.btn-cancel:hover { background: var(--bg-hover); }
.btn-save {
  padding: 8px 20px; border-radius: var(--radius); font-size: 13px; font-weight: 500;
  background: var(--accent); color: white; transition: background var(--transition);
}
.btn-save:hover:not(:disabled) { background: var(--accent-hover); }
.btn-save:disabled { opacity: 0.4; cursor: not-allowed; }
</style>
