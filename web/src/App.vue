<script setup lang="ts">
import { onMounted, watch } from 'vue'
import { useRoute } from 'vue-router'
import { useChatStore } from './stores/chat'

const store = useChatStore()
const route = useRoute()

onMounted(async () => {
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
