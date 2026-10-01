import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'
import { renderApp, useSession } from '@/test/render'
import { server } from '@/test/server'

// Shapes follow AccountQuota, QuotaWindow and QuotaReset in
// be/internal/httpapi/quota.go and quota_resets.go.
const reset = {
  id: 'weekly',
  kind: 'weekly',
  title: 'Reset giới hạn 5 giờ',
  description: 'Xóa giới hạn 5 giờ ngay lập tức',
  clears: ['5 giờ'],
  left: 1,
  total: 2,
  usable: true,
}

const claude = {
  connectionId: 'c1',
  provider: 'claude',
  label: 'work@example.com',
  plan: 'Max',
  source: 'api',
  windows: [
    { name: '5 giờ', usedPct: 42.4, resetAt: '2030-01-01T10:00:00Z' },
    { name: 'Hằng tuần', usedPct: 91, resetAt: '2030-01-05T10:00:00Z' },
    { name: 'Dự phòng', usedPct: 0, unlimited: true },
  ],
  resets: [reset],
  fetchedAt: '2030-01-01T09:00:00Z',
}

const groq = {
  connectionId: 'c2',
  provider: 'groq',
  label: 'groq-main',
  source: 'headers',
  windows: [],
  resets: [],
  error: 'token đã hết hạn',
  fetchedAt: '2030-01-01T09:00:00Z',
}

function mockQuota(opts: { view?: string } = {}) {
  const seen = { refresh: 0, claims: [] as unknown[], saved: [] as unknown[] }
  server.use(
    http.get('*/quota', ({ request }) => {
      if (new URL(request.url).searchParams.get('refresh') === '1') {
        seen.refresh++
      }
      return HttpResponse.json({ accounts: [claude, groq] })
    }),
    http.get('*/ui-settings/quota-view', () =>
      HttpResponse.json(opts.view ? { view: opts.view } : {}),
    ),
    http.post('*/ui-settings/quota-view', async ({ request }) => {
      seen.saved.push(await request.json())
      return HttpResponse.json({ ok: true })
    }),
    http.post('*/quota/c1/reset', async ({ request }) => {
      seen.claims.push(await request.json())
      return HttpResponse.json({
        outcome: 'reset',
        requestId: 'r1',
        providerCode: '',
        message: '',
      })
    }),
  )
  return seen
}

describe('quota page', () => {
  it('shows each window with percent, aria-valuenow and a warning', async () => {
    useSession()
    mockQuota()
    renderApp('/quota')
    const card = await screen.findByRole('article', { name: /work@example.com/ })
    expect(within(card).getByText('Max')).toBeInTheDocument()
    expect(within(card).getByText(/api/i)).toBeInTheDocument()
    const five = within(card).getByRole('progressbar', { name: /5 giờ/ })
    expect(five).toHaveAttribute('aria-valuenow', '42')
    expect(within(card).getByText('42%')).toBeInTheDocument()
    const week = within(card).getByRole('progressbar', { name: /Hằng tuần/ })
    expect(week).toHaveAttribute('aria-valuenow', '91')
    expect(within(card).getByText(/Sắp hết/)).toBeInTheDocument()
  })

  it('shows an unlimited window without a progress bar', async () => {
    useSession()
    mockQuota()
    renderApp('/quota')
    const card = await screen.findByRole('article', { name: /work@example.com/ })
    expect(within(card).getByText('Không giới hạn')).toBeInTheDocument()
    expect(
      within(card).queryByRole('progressbar', { name: /Dự phòng/ }),
    ).not.toBeInTheDocument()
  })

  it('shows the error of one account on its own card', async () => {
    useSession()
    mockQuota()
    renderApp('/quota')
    const card = await screen.findByRole('article', { name: /groq-main/ })
    expect(within(card).getByText('token đã hết hạn')).toBeInTheDocument()
    expect(
      screen.getByRole('article', { name: /work@example.com/ }),
    ).toBeInTheDocument()
  })

  it('forces a refresh with ?refresh=1', async () => {
    useSession()
    const seen = mockQuota()
    const user = userEvent.setup()
    renderApp('/quota')
    await screen.findByRole('article', { name: /work@example.com/ })
    await user.click(screen.getByRole('button', { name: 'Làm mới' }))
    await waitFor(() => expect(seen.refresh).toBe(1))
  })

  it('asks for confirmation, claims the reset and shows the result', async () => {
    useSession()
    const seen = mockQuota()
    const user = userEvent.setup()
    renderApp('/quota')
    await screen.findByRole('article', { name: /work@example.com/ })
    await user.click(screen.getByRole('button', { name: /Claim reset/ }))
    const dialog = await screen.findByRole('alertdialog')
    expect(seen.claims).toHaveLength(0)
    await user.click(within(dialog).getByRole('button', { name: /Xác nhận/ }))
    await waitFor(() => expect(seen.claims).toHaveLength(1))
    expect(seen.claims[0]).toMatchObject({ resetId: 'weekly' })
    expect(await screen.findByText(/Đã reset thành công/)).toBeInTheDocument()
  })

  it('shows the refusal of a blocked claim', async () => {
    useSession()
    mockQuota()
    server.use(
      http.post('*/quota/c1/reset', () =>
        HttpResponse.json(
          { error: 'just_reset', blocked: 'just_reset' },
          { status: 409 },
        ),
      ),
    )
    const user = userEvent.setup()
    renderApp('/quota')
    await screen.findByRole('article', { name: /work@example.com/ })
    await user.click(screen.getByRole('button', { name: /Claim reset/ }))
    const dialog = await screen.findByRole('alertdialog')
    await user.click(within(dialog).getByRole('button', { name: /Xác nhận/ }))
    expect(await screen.findByText(/Claim thất bại/)).toBeInTheDocument()
  })

  it('does not offer a claim when no account has resets', async () => {
    useSession()
    server.use(
      http.get('*/quota', () => HttpResponse.json({ accounts: [groq] })),
      http.get('*/ui-settings/quota-view', () => HttpResponse.json({})),
    )
    renderApp('/quota')
    await screen.findByRole('article', { name: /groq-main/ })
    expect(
      screen.queryByRole('button', { name: /Claim reset/ }),
    ).not.toBeInTheDocument()
  })

  it('saves the chosen view on the server and renders a table', async () => {
    useSession()
    const seen = mockQuota()
    const user = userEvent.setup()
    renderApp('/quota')
    await screen.findByRole('article', { name: /work@example.com/ })
    await user.click(screen.getByRole('radio', { name: 'Bảng' }))
    await waitFor(() => expect(seen.saved).toEqual([{ view: 'table' }]))
    expect(await screen.findByRole('table')).toBeInTheDocument()
  })

  it('opens in the saved table view', async () => {
    useSession()
    mockQuota({ view: 'table' })
    renderApp('/quota')
    expect(await screen.findByRole('table')).toBeInTheDocument()
    expect(screen.getByText('work@example.com')).toBeInTheDocument()
  })

  it('shows an empty state and an error state with retry', { timeout: 20000 }, async () => {
    useSession()
    server.use(
      http.get('*/quota', () => HttpResponse.json({ accounts: [] })),
      http.get('*/ui-settings/quota-view', () => HttpResponse.json({})),
    )
    const first = renderApp('/quota')
    expect(await screen.findByText('Chưa có tài khoản nào')).toBeInTheDocument()
    first.unmount()

    let ok = false
    server.use(
      http.get('*/quota', () =>
        ok
          ? HttpResponse.json({ accounts: [groq] })
          : HttpResponse.json({ error: 'cannot read connections' }, { status: 500 }),
      ),
    )
    const user = userEvent.setup()
    renderApp('/quota')
    expect(
      await screen.findByText('cannot read connections', {}, { timeout: 8000 }),
    ).toBeInTheDocument()
    ok = true
    await user.click(screen.getByRole('button', { name: 'Thử lại' }))
    expect(await screen.findByRole('article', { name: /groq-main/ })).toBeInTheDocument()
  })
})
