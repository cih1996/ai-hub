export interface Provider {
  id: string
  name: string
  mode: string
  base_url: string
  api_key: string
  model_id: string
  is_default: boolean
  created_at: string
  updated_at: string
}

export interface Session {
  id: number
  title: string
  provider_id: string
  streaming: boolean
  created_at: string
  updated_at: string
}

export interface Message {
  id: number
  session_id: number
  role: 'user' | 'assistant'
  content: string
  created_at: string
}

export interface WSMessage {
  type: 'chat' | 'stop' | 'subscribe' | 'error' | 'chunk' | 'thinking' | 'tool_start' | 'tool_input' | 'tool_result' | 'done' | 'session_created' | 'streaming_status' | 'session_update'
  session_id: number
  content: string
  tool_id?: string
  tool_name?: string
}

export interface ToolCall {
  id: string
  name: string
  input: string
  status: 'running' | 'done'
}
