import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type {
  ErrorGroup,
  ErrorReviewState,
  ErrorVerdict,
  UpstreamError,
} from '@/api/errors'
import { renderApp, useSession } from '@/test/render'
import { server } from '@/test/server'

const row = (over: Partial<UpstreamError> = {}): UpstreamError => ({
  id: 12,
  at: '2026-10-02T08:30:00Z',
  provider: 'groq',
  connectionId: 'c1',
  model: 'llama-3.3-70b-versatile',
  client: 'cursor',
  endpoint: '/v1/chat/completions',
  status: 429,
  latencyMs: 840,
  class: 'rate_limit',
  signature: 'rate limit reached',
  message: 'Rate limit reached for model',
  quotaLeft: -1,
  ...over,
})

const group = (over: Partial<ErrorGroup> = {}): ErrorGroup => ({
  signature: 'rate limit reached',
  provider: 'groq',
  status: 429,
  classes: { rate_limit: 5, server: 1 },
  count: 6,
  first: '2026-10-02T08:00:00Z',
  last: '2026-10-02T08:30:00Z',
  models: ['llama-3.3-70b-versatile'],
  message: 'Rate limit reached for model',
  lastId: 12,
  medianMs: 800,
  ...over,
})

const review = (over: Partial<ErrorReviewState> = {}): ErrorReviewState => ({
  enabled: false,
  model: '',
  minErrors: 3,
  replay: false,
  ready: false,
  lastError: '',
  ...over,
})

interface Mock {
  lists: URL[]
  stats: URL[]
  posts: unknown[]
}

function mockApi(opts: {
  errors?: UpstreamError[]
  groups?: ErrorGroup[]
  review?: ErrorReviewState
  verdicts?: ErrorVerdict[]
  detail?: UpstreamError
} = {}): Mock {
  const mock: Mock = { lists: [], stats: [], posts: [] }
  server.use(
    http.get('*/errors/stats', ({ request }) => {
      mock.stats.push(new URL(request.url))
      return HttpResponse.json({ groups: opts.groups ?? [] })
    }),
    http.get('*/errors/review', () =>
      HttpResponse.json(opts.review ?? review()),
    ),
    http.post('*/errors/review', async ({ request }) => {
      const body = await request.json()
      mock.posts.push(body)
      return HttpResponse.json({
        ...review(),
        ...(body as object),
        ready: true,
      })
    }),
    http.get('*/errors/verdicts', () =>
      HttpResponse.json({ verdicts: opts.verdicts ?? [] }),
    ),
    http.get('*/errors/:id', () =>
      HttpResponse.json(opts.detail ?? row({ reqBody: '{"a":1}', respBody: '{"e":2}' })),
    ),
    http.get('*/errors', ({ request }) => {
      mock.lists.push(new URL(request.url))
      return HttpResponse.json({ errors: opts.errors ?? [] })
    }),
  )
  return mock
}

afterEach(() => {
  vi.useRealTimers()
})

describe('errors page', { timeout: 15_000 }, () => {
  it('shows the empty state when there are no errors', async () => {
    useSession()
    mockApi()
    renderApp('/errors')
    expect(await screen.findByText('Chưa có lỗi')).toBeInTheDocument()
  })

  it('shows a retryable error state when the list fails', async () => {
    useSession()
    mockApi()
    let calls = 0
    server.use(
      http.get('*/errors', () => {
        calls++
        return calls <= 3
          ? HttpResponse.json({ error: 'cannot read errors' }, { status: 500 })
          : HttpResponse.json({ errors: [row()] })
      }),
    )
    const user = userEvent.setup()
    renderApp('/errors')
    // A 500 is retried twice with backoff before the error state shows.
    expect(
      await screen.findByText('cannot read errors', {}, { timeout: 6000 }),
    ).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Thử lại' }))
    expect(await screen.findByText('Rate limit reached for model')).toBeInTheDocument()
  })

  it('sends the filters from the URL and lists the errors', async () => {
    useSession()
    const mock = mockApi({ errors: [row()], groups: [group()] })
    renderApp('/errors?provider=groq&class=rate_limit&status=429&signature=rate&since=2026-10-01T00:00:00Z&limit=50')
    expect(await screen.findByText('Rate limit reached for model')).toBeInTheDocument()
    const q = mock.lists[0].searchParams
    expect(q.get('provider')).toBe('groq')
    expect(q.get('class')).toBe('rate_limit')
    expect(q.get('status')).toBe('429')
    expect(q.get('signature')).toBe('rate')
    expect(q.get('since')).toBe('2026-10-01T00:00:00Z')
    expect(q.get('limit')).toBe('50')
    // The stats follow the provider and the time range.
    await waitFor(() => expect(mock.stats.length).toBeGreaterThan(0))
    expect(mock.stats[0].searchParams.get('provider')).toBe('groq')
    expect(mock.stats[0].searchParams.get('since')).toBe('2026-10-01T00:00:00Z')
  })

  it('writes a changed filter to the URL and asks again with it', async () => {
    useSession()
    const mock = mockApi({ errors: [row()] })
    const user = userEvent.setup()
    const app = renderApp('/errors')
    await screen.findByText('Rate limit reached for model')
    await user.click(screen.getByRole('combobox', { name: 'Phân loại' }))
    await user.click(await screen.findByRole('option', { name: 'Giới hạn tốc độ' }))
    await waitFor(() =>
      expect(app.router.state.location.search).toContain('class=rate_limit'),
    )
    await waitFor(() =>
      expect(mock.lists.some((u) => u.searchParams.get('class') === 'rate_limit')).toBe(true),
    )
  })

  it('shows the stats as cards', async () => {
    useSession()
    mockApi({ errors: [row()], groups: [group(), group({ signature: 'x', provider: 'zen', count: 4, classes: { auth: 4 } })] })
    renderApp('/errors')
    const total = await screen.findByLabelText('Tổng số lỗi')
    expect(total).toHaveTextContent('10')
    expect(screen.getByLabelText('Lỗi theo phân loại')).toHaveTextContent('Giới hạn tốc độ')
    expect(screen.getByLabelText('Lỗi theo nhà cung cấp')).toHaveTextContent('zen')
  })

  it('opens an error with its request and response and flags a clipped body', async () => {
    useSession()
    const clipped = 'x'.repeat(64 * 1024) + '…'
    mockApi({
      errors: [row()],
      detail: row({ reqBody: '{"model":"m"}', respBody: clipped }),
    })
    const user = userEvent.setup()
    renderApp('/errors')
    await user.click(await screen.findByRole('button', { name: 'Xem chi tiết lỗi #12' }))
    const sheet = await screen.findByRole('dialog')
    expect(await within(sheet).findByText('{"model":"m"}')).toBeInTheDocument()
    expect(within(sheet).getByText(/Response đã bị cắt ở 64 KiB/)).toBeInTheDocument()
    expect(within(sheet).queryByText(/Request đã bị cắt/)).not.toBeInTheDocument()
    expect(within(sheet).getByRole('button', { name: 'Sao chép request' })).toBeInTheDocument()
  })

  it('says so when the server hid the bodies', async () => {
    useSession()
    mockApi({ errors: [row()], detail: row({ reqBody: '', respBody: '' }) })
    const user = userEvent.setup()
    renderApp('/errors')
    await user.click(await screen.findByRole('button', { name: 'Xem chi tiết lỗi #12' }))
    expect(await screen.findByText(/Máy chủ đã ẩn nội dung/)).toBeInTheDocument()
  })

  it('saves the review settings with the exact body', async () => {
    useSession()
    const mock = mockApi({ review: review({ model: 'groq/llama', minErrors: 3 }) })
    const user = userEvent.setup()
    renderApp('/errors?tab=review')
    const model = await screen.findByLabelText('Model')
    await waitFor(() => expect(model).toHaveValue('groq/llama'))
    await user.click(screen.getByRole('switch', { name: 'Bật AI error review' }))
    await user.click(screen.getByRole('switch', { name: 'Replay request lỗi trước khi kết luận' }))
    await user.clear(model)
    await user.type(model, 'zen/big')
    const min = screen.getByLabelText('Số lỗi tối thiểu')
    await user.clear(min)
    await user.type(min, '5')
    await user.click(screen.getByRole('button', { name: 'Lưu cấu hình' }))
    await waitFor(() => expect(mock.posts).toHaveLength(1))
    expect(mock.posts[0]).toEqual({ enabled: true, model: 'zen/big', minErrors: 5, replay: true })
  })

  it('runs the review now', async () => {
    useSession()
    const mock = mockApi({ review: review({ enabled: true, model: 'groq/llama', ready: true }) })
    const user = userEvent.setup()
    renderApp('/errors?tab=review')
    await user.click(await screen.findByRole('button', { name: 'Chạy ngay' }))
    await waitFor(() => expect(mock.posts).toHaveLength(1))
    expect(mock.posts[0]).toMatchObject({ run: true, model: 'groq/llama', enabled: true })
  })

  it('shows the cause and confidence of a verdict', async () => {
    useSession()
    mockApi({
      review: review({ enabled: true, model: 'groq/llama', ready: true }),
      verdicts: [
        {
          id: 1, at: '2026-10-02T08:40:00Z', provider: 'groq', signature: 'rate limit reached',
          action: 'blacklist', applied: true, verified: true, detail: 'field top_k',
          cause: 'Provider rejects the top_k field', reason: 'replay confirmed', note: 'removed',
          by: 'groq/llama', errors: 6, lastError: '', replayed: true,
        },
      ],
    })
    renderApp('/errors?tab=review')
    expect(await screen.findByText('Provider rejects the top_k field')).toBeInTheDocument()
    expect(screen.getByText('Tin cậy cao')).toBeInTheDocument()
    expect(screen.getByText('Thêm vào blacklist')).toBeInTheDocument()
  })

  it('refreshes every 30 seconds and stops when paused', async () => {
    useSession()
    const mock = mockApi({ errors: [row()] })
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTimeAsync })
    vi.useFakeTimers({ shouldAdvanceTime: true })
    renderApp('/errors')
    await screen.findByText('Rate limit reached for model')
    const before = mock.lists.length
    await vi.advanceTimersByTimeAsync(31_000)
    await waitFor(() => expect(mock.lists.length).toBeGreaterThan(before))

    await user.click(screen.getByRole('button', { name: 'Tạm dừng làm mới' }))
    expect(screen.getByRole('button', { name: 'Tiếp tục làm mới' })).toBeInTheDocument()
    const paused = mock.lists.length
    await vi.advanceTimersByTimeAsync(95_000)
    expect(mock.lists.length).toBe(paused)
  })
})
