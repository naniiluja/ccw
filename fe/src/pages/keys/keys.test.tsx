import { screen, waitFor, within, fireEvent } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'
import { renderApp, useSession as mockSession } from '@/test/render'
import { server } from '@/test/server'

const FULL_KEY = 'sk-ccw-0123456789abcdef0123456789abcdef0123456789abcdef'

const baseKey = {
  id: 'k1',
  name: 'Máy build',
  masked: 'sk-ccw-012••••••••cdef',
  enabled: true,
  models: ['groq/llama-3.3-70b-versatile'],
  expiresAt: '',
  rpm: 60,
  lastUsed: '2026-09-30T10:00:00Z',
  createdAt: '2026-09-01T08:00:00Z',
}

interface Calls {
  posts: { path: string; body: unknown }[]
}

function mockKeys(keys = [baseKey], opts: { activeFails?: boolean } = {}) {
  const calls: Calls = { posts: [] }
  let list = keys
  server.use(
    http.get('*/keys', () => HttpResponse.json({ keys: list })),
    http.get('*/v1/models', () =>
      HttpResponse.json({
        object: 'list',
        data: [{ id: 'groq/llama-3.3-70b-versatile' }, { id: 'openai/gpt-5' }],
      }),
    ),
    http.post('*/keys', async ({ request }) => {
      const body = await request.json()
      calls.posts.push({ path: '/keys', body })
      list = [...list, { ...baseKey, id: 'k2', name: 'Khóa mới' }]
      return HttpResponse.json({ ...baseKey, id: 'k2', name: 'Khóa mới', key: FULL_KEY })
    }),
    http.post('*/keys/:id/reveal', ({ params }) => {
      calls.posts.push({ path: `/keys/${String(params.id)}/reveal`, body: null })
      return HttpResponse.json({ key: FULL_KEY })
    }),
    http.post('*/keys/:id/active', async ({ request }) => {
      calls.posts.push({ path: '/active', body: await request.json() })
      if (opts.activeFails) {
        return HttpResponse.json({ error: 'cannot' }, { status: 500 })
      }
      return HttpResponse.json({ ok: true })
    }),
    http.post('*/keys/:id/limits', async ({ request }) => {
      calls.posts.push({ path: '/limits', body: await request.json() })
      return HttpResponse.json({ ok: true })
    }),
    http.post('*/keys/:id/delete', ({ params }) => {
      calls.posts.push({ path: `/keys/${String(params.id)}/delete`, body: null })
      return HttpResponse.json({ ok: true })
    }),
  )
  return calls
}

async function openPage() {
  mockSession()
  const utils = renderApp('/keys')
  await screen.findByRole('heading', { name: 'Khóa API' })
  return utils
}

interface Cached {
  getAll(): unknown[]
}

function cacheDump(client: { getQueryCache(): Cached; getMutationCache(): Cached }) {
  const safe = (v: unknown) =>
    JSON.stringify(v, (_k, x: unknown) => (typeof x === 'function' ? undefined : x))
  return safe(client.getQueryCache().getAll()) + safe(client.getMutationCache().getAll())
}

describe('trang khóa API', () => {
  it('hiện trạng thái rỗng khi chưa có khóa', async () => {
    mockKeys([])
    await openPage()
    expect(await screen.findByText('Chưa có khóa API nào')).toBeInTheDocument()
  })

  it('hiện lỗi tải kèm nút thử lại', { timeout: 15000 }, async () => {
    mockSession()
    server.use(http.get('*/keys', () => HttpResponse.json({ error: 'hỏng' }, { status: 500 })))
    renderApp('/keys')
    // The query client retries a 5xx twice (1s, then 2s) before it reports.
    expect(await screen.findByText('hỏng', {}, { timeout: 8000 })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Thử lại' })).toBeInTheDocument()
  })

  it('tạo khóa: hiện một lần, không đóng được trước khi xác nhận, không vào cache', async () => {
    const user = userEvent.setup()
    mockKeys()
    const { queryClient } = await openPage()
    await screen.findAllByText('Máy build')

    await user.click(screen.getByRole('button', { name: 'Tạo khóa' }))
    const create = await screen.findByRole('dialog', { name: 'Tạo khóa API' })
    await user.type(within(create).getByLabelText('Tên khóa'), 'Khóa mới')
    await user.click(within(create).getByRole('button', { name: 'Tạo' }))

    const secret = await screen.findByRole('dialog', { name: 'Khóa API mới' })
    expect(within(secret).getByLabelText('Khóa API')).toHaveValue(FULL_KEY)
    expect(within(secret).getByText(/không xem lại được/i)).toBeInTheDocument()
    expect(cacheDump(queryClient)).not.toContain(FULL_KEY)

    // Escape does not close it, and the button stays disabled until confirmed.
    await user.keyboard('{Escape}')
    expect(screen.getByRole('dialog', { name: 'Khóa API mới' })).toBeInTheDocument()
    const close = within(secret).getByRole('button', { name: 'Đóng' })
    expect(close).toBeDisabled()

    await user.click(within(secret).getByRole('checkbox', { name: /đã lưu khóa/i }))
    await user.click(close)
    await waitFor(() => expect(screen.queryByRole('dialog', { name: 'Khóa API mới' })).toBeNull())
    expect(document.body.innerHTML).not.toContain(FULL_KEY)
    expect(cacheDump(queryClient)).not.toContain(FULL_KEY)
  })

  it('reveal cần xác nhận rồi mới gọi máy chủ', async () => {
    const user = userEvent.setup()
    const calls = mockKeys()
    await openPage()
    await screen.findAllByText('Máy build')

    await user.click(screen.getAllByRole('button', { name: 'Xem khóa Máy build' })[0])
    const confirm = await screen.findByRole('alertdialog')
    expect(calls.posts).toHaveLength(0)
    await user.click(within(confirm).getByRole('button', { name: 'Xem khóa' }))

    const secret = await screen.findByRole('dialog', { name: 'Khóa API' })
    expect(within(secret).getByLabelText('Khóa API')).toHaveValue(FULL_KEY)
    expect(calls.posts[0].path).toBe('/keys/k1/reveal')
  })

  it('bật tắt lạc quan và hoàn tác khi lỗi', async () => {
    const user = userEvent.setup()
    mockKeys([baseKey], { activeFails: true })
    await openPage()
    await screen.findAllByText('Máy build')

    const sw = screen.getAllByRole('switch', { name: 'Bật khóa Máy build' })[0]
    expect(sw).toBeChecked()
    await user.click(sw)
    expect(await screen.findByText(/không đổi được trạng thái/i)).toBeInTheDocument()
    expect(screen.getAllByRole('switch', { name: 'Bật khóa Máy build' })[0]).toBeChecked()
  })

  it('giới hạn: chặn RPM âm và hạn dùng quá khứ, gửi giá trị hợp lệ', async () => {
    const user = userEvent.setup()
    const calls = mockKeys()
    await openPage()
    await screen.findAllByText('Máy build')

    await user.click(screen.getAllByRole('button', { name: 'Giới hạn khóa Máy build' })[0])
    const dlg = await screen.findByRole('dialog', { name: 'Giới hạn khóa' })
    const rpm = within(dlg).getByLabelText('Số yêu cầu tối đa mỗi phút')
    await user.clear(rpm)
    await user.type(rpm, '-5')
    await user.click(within(dlg).getByRole('button', { name: 'Lưu' }))
    expect(await within(dlg).findByText(/số nguyên từ 0 đến 100000/i)).toBeInTheDocument()
    expect(calls.posts).toHaveLength(0)

    await user.clear(rpm)
    await user.type(rpm, '120')
    fireEvent.change(within(dlg).getByLabelText('Hạn dùng'), {
      target: { value: '2020-01-01T10:00' },
    })
    await user.click(within(dlg).getByRole('button', { name: 'Lưu' }))
    expect(await within(dlg).findByText(/phải ở tương lai/i)).toBeInTheDocument()
    expect(calls.posts).toHaveLength(0)

    fireEvent.change(within(dlg).getByLabelText('Hạn dùng'), {
      target: { value: '2099-01-01T10:00' },
    })
    await user.click(within(dlg).getByRole('button', { name: 'Lưu' }))
    await waitFor(() => expect(calls.posts).toHaveLength(1))
    const body = calls.posts[0].body as { rpm: number; expiresAt: string }
    expect(body.rpm).toBe(120)
    expect(body.expiresAt).toMatch(/^2098-12-31T|^2099-01-01T/)
  })

  it('xóa nêu tên khóa và gọi delete sau khi xác nhận', async () => {
    const user = userEvent.setup()
    const calls = mockKeys()
    await openPage()
    await screen.findAllByText('Máy build')

    await user.click(screen.getAllByRole('button', { name: 'Xóa khóa Máy build' })[0])
    const confirm = await screen.findByRole('alertdialog')
    expect(within(confirm).getByText(/Máy build/)).toBeInTheDocument()
    await user.click(within(confirm).getByRole('button', { name: 'Xóa' }))
    await waitFor(() => expect(calls.posts.some((p) => p.path === '/keys/k1/delete')).toBe(true))
  })

  it('usage: có dữ liệu thì hiện bảng số liệu, rỗng thì báo rỗng', async () => {
    const user = userEvent.setup()
    mockKeys()
    let rows: unknown[] = [
      { day: '2026-09-30', model: 'openai/gpt-5', inputTokens: 1200, outputTokens: 300, requests: 7 },
    ]
    server.use(
      http.get('*/keys/:id/usage', () =>
        HttpResponse.json({
          rows,
          rpm: 60,
          lastMinute: 3,
          expiresAt: '',
          lastUsed: '',
          today: '2026-10-02',
        }),
      ),
    )
    await openPage()
    await screen.findAllByText('Máy build')

    await user.click(screen.getAllByRole('button', { name: 'Lượng dùng khóa Máy build' })[0])
    const dlg = await screen.findByRole('dialog', { name: /Lượng dùng/ })
    const table = await within(dlg).findByRole('table')
    expect(within(table).getByText('openai/gpt-5')).toBeInTheDocument()
    expect(within(table).getByText('1.200')).toBeInTheDocument()
    await user.keyboard('{Escape}')
    await waitFor(() => expect(screen.queryByRole('dialog', { name: /Lượng dùng/ })).toBeNull())

    rows = []
    await user.click(screen.getAllByRole('button', { name: 'Lượng dùng khóa Máy build' })[0])
    const dlg2 = await screen.findByRole('dialog', { name: /Lượng dùng/ })
    expect(
      await within(dlg2).findByText('Chưa có lượng dùng trong 30 ngày qua'),
    ).toBeInTheDocument()
  })
})
