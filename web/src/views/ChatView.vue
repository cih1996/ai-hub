<script setup lang="ts">
import Sidebar from '../components/Sidebar.vue'
import ChatPanel from '../components/ChatPanel.vue'
import StatusBar from '../components/StatusBar.vue'
</script>

<template>
  <div class="layout">
    <Sidebar />
    <main class="main">
      <StatusBar />
      <ChatPanel />
    </main>
  </div>
</template>

<style scoped>
.layout {
  display: flex;
  height: 100vh;
  width: 100vw;
}
.main {
  flex: 1;
  display: flex;
  flex-direction: column;
  min-width: 0;
  background: var(--bg-primary);
}
</style>
