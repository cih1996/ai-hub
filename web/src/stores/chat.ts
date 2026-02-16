import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import type { Session, Message, Provider, WSMessage, ToolCall } from '../types'
import * as api from '../composables/api'

export const useChatStore = defineStore('chat', () => {
  const sessions = ref<Session[]>([])
  const currentSessionId = ref<number>(0)
  const messages = ref<Message[]>([])
  const providers = ref<Provider[]>([])
  const streaming = ref(false)
  const streamingContent = ref('')
  const thinkingContent = ref('')
  const toolCalls = ref<ToolCall[]>([])
  const ws = ref<WebSocket | null>(null)

  const currentSession = computed(() =>
    sessions.value.find((s) => s.id === currentSessionId.value)
  )

  const defaultProvider = computed(() =>
    providers.value.find((p) => p.is_default) || providers.value[0]
  )

  function connectWS() {
    if (ws.value && ws.value.readyState === WebSocket.OPEN) return

    const protocol = location.protocol === 'https:' ? 'wss:' : 'ws:'
    const wsUrl = `${protocol}//${location.host}/ws/chat`
    ws.value = new WebSocket(wsUrl)

    ws.value.onopen = () => {
      // Reattach to active stream if viewing a session
      if (currentSessionId.value > 0) {
        ws.value?.send(JSON.stringify({ type: 'subscribe', session_id: currentSessionId.value }))
      }
    }

    ws.value.onmessage = (event) => {
      const msg: WSMessage = JSON.parse(event.data)

      // session_created: add to list if not already present
      if (msg.type === 'session_created') {
        const newSession: Session = JSON.parse(msg.content)
        // Deduplicate: broadcast sends to all clients including the originator
        if (!sessions.value.some((s) => s.id === newSession.id)) {
          sessions.value.unshift(newSession)
        }
        // Only take over navigation if we're the one who created it (id was 0)
        if (currentSessionId.value === 0) {
          currentSessionId.value = newSession.id
          window.history.replaceState({}, '', `/chat/${newSession.id}`)
        }
        return
      }

      // session_update: broadcast from server about any session's streaming status
      if (msg.type === 'session_update') {
        const s = sessions.value.find((s) => s.id === msg.session_id)
        if (s) {
          s.streaming = msg.content === 'streaming'
        }
        return
      }

      // All other events: ignore if not for the current session
      if (msg.session_id !== currentSessionId.value) return

      switch (msg.type) {
        case 'streaming_status':
          streaming.value = true
          break
        case 'thinking':
          thinkingContent.value += msg.content
          break
        case 'tool_start': {
          const tc: ToolCall = {
            id: msg.tool_id || String(Date.now()),
            name: msg.tool_name || msg.content,
            input: '',
            status: 'running',
          }
          toolCalls.value.push(tc)
          break
        }
        case 'tool_input': {
          const tc = toolCalls.value.find((t) => t.id === msg.tool_id)
          if (tc) {
            tc.input += msg.content
          }
          break
        }
        case 'tool_result': {
          const tc = toolCalls.value.find((t) => t.id === msg.tool_id)
          if (tc) {
            tc.status = 'done'
          }
          break
        }
        case 'chunk':
          for (const tc of toolCalls.value) {
            if (tc.status === 'running') tc.status = 'done'
          }
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
          thinkingContent.value = ''
          toolCalls.value = []
          streaming.value = false
          break
        case 'error':
          streamingContent.value = ''
          thinkingContent.value = ''
          toolCalls.value = []
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
    streaming.value = false
    streamingContent.value = ''
    thinkingContent.value = ''
    toolCalls.value = []
    if (id === 0) {
      messages.value = []
    } else {
      messages.value = await api.getMessages(id)
      // Subscribe to check if this session is still streaming
      if (ws.value && ws.value.readyState === WebSocket.OPEN) {
        ws.value.send(JSON.stringify({ type: 'subscribe', session_id: id }))
      }
    }
  }

  function newChat() {
    currentSessionId.value = 0
    messages.value = []
    streaming.value = false
    streamingContent.value = ''
    thinkingContent.value = ''
    toolCalls.value = []
  }

  async function deleteSessionById(id: number) {
    await api.deleteSession(id)
    sessions.value = sessions.value.filter((s) => s.id !== id)
    if (currentSessionId.value === id) {
      newChat()
    }
  }

  async function sendMessage(content: string) {
    if (streaming.value) return

    messages.value.push({
      id: Date.now(),
      session_id: currentSessionId.value,
      role: 'user',
      content,
      created_at: new Date().toISOString(),
    })

    streaming.value = true
    streamingContent.value = ''
    thinkingContent.value = ''
    toolCalls.value = []

    try {
      const resp = await api.sendChat(currentSessionId.value, content)
      // If it was a new session (id=0), update to the real session ID
      if (currentSessionId.value === 0 && resp.session_id) {
        currentSessionId.value = resp.session_id
        window.history.replaceState({}, '', `/chat/${resp.session_id}`)
      }
      // Subscribe to this session's stream events
      if (ws.value && ws.value.readyState === WebSocket.OPEN) {
        ws.value.send(JSON.stringify({ type: 'subscribe', session_id: resp.session_id }))
      }
    } catch (e: any) {
      streaming.value = false
      messages.value.push({
        id: Date.now(),
        session_id: currentSessionId.value,
        role: 'assistant',
        content: `Error: ${e.message}`,
        created_at: new Date().toISOString(),
      })
    }
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
    streaming,
    streamingContent,
    thinkingContent,
    toolCalls,
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
