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
        { path: 'teams', name: 'teams', component: () => import('./views/TeamsView.vue') },
        { path: 'services', name: 'services', component: () => import('./views/ServicesView.vue') },
        // Legacy routes - redirect to settings sub-pages
        { path: 'manage', redirect: '/settings/rules' },
        { path: 'extensions', redirect: '/settings/extensions' },
        { path: 'automation', redirect: '/settings/im' },
        { path: 'token-usage', redirect: '/settings/data' },
        { path: 'skills', redirect: '/settings/extensions?tab=skills' },
        { path: 'mcp', redirect: '/settings/extensions?tab=mcp' },
        { path: 'triggers', redirect: '/settings/triggers' },
        { path: 'channels', redirect: '/settings/im' },
      ],
    },
    {
      path: '/settings',
      component: () => import('./views/SettingsLayout.vue'),
      children: [
        { path: '', name: 'settings', component: () => import('./views/SettingsView.vue') },
        { path: 'rules', name: 'settings-rules', component: () => import('./views/ManageView.vue') },
        { path: 'extensions', name: 'settings-extensions', component: () => import('./views/ExtensionsView.vue') },
        { path: 'data', name: 'settings-data', component: () => import('./views/TokenUsageView.vue') },
        { path: 'im', name: 'settings-im', component: () => import('./views/AutomationView.vue'), props: { defaultTab: 'channels' } },
        { path: 'triggers', name: 'settings-triggers', component: () => import('./views/AutomationView.vue'), props: { defaultTab: 'triggers' } },
      ],
    },
    {
      path: '/shadow-ai',
      component: () => import('./views/ShadowAILayout.vue'),
      children: [
        { path: '', redirect: '/shadow-ai/overview' },
        { path: 'overview', name: 'shadow-overview', component: () => import('./views/ShadowOverviewView.vue') },
        { path: 'config', name: 'shadow-config', component: () => import('./views/ShadowConfigView.vue') },
        { path: 'logs', name: 'shadow-logs', component: () => import('./views/ShadowLogsView.vue') },
        { path: 'memory', name: 'shadow-memory', component: () => import('./views/ShadowMemoryView.vue') },
        { path: 'patrol', name: 'shadow-patrol', component: () => import('./views/ShadowPatrolView.vue') },
        { path: 'router', name: 'shadow-router', component: () => import('./views/ShadowRouterView.vue') },
      ],
    },
  ],
})

export default router
