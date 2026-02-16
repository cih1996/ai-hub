import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import type { Session, Message, Provider, WSMessage } from '../types'
import * as api from '../composables/api'

export const useChatStore = defineStore('chat', () => {
  const sessions = ref<Session[]>([])
  const currentSessionId = ref<number>(0) // 0 = new empty chat
  const messages = ref<Message[]>([])
  const providers = ref<Provider[]>([])
  const streaming = ref(false)
  const streamingContent = ref('')
  const ws = ref<WebSocket | null>(null)

  const currentSession = computed(() =>
    sessions.value.find((s) => s.id === currentSessionId.value)
  )

  const defaultProvider = computed(() =>
    providers.value.find((p) => p.is_default) || providers.value[0]
  )

  // Always show chat panel (even when id=0, it's a blank chat)
  const hasChatPanel = computed(() => true)

  function connectWS() {
    if (ws.value && ws.value.readyState === WebSocket.OPEN) return

    const protocol = location.protocol === 'https:' ? 'wss:' : 'ws:'
    const wsUrl = `${protocol}//${location.host}/ws/chat`
    ws.value = new WebSocket(wsUrl)

    ws.value.onmessage = (event) => {
      const msg: WSMessage = JSON.parse(event.data)
      switch (msg.type) {
        case 'session_created': {
          // Backend created a new session for us
          const newSession: Session = JSON.parse(msg.content)
          sessions.value.unshift(newSession)
          currentSessionId.value = newSession.id
          // Update URL without full navigation
          window.history.replaceState({}, '', `/chat/${newSession.id}`)
          break
        }
        case 'chunk':
          streamingContent.value += msg.content
          break
        case 'done':
          if (streamingContent.value) {
            messages.value.push({
              id: Date.now(),
              session_id: msg.session_id,
              role: 'assistant',
              content: streamingContent.value,
              created_at: new Date().toISOString(),
            })
          }
          streamingContent.value = ''
          streaming.value = false
          break
        case 'error':
          streamingContent.value = ''
          streaming.value = false
          messages.value.push({
            id: Date.now(),
            session_id: msg.session_id,
            role: 'assistant',
            content: `Error: ${msg.content}`,
            created_at: new Date().toISOString(),
          })
          break
      }
    }

    ws.value.onclose = () => {
      setTimeout(connectWS, 2000)
    }
  }

  async function loadProviders() {
    providers.value = await api.listProviders()
  }

  async function loadSessions() {
    sessions.value = await api.listSessions()
  }

  async function selectSession(id: number) {
    currentSessionId.value = id
    if (id === 0) {
      messages.value = []
    } else {
      messages.value = await api.getMessages(id)
    }
  }

  function newChat() {
    currentSessionId.value = 0
    messages.value = []
  }

  async function deleteSessionById(id: number) {
    await api.deleteSession(id)
    sessions.value = sessions.value.filter((s) => s.id !== id)
    if (currentSessionId.value === id) {
      newChat()
    }
  }

  function sendMessage(content: string) {
    if (!ws.value || ws.value.readyState !== WebSocket.OPEN) {
      connectWS()
      setTimeout(() => sendMessage(content), 500)
      return
    }
    if (streaming.value) return

    // Add user message to local list immediately
    messages.value.push({
      id: Date.now(),
      session_id: currentSessionId.value,
      role: 'user',
      content,
      created_at: new Date().toISOString(),
    })

    streaming.value = true
    streamingContent.value = ''

    // session_id=0 means "create new session for me"
    const msg: WSMessage = {
      type: 'chat',
      session_id: currentSessionId.value,
      content,
    }
    ws.value.send(JSON.stringify(msg))
  }

  function stopStreaming() {
    if (ws.value && ws.value.readyState === WebSocket.OPEN) {
      ws.value.send(JSON.stringify({ type: 'stop' }))
    }
  }

  return {
    sessions,
    currentSessionId,
    currentSession,
    messages,
    providers,
    defaultProvider,
    hasChatPanel,
    streaming,
    streamingContent,
    connectWS,
    loadProviders,
    loadSessions,
    selectSession,
    newChat,
    deleteSessionById,
    sendMessage,
    stopStreaming,
  }
})
