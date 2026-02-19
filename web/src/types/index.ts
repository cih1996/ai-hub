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
  work_dir: string
  streaming: boolean
  has_triggers: boolean
  process_alive: boolean
  process_state: string
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
  type: 'chat' | 'stop' | 'subscribe' | 'error' | 'chunk' | 'thinking' | 'tool_start' | 'tool_input' | 'tool_result' | 'done' | 'session_created' | 'streaming_status' | 'session_update' | 'session_title_update'
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

export interface Trigger {
  id: number
  session_id: number
  content: string
  trigger_time: string
  max_fires: number
  enabled: boolean
  fired_count: number
  status: string
  next_fire_at: string
  last_fired_at: string
  created_at: string
  updated_at: string
}
