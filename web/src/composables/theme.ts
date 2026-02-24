import { ref, onMounted, onUnmounted } from 'vue'

export type ThemeMode = 'system' | 'light' | 'dark'

const STORAGE_KEY = 'theme'

// Module-level singleton — shared across all components
const mode = ref<ThemeMode>(
  (localStorage.getItem(STORAGE_KEY) as ThemeMode) || 'system'
)

function getSystemTheme(): 'light' | 'dark' {
  return window.matchMedia('(prefers-color-scheme: light)').matches ? 'light' : 'dark'
}

function applyTheme(m: ThemeMode) {
  const resolved = m === 'system' ? getSystemTheme() : m
  document.documentElement.setAttribute('data-theme', resolved)
}

export function useTheme() {
  let mediaQuery: MediaQueryList | null = null
  let handler: ((e: MediaQueryListEvent) => void) | null = null

  function setMode(newMode: ThemeMode) {
    mode.value = newMode
    if (newMode === 'system') {
      localStorage.removeItem(STORAGE_KEY)
    } else {
      localStorage.setItem(STORAGE_KEY, newMode)
    }
    applyTheme(newMode)
  }

  onMounted(() => {
    mediaQuery = window.matchMedia('(prefers-color-scheme: light)')
    handler = () => {
      if (mode.value === 'system') {
        applyTheme('system')
      }
    }
    mediaQuery.addEventListener('change', handler)
    // Ensure theme is applied on mount
    applyTheme(mode.value)
  })

  onUnmounted(() => {
    if (mediaQuery && handler) {
      mediaQuery.removeEventListener('change', handler)
    }
  })

  function toggle() {
    const order: ThemeMode[] = ['system', 'light', 'dark']
    const idx = order.indexOf(mode.value)
    const next = order[(idx + 1) % order.length] ?? 'system'
    setMode(next)
  }

  return { mode, setMode, toggle }
}
