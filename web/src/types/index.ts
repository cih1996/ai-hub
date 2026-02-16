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
  type: 'chat' | 'stop' | 'error' | 'chunk' | 'done' | 'session_created' | 'title_update'
  session_id: number
  content: string
}
