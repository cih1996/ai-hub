<script setup lang="ts">
import { ref, onMounted, onUnmounted, provide } from 'vue'
import Sidebar from '../components/Sidebar.vue'

const isMobile = ref(false)
const sidebarOpen = ref(false)

function checkMobile() {
  isMobile.value = window.innerWidth < 768
  if (!isMobile.value) sidebarOpen.value = false
}

function openSidebar() { sidebarOpen.value = true }
function closeSidebar() { sidebarOpen.value = false }

provide('isMobile', isMobile)
provide('sidebarOpen', sidebarOpen)
provide('openSidebar', openSidebar)
provide('closeSidebar', closeSidebar)

onMounted(() => {
  checkMobile()
  window.addEventListener('resize', checkMobile)
})
onUnmounted(() => {
  window.removeEventListener('resize', checkMobile)
})
</script>

<template>
  <div class="layout">
    <!-- Mobile overlay -->
    <Transition name="fade">
      <div v-if="isMobile && sidebarOpen" class="sidebar-overlay" @click="closeSidebar"></div>
    </Transition>
    <Transition name="slide-sidebar">
      <Sidebar v-if="!isMobile || sidebarOpen" :class="{ 'sidebar-mobile': isMobile }" />
    </Transition>
    <main class="main">
      <router-view />
    </main>
  </div>
</template>

<style scoped>
.layout {
  display: flex;
  height: 100vh;
  width: 100vw;
  overflow: hidden;
}
.main {
  flex: 1;
  display: flex;
  flex-direction: column;
  min-width: 0;
  background: var(--bg-primary);
}
.sidebar-overlay {
  position: fixed;
  inset: 0;
  background: rgba(0, 0, 0, 0.4);
  z-index: 999;
}
.sidebar-mobile {
  position: fixed !important;
  left: 0;
  top: 0;
  z-index: 1000;
  width: 280px !important;
  min-width: 280px !important;
  height: 100vh;
  box-shadow: 4px 0 24px rgba(0, 0, 0, 0.3);
}
/* Transitions */
.fade-enter-active, .fade-leave-active { transition: opacity 0.3s ease; }
.fade-enter-from, .fade-leave-to { opacity: 0; }
.slide-sidebar-enter-active, .slide-sidebar-leave-active { transition: transform 0.3s ease; }
.slide-sidebar-enter-from, .slide-sidebar-leave-to { transform: translateX(-100%); }
</style>
