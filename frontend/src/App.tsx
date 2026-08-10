import { useCallback, useEffect, useMemo, useState } from 'react'
import type { FormEvent } from 'react'
import {
  api,
  loadSettings,
  saveSettings,
} from './api'
import type {
  DeliveryAttempt,
  Endpoint,
  EndpointHealth,
  EventRow,
  Settings,
  Stats,
} from './api'
import {
  demoAttempts,
  demoEndpointHealth,
  demoEndpoints,
  demoEvents,
  demoStats,
  isGitHubPagesHost,
} from './demoData'
import './App.css'

const statusFilters = ['', 'pending', 'retrying', 'delivered', 'dead_letter', 'processing']

function App() {
  const [settings, setSettings] = useState<Settings>(() => loadSettings())
  const [draftSettings, setDraftSettings] = useState<Settings>(() => loadSettings())
  const [health, setHealth] = useState<string>('checking…')
  const [stats, setStats] = useState<Stats | null>(null)
  const [endpoints, setEndpoints] = useState<Endpoint[]>([])
  const [endpointHealth, setEndpointHealth] = useState<EndpointHealth[]>([])
  const [events, setEvents] = useState<EventRow[]>([])
  const [filter, setFilter] = useState('')
  const [selectedId, setSelectedId] = useState<string | null>(null)
  const [attempts, setAttempts] = useState<DeliveryAttempt[]>([])
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  const [demoMode, setDemoMode] = useState(false)

  const [endpointName, setEndpointName] = useState('mock-receiver')
  const [endpointUrl, setEndpointUrl] = useState('http://localhost:8090/webhook')
  const [endpointSecret, setEndpointSecret] = useState('demo-secret')

  const [eventEndpointId, setEventEndpointId] = useState('')
  const [eventType, setEventType] = useState('order.paid')
  const [eventPayload, setEventPayload] = useState('{"order_id":"ord_123","amount":49.99}')

  const selectedEvent = useMemo(
    () => events.find((e) => e.id === selectedId) || null,
    [events, selectedId],
  )

  const refresh = useCallback(async () => {
    try {
      setError(null)
      const [h, s, eps, eph, evs] = await Promise.all([
        api.health(),
        api.stats(),
        api.listEndpoints(),
        api.endpointHealth(),
        api.listEvents(filter || undefined),
      ])
      setDemoMode(false)
      setHealth(`${h.status} · queue=${h.queue_driver}`)
      setStats(s)
      setEndpoints(eps || [])
      setEndpointHealth(eph || [])
      setEvents(evs || [])
      if (!eventEndpointId && eps?.length) {
        setEventEndpointId(eps[0].id)
      }
      if (selectedId) {
        const detail = await api.getEvent(selectedId)
        setAttempts(detail.attempts || [])
      }
    } catch (err) {
      // GitHub Pages is HTTPS and cannot call http://localhost (browser mixed-content block).
      setDemoMode(true)
      setHealth('demo mode')
      setStats(demoStats)
      setEndpoints(demoEndpoints)
      setEndpointHealth(demoEndpointHealth)
      const filtered = filter
        ? demoEvents.filter((e) => e.status === filter)
        : demoEvents
      setEvents(filtered)
      if (!eventEndpointId) setEventEndpointId(demoEndpoints[0].id)
      if (selectedId && demoAttempts[selectedId]) {
        setAttempts(demoAttempts[selectedId])
      } else if (selectedId) {
        setAttempts([])
      }
      const msg = err instanceof Error ? err.message : 'failed to load'
      if (isGitHubPagesHost()) {
        setError(
          'Live API unreachable from GitHub Pages (HTTPS cannot call http://localhost). Showing demo data. For a live demo run: make api && make worker && make frontend-dev',
        )
      } else {
        setError(`${msg}. Start the API with make api (and make worker / make mock).`)
      }
    }
  }, [filter, selectedId, eventEndpointId])

  useEffect(() => {
    void refresh()
    const id = window.setInterval(() => void refresh(), 2500)
    return () => window.clearInterval(id)
  }, [refresh])

  function applySettings(e: FormEvent) {
    e.preventDefault()
    saveSettings(draftSettings)
    setSettings(draftSettings)
  }

  async function onCreateEndpoint(e: FormEvent) {
    e.preventDefault()
    setBusy(true)
    try {
      const ep = await api.createEndpoint({
        name: endpointName,
        url: endpointUrl,
        secret: endpointSecret || undefined,
      })
      setEventEndpointId(ep.id)
      await refresh()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'create endpoint failed')
    } finally {
      setBusy(false)
    }
  }

  async function onCreateEvent(e: FormEvent) {
    e.preventDefault()
    setBusy(true)
    try {
      let payload: unknown = {}
      try {
        payload = JSON.parse(eventPayload)
      } catch {
        throw new Error('payload must be valid JSON')
      }
      const res = await api.createEvent({
        endpoint_id: eventEndpointId,
        event_type: eventType,
        payload,
      })
      setSelectedId(res.event.id)
      await refresh()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'create event failed')
    } finally {
      setBusy(false)
    }
  }

  async function onSelectEvent(id: string) {
    setSelectedId(id)
    if (demoMode && demoAttempts[id]) {
      setAttempts(demoAttempts[id])
      return
    }
    try {
      const detail = await api.getEvent(id)
      setAttempts(detail.attempts || [])
    } catch (err) {
      if (demoAttempts[id]) {
        setAttempts(demoAttempts[id])
        return
      }
      setError(err instanceof Error ? err.message : 'load event failed')
    }
  }

  async function onReplay(id: string) {
    setBusy(true)
    try {
      await api.replayEvent(id)
      await refresh()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'replay failed')
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="page">
      <header className="hero">
        <div>
          <p className="eyebrow">Webhook Delivery Platform</p>
          <h1>Delivery Console</h1>
          <p className="sub">
            Inspect signed deliveries, retries, dead letters, and manual replay. API key currently:{' '}
            <code>{settings.apiKey}</code>
          </p>
        </div>
        <div className={`pill ${health.startsWith('ok') ? 'ok' : demoMode ? 'warn' : 'bad'}`}>
          {health}
        </div>
      </header>

      {demoMode && (
        <div className="banner warn">
          Demo mode — sample data only. For live deliveries locally: <code>make api</code>,{' '}
          <code>make worker</code>, <code>make mock</code>, then open{' '}
          <code>http://localhost:5173</code> (not the GitHub Pages URL).
        </div>
      )}
      {error && <div className="banner error">{error}</div>}

      <section className="grid stats">
        {[
          ['total', stats?.total],
          ['pending', stats?.pending],
          ['retrying', stats?.retrying],
          ['delivered', stats?.delivered],
          ['dead_letter', stats?.dead_letter],
          ['processing', stats?.processing],
        ].map(([label, value]) => (
          <article key={label as string}>
            <span>{label}</span>
            <strong>{value ?? '—'}</strong>
          </article>
        ))}
      </section>

      <section className="panel">
        <h2>Connection</h2>
        <form className="row" onSubmit={applySettings}>
          <label>
            API Base
            <input
              value={draftSettings.apiBase}
              onChange={(e) => setDraftSettings({ ...draftSettings, apiBase: e.target.value })}
            />
          </label>
          <label>
            API Key
            <input
              value={draftSettings.apiKey}
              onChange={(e) => setDraftSettings({ ...draftSettings, apiKey: e.target.value })}
            />
          </label>
          <button type="submit">Save</button>
        </form>
        <p className="hint">
          Live mode needs a local API. On GitHub Pages the browser blocks HTTPS → http://localhost,
          so you&apos;ll see demo data. For live traffic use <code>http://localhost:5173</code> with{' '}
          <code>make api</code> / <code>make worker</code> / <code>make mock</code> running.
        </p>
      </section>

      <div className="split">
        <section className="panel">
          <h2>Create endpoint</h2>
          <form className="stack" onSubmit={onCreateEndpoint}>
            <label>
              Name
              <input value={endpointName} onChange={(e) => setEndpointName(e.target.value)} />
            </label>
            <label>
              URL
              <input value={endpointUrl} onChange={(e) => setEndpointUrl(e.target.value)} />
            </label>
            <label>
              HMAC secret
              <input value={endpointSecret} onChange={(e) => setEndpointSecret(e.target.value)} />
            </label>
            <button disabled={busy} type="submit">
              Register endpoint
            </button>
          </form>

          <h3>Endpoints</h3>
          <ul className="list">
            {endpoints.map((ep) => {
              const health = endpointHealth.find((h) => h.id === ep.id)
              return (
                <li key={ep.id}>
                  <button type="button" className="linkish" onClick={() => setEventEndpointId(ep.id)}>
                    <strong>
                      {ep.name} {ep.is_active ? '' : '(inactive)'}
                    </strong>
                    <span>{ep.url}</span>
                    {health && (
                      <span>
                        delivered {health.delivered}/{health.events_total} · dead{' '}
                        {health.dead_letter} · avg {health.avg_latency_ms.toFixed(0)}ms
                      </span>
                    )}
                  </button>
                  <button
                    type="button"
                    className="linkish"
                    disabled={busy}
                    onClick={() =>
                      void (async () => {
                        setBusy(true)
                        try {
                          await api.setEndpointActive(ep.id, !ep.is_active)
                          await refresh()
                        } catch (err) {
                          setError(err instanceof Error ? err.message : 'toggle failed')
                        } finally {
                          setBusy(false)
                        }
                      })()
                    }
                  >
                    {ep.is_active ? 'Disable endpoint' : 'Enable endpoint'}
                  </button>
                </li>
              )
            })}
            {!endpoints.length && <li className="muted">No endpoints yet</li>}
          </ul>
        </section>

        <section className="panel">
          <h2>Publish event</h2>
          <form className="stack" onSubmit={onCreateEvent}>
            <label>
              Endpoint
              <select
                value={eventEndpointId}
                onChange={(e) => setEventEndpointId(e.target.value)}
                required
              >
                <option value="" disabled>
                  Select endpoint
                </option>
                {endpoints.map((ep) => (
                  <option key={ep.id} value={ep.id}>
                    {ep.name}
                  </option>
                ))}
              </select>
            </label>
            <label>
              Event type
              <input value={eventType} onChange={(e) => setEventType(e.target.value)} />
            </label>
            <label>
              Payload JSON
              <textarea
                rows={6}
                value={eventPayload}
                onChange={(e) => setEventPayload(e.target.value)}
              />
            </label>
            <button disabled={busy || !eventEndpointId} type="submit">
              Send event
            </button>
          </form>
        </section>
      </div>

      <section className="panel">
        <div className="row between">
          <h2>Events</h2>
          <label className="inline">
            Status
            <select value={filter} onChange={(e) => setFilter(e.target.value)}>
              {statusFilters.map((s) => (
                <option key={s || 'all'} value={s}>
                  {s || 'all'}
                </option>
              ))}
            </select>
          </label>
        </div>
        <div className="table-wrap">
          <table>
            <thead>
              <tr>
                <th>Type</th>
                <th>Status</th>
                <th>Attempts</th>
                <th>Endpoint</th>
                <th>Updated</th>
              </tr>
            </thead>
            <tbody>
              {events.map((ev) => (
                <tr
                  key={ev.id}
                  className={ev.id === selectedId ? 'selected' : ''}
                  onClick={() => void onSelectEvent(ev.id)}
                >
                  <td>{ev.event_type}</td>
                  <td>
                    <span className={`tag ${ev.status}`}>{ev.status}</span>
                  </td>
                  <td>
                    {ev.attempt_count}/{ev.max_attempts}
                  </td>
                  <td>{ev.endpoint_name || ev.endpoint_url}</td>
                  <td>{new Date(ev.updated_at).toLocaleString()}</td>
                </tr>
              ))}
              {!events.length && (
                <tr>
                  <td colSpan={5} className="muted">
                    No events yet
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        </div>
      </section>

      {selectedEvent && (
        <section className="panel">
          <div className="row between">
            <h2>Event detail</h2>
            <button disabled={busy} type="button" onClick={() => void onReplay(selectedEvent.id)}>
              Replay / requeue
            </button>
          </div>
          <pre className="code">{JSON.stringify(selectedEvent, null, 2)}</pre>
          <h3>Retry timeline</h3>
          <ul className="timeline">
            {attempts.map((a) => (
              <li key={a.id}>
                <div>
                  <strong>
                    #{a.attempt_number} · {a.status}
                  </strong>
                  <span>
                    HTTP {a.http_status ?? '—'} · {a.latency_ms ?? '—'}ms ·{' '}
                    {new Date(a.created_at).toLocaleString()}
                  </span>
                  {a.error && <em>{a.error}</em>}
                </div>
              </li>
            ))}
            {!attempts.length && <li className="muted">No attempts recorded yet</li>}
          </ul>
        </section>
      )}
    </div>
  )
}

export default App
