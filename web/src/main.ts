import { createApp } from 'vue'
import { createPinia } from 'pinia'
import router from './router'
import App from './App.vue'
import './style.css'

// Pre-mount theme to avoid flash
;(() => {
  const saved = localStorage.getItem('theme') as 'light' | 'dark' | null
  const theme = saved || (window.matchMedia('(prefers-color-scheme: light)').matches ? 'light' : 'dark')
  document.documentElement.setAttribute('data-theme', theme)
})()

const app = createApp(App)
app.use(createPinia())
app.use(router)
app.mount('#app')
