export interface Provider {
  id: string
  name: string
  mode: string
  auth_mode: string
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
  group_name: string
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
  metadata?: string
  created_at: string
}

export interface StepInfo {
  type: 'thinking' | 'tool'
  name?: string
  input?: string
  status?: string
}

export interface StepsMetadata {
  steps: StepInfo[]
  thinking?: string
}

export interface WSMessage {
  type: 'chat' | 'stop' | 'subscribe' | 'error' | 'chunk' | 'thinking' | 'tool_start' | 'tool_input' | 'tool_result' | 'done' | 'session_created' | 'streaming_status' | 'session_update' | 'session_title_update' | 'process_update' | 'message_queued' | 'token_usage'
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

export interface Channel {
  id: number
  name: string
  platform: string
  session_id: number
  config: string
  enabled: boolean
  created_at: string
  updated_at: string
}

export interface TokenUsage {
  id: number
  session_id: number
  message_id: number
  input_tokens: number
  output_tokens: number
  cache_creation_input_tokens: number
  cache_read_input_tokens: number
  created_at: string
}

export interface TokenUsageStats {
  total_input_tokens: number
  total_output_tokens: number
  total_cache_creation_tokens: number
  total_cache_read_tokens: number
  count: number
}
