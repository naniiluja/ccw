import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'
import type { DriftChange } from '@/api/drift'
import { renderApp, useSession } from '@/test/render'
import { server } from '@/test/server'

const change = (over: Partial<DriftChange>): DriftChange => ({
  id: 1,
  at: '2026-10-01T08:30:00Z',
  direction: 'response',
  provider: 'groq',
  endpoint: '/chat/completions',
  event: '',
  path: 'usage.total_tokens',
  kind: 'changed',
  oldType: 'number',
  newType: 'string',
  sample: '{"usage":{"total_tokens":"12"}}',
  acked: false,
  ...over,
})

const changes = [
  change({ id: 1 }),
  change({
    id: 2,
    provider: 'openai',
    direction: 'request',
    path: 'messages[].name',
    kind: 'added',
    oldType: '',
    newType: 'string',
    verdict: 'data_noise',
    verdictConf: 0.91,
    autoAcked: true,
    acked: true,
  }),
]

interface Calls {
  lists: URL[]
  acks: unknown[]
  reviews: unknown[]
  seeds: unknown[]
}

function mockDrift(over: { ackStatus?: number; list?: DriftChange[] } = {}) {
  const calls: Calls = { lists: [], acks: [], reviews: [], seeds: [] }
  const list = over.list ?? changes
  server.use(
    http.get('*/drift/changes', ({ request }) => {
      calls.lists.push(new URL(request.url))
      return HttpResponse.json({
        changes: list,
        unacked: list.filter((c) => !c.acked).length,
      })
    }),
    http.post('*/drift/ack', async ({ request }) => {
      calls.acks.push(await request.json())
      if (over.ackStatus) {
        return HttpResponse.json(
          { error: 'cannot acknowledge' },
          { status: over.ackStatus },
        )
      }
      return HttpResponse.json({ acked: 1 })
    }),
    http.get('*/drift/fields', () =>
      HttpResponse.json({
        fields: [
          {
            key: 'response|groq|/chat/completions@x|k',
            path: 'choices[].message.content',
            type: 'string',
            seen: 120,
            firstObs: 1,
            lastObs: 120,
            gone: false,
            lastAt: '2026-10-01T08:30:00Z',
          },
        ],
      }),
    ),
    http.get('*/drift/review', () =>
      HttpResponse.json({
        enabled: false,
        decisionModel: '',
        resolverModel: '',
        ackConfidence: 0.6,
        ready: false,
        lastError: '',
      }),
    ),
    http.post('*/drift/review', async ({ request }) => {
      const body = await request.json()
      calls.reviews.push(body)
      return HttpResponse.json({
        enabled: false,
        decisionModel: '',
        resolverModel: '',
        ackConfidence: 0.6,
        ready: true,
        lastError: '',
        ...(body as object),
      })
    }),
    http.post('*/api/drift/seed', async ({ request }) => {
      calls.seeds.push(await request.json())
      return HttpResponse.json({ learned: 3 })
    }),
  )
  return calls
}

describe('Drift page', () => {
  it('lists changes with old and new type and the unacked count', async () => {
    useSession()
    mockDrift()
    renderApp('/drift')
    expect(await screen.findByText('usage.total_tokens')).toBeInTheDocument()
    expect(screen.getByText('messages[].name')).toBeInTheDocument()
    expect(screen.getByTestId('unacked-count')).toHaveTextContent('1')
    expect(screen.getByText(/data_noise/)).toBeInTheDocument()
    expect(screen.getByText(/91%/)).toBeInTheDocument()
  })

  it('puts filters into the URL and the request query', async () => {
    useSession()
    const calls = mockDrift()
    const user = userEvent.setup()
    const { router } = renderApp('/drift')
    await screen.findByText('usage.total_tokens')
    await user.click(screen.getByRole('switch', { name: /chỉ chưa xác nhận/i }))
    await waitFor(() =>
      expect(calls.lists.at(-1)?.searchParams.get('unacked')).toBe('1'),
    )
    expect(router.state.location.search).toContain('unacked=1')
    await user.type(
      screen.getByRole('textbox', { name: /nhà cung cấp/i }),
      'groq',
    )
    await waitFor(() =>
      expect(calls.lists.at(-1)?.searchParams.get('provider')).toBe('groq'),
    )
    await user.click(screen.getByRole('combobox', { name: /hướng/i }))
    await user.click(await screen.findByRole('option', { name: 'Yêu cầu' }))
    await waitFor(() =>
      expect(calls.lists.at(-1)?.searchParams.get('direction')).toBe('request'),
    )
    expect(router.state.location.search).toContain('direction=request')
  })

  it('opens the sheet of the change named by ?change= and drops the param on close', async () => {
    useSession()
    mockDrift()
    const user = userEvent.setup()
    const { router } = renderApp('/drift?change=1')
    const sheet = await screen.findByRole('dialog')
    expect(within(sheet).getByText('number')).toBeInTheDocument()
    await user.keyboard('{Escape}')
    await waitFor(() => expect(router.state.location.search).not.toContain('change='))
  })

  it('drops ?change= from the URL after the change is acknowledged from the sheet', async () => {
    useSession()
    const calls = mockDrift()
    const user = userEvent.setup()
    const { router } = renderApp('/drift?change=1')
    const sheet = await screen.findByRole('dialog')
    await user.click(within(sheet).getByRole('button', { name: 'Xác nhận thay đổi' }))
    await waitFor(() => expect(calls.acks).toEqual([{ ids: [1] }]))
    await waitFor(() => expect(router.state.location.search).not.toContain('change='))
  })

  it('opens the old and new diff and the sample in a sheet', async () => {
    useSession()
    mockDrift()
    const user = userEvent.setup()
    renderApp('/drift')
    await user.click(
      await screen.findByRole('button', {
        name: /xem chi tiết usage\.total_tokens/i,
      }),
    )
    const sheet = await screen.findByRole('dialog')
    expect(within(sheet).getByText('number')).toBeInTheDocument()
    expect(within(sheet).getByText('string')).toBeInTheDocument()
    expect(within(sheet).getByText(/"total_tokens":"12"/)).toBeInTheDocument()
  })

  it('acks one change optimistically and sends its id', async () => {
    useSession()
    const calls = mockDrift()
    const user = userEvent.setup()
    renderApp('/drift')
    await user.click(
      await screen.findByRole('button', {
        name: /xác nhận usage\.total_tokens/i,
      }),
    )
    await waitFor(() => expect(calls.acks).toEqual([{ ids: [1] }]))
  })

  it('rolls an ack back when the server answers 500', async () => {
    useSession()
    mockDrift({ ackStatus: 500 })
    const user = userEvent.setup()
    renderApp('/drift')
    await screen.findByText('usage.total_tokens')
    expect(screen.getByTestId('unacked-count')).toHaveTextContent('1')
    await user.click(
      screen.getByRole('button', { name: /xác nhận usage\.total_tokens/i }),
    )
    expect(await screen.findByText(/không xác nhận được/i)).toBeInTheDocument()
    await waitFor(() =>
      expect(screen.getByTestId('unacked-count')).toHaveTextContent('1'),
    )
    expect(
      screen.getByRole('button', { name: /xác nhận usage\.total_tokens/i }),
    ).toBeInTheDocument()
  })

  it('acks the selected changes in bulk with exactly those ids', async () => {
    useSession()
    const calls = mockDrift({
      list: [
        change({ id: 7 }),
        change({ id: 8, path: 'id' }),
        change({ id: 9, path: 'model' }),
      ],
    })
    const user = userEvent.setup()
    renderApp('/drift')
    await screen.findByText('model')
    await user.click(
      screen.getByRole('checkbox', { name: /chọn usage\.total_tokens/i }),
    )
    await user.click(screen.getByRole('checkbox', { name: /chọn model/i }))
    await user.click(
      screen.getByRole('button', { name: /xác nhận 2 đã chọn/i }),
    )
    await waitFor(() => expect(calls.acks).toEqual([{ ids: [7, 9] }]))
  })

  it('shows an empty state when there are no changes', async () => {
    useSession()
    mockDrift({ list: [] })
    renderApp('/drift')
    expect(await screen.findByText(/chưa có thay đổi nào/i)).toBeInTheDocument()
  })

  it('shows an error state with a retry', async () => {
    useSession()
    mockDrift()
    server.use(
      http.get('*/drift/changes', () =>
        HttpResponse.json({ error: 'cannot read changes' }, { status: 500 }),
      ),
    )
    renderApp('/drift')
    expect(await screen.findByText('cannot read changes')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /thử lại/i })).toBeInTheDocument()
  })

  it('lists the watched fields per provider', async () => {
    useSession()
    mockDrift()
    const user = userEvent.setup()
    renderApp('/drift')
    await user.click(
      await screen.findByRole('tab', { name: /trường theo dõi/i }),
    )
    expect(
      await screen.findByText('choices[].message.content'),
    ).toBeInTheDocument()
    expect(screen.getAllByText('groq').length).toBeGreaterThan(0)
  })

  it('saves the review config with the exact body', async () => {
    useSession()
    const calls = mockDrift()
    const user = userEvent.setup()
    renderApp('/drift')
    await user.click(await screen.findByRole('tab', { name: /cấu hình/i }))
    await user.type(
      await screen.findByLabelText(/model quyết định/i),
      'typesafe/jev-latest',
    )
    await user.type(
      screen.getByLabelText(/model giải quyết/i),
      'antigravity/gemini-3.8-flash',
    )
    await user.click(screen.getByRole('switch', { name: /bật ai review/i }))
    await user.click(screen.getByRole('button', { name: /lưu cấu hình/i }))
    await waitFor(() =>
      expect(calls.reviews).toEqual([
        {
          enabled: true,
          decisionModel: 'typesafe/jev-latest',
          resolverModel: 'antigravity/gemini-3.8-flash',
          ackConfidence: 0.6,
        },
      ]),
    )
  })

  it('runs the review now', async () => {
    useSession()
    const calls = mockDrift()
    const user = userEvent.setup()
    renderApp('/drift')
    await user.click(await screen.findByRole('tab', { name: /cấu hình/i }))
    await user.click(await screen.findByRole('button', { name: /chạy ngay/i }))
    await waitFor(() => expect(calls.reviews).toEqual([{ run: true }]))
  })

  it('asks for confirmation before seeding', async () => {
    useSession()
    const calls = mockDrift()
    const user = userEvent.setup()
    renderApp('/drift')
    await user.click(await screen.findByRole('tab', { name: /cấu hình/i }))
    await user.type(await screen.findByLabelText('Nhà cung cấp'), 'groq')
    await user.type(screen.getByLabelText('Endpoint'), '/chat/completions')
    await user.click(screen.getByLabelText(/tài liệu mẫu/i))
    await user.paste('{"a":1}')
    await user.click(screen.getByRole('button', { name: /khởi tạo mẫu/i }))
    const dialog = await screen.findByRole('alertdialog')
    expect(calls.seeds).toEqual([])
    expect(
      within(dialog).getByText(/không ghi nhận thay đổi/i),
    ).toBeInTheDocument()
    await user.click(within(dialog).getByRole('button', { name: /^khởi tạo$/i }))
    await waitFor(() =>
      expect(calls.seeds).toEqual([
        {
          direction: 'response',
          provider: 'groq',
          endpoint: '/chat/completions',
          sse: false,
          documents: ['{"a":1}'],
        },
      ]),
    )
  })
})
