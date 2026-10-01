import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'
import { renderApp, useSession as seedSession } from '@/test/render'
import { server } from '@/test/server'

// A failing 5xx query is retried twice (1s then 2s) before it shows an error.
const slow = { timeout: 8000 }

// The shapes below are the ones be/internal/httpapi sends (accounts.go,
// models.go, rotation.go, providers.go).
const providerList = {
  providers: [
    { id: 'groq', setup: 'key', auth: 'apikey' },
    {
      id: 'acme',
      setup: 'key',
      auth: 'apikey',
      declared: true,
      name: 'Acme AI',
      color: '#336699',
      api: 'openai',
    },
    { id: 'opencode', setup: 'key', auth: 'apikey', name: 'OpenCode Zen' },
  ],
}

const accounts = {
  accounts: [
    { id: 'a1', provider: 'groq', label: 'one', isActive: true, standby: false, baseUrl: '' },
    { id: 'a2', provider: 'groq', label: 'two', isActive: true, standby: false, baseUrl: '' },
    { id: 'a3', provider: 'acme', label: 'three', isActive: true, standby: false, baseUrl: '' },
  ],
}

const row = (model: string, extra: Record<string, unknown> = {}) => ({
  provider: 'groq',
  model,
  active: true,
  stale: false,
  firstSeen: '2026-09-01T00:00:00Z',
  lastSeen: '2026-10-01T00:00:00Z',
  testOk: false,
  ...extra,
})

const table = (models: unknown[]) => ({
  models,
  ok: true,
  fetchedAt: '2026-10-01T00:00:00Z',
  policy: { autoTest: false, onlyFree: false },
  running: false,
})

const defs = {
  providers: [
    {
      id: 'acme',
      name: 'Acme AI',
      kind: 'apikey',
      api: 'openai',
      baseUrl: 'https://api.acme.test/v1',
      headers: { 'x-secret': 'super-secret-value' },
    },
  ],
}

interface Calls {
  bodies: Record<string, unknown[]>
}

function mockBase(models: unknown[] = []) {
  const calls: Calls = { bodies: {} }
  const record = (key: string, body: unknown) => {
    ;(calls.bodies[key] ??= []).push(body)
  }
  server.use(
    http.get('*/providers', () => HttpResponse.json(providerList)),
    http.get('*/accounts', () => HttpResponse.json(accounts)),
    http.get('*/provider-defs', () => HttpResponse.json(defs)),
    http.get('*/providers/:id/model-table', () =>
      HttpResponse.json(table(models)),
    ),
    http.get('*/providers/:id/rotation', () =>
      HttpResponse.json({
        rotation: { mode: 'round-robin', sticky: 1, order: null },
        next: 'a1',
        used: 0,
      }),
    ),
    http.get('*/providers/:id/model-policy', () =>
      HttpResponse.json({
        policy: { autoTest: false, onlyFree: false },
        running: false,
      }),
    ),
  )
  return { calls, record }
}

async function openProvider(name: RegExp, tab?: string) {
  const user = userEvent.setup()
  seedSession()
  renderApp('/providers')
  await user.click(await screen.findByRole('button', { name }))
  if (tab) await user.click(await screen.findByRole('tab', { name: tab }))
  return user
}

describe('providers list', () => {
  it('shows each provider with its account count', async () => {
    mockBase()
    seedSession()
    renderApp('/providers')
    const groq = await screen.findByRole('button', { name: /groq/i })
    expect(within(groq).getByText('2 tài khoản')).toBeInTheDocument()
    const acme = screen.getByRole('button', { name: /Acme AI/ })
    expect(within(acme).getByText('1 tài khoản')).toBeInTheDocument()
  })

  it('shows an error with a retry button when the list fails', async () => {
    let fail = true
    mockBase()
    server.use(
      http.get('*/providers', () =>
        fail
          ? HttpResponse.json({ error: 'cannot read' }, { status: 500 })
          : HttpResponse.json(providerList),
      ),
    )
    const user = userEvent.setup()
    seedSession()
    renderApp('/providers')
    const retry = await screen.findByRole(
      'button',
      { name: 'Thử lại' },
      slow,
    )
    fail = false
    await user.click(retry)
    expect(
      await screen.findByRole('button', { name: /groq/i }),
    ).toBeInTheDocument()
  }, 15_000)
})

describe('models tab', () => {
  const models = [
    row('zeta-model'),
    row('alpha-model', { active: false }),
    row('mid-model', { testOk: true, testMs: 120, testAt: '2026-10-01T01:00:00Z' }),
  ]

  it('lists the models and filters them by name', async () => {
    mockBase(models)
    const user = await openProvider(/groq/i)
    expect(await screen.findByText('alpha-model')).toBeInTheDocument()
    expect(screen.getByText('zeta-model')).toBeInTheDocument()
    await user.type(screen.getByRole('textbox', { name: 'Lọc model' }), 'mid')
    await waitFor(() =>
      expect(screen.queryByText('zeta-model')).not.toBeInTheDocument(),
    )
    expect(screen.getByText('mid-model')).toBeInTheDocument()
  })

  it('sorts by model name when the header is pressed', async () => {
    mockBase(models)
    const user = await openProvider(/groq/i)
    await screen.findByText('alpha-model')
    await user.click(screen.getByRole('button', { name: 'Sắp xếp theo model' }))
    const names = () =>
      screen
        .getAllByRole('row')
        .slice(1)
        .map((r) => within(r).getByTestId('model-name').textContent)
    await waitFor(() =>
      expect(names()).toEqual(['alpha-model', 'mid-model', 'zeta-model']),
    )
    await user.click(screen.getByRole('button', { name: 'Sắp xếp theo model' }))
    await waitFor(() =>
      expect(names()).toEqual(['zeta-model', 'mid-model', 'alpha-model']),
    )
  })

  it('shows an empty state when the provider has no models', async () => {
    mockBase([])
    await openProvider(/groq/i)
    expect(await screen.findByText('Chưa có model nào')).toBeInTheDocument()
  })

  it('shows an error with retry when the table fails', { timeout: 15_000 }, async () => {
    mockBase()
    server.use(
      http.get('*/providers/:id/model-table', () =>
        HttpResponse.json({ error: 'boom' }, { status: 500 }),
      ),
    )
    await openProvider(/groq/i)
    expect(await screen.findByText('boom', undefined, slow)).toBeInTheDocument()
  })

  it('turns the selected models on or off with the right body', async () => {
    const { calls, record } = mockBase(models)
    server.use(
      http.post('*/providers/groq/models/active', async ({ request }) => {
        record('active', await request.json())
        return HttpResponse.json({ updated: 2 })
      }),
    )
    const user = await openProvider(/groq/i)
    await screen.findByText('alpha-model')
    await user.click(screen.getByRole('checkbox', { name: 'Chọn alpha-model' }))
    await user.click(screen.getByRole('checkbox', { name: 'Chọn zeta-model' }))
    await user.click(screen.getByRole('button', { name: 'Tắt đã chọn' }))
    await waitFor(() => expect(calls.bodies.active).toHaveLength(1))
    const body = calls.bodies.active[0] as { models: string[]; active: boolean }
    expect([...body.models].sort()).toEqual(['alpha-model', 'zeta-model'])
    expect(body.active).toBe(false)
  })

  it('asks for confirmation before deleting models', async () => {
    const { calls, record } = mockBase(models)
    server.use(
      http.post('*/providers/groq/models/delete', async ({ request }) => {
        record('delete', await request.json())
        return HttpResponse.json({ deleted: 1 })
      }),
    )
    const user = await openProvider(/groq/i)
    await screen.findByText('alpha-model')
    await user.click(screen.getByRole('checkbox', { name: 'Chọn alpha-model' }))
    await user.click(screen.getByRole('button', { name: 'Xóa đã chọn' }))
    const dialog = await screen.findByRole('alertdialog')
    expect(calls.bodies.delete).toBeUndefined()
    await user.click(within(dialog).getByRole('button', { name: 'Xóa model' }))
    await waitFor(() => expect(calls.bodies.delete).toHaveLength(1))
    expect(calls.bodies.delete[0]).toEqual({ models: ['alpha-model'] })
  })

  it('does not delete when the confirmation is cancelled', async () => {
    const { calls } = mockBase(models)
    const user = await openProvider(/groq/i)
    await screen.findByText('alpha-model')
    await user.click(screen.getByRole('checkbox', { name: 'Chọn alpha-model' }))
    await user.click(screen.getByRole('button', { name: 'Xóa đã chọn' }))
    const dialog = await screen.findByRole('alertdialog')
    await user.click(within(dialog).getByRole('button', { name: 'Hủy' }))
    expect(calls.bodies.delete).toBeUndefined()
  })

  it('tests a model and shows its result', async () => {
    const { calls, record } = mockBase(models)
    server.use(
      http.post('*/providers/groq/models/test', async ({ request }) => {
        record('test', await request.json())
        return HttpResponse.json({
          ok: true,
          status: 200,
          ms: 321,
          message: 'OK',
          model: 'zeta-model',
        })
      }),
    )
    const user = await openProvider(/groq/i)
    await screen.findByText('alpha-model')
    await user.click(screen.getByRole('button', { name: 'Thử zeta-model' }))
    const result = await screen.findByRole('status', {
      name: 'Kết quả thử model',
    })
    expect(within(result).getByText('zeta-model')).toBeInTheDocument()
    expect(within(result).getByText(/321 ms/)).toBeInTheDocument()
    expect(calls.bodies.test[0]).toEqual({ model: 'zeta-model' })
  })

  it('shows a failed test with the server message', async () => {
    mockBase(models)
    server.use(
      http.post('*/providers/groq/models/test', () =>
        HttpResponse.json({
          ok: false,
          status: 429,
          ms: 50,
          message: 'rate limited',
        }),
      ),
    )
    const user = await openProvider(/groq/i)
    await screen.findByText('alpha-model')
    await user.click(screen.getByRole('button', { name: 'Thử alpha-model' }))
    const result = await screen.findByRole('status', {
      name: 'Kết quả thử model',
    })
    expect(within(result).getByText('rate limited')).toBeInTheDocument()
  })

  it('pages through a long list on the client', async () => {
    const many = Array.from({ length: 25 }, (_, i) =>
      row(`model-${String(i).padStart(2, '0')}`),
    )
    mockBase(many)
    const user = await openProvider(/groq/i)
    await screen.findByText('model-00')
    expect(screen.queryByText('model-24')).not.toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Trang sau' }))
    await user.click(screen.getByRole('button', { name: 'Trang sau' }))
    expect(await screen.findByText('model-24')).toBeInTheDocument()
  })
})

describe('rotation tab', () => {
  it('saves the mode and sticky count', async () => {
    const { calls, record } = mockBase()
    server.use(
      http.post('*/providers/groq/rotation', async ({ request }) => {
        record('rotation', await request.json())
        return HttpResponse.json({
          rotation: { mode: 'fallback', sticky: 5, order: null },
        })
      }),
    )
    const user = await openProvider(/groq/i, 'Xoay vòng')
    const sticky = await screen.findByRole('spinbutton', {
      name: 'Số request liên tiếp',
    })
    await user.clear(sticky)
    await user.type(sticky, '5')
    await user.click(screen.getByRole('radio', { name: /Dự phòng/ }))
    await user.click(screen.getByRole('button', { name: 'Lưu xoay vòng' }))
    await waitFor(() => expect(calls.bodies.rotation).toHaveLength(1))
    expect(calls.bodies.rotation[0]).toEqual({ mode: 'fallback', sticky: 5 })
  })

  it('rejects an out-of-range sticky count before sending it', async () => {
    const { calls } = mockBase()
    const user = await openProvider(/groq/i, 'Xoay vòng')
    const sticky = await screen.findByRole('spinbutton', {
      name: 'Số request liên tiếp',
    })
    await user.clear(sticky)
    await user.type(sticky, '5000')
    await user.click(screen.getByRole('button', { name: 'Lưu xoay vòng' }))
    expect(await screen.findByText('Nhập số từ 1 đến 1000')).toBeInTheDocument()
    expect(calls.bodies.rotation).toBeUndefined()
  })

  it('shows the error the server sends', async () => {
    mockBase()
    server.use(
      http.post('*/providers/groq/rotation', () =>
        HttpResponse.json({ error: 'sticky is 1 to 1000' }, { status: 400 }),
      ),
    )
    const user = await openProvider(/groq/i, 'Xoay vòng')
    await screen.findByRole('spinbutton', { name: 'Số request liên tiếp' })
    await user.click(screen.getByRole('button', { name: 'Lưu xoay vòng' }))
    expect(await screen.findByText('sticky is 1 to 1000')).toBeInTheDocument()
  })
})

describe('model policy tab', () => {
  it('saves the policy', async () => {
    const { calls, record } = mockBase()
    server.use(
      http.post('*/providers/groq/model-policy', async ({ request }) => {
        record('policy', await request.json())
        return HttpResponse.json({
          policy: { autoTest: true, onlyFree: false },
          running: false,
        })
      }),
    )
    const user = await openProvider(/groq/i, 'Chính sách model')
    await user.click(await screen.findByRole('switch', { name: 'Tự động thử model' }))
    await user.click(screen.getByRole('button', { name: 'Lưu chính sách' }))
    await waitFor(() => expect(calls.bodies.policy).toHaveLength(1))
    expect(calls.bodies.policy[0]).toEqual({ autoTest: true, onlyFree: false })
  })
})

describe('definition tab', () => {
  it('creates a custom provider and shows a 400 next to its field', async () => {
    const { calls, record } = mockBase()
    let attempt = 0
    server.use(
      http.post('*/provider-defs', async ({ request }) => {
        record('def', await request.json())
        attempt += 1
        if (attempt === 1) {
          return HttpResponse.json(
            { error: 'baseUrl: an http(s) URL' },
            { status: 400 },
          )
        }
        return HttpResponse.json({ id: 'newone' })
      }),
    )
    const user = await openProvider(/groq/i, 'Định nghĩa')
    await user.click(
      await screen.findByRole('button', { name: 'Thêm nhà cung cấp tùy chỉnh' }),
    )
    const dialog = await screen.findByRole('dialog')
    await user.type(within(dialog).getByLabelText('Mã nhà cung cấp'), 'newone')
    await user.type(within(dialog).getByLabelText('Địa chỉ gốc (baseUrl)'), 'htp:/x')
    await user.click(within(dialog).getByRole('button', { name: 'Lưu' }))
    const alert = await within(dialog).findByText('baseUrl: an http(s) URL')
    expect(alert).toBeInTheDocument()
    const input = within(dialog).getByLabelText('Địa chỉ gốc (baseUrl)')
    expect(input).toHaveAttribute('aria-invalid', 'true')
    await user.clear(input)
    await user.type(input, 'https://api.new.test/v1')
    await user.click(within(dialog).getByRole('button', { name: 'Lưu' }))
    await waitFor(() => expect(calls.bodies.def).toHaveLength(2))
    expect(calls.bodies.def[1]).toMatchObject({
      id: 'newone',
      kind: 'apikey',
      api: 'openai',
      baseUrl: 'https://api.new.test/v1',
    })
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
  })

  it('deletes a custom provider only after confirmation', async () => {
    const { calls, record } = mockBase()
    server.use(
      http.post('*/provider-defs/acme/delete', async ({ request }) => {
        record('delete', await request.json())
        return HttpResponse.json({ deleted: true })
      }),
    )
    const user = await openProvider(/Acme AI/, 'Định nghĩa')
    await user.click(await screen.findByRole('button', { name: 'Xóa nhà cung cấp' }))
    const dialog = await screen.findByRole('alertdialog')
    expect(calls.bodies.delete).toBeUndefined()
    await user.click(within(dialog).getByRole('button', { name: 'Xóa' }))
    await waitFor(() => expect(calls.bodies.delete).toHaveLength(1))
  })

  it('shows the server refusal when a provider still has accounts', async () => {
    mockBase()
    server.use(
      http.post('*/provider-defs/acme/delete', () =>
        HttpResponse.json({ error: 'delete its 1 account(s) first' }, { status: 400 }),
      ),
    )
    const user = await openProvider(/Acme AI/, 'Định nghĩa')
    await user.click(await screen.findByRole('button', { name: 'Xóa nhà cung cấp' }))
    const dialog = await screen.findByRole('alertdialog')
    await user.click(within(dialog).getByRole('button', { name: 'Xóa' }))
    expect(
      await within(dialog).findByText('delete its 1 account(s) first'),
    ).toBeInTheDocument()
  })

  it('does not offer to edit a built-in provider', async () => {
    mockBase()
    await openProvider(/groq/i, 'Định nghĩa')
    expect(
      await screen.findByText(/nhà cung cấp dựng sẵn/i),
    ).toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: 'Xóa nhà cung cấp' }),
    ).not.toBeInTheDocument()
  })
})

describe('zen sessions', () => {
  const zen = {
    count: 1,
    ttlSeconds: 3600,
    maxSessions: 100,
    sessions: [
      {
        id: 'ses_1',
        caller: 'caller-abc',
        source: 'header',
        uses: 4,
        created: '2026-10-01T00:00:00Z',
        lastUsed: '2026-10-01T00:10:00Z',
        expiresInSeconds: 3000,
      },
    ],
  }

  it('lists held sessions for the Zen provider', async () => {
    mockBase()
    server.use(http.get('*/api/zen/sessions', () => HttpResponse.json(zen)))
    await openProvider(/OpenCode Zen/, 'Phiên Zen')
    expect(await screen.findByText('caller-abc')).toBeInTheDocument()
  })

  it('shows an empty state without sessions', async () => {
    mockBase()
    server.use(
      http.get('*/api/zen/sessions', () =>
        HttpResponse.json({ ...zen, count: 0, sessions: [] }),
      ),
    )
    await openProvider(/OpenCode Zen/, 'Phiên Zen')
    expect(await screen.findByText('Chưa có phiên nào')).toBeInTheDocument()
  })

  it('shows an error when the sessions cannot be read', async () => {
    mockBase()
    server.use(
      http.get('*/api/zen/sessions', () =>
        HttpResponse.json({ error: 'forbidden' }, { status: 403 }),
      ),
    )
    await openProvider(/OpenCode Zen/, 'Phiên Zen')
    expect(await screen.findByText('forbidden')).toBeInTheDocument()
  })

  it('has no Zen tab for other providers', async () => {
    mockBase()
    await openProvider(/groq/i)
    await screen.findByRole('tab', { name: 'Model' })
    expect(screen.queryByRole('tab', { name: 'Phiên Zen' })).not.toBeInTheDocument()
  })
})
