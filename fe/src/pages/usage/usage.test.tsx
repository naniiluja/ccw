import { screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'
import { renderApp, useSession } from '@/test/render'
import { server } from '@/test/server'

// Shape follows store.UsageRow in be/internal/store/usage.go.
const rows = [
  { day: '2030-01-02', connectionId: 'c1', model: 'claude-sonnet', inputTokens: 1000, outputTokens: 200, requests: 5 },
  { day: '2030-01-02', connectionId: 'c2', model: 'llama-3.3', inputTokens: 300, outputTokens: 50, requests: 2 },
  { day: '2030-01-01', connectionId: 'c1', model: 'claude-sonnet', inputTokens: 5000, outputTokens: 900, requests: 9 },
]

function mockUsage(usage: unknown[]) {
  server.use(
    http.get('*/usage', () => HttpResponse.json({ usage })),
    http.get('*/accounts', () =>
      HttpResponse.json({
        accounts: [
          { id: 'c1', provider: 'claude', label: 'work@example.com', isActive: true },
          { id: 'c2', provider: 'groq', label: 'groq-main', isActive: true },
        ],
      }),
    ),
  )
}

const bodyRows = () =>
  within(screen.getByRole('table')).getAllByRole('row').slice(1)

describe('usage page', () => {
  it('shows the chart and a detail table with account names', async () => {
    useSession()
    mockUsage(rows)
    renderApp('/usage')
    expect(await screen.findByRole('img', { name: /Biểu đồ/ })).toBeInTheDocument()
    expect(bodyRows()).toHaveLength(3)
    expect(screen.getAllByText('work@example.com').length).toBeGreaterThan(0)
    expect(screen.getByText('6.300')).toBeInTheDocument()
  })

  it('filters by model', async () => {
    useSession()
    mockUsage(rows)
    const user = userEvent.setup()
    renderApp('/usage')
    await screen.findByRole('table')
    await user.click(screen.getByRole('combobox', { name: 'Model' }))
    await user.click(await screen.findByRole('option', { name: 'llama-3.3' }))
    expect(bodyRows()).toHaveLength(1)
    expect(within(bodyRows()[0]).getByText('llama-3.3')).toBeInTheDocument()
  })

  it('sorts by a column header', async () => {
    useSession()
    mockUsage(rows)
    const user = userEvent.setup()
    renderApp('/usage')
    await screen.findByRole('table')
    await user.click(screen.getByRole('button', { name: /Số request/ }))
    expect(within(bodyRows()[0]).getByText('2')).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: /Số request/ }))
    expect(within(bodyRows()[0]).getByText('9')).toBeInTheDocument()
  })

  it('shows an empty state without data', async () => {
    useSession()
    mockUsage([])
    renderApp('/usage')
    expect(await screen.findByText('Chưa có dữ liệu lượng dùng')).toBeInTheDocument()
  })

  it('shows an error state with retry', { timeout: 20000 }, async () => {
    useSession()
    let ok = false
    server.use(
      http.get('*/usage', () =>
        ok
          ? HttpResponse.json({ usage: rows })
          : HttpResponse.json({ error: 'cannot read usage' }, { status: 500 }),
      ),
      http.get('*/accounts', () => HttpResponse.json({ accounts: [] })),
    )
    const user = userEvent.setup()
    renderApp('/usage')
    expect(
      await screen.findByText('cannot read usage', {}, { timeout: 8000 }),
    ).toBeInTheDocument()
    ok = true
    await user.click(screen.getByRole('button', { name: 'Thử lại' }))
    expect(await screen.findByRole('table')).toBeInTheDocument()
  })
})
