// A thin wrapper over fetch for the ccw server. The server answers the browser
// with JSON when the request says Accept: application/json, and reports every
// failure as {"error": "..."}; ApiError carries that message and the status.

export class ApiError extends Error {
  readonly status: number
  /** Seconds from a Retry-After header, when the server sent one. */
  readonly retryAfter?: number

  constructor(message: string, status: number, retryAfter?: number) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.retryAfter = retryAfter
  }
}

export const isApiError = (e: unknown): e is ApiError => e instanceof ApiError

// These two routes read a form body; every other write takes JSON.
const formRoutes = new Set(['/login', '/accounts'])

export interface RequestOptions {
  method?: string
  /** Request body: an object, sent as form fields or JSON depending on the route. */
  body?: Record<string, unknown>
  signal?: AbortSignal
}

function encode(
  path: string,
  method: string,
  body: Record<string, unknown> | undefined,
): { headers: Record<string, string>; body?: string } {
  if (body === undefined) return { headers: {} }
  if (method === 'POST' && formRoutes.has(path)) {
    const form = new URLSearchParams()
    for (const [key, value] of Object.entries(body)) {
      if (value !== undefined && value !== null) form.set(key, String(value))
    }
    return {
      headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
      body: form.toString(),
    }
  }
  return {
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  }
}

function parseRetryAfter(res: Response): number | undefined {
  const seconds = Number(res.headers.get('Retry-After'))
  return Number.isFinite(seconds) && seconds > 0 ? seconds : undefined
}

async function failure(res: Response): Promise<ApiError> {
  let message = `Yêu cầu thất bại (${res.status})`
  try {
    const data: unknown = await res.json()
    if (data && typeof data === 'object' && 'error' in data) {
      const text = (data as { error: unknown }).error
      if (typeof text === 'string' && text) message = text
    }
  } catch {
    // The body was not JSON; keep the generic message.
  }
  return new ApiError(message, res.status, parseRetryAfter(res))
}

export async function api<T = unknown>(
  path: string,
  { method = 'GET', body, signal }: RequestOptions = {},
): Promise<T> {
  const payload = encode(path, method, body)
  let res: Response
  try {
    res = await fetch(path, {
      method,
      credentials: 'same-origin',
      headers: { Accept: 'application/json', ...payload.headers },
      body: payload.body,
      signal,
    })
  } catch (e) {
    if (e instanceof DOMException && e.name === 'AbortError') throw e
    throw new ApiError('Không kết nối được tới máy chủ', 0)
  }
  if (!res.ok) throw await failure(res)
  if (res.status === 204) return undefined as T
  const text = await res.text()
  return (text ? JSON.parse(text) : undefined) as T
}
