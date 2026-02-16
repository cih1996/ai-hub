import { createApp } from 'vue'
import { createPinia } from 'pinia'
import { createRouter, createWebHistory } from 'vue-router'
import App from './App.vue'
import './style.css'

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
      ],
    },
    { path: '/settings', name: 'settings', component: () => import('./views/SettingsView.vue') },
  ],
})

const app = createApp(App)
app.use(createPinia())
app.use(router)
app.mount('#app')
