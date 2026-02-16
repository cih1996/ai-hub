import type { Provider, Session, Message } from '../types'

const BASE = '/api/v1'

async function request<T>(url: string, options?: RequestInit): Promise<T> {
  const res = await fetch(BASE + url, {
    headers: { 'Content-Type': 'application/json' },
    ...options,
  })
  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: res.statusText }))
    throw new Error(err.error || res.statusText)
  }
  return res.json()
}

// Providers
export const listProviders = () => request<Provider[]>('/providers')
export const createProvider = (p: Partial<Provider>) =>
  request<Provider>('/providers', { method: 'POST', body: JSON.stringify(p) })
export const updateProvider = (id: string, p: Partial<Provider>) =>
  request<Provider>(`/providers/${id}`, { method: 'PUT', body: JSON.stringify(p) })
export const deleteProvider = (id: string) =>
  request<{ ok: boolean }>(`/providers/${id}`, { method: 'DELETE' })

// Sessions
export const listSessions = () => request<Session[]>('/sessions')
export const getSession = (id: number) => request<Session>(`/sessions/${id}`)
export const updateSession = (id: number, s: Partial<Session>) =>
  request<Session>(`/sessions/${id}`, { method: 'PUT', body: JSON.stringify(s) })
export const deleteSession = (id: number) =>
  request<{ ok: boolean }>(`/sessions/${id}`, { method: 'DELETE' })
export const getMessages = (sessionId: number) =>
  request<Message[]>(`/sessions/${sessionId}/messages`)

// Chat
export const sendChat = (sessionId: number, content: string) =>
  request<{ session_id: number; status: string }>('/chat/send', {
    method: 'POST',
    body: JSON.stringify({ session_id: sessionId, content }),
  })

// Status
export interface DepsStatus {
  node_installed: boolean
  node_version: string
  npm_installed: boolean
  npm_version: string
  claude_installed: boolean
  claude_version: string
  installing: boolean
  install_error: string
  install_hint: string
}
export const getStatus = () => request<DepsStatus>('/status')
export const retryInstall = () =>
  request<{ ok: boolean }>('/status/retry-install', { method: 'POST' })
