import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'
import { renderApp, useSession as mockSession } from '@/test/render'
import { server } from '@/test/server'

const accounts = [
  {
    id: 'a1',
    provider: 'groq',
    label: 'Groq chính',
    isActive: true,
    standby: false,
    baseUrl: '',
  },
  {
    id: 'a2',
    provider: 'openrouter',
    label: 'OpenRouter phụ',
    isActive: false,
    standby: false,
    baseUrl: 'https://example.test/v1',
  },
]

const providers = [
  { id: 'groq', setup: 'key', auth: 'apikey' },
  { id: 'cloudflare-ai', setup: 'account', auth: 'apikey' },
  { id: 'claude', setup: 'oauth', auth: 'oauth' },
  { id: 'github', setup: 'oauth', auth: 'oauth' },
]

const redirect = () => HttpResponse.json({ ok: true })

function mockList(list: unknown[] = accounts, tests: Record<string, unknown> = {}) {
  server.use(
    http.get('*/accounts', () => HttpResponse.json({ accounts: list })),
    http.get('*/account-tests', () => HttpResponse.json(tests)),
    http.get('*/providers', () => HttpResponse.json({ providers })),
  )
}

async function openPage() {
  mockSession()
  const user = userEvent.setup()
  renderApp('/accounts')
  await screen.findByRole('heading', { name: 'Tài khoản' })
  return user
}

describe('accounts list', () => {
  it('lists accounts with their last test and never shows a secret', async () => {
    mockList(accounts, {
      a1: { ok: true, status: 200, ms: 321, message: 'ok', model: 'm', at: '2026-01-01T00:00:00Z' },
      a2: { ok: false, status: 401, ms: 40, message: 'bad key', model: 'm', at: '2026-01-01T00:00:00Z' },
    })
    await openPage()
    const row1 = (await screen.findByText('Groq chính')).closest('tr')!
    expect(within(row1).getByText('groq')).toBeInTheDocument()
    expect(await within(row1).findByText(/Hoạt động/)).toBeInTheDocument()
    const row2 = screen.getByText('OpenRouter phụ').closest('tr')!
    expect(await within(row2).findByText(/Lỗi/)).toBeInTheDocument()
    expect(document.body.textContent).not.toMatch(/secret|sk-/i)
  })

  it('filters by provider and by label', async () => {
    mockList()
    const user = await openPage()
    await screen.findByText('Groq chính')
    await user.type(screen.getByRole('searchbox', { name: 'Tìm theo nhãn' }), 'phụ')
    expect(screen.queryByText('Groq chính')).not.toBeInTheDocument()
    expect(screen.getByText('OpenRouter phụ')).toBeInTheDocument()
  })

  it('shows a guide when there is no account', async () => {
    mockList([])
    await openPage()
    expect(await screen.findByText('Chưa có tài khoản nào')).toBeInTheDocument()
  })

  it('shows an error with a retry that loads the list', async () => {
    let fail = true
    server.use(
      http.get('*/accounts', () =>
        fail
          ? HttpResponse.json({ error: 'cannot read connections' }, { status: 500 })
          : HttpResponse.json({ accounts }),
      ),
      http.get('*/account-tests', () => HttpResponse.json({})),
    )
    const user = await openPage()
    expect(await screen.findByRole('alert')).toHaveTextContent('cannot read connections')
    fail = false
    await user.click(screen.getByRole('button', { name: 'Thử lại' }))
    expect(await screen.findByText('Groq chính')).toBeInTheDocument()
  })
})

describe('adding an account', () => {
  it('picks a type with the keyboard and posts a form-encoded key', async () => {
    mockList()
    let body = ''
    let type = ''
    server.use(
      http.post('*/accounts', async ({ request }) => {
        type = request.headers.get('content-type') ?? ''
        body = await request.text()
        return redirect()
      }),
    )
    const user = await openPage()
    await screen.findByText('Groq chính')
    await user.click(screen.getByRole('button', { name: 'Thêm tài khoản' }))
    const dialog = await screen.findByRole('dialog')
    const apiKind = within(dialog).getByRole('radio', { name: /Khóa API/ })
    apiKind.focus()
    // Radix checks a radio on focus only while the arrow key is still held.
    await user.keyboard('{ArrowDown>}')
    await waitFor(() =>
      expect(within(dialog).getByRole('radio', { name: /OAuth/ })).toBeChecked(),
    )
    await user.keyboard('{/ArrowDown}{ArrowUp>}')
    await waitFor(() => expect(apiKind).toBeChecked())
    await user.keyboard('{/ArrowUp}')
    await user.click(within(dialog).getByRole('button', { name: 'Tiếp tục' }))
    await user.click(within(dialog).getByRole('combobox', { name: 'Nhà cung cấp' }))
    await user.click(await screen.findByRole('option', { name: 'groq' }))
    const key = within(dialog).getByLabelText('Khóa API')
    expect(key).toHaveAttribute('type', 'password')
    await user.type(key, 'gsk-test-123')
    await user.type(within(dialog).getByLabelText(/Nhãn/), 'Mới')
    await user.click(within(dialog).getByRole('button', { name: 'Thêm' }))
    await waitFor(() => expect(body).not.toBe(''))
    expect(type).toContain('application/x-www-form-urlencoded')
    const form = new URLSearchParams(body)
    expect(form.get('provider')).toBe('groq')
    expect(form.get('secret')).toBe('gsk-test-123')
    expect(form.get('label')).toBe('Mới')
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
  })

  it('shows a 400 on the field and clears the key', async () => {
    mockList()
    server.use(
      http.post('*/accounts', () =>
        HttpResponse.json({ error: 'key is required' }, { status: 400 }),
      ),
    )
    const user = await openPage()
    await user.click(await screen.findByRole('button', { name: 'Thêm tài khoản' }))
    const dialog = await screen.findByRole('dialog')
    await user.click(within(dialog).getByRole('button', { name: 'Tiếp tục' }))
    await user.click(within(dialog).getByRole('combobox', { name: 'Nhà cung cấp' }))
    await user.click(await screen.findByRole('option', { name: 'groq' }))
    const key = within(dialog).getByLabelText('Khóa API')
    await user.type(key, '   x')
    await user.click(within(dialog).getByRole('button', { name: 'Thêm' }))
    expect(await within(dialog).findByText('key is required')).toBeInTheDocument()
    expect(key).toHaveValue('')
  })

  it('finishes an OAuth sign-in by pasting the code', async () => {
    mockList()
    let finish: Record<string, string> = {}
    server.use(
      http.post('*/oauth/claude/start', () =>
        HttpResponse.json({
          url: 'https://auth.example.test/authorize?state=st1',
          state: 'st1',
          redirect: 'http://localhost/cb',
        }),
      ),
      http.post('*/oauth/claude/finish', async ({ request }) => {
        finish = (await request.json()) as Record<string, string>
        return HttpResponse.json({ connection: { id: 'n1', provider: 'claude', label: 'x' } })
      }),
    )
    const user = await openPage()
    await user.click(await screen.findByRole('button', { name: 'Thêm tài khoản' }))
    const dialog = await screen.findByRole('dialog')
    await user.click(within(dialog).getByRole('radio', { name: /OAuth/ }))
    await user.click(within(dialog).getByRole('button', { name: 'Tiếp tục' }))
    await user.click(within(dialog).getByRole('combobox', { name: 'Nhà cung cấp' }))
    await user.click(await screen.findByRole('option', { name: 'claude' }))
    await user.click(within(dialog).getByRole('button', { name: 'Bắt đầu đăng nhập' }))
    const link = await within(dialog).findByRole('link', { name: /Mở trang đăng nhập/ })
    expect(link).toHaveAttribute('target', '_blank')
    await user.type(within(dialog).getByLabelText(/Dán URL hoặc mã/), 'abc#st1')
    await user.click(within(dialog).getByRole('button', { name: 'Hoàn tất' }))
    await waitFor(() => expect(finish.state).toBe('st1'))
    expect(finish.input).toBe('abc#st1')
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
  })

  it('polls the GitHub device flow until a connection exists', async () => {
    mockList()
    let polls = 0
    server.use(
      http.post('*/oauth/github/start', () =>
        HttpResponse.json({
          device: true,
          deviceCode: 'dc1',
          userCode: 'ABCD-1234',
          verificationUri: 'https://github.com/login/device',
          interval: 0,
          expiresIn: 600,
        }),
      ),
      http.post('*/oauth/github/poll', () => {
        polls += 1
        return polls < 2
          ? HttpResponse.json({ status: 'pending' })
          : HttpResponse.json({
              status: 'done',
              connection: { id: 'g1', provider: 'github', label: 'me' },
            })
      }),
    )
    const user = await openPage()
    await user.click(await screen.findByRole('button', { name: 'Thêm tài khoản' }))
    const dialog = await screen.findByRole('dialog')
    await user.click(within(dialog).getByRole('radio', { name: /thiết bị/i }))
    await user.click(within(dialog).getByRole('button', { name: 'Tiếp tục' }))
    await user.click(within(dialog).getByRole('combobox', { name: 'Nhà cung cấp' }))
    await user.click(await screen.findByRole('option', { name: 'github' }))
    await user.click(within(dialog).getByRole('button', { name: 'Bắt đầu đăng nhập' }))
    expect(await within(dialog).findByText('ABCD-1234')).toBeInTheDocument()
    await waitFor(() => expect(polls).toBe(2), { timeout: 5000 })
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument(), {
      timeout: 5000,
    })
  })
})

describe('account actions', () => {
  it('rolls the active switch back when the server answers 500', async () => {
    mockList()
    server.use(
      http.post('*/accounts/a1/active', () =>
        HttpResponse.json({ error: 'cannot update connection' }, { status: 500 }),
      ),
    )
    const user = await openPage()
    const sw = await screen.findByRole('switch', { name: 'Bật Groq chính' })
    expect(sw).toBeChecked()
    await user.click(sw)
    await waitFor(() => expect(screen.getByRole('switch', { name: 'Bật Groq chính' })).toBeChecked())
    expect(await screen.findByText(/cannot update connection/)).toBeInTheDocument()
  })

  it('sends the new state when the switch is turned on', async () => {
    mockList()
    let sent: unknown
    server.use(
      http.post('*/accounts/a2/active', async ({ request }) => {
        sent = await request.json()
        return HttpResponse.json({ ok: true, active: true })
      }),
    )
    const user = await openPage()
    await user.click(await screen.findByRole('switch', { name: 'Bật OpenRouter phụ' }))
    await waitFor(() => expect(sent).toEqual({ active: true }))
  })

  it('renames an account', async () => {
    mockList()
    let sent: unknown
    server.use(
      http.post('*/accounts/a1/label', async ({ request }) => {
        sent = await request.json()
        return HttpResponse.json({ ok: true })
      }),
    )
    const user = await openPage()
    await user.click(await screen.findByRole('button', { name: 'Đổi nhãn Groq chính' }))
    const dialog = await screen.findByRole('dialog')
    const input = within(dialog).getByLabelText('Nhãn')
    await user.clear(input)
    await user.type(input, 'Tên mới')
    await user.click(within(dialog).getByRole('button', { name: 'Lưu' }))
    await waitFor(() => expect(sent).toEqual({ label: 'Tên mới' }))
  })

  it('asks for confirmation naming the account before deleting', async () => {
    mockList()
    let deleted = 0
    server.use(
      http.post('*/accounts/a1/delete', () => {
        deleted += 1
        return redirect()
      }),
    )
    const user = await openPage()
    await user.click(await screen.findByRole('button', { name: 'Xóa Groq chính' }))
    const confirm = await screen.findByRole('alertdialog')
    expect(confirm).toHaveTextContent('Groq chính')
    expect(deleted).toBe(0)
    await user.click(within(confirm).getByRole('button', { name: 'Xóa tài khoản' }))
    await waitFor(() => expect(deleted).toBe(1))
  })

  it('shows the result and duration of a test', async () => {
    mockList()
    server.use(
      http.post('*/accounts/a1/test', () =>
        HttpResponse.json({ ok: true, status: 200, ms: 457, message: 'ok', model: 'llama', at: '2026-01-01T00:00:00Z' }),
      ),
    )
    const user = await openPage()
    await user.click(await screen.findByRole('button', { name: 'Kiểm tra Groq chính' }))
    expect(await screen.findByText(/457 ms/)).toBeInTheDocument()
  })

  it('lists the models of an account', async () => {
    mockList()
    server.use(
      http.get('*/accounts/a1/models', () =>
        HttpResponse.json({ data: [{ id: 'llama-3.3' }, { id: 'mixtral' }] }),
      ),
    )
    const user = await openPage()
    await user.click(await screen.findByRole('button', { name: 'Xem model của Groq chính' }))
    const dialog = await screen.findByRole('dialog')
    expect(await within(dialog).findByText('llama-3.3')).toBeInTheDocument()
    expect(within(dialog).getByText('mixtral')).toBeInTheDocument()
  })
})
