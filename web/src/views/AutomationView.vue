<script setup lang="ts">
import { ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import TriggersView from './TriggersView.vue'
import ChannelsView from './ChannelsView.vue'

const route = useRoute()
const router = useRouter()

const tabs = [
  { key: 'triggers', label: '定时' },
  { key: 'channels', label: '通讯' },
]

const activeTab = ref((route.query.tab as string) || 'triggers')

watch(() => route.query.tab, (val) => {
  if (val && typeof val === 'string') activeTab.value = val
})

function switchTab(key: string) {
  activeTab.value = key
  router.replace({ query: { tab: key } })
}
</script>

<template>
  <div class="tab-container">
    <div class="tab-header">
      <div class="tab-bar">
        <button
          v-for="t in tabs" :key="t.key"
          class="tab-btn" :class="{ active: activeTab === t.key }"
          @click="switchTab(t.key)"
        >{{ t.label }}</button>
      </div>
    </div>
    <div class="tab-body">
      <TriggersView v-if="activeTab === 'triggers'" />
      <ChannelsView v-if="activeTab === 'channels'" />
    </div>
  </div>
</template>

<style scoped>
.tab-container { display: flex; flex-direction: column; height: 100%; overflow: hidden; }
.tab-header { padding: 16px 24px 0; flex-shrink: 0; }
.tab-bar { display: flex; gap: 0; border-bottom: 1px solid var(--border); }
.tab-btn {
  padding: 8px 16px; font-size: 13px; font-weight: 500;
  color: var(--text-muted); border-bottom: 2px solid transparent;
  transition: all var(--transition); cursor: pointer; background: none;
}
.tab-btn:hover { color: var(--text-primary); }
.tab-btn.active { color: var(--accent); border-bottom-color: var(--accent); }
.tab-body { flex: 1; overflow-y: auto; }
</style>
