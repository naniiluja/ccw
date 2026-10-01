import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { useQuery } from '@tanstack/react-query'
import { describe, expect, it } from 'vitest'
import { api } from '@/api/client'
import { renderApp, useSession } from '@/test/render'
import { server } from '@/test/server'
import { routeTable } from './routes'

function Probe() {
  const q = useQuery({ queryKey: ['probe'], queryFn: () => api('/api/probe') })
  return <p>{q.isError ? 'probe failed' : 'probe pending'}</p>
}

describe('shell', () => {
  it('lists every navigation entry of the route table', async () => {
    useSession()
    renderApp('/')
    await screen.findByRole('heading', { name: 'Tổng quan' })
    const nav = screen.getByRole('navigation', { name: 'Điều hướng chính' })
    for (const route of routeTable.filter((r) => r.nav)) {
      expect(
        within(nav).getByRole('link', { name: route.title }),
      ).toBeInTheDocument()
    }
  })

  it('shows a Vietnamese not-found page inside the shell', async () => {
    useSession()
    renderApp('/khong-co-trang-nay')
    expect(
      (await screen.findAllByText('Không tìm thấy trang')).length,
    ).toBeGreaterThan(0)
  })

  it('opens the command palette with Ctrl+K and jumps to a page', async () => {
    useSession()
    const user = userEvent.setup()
    const app = renderApp('/')
    await screen.findByRole('heading', { name: 'Tổng quan' })
    await user.keyboard('{Control>}k{/Control}')
    await user.click(await screen.findByRole('option', { name: /Khóa API/ }))
    await waitFor(() => expect(app.router.state.location.pathname).toBe('/keys'))
  })

  it('writes the chosen theme to localStorage and the dark class', async () => {
    useSession()
    const user = userEvent.setup()
    renderApp('/')
    await user.click(await screen.findByRole('button', { name: 'Đổi giao diện' }))
    await user.click(await screen.findByRole('menuitem', { name: 'Tối' }))
    await waitFor(() =>
      expect(document.documentElement).toHaveClass('dark'),
    )
    expect(localStorage.getItem('ccw-theme')).toBe('dark')
  })
})

describe('auth flow', () => {
  it('sends a signed-out visitor to the login page and keeps the redirect', async () => {
    useSession({ authenticated: false })
    const app = renderApp('/accounts')
    expect(
      await screen.findByRole('heading', { name: 'Đăng nhập' }),
    ).toBeInTheDocument()
    expect(app.router.state.location.pathname).toBe('/login')
    expect(app.router.state.location.search).toBe('?redirect=%2Faccounts')
  })

  it('goes straight in when the server asks for no login', async () => {
    useSession({ authRequired: false, authenticated: true })
    renderApp('/login')
    expect(
      await screen.findByRole('heading', { name: 'Tổng quan' }),
    ).toBeInTheDocument()
  })

  it('signs in with the right password and lands on the overview', async () => {
    const session = useSession({ authenticated: false })
    let body = ''
    server.use(
      http.post('*/login', async ({ request }) => {
        body = await request.text()
        expect(request.headers.get('content-type')).toContain(
          'application/x-www-form-urlencoded',
        )
        expect(request.headers.get('accept')).toBe('application/json')
        session.authenticated = true
        return HttpResponse.json({ ok: true })
      }),
    )
    const user = userEvent.setup()
    renderApp('/login')
    await user.type(await screen.findByLabelText('Mật khẩu'), 'a-long-password-1')
    await user.click(screen.getByRole('button', { name: 'Đăng nhập' }))
    expect(
      await screen.findByRole('heading', { name: 'Tổng quan' }),
    ).toBeInTheDocument()
    expect(body).toBe('password=a-long-password-1')
  })

  it('shows an error for a wrong password', async () => {
    useSession({ authenticated: false })
    server.use(
      http.post('*/login', () =>
        HttpResponse.json({ error: 'Wrong password. Try again.' }, { status: 401 }),
      ),
    )
    const user = userEvent.setup()
    renderApp('/login')
    await user.type(await screen.findByLabelText('Mật khẩu'), 'not-the-password')
    await user.click(screen.getByRole('button', { name: 'Đăng nhập' }))
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'Mật khẩu không đúng',
    )
  })

  it('counts down the Retry-After of a 429 and blocks the button', async () => {
    useSession({ authenticated: false })
    server.use(
      http.post('*/login', () =>
        HttpResponse.json(
          { error: 'Too many attempts. Wait a few minutes.' },
          { status: 429, headers: { 'Retry-After': '3' } },
        ),
      ),
    )
    const user = userEvent.setup()
    renderApp('/login')
    await user.type(await screen.findByLabelText('Mật khẩu'), 'not-the-password')
    await user.click(screen.getByRole('button', { name: 'Đăng nhập' }))
    expect(await screen.findByText(/Thử lại sau 3 giây/)).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /Đăng nhập/ })).toBeDisabled()
    expect(
      await screen.findByText(/Thử lại sau 2 giây/, {}, { timeout: 2500 }),
    ).toBeInTheDocument()
  })

  it('sends any query that gets a 401 back to the login page', async () => {
    useSession()
    server.use(
      http.get('*/api/probe', () =>
        HttpResponse.json({ error: 'unauthorized' }, { status: 401 }),
      ),
    )
    const app = renderApp('/probe', [{ path: '/probe', element: <Probe /> }])
    expect(
      await screen.findByRole('heading', { name: 'Đăng nhập' }),
    ).toBeInTheDocument()
    expect(app.router.state.location.pathname).toBe('/login')
    expect(app.router.state.location.search).toBe('?redirect=%2Fprobe')
  })
})
