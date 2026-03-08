<script setup lang="ts">
import { onMounted, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useChatStore } from './stores/chat'

const store = useChatStore()
const route = useRoute()
const router = useRouter()

onMounted(async () => {
  // Check if first run (skip if already on init page or completed)
  const initCompleted = localStorage.getItem('ai-hub-init-completed')
  const forceFirstRun = new URLSearchParams(window.location.search).get('force_first_run') === 'true'

  if (route.name !== 'init' && (!initCompleted || forceFirstRun)) {
    try {
      const url = forceFirstRun
        ? '/api/v1/system/init-status?force_first_run=true'
        : '/api/v1/system/init-status'
      const res = await fetch(url)
      const status = await res.json()

      if (status.is_first_run || forceFirstRun) {
        router.push('/init')
        return
      }
    } catch (e) {
      console.error('Failed to check init status:', e)
    }
  }

  store.connectWS()
  await store.loadProviders()
  await store.loadSessions()

  // Restore session from URL on page refresh
  const idParam = route.params.id
  if (idParam) {
    const id = Number(idParam)
    if (id > 0) {
      await store.selectSession(id)
    }
  }
})

// Watch route.params.id changes to handle URL navigation
watch(
  () => route.params.id,
  async (newId) => {
    if (newId) {
      const id = Number(newId)
      if (id > 0) {
        await store.selectSession(id)
      }
    }
  }
)
</script>

<template>
  <router-view />
</template>
