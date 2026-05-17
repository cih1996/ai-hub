import { defineStore } from 'pinia'
import { ref, computed, nextTick } from 'vue'
import type { Session, Message, Provider, WSMessage, ToolCall, StepsMetadata, TokenUsage, ContextUsage } from '../types'
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
  const compressing = ref(false)
  const recovering = ref(false)  // "No conversation found" auto-recovery in progress
  const tokenUsageMap = ref<Record<number, TokenUsage>>({})
  const latestTokenUsage = ref<TokenUsage | null>(null)
  const sessionTokenTotals = ref<Record<number, number>>() // session_id -> total tokens
  const contextUsage = ref<ContextUsage | null>(null)
  const hasMoreMessages = ref(false)
  const loadingMore = ref(false)
  // Message queue: messages typed while AI is streaming, sent as batch when done
  const messageQueue = ref<string[]>([])
  const messageQueueSessionId = ref<number>(0) // session the queue belongs to
  const ws = ref<WebSocket | null>(null)
  const wsConnected = ref(false)
  let wsReconnectDelay = 1000 // exponential backoff: 1s → 2s → 4s → ... → 30s
  let wsReconnectTimer: ReturnType<typeof setTimeout> | null = null
  // FIX #112: session_id to suppress WS streaming events for during selectSession reload.
  // Prevents pre-subscribe chunks from accumulating before the buffer replay fires.
  let _suppressChunksFor = 0


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

  /** Translate internal error messages to user-friendly text */
  function formatUserError(raw: string): string {
    const msg = (raw || '').trim()
    if (!msg) return '处理时出现问题，请重试'
    const lower = msg.toLowerCase()
    if (lower.includes('no conversation found')) return '会话已重置，请重新发送消息'
    if (lower.includes('provider not found')) return 'AI 服务配置异常，请检查供应商设置'
    if (lower.includes('context window') || lower.includes('token') && lower.includes('limit'))
      return '对话内容过长，已自动压缩上下文，请重试'
    if (lower.includes('connection refused') || lower.includes('timeout'))
      return 'AI 服务连接超时，请稍后重试'
    if (lower.includes('permission denied') || lower.includes('unauthorized'))
      return 'AI 服务认证失败，请检查 API Key 配置'
    return `处理异常：${msg}`
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

  function isLegacyCompressedMessage(message: Message) {
    return /^【系统】上下文已压缩（.+ 模式），会话已重置。$/.test((message.content || '').trim())
  }

  function filterVisibleMessages(list: Message[]) {
    return list.filter((message) => !isLegacyCompressedMessage(message))
  }

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
            flushQueue()
            api.getMessagesPaginated(msg.session_id, 50).then((resp) => {
              messages.value = filterVisibleMessages(resp.messages)
              hasMoreMessages.value = resp.has_more
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

      // context_reset: reload messages after context reset (manual or auto)
      if (msg.type === 'context_reset') {
        compressing.value = false
        if (msg.session_id === currentSessionId.value) {
          streaming.value = false
          streamingContent.value = ''
          contextUsage.value = null // reset energy bar after compression
          api.getMessagesPaginated(msg.session_id, 50).then((resp) => {
            messages.value = filterVisibleMessages(resp.messages)
            hasMoreMessages.value = resp.has_more
          })
        }
        return
      }

      // context_usage: update energy progress bar
      if (msg.type === 'context_usage') {
        if (msg.session_id === currentSessionId.value) {
          try {
            contextUsage.value = JSON.parse(msg.content)
          } catch { /* ignore */ }
        }
        return
      }

      // compressing: AI is compressing context before responding
      if (msg.type === 'compressing') {
        if (msg.session_id === currentSessionId.value) {
          compressing.value = true
          streaming.value = true
        }
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
          compressing.value = false
          recovering.value = false  // Recovery succeeded — streaming content arrived
          for (const tc of toolCalls.value) {
            if (tc.status === 'running') tc.status = 'done'
          }
          streamingContent.value += msg.content
          break
        case 'done': {
          compressing.value = false
          recovering.value = false
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
              content: streamingContent.value,
              metadata: metadata || undefined,
              created_at: new Date().toISOString(),
            })
          }
          streamingContent.value = ''
          thinkingContent.value = ''
          toolCalls.value = []
          streaming.value = false
          // Flush queued messages (typed while AI was streaming)
          flushQueue()
          break
        }
        case 'error': {
          compressing.value = false
          const rawLower = (msg.content || '').toLowerCase()
          const isNoConv = rawLower.includes('no conversation found')
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
              content: streamingContent.value,
              metadata,
              created_at: new Date().toISOString(),
            })
          }
          streamingContent.value = ''
          thinkingContent.value = ''
          toolCalls.value = []
          if (isNoConv) {
            // Backend is auto-recovering — don't show error, show recovery state
            recovering.value = true
            streaming.value = true  // Keep streaming indicator active during recovery
          } else {
            streaming.value = false
            messages.value.push({
              id: Date.now() + 1,
              session_id: msg.session_id,
              role: 'assistant',
              content: formatUserError(msg.content),
              created_at: new Date().toISOString(),
            })
          }
          detectUsageLimitWarning(msg.content)
          break
        }
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
    contextUsage.value = null
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
      messages.value = filterVisibleMessages(resp.messages)
      hasMoreMessages.value = resp.has_more
      // If successful, update workDir from sessions list (if available)
      const s = sessions.value.find((s) => s.id === id)
      workDir.value = s?.work_dir || ''
      // Fetch initial context usage for energy progress bar (non-blocking, uses real API input_tokens)
      api.getSessionContextUsage(id).then((snap) => {
        if (snap.compression_enabled && snap.provider_max_tokens > 0) {
          contextUsage.value = {
            estimated_tokens: snap.estimated_tokens,
            threshold_percent: snap.threshold_percent,
            threshold_tokens: snap.threshold_tokens,
            display_percent: snap.display_percent,
            compression_enabled: snap.compression_enabled,
          }
        }
      }).catch(() => { /* no data yet, that's fine */ })
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
      const visibleMessages = filterVisibleMessages(resp.messages)
      if (visibleMessages.length > 0) {
        messages.value = [...visibleMessages, ...messages.value]
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
    contextUsage.value = null
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

  async function sendMessage(content: string, attachments?: api.ChatAttachmentPayload[]) {
    // If currently streaming, queue the message for later
    if (streaming.value) {
      messageQueue.value.push(content)
      if (!messageQueueSessionId.value) {
        messageQueueSessionId.value = currentSessionId.value
      }
      return
    }
    clearUsageLimitWarning()

    // Build display content including inline images (matches backend buildStoredUserContent)
    let displayContent = content
    if (attachments && attachments.length > 0) {
      const imgParts: string[] = []
      if (content.trim()) imgParts.push(content.trim())
      for (const att of attachments) {
        if (att.type === 'image' && att.data && att.mime_type) {
          imgParts.push(`![${att.name || '图片'}](data:${att.mime_type};base64,${att.data})`)
        }
      }
      displayContent = imgParts.join('\n\n')
    }

    messages.value.push({
      id: Date.now(),
      session_id: currentSessionId.value,
      role: 'user',
      content: displayContent,
      created_at: new Date().toISOString(),
    })

    streaming.value = true
    streamingContent.value = ''
    thinkingContent.value = ''
    toolCalls.value = []

    try {
      const pid = currentSessionId.value === 0 ? pendingProviderId.value : undefined
      const gname = currentSessionId.value === 0 ? pendingGroupName.value : undefined
      const resp = await api.sendChat(currentSessionId.value, content, workDir.value || undefined, undefined, pid || undefined, gname || undefined, attachments)
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
      const errMsg = formatUserError(String(e?.message || ''))
      const rawLower = String(e?.message || '').toLowerCase()
      if (rawLower.includes('no conversation found')) {
        // Backend auto-recovery — show recovery state instead of error
        recovering.value = true
        streaming.value = true
      } else {
        streaming.value = false
        detectUsageLimitWarning(String(e?.message || ''))
        messages.value.push({
          id: Date.now(),
          session_id: currentSessionId.value,
          role: 'assistant',
          content: errMsg,
          created_at: new Date().toISOString(),
        })
      }
    }
  }

  function removeFromQueue(index: number) {
    messageQueue.value.splice(index, 1)
  }

  function flushQueue() {
    if (messageQueue.value.length === 0 || !messageQueueSessionId.value) return
    const targetSession = messageQueueSessionId.value
    const queued = messageQueue.value.splice(0) // take all and clear
    messageQueueSessionId.value = 0
    const combined = queued.length === 1 ? (queued[0] || '') : queued.map((q, i) => `[消息 ${i + 1}] ${q}`).join('\n\n')
    // Temporarily switch to the queue's session for sending
    const savedSession = currentSessionId.value
    currentSessionId.value = targetSession
    sendMessage(combined)
    // Restore if user had switched away
    if (savedSession !== targetSession) {
      // Don't restore — the sendMessage already updated state for targetSession.
      // User will see the target session's messages after flush.
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

  async function resetContext(keepLast = 0) {
    if (!currentSessionId.value || streaming.value) return
    try {
      const result = await api.resetSession(currentSessionId.value, keepLast)
      await selectSession(currentSessionId.value)
      await loadSessions()
      return result
    } catch (e: any) {
      messages.value.push({
        id: Date.now(),
        session_id: currentSessionId.value,
        role: 'assistant',
        content: `重置失败: ${e.message}`,
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
    compressing,
    recovering,
    tokenUsageMap,
    latestTokenUsage,
    sessionTokenTotals,
    contextUsage,
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
    resetContext,
    currentProvider,
    switchProviderForSession,
    providerSwitching,
    usageLimitWarning,
    clearUsageLimitWarning,
    inputFocusTrigger,
    triggerInputFocus,
    messageQueue,
    messageQueueSessionId,
    removeFromQueue,
    flushQueue,
  }
})
