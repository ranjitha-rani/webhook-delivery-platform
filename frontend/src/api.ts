export type Endpoint = {
  id: string
  tenant_id: string
  name: string
  url: string
  secret: string
  is_active: boolean
  created_at: string
}

export type EventRow = {
  id: string
  tenant_id: string
  endpoint_id: string
  event_type: string
  payload: unknown
  idempotency_key: string
  status: string
  attempt_count: number
  max_attempts: number
  next_attempt_at: string
  last_error?: string
  created_at: string
  updated_at: string
  endpoint_url?: string
  endpoint_name?: string
}

export type DeliveryAttempt = {
  id: string
  event_id: string
  attempt_number: number
  status: string
  http_status?: number
  response_body?: string
  latency_ms?: number
  error?: string
  created_at: string
}

export type Stats = {
  pending: number
  retrying: number
  delivered: number
  dead_letter: number
  processing: number
  total: number
}

export type EndpointHealth = {
  id: string
  name: string
  url: string
  is_active: boolean
  events_total: number
  delivered: number
  dead_letter: number
  retrying: number
  success_attempts: number
  failed_attempts: number
  avg_latency_ms: number
}

const storageKey = 'webhook_console_settings'

export type Settings = {
  apiBase: string
  apiKey: string
}

export function loadSettings(): Settings {
  const raw = localStorage.getItem(storageKey)
  if (raw) {
    try {
      return JSON.parse(raw) as Settings
    } catch {
      /* ignore */
    }
  }
  return {
    apiBase: import.meta.env.VITE_API_BASE || 'http://localhost:8080',
    apiKey: import.meta.env.VITE_API_KEY || 'dev-tenant-key',
  }
}

export function saveSettings(settings: Settings) {
  localStorage.setItem(storageKey, JSON.stringify(settings))
}

async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
  const settings = loadSettings()
  const headers = new Headers(init.headers)
  headers.set('X-API-Key', settings.apiKey)
  if (init.body && !headers.has('Content-Type')) {
    headers.set('Content-Type', 'application/json')
  }
  const res = await fetch(`${settings.apiBase.replace(/\/$/, '')}${path}`, {
    ...init,
    headers,
  })
  if (!res.ok) {
    let message = res.statusText
    try {
      const body = await res.json()
      if (body?.error) message = body.error
    } catch {
      /* ignore */
    }
    throw new Error(message)
  }
  if (res.status === 204) return undefined as T
  return res.json() as Promise<T>
}

export const api = {
  health: () => request<{ status: string; queue_driver: string }>('/healthz'),
  stats: () => request<Stats>('/api/v1/stats'),
  listEndpoints: () => request<Endpoint[]>('/api/v1/endpoints'),
  endpointHealth: () => request<EndpointHealth[]>('/api/v1/endpoints/health'),
  createEndpoint: (body: { name: string; url: string; secret?: string }) =>
    request<Endpoint>('/api/v1/endpoints', { method: 'POST', body: JSON.stringify(body) }),
  setEndpointActive: (id: string, is_active: boolean) =>
    request<Endpoint>(`/api/v1/endpoints/${id}`, {
      method: 'PATCH',
      body: JSON.stringify({ is_active }),
    }),
  listEvents: (status?: string) =>
    request<EventRow[]>(`/api/v1/events${status ? `?status=${encodeURIComponent(status)}` : ''}`),
  createEvent: (body: {
    endpoint_id: string
    event_type: string
    payload: unknown
    idempotency_key?: string
  }) => request<{ event: EventRow; replayed: boolean }>('/api/v1/events', {
    method: 'POST',
    body: JSON.stringify(body),
  }),
  getEvent: (id: string) =>
    request<{ event: EventRow; attempts: DeliveryAttempt[] }>(`/api/v1/events/${id}`),
  replayEvent: (id: string) =>
    request<EventRow>(`/api/v1/events/${id}/replay`, { method: 'POST' }),
}
