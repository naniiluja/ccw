import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'
import { renderApp, useSession as mockSession } from '@/test/render'
import { server } from '@/test/server'

interface Filter {
  id: string
  provider: string
  kind: string
  pattern: string
  note: string
  enabled: boolean
}

const seed = (): Filter[] => [
  { id: 'f1', provider: '*', kind: 'schema', pattern: 'encrypted', note: 'Cursor schema', enabled: true },
  { id: 'f2', provider: 'antigravity', kind: 'system', pattern: '^x-billing:.*$', note: '', enabled: true },
  { id: 'f3', provider: 'groq', kind: 'field', pattern: 'thinking', note: 'groq rejects it', enabled: false },
]

const unsafe =
  'this pattern would strip an essential field from every request; narrow it'

function mockServer(initial: Filter[] = seed()) {
  const state = { list: initial, saved: [] as Filter[], deleted: [] as string[], saveStatus: 200 }
  mockSession()
  server.use(
    http.get('*/providers', () =>
      HttpResponse.json({
        providers: [
          { id: 'groq', setup: 'key', auth: 'apikey' },
          { id: 'antigravity', setup: 'oauth', auth: 'oauth' },
        ],
      }),
    ),
    http.get('*/filters', () => HttpResponse.json({ filters: state.list })),
    http.post('*/filters', async ({ request }) => {
      const f = (await request.json()) as Filter
      if (f.pattern === 'messages') {
        return HttpResponse.json({ error: unsafe }, { status: 400 })
      }
      if (state.saveStatus !== 200) {
        return HttpResponse.json({ error: 'cannot save filter' }, { status: state.saveStatus })
      }
      const stored = { ...f, id: f.id || 'new1' }
      state.saved.push(stored)
      state.list = f.id
        ? state.list.map((x) => (x.id === f.id ? stored : x))
        : [...state.list, stored]
      return HttpResponse.json(stored)
    }),
    http.post('*/filters/:id/delete', ({ params }) => {
      state.deleted.push(String(params.id))
      state.list = state.list.filter((x) => x.id !== params.id)
      return HttpResponse.json({ ok: true })
    }),
  )
  return state
}

describe('filters page', () => {
  it('groups rules by provider', async () => {
    mockServer()
    renderApp('/filters')
    expect(await screen.findByText('encrypted')).toBeInTheDocument()
    expect(screen.getByRole('region', { name: 'Tất cả nhà cung cấp' })).toBeInTheDocument()
    const group = screen.getByRole('region', { name: 'antigravity' })
    expect(within(group).getByText('^x-billing:.*$')).toBeInTheDocument()
  })

  it('shows an empty state that explains the blacklist', async () => {
    mockServer([])
    renderApp('/filters')
    expect(await screen.findByText(/Blacklist field loại bỏ/)).toBeInTheDocument()
  })

  it('shows an error state with a retry', async () => {
    mockSession()
    let fail = true
    server.use(
      http.get('*/providers', () => HttpResponse.json({ providers: [] })),
      http.get('*/filters', () =>
        fail
          ? HttpResponse.json({ error: 'cannot read filters' }, { status: 403 })
          : HttpResponse.json({ filters: seed() }),
      ),
    )
    const user = userEvent.setup()
    renderApp('/filters')
    expect(await screen.findByText('cannot read filters')).toBeInTheDocument()
    fail = false
    await user.click(screen.getByRole('button', { name: 'Thử lại' }))
    expect(await screen.findByText('encrypted')).toBeInTheDocument()
  })

  it('filters by provider', async () => {
    mockServer()
    const user = userEvent.setup()
    renderApp('/filters')
    await screen.findByText('encrypted')
    await user.click(screen.getByRole('combobox', { name: 'Lọc theo nhà cung cấp' }))
    await user.click(await screen.findByRole('option', { name: 'groq' }))
    expect(screen.queryByText('encrypted')).not.toBeInTheDocument()
    expect(screen.getByText('thinking')).toBeInTheDocument()
  })

  it('creates a rule', async () => {
    const state = mockServer()
    const user = userEvent.setup()
    renderApp('/filters')
    await screen.findByText('encrypted')
    await user.click(screen.getByRole('button', { name: 'Thêm quy tắc' }))
    const dialog = await screen.findByRole('dialog')
    await user.type(within(dialog).getByLabelText('Mẫu'), 'tools.*.strict')
    await user.type(within(dialog).getByLabelText('Ghi chú'), 'strict mode')
    await user.click(within(dialog).getByRole('button', { name: 'Lưu' }))
    expect(await screen.findByText('tools.*.strict')).toBeInTheDocument()
    expect(state.saved[0]).toMatchObject({ provider: '*', pattern: 'tools.*.strict', enabled: true })
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  })

  it('shows an unsafe-filter refusal beside the pattern field', async () => {
    mockServer()
    const user = userEvent.setup()
    renderApp('/filters')
    await screen.findByText('encrypted')
    await user.click(screen.getByRole('button', { name: 'Thêm quy tắc' }))
    const dialog = await screen.findByRole('dialog')
    await user.type(within(dialog).getByLabelText('Mẫu'), 'messages')
    await user.click(within(dialog).getByRole('button', { name: 'Lưu' }))
    expect(await within(dialog).findByText(unsafe)).toBeInTheDocument()
    expect(within(dialog).getByLabelText('Mẫu')).toHaveAccessibleDescription(unsafe)
  })

  it('validates an empty pattern before sending', async () => {
    const state = mockServer()
    const user = userEvent.setup()
    renderApp('/filters')
    await screen.findByText('encrypted')
    await user.click(screen.getByRole('button', { name: 'Thêm quy tắc' }))
    const dialog = await screen.findByRole('dialog')
    await user.click(within(dialog).getByRole('button', { name: 'Lưu' }))
    expect(await within(dialog).findByText('Nhập mẫu cần loại bỏ.')).toBeInTheDocument()
    expect(state.saved).toHaveLength(0)
  })

  it('toggles a rule and rolls back when the server fails', async () => {
    const state = mockServer()
    const user = userEvent.setup()
    renderApp('/filters')
    await screen.findByText('encrypted')
    const sw = screen.getByRole('switch', { name: 'Bật quy tắc encrypted' })
    expect(sw).toBeChecked()
    await user.click(sw)
    await waitFor(() => expect(state.saved[0]).toMatchObject({ id: 'f1', enabled: false }))
    await waitFor(() => expect(sw).not.toBeChecked())

    state.saveStatus = 500
    await user.click(screen.getByRole('switch', { name: 'Bật quy tắc thinking' }))
    await waitFor(() =>
      expect(screen.getByRole('switch', { name: 'Bật quy tắc thinking' })).not.toBeChecked(),
    )
    expect(await screen.findByText('Không đổi được trạng thái quy tắc')).toBeInTheDocument()
  })

  it('asks for confirmation naming the pattern before deleting', async () => {
    const state = mockServer()
    const user = userEvent.setup()
    renderApp('/filters')
    await screen.findByText('encrypted')
    await user.click(screen.getByRole('button', { name: 'Xóa quy tắc encrypted' }))
    const dialog = await screen.findByRole('alertdialog')
    expect(within(dialog).getByText('encrypted')).toBeInTheDocument()
    expect(state.deleted).toHaveLength(0)
    await user.click(within(dialog).getByRole('button', { name: 'Hủy' }))
    expect(state.deleted).toHaveLength(0)

    await user.click(screen.getByRole('button', { name: 'Xóa quy tắc encrypted' }))
    await user.click(
      await within(await screen.findByRole('alertdialog')).findByRole('button', { name: 'Xóa' }),
    )
    await waitFor(() => expect(state.deleted).toEqual(['f1']))
    await waitFor(() => expect(screen.queryByText('encrypted')).not.toBeInTheDocument())
  })
})
