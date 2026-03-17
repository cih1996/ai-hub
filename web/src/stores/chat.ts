import { defineStore } from 'pinia'
import { ref, computed, nextTick } from 'vue'
import type { Session, Message, Provider, WSMessage, ToolCall, StepsMetadata, TokenUsage } from '../types'
import * as api from '../composables/api'
import router from '../router'

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
  const hasMoreMessages = ref(false)
  const loadingMore = ref(false)
  const ws = ref<WebSocket | null>(null)
  const wsConnected = ref(false)
  let wsReconnectDelay = 1000 // exponential backoff: 1s → 2s → 4s → ... → 30s
  let wsReconnectTimer: ReturnType<typeof setTimeout> | null = null
  // FIX #112: session_id to suppress WS streaming events for during selectSession reload.
  // Prevents pre-subscribe chunks from accumulating before the buffer replay fires.
  let _suppressChunksFor = 0

  // Attention mode v2 status tracking
  const attentionStatus = ref('')  // Current status message
  const attentionActive = ref(false)  // Whether attention mode is currently running

  const workDir = ref('')
  const pendingProviderId = ref('')  // provider selected in new-chat dialog
  const pendingGroupName = ref('')   // group_name selected in new-chat dialog

  // Input focus event trigger for sidebar highlight
  const inputFocusTrigger = ref(0)
  function triggerInputFocus() {
    inputFocusTrigger.value++
  }

  // Upstream quota/rate-limit warning (e.g. "You've hit your limit").
  const usageLimitWarning = ref('')

  function clearUsageLimitWarning() {
    usageLimitWarning.value = ''
  }

  function detectUsageLimitWarning(raw: string) {
    const msg = (raw || '').trim()
    if (!msg) return
    const lower = msg.toLowerCase()
    const hit =
      lower.includes("you've hit your limit") ||
      lower.includes('hit your limit') ||
      lower.includes('rate limit') ||
      lower.includes('quota') ||
      msg.includes('额度') ||
      msg.includes('配额')
    if (!hit) return

    const m = msg.match(/resets[^.。\n]*/i)
    if (m?.[0]) {
      usageLimitWarning.value = `当前账号额度已用尽，${m[0]}。请切换供应商或稍后重试。`
    } else {
      usageLimitWarning.value = '当前账号额度已用尽，请切换供应商或等待额度重置后再试。'
    }
  }

  // Model switch debounce lock
  const providerSwitching = ref(false)

  const currentSession = computed(() =>
    sessions.value.find((s) => s.id === currentSessionId.value)
  )

  // Count of sessions currently streaming (in conversation)
  const busySessionCount = computed(() =>
    sessions.value.filter((s) => s.streaming).length
  )

  // Other sessions streaming status (for AI worker status component)
  const otherSessionsStreaming = computed(() => {
    const result: Record<number, boolean> = {}
    for (const s of sessions.value) {
      if (s.id !== currentSessionId.value && s.streaming) {
        result[s.id] = true
      }
    }
    return result
  })

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
            // Clear attention status when session becomes idle
            attentionActive.value = false
            attentionStatus.value = ''
            api.getMessagesPaginated(msg.session_id, 50).then((resp) => {
              messages.value = resp.messages
              hasMoreMessages.value = resp.has_more
            })
          }
        }
        return
      }

      // attention_status: attention mode status update
      if (msg.type === 'attention_status') {
        if (msg.session_id === currentSessionId.value) {
          attentionActive.value = true
          attentionStatus.value = msg.content
        }
        return
      }

      // attention_clear: clear attention mode status
      if (msg.type === 'attention_clear') {
        if (msg.session_id === currentSessionId.value) {
          attentionActive.value = false
          attentionStatus.value = ''
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

      // FIX #112: suppress streaming events for a session being reloaded.
      // During the async getMessages window in selectSession, the server may still
      // send this session's chunks (old subscription). Allowing them to accumulate
      // before the subscribe-replay fires doubles the content.
      if (_suppressChunksFor > 0 && msg.session_id === _suppressChunksFor) return
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
          detectUsageLimitWarning(msg.content)
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
    if (id === 0) {
      newChat()
      return
    }

    // FIX #112 (Case 1): navigating back to the SAME session while it is still streaming
    // (e.g. user went to Settings page and returned). The WS is already subscribed and
    // chunks are continuously arriving — just return. Resetting + re-subscribing would
    // cause SwapSendAndReplay to replay the full buffer on top of already-live chunks,
    // doubling the content.
    if (id === currentSessionId.value && streaming.value) {
      return
    }

    currentSessionId.value = id
    streaming.value = false
    streamingContent.value = ''
    thinkingContent.value = ''
    toolCalls.value = []
    latestTokenUsage.value = null
    clearUsageLimitWarning()

    // FIX #112 (Case 2): block incoming WS streaming events for `id` during the
    // async getMessages load. The server-side ActiveStream keeps the old sendFn
    // pointing at this WS connection even after a session switch, so chunks for `id`
    // can arrive and accumulate in streamingContent before subscribe fires its replay,
    // resulting in doubled content. We suppress them here and clear the guard just
    // before subscribe so the replay lands on a clean slate.
    _suppressChunksFor = id

    // Try to load messages for this session (paginated: latest 50)
    try {
      const resp = await api.getMessagesPaginated(id, 50)
      messages.value = resp.messages
      hasMoreMessages.value = resp.has_more
      // If successful, update workDir from sessions list (if available)
      const s = sessions.value.find((s) => s.id === id)
      workDir.value = s?.work_dir || ''
    } catch (err: any) {
      // If session doesn't exist (404), redirect to new chat
      // Check error message since request() throws plain Error without response property
      const errorMsg = err.message || String(err)
      if (errorMsg.includes('session not found') || errorMsg.includes('404')) {
        _suppressChunksFor = 0
        console.log('[selectSession] Session not found, redirecting to /chat. Error:', errorMsg)
        newChat()
        // Use nextTick to ensure router navigation happens after state updates
        nextTick(() => {
          console.log('[selectSession] Executing router.replace("/chat")')
          router.replace('/chat').then(() => {
            console.log('[selectSession] Router navigation completed')
          }).catch((navErr) => {
            console.error('[selectSession] Router navigation failed:', navErr)
          })
        })
        return
      }
      // For other errors, still show the session but with empty messages
      messages.value = []
      console.error('Failed to load messages:', err)
    }

    // Clear suppression BEFORE subscribe so the replay events are processed normally
    _suppressChunksFor = 0

    // Load token usage for this session's messages
    try {
      const resp = await api.getSessionTokenUsage(id)
      for (const r of resp.records) {
        if (r.message_id) tokenUsageMap.value[r.message_id] = r
      }
    } catch { /* ignore if no usage data */ }
    // Subscribe to check if this session is still streaming.
    // At this point _suppressChunksFor is 0, so the replay from SwapSendAndReplay
    // lands on a clean streamingContent — no double-write.
    if (ws.value && ws.value.readyState === WebSocket.OPEN) {
      ws.value.send(JSON.stringify({ type: 'subscribe', session_id: id }))
    }
  }

  async function loadMoreMessages() {
    if (!hasMoreMessages.value || loadingMore.value || currentSessionId.value <= 0) return
    loadingMore.value = true
    try {
      const oldestId = messages.value.length > 0 ? messages.value[0]!.id : 0
      const resp = await api.getMessagesPaginated(currentSessionId.value, 50, oldestId)
      if (resp.messages.length > 0) {
        messages.value = [...resp.messages, ...messages.value]
      }
      hasMoreMessages.value = resp.has_more
    } catch (e) {
      console.error('Failed to load more messages:', e)
    } finally {
      loadingMore.value = false
    }
  }

  function newChat(providerId?: string, groupName?: string) {
    currentSessionId.value = 0
    messages.value = []
    hasMoreMessages.value = false
    streaming.value = false
    streamingContent.value = ''
    thinkingContent.value = ''
    toolCalls.value = []
    workDir.value = ''
    latestTokenUsage.value = null
    clearUsageLimitWarning()
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
    clearUsageLimitWarning()

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
      detectUsageLimitWarning(String(e?.message || ''))
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
    // Save any already-received content before stopping
    if (streamingContent.value || toolCalls.value.length > 0 || thinkingContent.value) {
      let metadata: string | undefined
      if (toolCalls.value.length > 0 || thinkingContent.value) {
        const steps: StepsMetadata['steps'] = []
        if (thinkingContent.value) {
          steps.push({ type: 'thinking', name: 'Thinking', status: 'interrupted' })
        }
        for (const tc of toolCalls.value) {
          steps.push({ type: 'tool', name: tc.name, input: tc.input?.slice(0, 300), status: 'interrupted' })
        }
        metadata = JSON.stringify({ steps, thinking: thinkingContent.value?.slice(0, 200) })
      }
      const content = streamingContent.value
        ? streamingContent.value + '\n\n*[已中断]*'
        : '[任务已中断，详见执行步骤]'
      messages.value.push({
        id: Date.now(),
        session_id: currentSessionId.value!,
        role: 'assistant',
        content,
        metadata,
        created_at: new Date().toISOString(),
      })
    }
    // Clear streaming state
    streamingContent.value = ''
    thinkingContent.value = ''
    toolCalls.value = []
    streaming.value = false
    // Send stop signal to backend
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
    if (!currentSessionId.value || streaming.value || providerSwitching.value) return
    providerSwitching.value = true
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
    } finally {
      providerSwitching.value = false
    }
  }

  return {
    sessions,
    currentSessionId,
    currentSession,
    busySessionCount,
    otherSessionsStreaming,
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
    loadMoreMessages,
    hasMoreMessages,
    loadingMore,
    newChat,
    deleteSessionById,
    sendMessage,
    stopStreaming,
    compressContext,
    currentProvider,
    switchProviderForSession,
    providerSwitching,
    usageLimitWarning,
    clearUsageLimitWarning,
    inputFocusTrigger,
    triggerInputFocus,
    // Attention mode v2
    attentionActive,
    attentionStatus,
  }
})
