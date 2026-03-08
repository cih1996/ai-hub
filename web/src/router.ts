import { createRouter, createWebHistory } from 'vue-router'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/', redirect: '/chat' },
    { path: '/init', name: 'init', component: () => import('./views/InitGuideView.vue') },
    {
      path: '/',
      component: () => import('./views/MainLayout.vue'),
      children: [
        { path: 'chat/:id?', name: 'chat', component: () => import('./views/ChatView.vue') },
        { path: 'manage', name: 'manage', component: () => import('./views/ManageView.vue') },
        { path: 'extensions', name: 'extensions', component: () => import('./views/ExtensionsView.vue') },
        { path: 'services', name: 'services', component: () => import('./views/ServicesView.vue') },
        { path: 'automation', name: 'automation', component: () => import('./views/AutomationView.vue') },
        { path: 'token-usage', name: 'token-usage', component: () => import('./views/TokenUsageView.vue') },
        // Legacy redirects for backward compatibility
        { path: 'skills', redirect: '/extensions?tab=skills' },
        { path: 'mcp', redirect: '/extensions?tab=mcp' },
        { path: 'triggers', redirect: '/automation?tab=triggers' },
        { path: 'channels', redirect: '/automation?tab=channels' },
      ],
    },
    { path: '/settings', name: 'settings', component: () => import('./views/SettingsView.vue') },
  ],
})

export default router
