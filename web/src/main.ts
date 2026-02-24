import { createApp } from 'vue'
import { createPinia } from 'pinia'
import { createRouter, createWebHistory } from 'vue-router'
import App from './App.vue'
import './style.css'

// Pre-mount theme to avoid flash
;(() => {
  const saved = localStorage.getItem('theme') as 'light' | 'dark' | null
  const theme = saved || (window.matchMedia('(prefers-color-scheme: light)').matches ? 'light' : 'dark')
  document.documentElement.setAttribute('data-theme', theme)
})()

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/', redirect: '/chat' },
    {
      path: '/',
      component: () => import('./views/MainLayout.vue'),
      children: [
        { path: 'chat/:id?', name: 'chat', component: () => import('./views/ChatView.vue') },
        { path: 'manage', name: 'manage', component: () => import('./views/ManageView.vue') },
        { path: 'skills', name: 'skills', component: () => import('./views/SkillsView.vue') },
        { path: 'mcp', name: 'mcp', component: () => import('./views/McpView.vue') },
        { path: 'triggers', name: 'triggers', component: () => import('./views/TriggersView.vue') },
        { path: 'channels', name: 'channels', component: () => import('./views/ChannelsView.vue') },
        { path: 'token-usage', name: 'token-usage', component: () => import('./views/TokenUsageView.vue') },
      ],
    },
    { path: '/settings', name: 'settings', component: () => import('./views/SettingsView.vue') },
  ],
})

const app = createApp(App)
app.use(createPinia())
app.use(router)
app.mount('#app')
