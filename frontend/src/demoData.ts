import type { DeliveryAttempt, Endpoint, EndpointHealth, EventRow, Stats } from './api'

export const demoStats: Stats = {
  total: 3,
  pending: 0,
  retrying: 1,
  delivered: 2,
  dead_letter: 0,
  processing: 0,
}

export const demoEndpoints: Endpoint[] = [
  {
    id: '11111111-2222-3333-4444-555555555555',
    tenant_id: '11111111-1111-1111-1111-111111111111',
    name: 'mock-receiver',
    url: 'http://localhost:8090/webhook',
    secret: 'demo-secret',
    is_active: true,
    created_at: new Date().toISOString(),
  },
]

export const demoEndpointHealth: EndpointHealth[] = [
  {
    id: '11111111-2222-3333-4444-555555555555',
    name: 'mock-receiver',
    url: 'http://localhost:8090/webhook',
    is_active: true,
    events_total: 3,
    delivered: 2,
    dead_letter: 0,
    retrying: 1,
    success_attempts: 2,
    failed_attempts: 1,
    avg_latency_ms: 48,
  },
]

export const demoEvents: EventRow[] = [
  {
    id: 'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa',
    tenant_id: '11111111-1111-1111-1111-111111111111',
    endpoint_id: '11111111-2222-3333-4444-555555555555',
    event_type: 'order.paid',
    payload: { order_id: 'ord_123', amount: 49.99 },
    idempotency_key: 'demo-key-1',
    status: 'delivered',
    attempt_count: 1,
    max_attempts: 8,
    next_attempt_at: new Date().toISOString(),
    created_at: new Date(Date.now() - 120000).toISOString(),
    updated_at: new Date(Date.now() - 110000).toISOString(),
    endpoint_url: 'http://localhost:8090/webhook',
    endpoint_name: 'mock-receiver',
  },
  {
    id: 'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb',
    tenant_id: '11111111-1111-1111-1111-111111111111',
    endpoint_id: '11111111-2222-3333-4444-555555555555',
    event_type: 'invoice.created',
    payload: { invoice_id: 'inv_9' },
    idempotency_key: 'demo-key-2',
    status: 'retrying',
    attempt_count: 2,
    max_attempts: 8,
    next_attempt_at: new Date(Date.now() + 30000).toISOString(),
    last_error: 'upstream status 502',
    created_at: new Date(Date.now() - 60000).toISOString(),
    updated_at: new Date(Date.now() - 20000).toISOString(),
    endpoint_url: 'http://localhost:8090/webhook',
    endpoint_name: 'mock-receiver',
  },
]

export const demoAttempts: Record<string, DeliveryAttempt[]> = {
  'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa': [
    {
      id: 'attempt-1',
      event_id: 'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa',
      attempt_number: 1,
      status: 'delivered',
      http_status: 200,
      latency_ms: 42,
      created_at: new Date(Date.now() - 110000).toISOString(),
    },
  ],
  'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb': [
    {
      id: 'attempt-2',
      event_id: 'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb',
      attempt_number: 1,
      status: 'failed',
      http_status: 502,
      error: 'upstream status 502',
      latency_ms: 51,
      created_at: new Date(Date.now() - 50000).toISOString(),
    },
    {
      id: 'attempt-3',
      event_id: 'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb',
      attempt_number: 2,
      status: 'failed',
      http_status: 502,
      error: 'upstream status 502',
      latency_ms: 48,
      created_at: new Date(Date.now() - 20000).toISOString(),
    },
  ],
}

export function isGitHubPagesHost(): boolean {
  return typeof window !== 'undefined' && window.location.hostname.endsWith('github.io')
}
