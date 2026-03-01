import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import type { Session, Message, Provider, WSMessage, ToolCall, StepsMetadata, TokenUsage } from '../types'
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
  const tokenUsageMap = ref<Record<number, TokenUsage>>({})
  const latestTokenUsage = ref<TokenUsage | null>(null)
  const sessionTokenTotals = ref<Record<number, number>>() // session_id -> total tokens
  const ws = ref<WebSocket | null>(null)
  const wsConnected = ref(false)
  let wsReconnectDelay = 1000 // exponential backoff: 1s → 2s → 4s → ... → 30s
  let wsReconnectTimer: ReturnType<typeof setTimeout> | null = null

  const workDir = ref('')
  const pendingProviderId = ref('')  // provider selected in new-chat dialog
  const pendingGroupName = ref('')   // group_name selected in new-chat dialog

  const currentSession = computed(() =>
    sessions.value.find((s) => s.id === currentSessionId.value)
  )

  const defaultProvider = computed(() =>
    providers.value.find((p) => p.is_default) || providers.value[0]
  )

  function connectWS() {
    if (ws.value && ws.value.readyState === WebSocket.OPEN) return
    if (wsReconnectTimer) {
      clearTimeout(wsReconnectTimer)
      wsReconnectTimer = null
    }

    const protocol = location.protocol === 'https:' ? 'wss:' : 'ws:'
    const wsUrl = `${protocol}//${location.host}/ws/chat`
    ws.value = new WebSocket(wsUrl)

    ws.value.onopen = () => {
      wsConnected.value = true
      wsReconnectDelay = 1000 // reset backoff on successful connect
      // Reattach to active stream if viewing a session
      if (currentSessionId.value > 0) {
        ws.value?.send(JSON.stringify({ type: 'subscribe', session_id: currentSessionId.value }))
      }
      // Refresh sessions and version after reconnect
      loadSessions()
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

      // session_title_update: AI-generated title from CLI
      if (msg.type === 'session_title_update') {
        const s = sessions.value.find((s) => s.id === msg.session_id)
        if (s) s.title = msg.content
        return
      }

      // session_update: broadcast from server about any session's streaming status
      if (msg.type === 'session_update') {
        const s = sessions.value.find((s) => s.id === msg.session_id)
        if (s) {
          const wasStreaming = s.streaming
          s.streaming = msg.content === 'streaming'
          // Sync process state from streaming status
          if (s.streaming) {
            s.process_alive = true
            s.process_state = 'busy'
          } else {
            s.process_alive = true
            s.process_state = 'idle'
          }
          // When current session transitions to idle, reload messages to catch results
          // (e.g. trigger-fired sessions where no subscribe was active during streaming)
          if (wasStreaming && !s.streaming && msg.session_id === currentSessionId.value) {
            streaming.value = false
            streamingContent.value = ''
            thinkingContent.value = ''
            toolCalls.value = []
            api.getMessages(msg.session_id).then((msgs) => {
              messages.value = msgs
            })
          }
        }
        return
      }

      // process_update: process state change from pool
      if (msg.type === 'process_update') {
        const s = sessions.value.find((s) => s.id === msg.session_id)
        if (s) {
          if (msg.content === 'process_exit') {
            s.process_alive = false
            s.process_state = ''
          } else if (msg.content.startsWith('process_alive:')) {
            s.process_alive = true
            s.process_state = msg.content.split(':')[1] || 'idle'
          }
        }
        return
      }

      // message_queued: a message was saved while session was streaming
      if (msg.type === 'message_queued') {
        // If viewing this session, add the queued message to the list
        if (msg.session_id === currentSessionId.value) {
          messages.value.push({
            id: Date.now(),
            session_id: msg.session_id,
            role: 'user',
            content: msg.content,
            created_at: new Date().toISOString(),
          })
        }
        return
      }

      // token_usage: store token usage for a message
      if (msg.type === 'token_usage') {
        try {
          const usage: TokenUsage = JSON.parse(msg.content)
          if (usage.message_id) {
            tokenUsageMap.value[usage.message_id] = usage
          }
          if (msg.session_id === currentSessionId.value) {
            latestTokenUsage.value = usage
          }
          // Update session totals cache
          if (!sessionTokenTotals.value) sessionTokenTotals.value = {}
          const prev = sessionTokenTotals.value[msg.session_id] || 0
          sessionTokenTotals.value[msg.session_id] = prev + (usage.input_tokens || 0) + (usage.output_tokens || 0) + (usage.cache_creation_input_tokens || 0) + (usage.cache_read_input_tokens || 0)
        } catch { /* ignore parse errors */ }
        return
      }

      // All other events: ignore if not for the current session
      if (msg.session_id !== currentSessionId.value) return

      switch (msg.type) {
        case 'streaming_status':
          if (msg.content === 'idle') {
            // Server says session is not streaming — correct local state
            streaming.value = false
            streamingContent.value = ''
            thinkingContent.value = ''
            toolCalls.value = []
          } else {
            streaming.value = true
          }
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
        case 'done': {
          // Build metadata from server response or local state
          let metadata = msg.content || ''
          if (!metadata && (thinkingContent.value || toolCalls.value.length > 0)) {
            const steps: StepsMetadata['steps'] = []
            if (thinkingContent.value) {
              steps.push({ type: 'thinking', name: 'Thinking', status: 'done' })
            }
            for (const tc of toolCalls.value) {
              steps.push({ type: 'tool', name: tc.name, input: tc.input?.slice(0, 300), status: 'done' })
            }
            const meta: StepsMetadata = {
              steps,
              thinking: thinkingContent.value?.slice(0, 200),
            }
            metadata = JSON.stringify(meta)
          }
          if (streamingContent.value || metadata) {
            messages.value.push({
              id: Date.now(),
              session_id: msg.session_id,
              role: 'assistant',
              content: streamingContent.value || '[任务已执行，详见执行步骤]',
              metadata: metadata || undefined,
              created_at: new Date().toISOString(),
            })
          }
          streamingContent.value = ''
          thinkingContent.value = ''
          toolCalls.value = []
          streaming.value = false
          break
        }
        case 'error':
          // Preserve already-received content before clearing
          if (streamingContent.value || toolCalls.value.length > 0 || thinkingContent.value) {
            let metadata: string | undefined
            if (toolCalls.value.length > 0 || thinkingContent.value) {
              const steps: StepsMetadata['steps'] = []
              if (thinkingContent.value) {
                steps.push({ type: 'thinking', name: 'Thinking', status: 'done' })
              }
              for (const tc of toolCalls.value) {
                steps.push({ type: 'tool', name: tc.name, input: tc.input?.slice(0, 300), status: 'done' })
              }
              metadata = JSON.stringify({ steps, thinking: thinkingContent.value?.slice(0, 200) })
            }
            messages.value.push({
              id: Date.now(),
              session_id: msg.session_id,
              role: 'assistant',
              content: streamingContent.value || '[任务已执行，详见执行步骤]',
              metadata,
              created_at: new Date().toISOString(),
            })
          }
          streamingContent.value = ''
          thinkingContent.value = ''
          toolCalls.value = []
          streaming.value = false
          messages.value.push({
            id: Date.now() + 1,
            session_id: msg.session_id,
            role: 'assistant',
            content: `Error: ${msg.content}`,
            created_at: new Date().toISOString(),
          })
          break
      }
    }

    ws.value.onclose = () => {
      wsConnected.value = false
      wsReconnectTimer = setTimeout(connectWS, wsReconnectDelay)
      wsReconnectDelay = Math.min(wsReconnectDelay * 2, 30000) // cap at 30s
    }

    ws.value.onerror = () => {
      // onerror is always followed by onclose, so just mark disconnected
      wsConnected.value = false
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
    latestTokenUsage.value = null
    if (id === 0) {
      messages.value = []
      workDir.value = ''
    } else {
      const s = sessions.value.find((s) => s.id === id)
      workDir.value = s?.work_dir || ''
      messages.value = await api.getMessages(id)
      // Load token usage for this session's messages
      try {
        const resp = await api.getSessionTokenUsage(id)
        for (const r of resp.records) {
          if (r.message_id) tokenUsageMap.value[r.message_id] = r
        }
      } catch { /* ignore if no usage data */ }
      // Subscribe to check if this session is still streaming
      if (ws.value && ws.value.readyState === WebSocket.OPEN) {
        ws.value.send(JSON.stringify({ type: 'subscribe', session_id: id }))
      }
    }
  }

  function newChat(providerId?: string, groupName?: string) {
    currentSessionId.value = 0
    messages.value = []
    streaming.value = false
    streamingContent.value = ''
    thinkingContent.value = ''
    toolCalls.value = []
    workDir.value = ''
    latestTokenUsage.value = null
    pendingProviderId.value = providerId || ''
    pendingGroupName.value = groupName || ''
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
      const pid = currentSessionId.value === 0 ? pendingProviderId.value : undefined
      const gname = currentSessionId.value === 0 ? pendingGroupName.value : undefined
      const resp = await api.sendChat(currentSessionId.value, content, workDir.value || undefined, undefined, pid || undefined, gname || undefined)
      // If it was a new session (id=0), update to the real session ID
      if (currentSessionId.value === 0 && resp.session_id) {
        pendingProviderId.value = ''  // clear after session created
        pendingGroupName.value = ''   // clear after session created
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

  async function compressContext() {
    if (!currentSessionId.value || streaming.value) return
    try {
      await api.compressSession(currentSessionId.value)
      await selectSession(currentSessionId.value)
      await loadSessions()
    } catch (e: any) {
      messages.value.push({
        id: Date.now(),
        session_id: currentSessionId.value,
        role: 'assistant',
        content: `压缩失败: ${e.message}`,
        created_at: new Date().toISOString(),
      })
    }
  }

  const currentProvider = computed(() => {
    const s = currentSession.value
    if (!s) return defaultProvider.value
    return providers.value.find((p) => String(p.id) === String(s.provider_id)) || defaultProvider.value
  })

  async function switchProviderForSession(providerId: string) {
    if (!currentSessionId.value || streaming.value) return
    try {
      await api.switchProvider(currentSessionId.value, providerId)
      // Update local session's provider_id
      const s = sessions.value.find((s) => s.id === currentSessionId.value)
      if (s) s.provider_id = providerId
      await selectSession(currentSessionId.value)
    } catch (e: any) {
      messages.value.push({
        id: Date.now(),
        session_id: currentSessionId.value,
        role: 'assistant',
        content: `切换失败: ${e.message}`,
        created_at: new Date().toISOString(),
      })
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
    tokenUsageMap,
    latestTokenUsage,
    sessionTokenTotals,
    workDir,
    pendingProviderId,
    pendingGroupName,
    wsConnected,
    connectWS,
    loadProviders,
    loadSessions,
    selectSession,
    newChat,
    deleteSessionById,
    sendMessage,
    stopStreaming,
    compressContext,
    currentProvider,
    switchProviderForSession,
  }
})
