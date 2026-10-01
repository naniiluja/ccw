import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'
import { lastDays, lowQuotaAccounts } from '@/api/overview'
import { renderApp, useSession } from '@/test/render'
import { server } from '@/test/server'

const accounts = [
  { id: 'c1', provider: 'claude', label: 'work@example.com', isActive: true, standby: false, baseUrl: '' },
  { id: 'c2', provider: 'groq', label: 'groq-main', isActive: true, standby: false, baseUrl: '' },
  { id: 'c3', provider: 'zen', label: 'zen-spare', isActive: true, standby: false, baseUrl: '' },
]

const tests = {
  c1: { ok: true, status: 200, ms: 100, message: '', model: 'm', at: '2030-01-01T00:00:00Z' },
  c2: { ok: false, status: 500, ms: 100, message: 'boom', model: 'm', at: '2030-01-01T00:00:00Z' },
}

const quota = (id: string, label: string, usedPct: number) => ({
  connectionId: id,
  provider: 'claude',
  label,
  source: 'api',
  windows: [{ name: '5 giờ', usedPct }],
  resets: [],
  fetchedAt: '2030-01-01T00:00:00Z',
})

const usage = Array.from({ length: 16 }, (_, i) => ({
  day: `2030-01-${String(i + 1).padStart(2, '0')}`,
  connectionId: 'c1',
  model: 'claude-sonnet',
  inputTokens: 1000 + i,
  outputTokens: 100,
  requests: 3,
}))

const group = {
  signature: 'rate limit reached',
  provider: 'groq',
  status: 429,
  classes: { rate_limit: 4 },
  count: 7,
  first: '2030-01-01T00:00:00Z',
  last: '2030-01-01T01:00:00Z',
  models: ['m'],
  message: 'Rate limit',
  lastId: 12,
  medianMs: 800,
}

const err = {
  id: 12,
  at: '2030-01-01T01:00:00Z',
  provider: 'groq',
  connectionId: 'c2',
  model: 'llama-3.3',
  client: 'cursor',
  endpoint: '/v1/chat/completions',
  status: 429,
  latencyMs: 800,
  class: 'rate_limit',
  signature: 'rate limit reached',
  message: 'Rate limit reached for model',
  quotaLeft: -1,
}

const change = {
  id: 5,
  at: '2030-01-01T02:00:00Z',
  direction: 'response',
  provider: 'groq',
  endpoint: '/v1/chat/completions',
  event: 'e',
  path: 'choices.0.extra',
  kind: 'added',
  oldType: '',
  newType: 'string',
  sample: '',
  acked: false,
}

interface Opts {
  accounts?: unknown[]
  quota?: unknown[]
  usage?: unknown[]
  groups?: unknown[]
  errors?: unknown[]
  changes?: unknown[]
  unacked?: number
  driftStatus?: number
}

function mock(o: Opts = {}) {
  server.use(
    http.get('*/accounts', () => HttpResponse.json({ accounts: o.accounts ?? accounts })),
    http.get('*/account-tests', () => HttpResponse.json(o.accounts?.length === 0 ? {} : tests)),
    http.get('*/quota', () =>
      HttpResponse.json({
        accounts: o.quota ?? [
          quota('c1', 'work@example.com', 80),
          quota('c2', 'groq-main', 95),
          quota('c3', 'zen-spare', 50),
        ],
      }),
    ),
    http.get('*/usage', () => HttpResponse.json({ usage: o.usage ?? usage })),
    http.get('*/errors/stats', () => HttpResponse.json({ groups: o.groups ?? [group] })),
    http.get('*/errors', () => HttpResponse.json({ errors: o.errors ?? [err] })),
    http.get('*/drift/changes', () =>
      o.driftStatus
        ? HttpResponse.json({ error: 'boom' }, { status: o.driftStatus })
        : HttpResponse.json({ changes: o.changes ?? [change], unacked: o.unacked ?? 3 }),
    ),
  )
}

const hrefs = () =>
  screen.getAllByRole('link').map((a) => a.getAttribute('href') ?? '')

describe('overview aggregation', () => {
  it('selects low quota accounts sorted by usage', () => {
    const list = lowQuotaAccounts([
      quota('a', 'a', 80),
      quota('b', 'b', 95),
      quota('c', 'c', 10),
    ] as never)
    expect(list.map((x) => x.account.connectionId)).toEqual(['b', 'a'])
  })

  it('keeps the latest 14 days oldest first', () => {
    const days = lastDays(usage, 14)
    expect(days).toHaveLength(14)
    expect(days[0].day).toBe('2030-01-03')
    expect(days.at(-1)?.day).toBe('2030-01-16')
  })
})

describe('overview page', () => {
  it('shows every block with data', async () => {
    useSession()
    mock()
    renderApp('/')
    await waitFor(() =>
      expect(screen.getByTestId('stat-accounts')).toHaveTextContent(/3.*1 đang lỗi/),
    )
    await waitFor(() => expect(screen.getByTestId('stat-quota')).toHaveTextContent('2'))
    await waitFor(() => expect(screen.getByTestId('stat-errors')).toHaveTextContent('7'))
    await waitFor(() => expect(screen.getByTestId('stat-drift')).toHaveTextContent('3'))
    expect(await screen.findByRole('img', { name: /Biểu đồ/ })).toBeInTheDocument()
    expect(screen.getByRole('table', { name: /14 ngày/ })).toBeInTheDocument()
    expect(await screen.findByText('Rate limit reached for model')).toBeInTheDocument()
  })

  it('links to the real routes', async () => {
    useSession()
    mock()
    renderApp('/')
    await screen.findByText('Rate limit reached for model')
    await screen.findByTestId('low-quota-item-0')
    await screen.findByText('choices.0.extra')
    const all = hrefs()
    expect(all).toContain('/accounts')
    expect(all).toContain('/quota')
    expect(all).toContain('/drift?unacked=1')
    expect(all.some((h) => h.startsWith('/drift?unacked=1&change='))).toBe(true)
    expect(all.some((h) => h.startsWith('/quota#account-'))).toBe(true)
    expect(all).toContain('/errors?error=12')
    expect(all.some((h) => h.startsWith('/errors?since='))).toBe(true)
  })

  it('sorts low quota accounts by usage', async () => {
    useSession()
    mock()
    renderApp('/')
    const first = await screen.findByTestId('low-quota-item-0')
    expect(first).toHaveTextContent('groq-main')
    expect(screen.getByTestId('low-quota-item-1')).toHaveTextContent('work@example.com')
    expect(screen.queryByTestId('low-quota-item-2')).not.toBeInTheDocument()
  })

  it('keeps the other blocks when one block fails', async () => {
    useSession()
    mock({ driftStatus: 500 })
    const user = userEvent.setup()
    renderApp('/')
    await waitFor(() => expect(screen.getByTestId('stat-accounts')).toHaveTextContent('3'))
    await waitFor(() => expect(screen.getByTestId('stat-errors')).toHaveTextContent('7'))
    const retries = await screen.findAllByRole('button', { name: 'Thử lại' })
    expect(retries.length).toBeGreaterThan(0)
    expect(await screen.findByTestId('low-quota-item-0')).toBeInTheDocument()
    server.use(
      http.get('*/drift/changes', () =>
        HttpResponse.json({ changes: [change], unacked: 3 }),
      ),
    )
    await user.click(retries[0])
    await waitFor(() => expect(screen.getByTestId('stat-drift')).toHaveTextContent('3'))
  })

  it('guides the first account when everything is empty', async () => {
    useSession()
    mock({ accounts: [], quota: [], usage: [], groups: [], errors: [], changes: [], unacked: 0 })
    renderApp('/')
    const link = await screen.findByRole('link', { name: /Thêm tài khoản đầu tiên/ })
    expect(link).toHaveAttribute('href', '/accounts')
    expect(await screen.findByText(/Chưa có dữ liệu lượng dùng/)).toBeInTheDocument()
  })
})
