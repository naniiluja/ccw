import { screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'
import { renderApp, useSession as stubSession } from '@/test/render'
import { server } from '@/test/server'

const contract = {
  authMode: 'password',
  sessionTtlSeconds: 86400,
  timezone: 'Asia/Ho_Chi_Minh',
  websearch: {
    provider: 'brave',
    model: 'gemini-2.5-flash',
    count: 7,
    url: 'https://search.example.test/api',
    keySet: true,
  },
}

function serve(body: unknown = contract, status = 200) {
  server.use(
    http.get('*/api/settings', () =>
      HttpResponse.json(body as object, { status }),
    ),
  )
}

async function open() {
  stubSession()
  renderApp('/settings')
  await screen.findByRole('heading', { name: 'Cài đặt', level: 1 })
}

describe('settings page', () => {
  it('shows every value next to the environment variable that sets it', async () => {
    serve()
    await open()
    expect(await screen.findByText('Asia/Ho_Chi_Minh')).toBeInTheDocument()
    expect(screen.getByText('brave')).toBeInTheDocument()
    expect(screen.getByText('gemini-2.5-flash')).toBeInTheDocument()
    expect(screen.getByText('7')).toBeInTheDocument()
    expect(
      screen.getByText('https://search.example.test/api'),
    ).toBeInTheDocument()
    for (const name of [
      'CCW_SEARCH_PROVIDER',
      'CCW_SEARCH_MODEL',
      'CCW_SEARCH_COUNT',
      'CCW_SEARCH_URL',
      'CCW_SEARCH_KEY',
      'CCW_TZ',
      'CCW_SESSION_TTL',
      'CCW_PASSWORD',
    ]) {
      expect(screen.getAllByText(name).length).toBeGreaterThan(0)
    }
    expect(screen.getAllByText(/chỉ đọc/i).length).toBeGreaterThan(0)
  })

  it('says whether the search key is set, never its value', async () => {
    serve()
    await open()
    expect(await screen.findByText('Đã đặt')).toBeInTheDocument()
  })

  it('says the search key is not set', async () => {
    serve({ ...contract, websearch: { ...contract.websearch, keySet: false } })
    await open()
    expect(await screen.findByText('Chưa đặt')).toBeInTheDocument()
    expect(screen.queryByText('Đã đặt')).not.toBeInTheDocument()
  })

  it('names the token variable in token mode', async () => {
    serve({ ...contract, authMode: 'token' })
    await open()
    expect((await screen.findAllByText('CCW_API_TOKEN')).length).toBeGreaterThan(
      0,
    )
  })

  it('has no input or save button', async () => {
    serve()
    await open()
    await screen.findByText('Asia/Ho_Chi_Minh')
    const main = screen.getByTestId('settings-page')
    expect(main.querySelectorAll('input, textarea, select')).toHaveLength(0)
    expect(within(main).queryByRole('button', { name: /lưu/i })).toBeNull()
  })

  it('shows the connection details from this origin', async () => {
    serve()
    await open()
    await screen.findByText('Asia/Ho_Chi_Minh')
    expect(screen.getByText(`${window.location.origin}/v1`)).toBeInTheDocument()
    expect(screen.getByText(`${window.location.origin}/mcp`)).toBeInTheDocument()
    expect(screen.getByText(/ANTHROPIC_BASE_URL/)).toBeInTheDocument()
    expect(screen.getByText(/-reset-password/)).toBeInTheDocument()
  })

  it('copies a snippet to the clipboard and shows a toast', async () => {
    serve()
    const user = userEvent.setup()
    await open()
    await screen.findByText('Asia/Ho_Chi_Minh')
    await user.click(
      screen.getByRole('button', { name: 'Sao chép cấu hình Claude Code' }),
    )
    expect(await navigator.clipboard.readText()).toContain('ANTHROPIC_BASE_URL')
    expect(await screen.findByText('Đã sao chép')).toBeInTheDocument()
  })

  it('offers a retry after a 500', async () => {
    serve({ error: 'boom' }, 500)
    const user = userEvent.setup()
    await open()
    const retry = await screen.findByRole('button', { name: 'Thử lại' })
    serve()
    await user.click(retry)
    expect(await screen.findByText('Asia/Ho_Chi_Minh')).toBeInTheDocument()
  })

  it('explains a 404 from a server without the route', async () => {
    serve({ error: 'not found' }, 404)
    await open()
    expect(await screen.findByText(/chưa hỗ trợ/i)).toBeInTheDocument()
  })
})
