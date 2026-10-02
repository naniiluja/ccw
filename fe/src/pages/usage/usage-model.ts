import type { UsageRow } from '@/api/usage'

export const allValue = 'all'

export interface Filters {
  model: string
  account: string
}

export interface DayTotal {
  day: string
  inputTokens: number
  outputTokens: number
  requests: number
}

export interface Totals {
  inputTokens: number
  outputTokens: number
  requests: number
  days: number
}

export function applyFilters(rows: UsageRow[], f: Filters): UsageRow[] {
  return rows.filter(
    (r) =>
      (f.model === allValue || r.model === f.model) &&
      (f.account === allValue || r.connectionId === f.account),
  )
}

/** Sums the rows per day, oldest day first, for the chart. */
export function byDay(rows: UsageRow[]): DayTotal[] {
  const days = new Map<string, DayTotal>()
  for (const r of rows) {
    const d = days.get(r.day) ?? {
      day: r.day,
      inputTokens: 0,
      outputTokens: 0,
      requests: 0,
    }
    d.inputTokens += r.inputTokens
    d.outputTokens += r.outputTokens
    d.requests += r.requests
    days.set(r.day, d)
  }
  return [...days.values()].sort((a, b) => a.day.localeCompare(b.day))
}

export function totalsOf(days: DayTotal[]): Totals {
  return days.reduce<Totals>(
    (t, d) => ({
      inputTokens: t.inputTokens + d.inputTokens,
      outputTokens: t.outputTokens + d.outputTokens,
      requests: t.requests + d.requests,
      days: t.days + 1,
    }),
    { inputTokens: 0, outputTokens: 0, requests: 0, days: 0 },
  )
}

export type SortKey =
  | 'day'
  | 'account'
  | 'model'
  | 'inputTokens'
  | 'outputTokens'
  | 'requests'

export interface Sort {
  key: SortKey
  dir: 'asc' | 'desc'
}

export function sortRows(
  rows: UsageRow[],
  sort: Sort,
  nameOf: (id: string) => string,
): UsageRow[] {
  const value = (r: UsageRow): string | number =>
    sort.key === 'account' ? nameOf(r.connectionId) : r[sort.key === 'day' ? 'day' : sort.key]
  const sign = sort.dir === 'asc' ? 1 : -1
  return [...rows].sort((a, b) => {
    const x = value(a)
    const y = value(b)
    const c =
      typeof x === 'number' && typeof y === 'number'
        ? x - y
        : String(x).localeCompare(String(y))
    return c * sign
  })
}

const numbers = new Intl.NumberFormat('vi-VN')
export const formatNumber = (n: number) => numbers.format(n)

const compact = new Intl.NumberFormat('vi-VN', { notation: 'compact' })
export const formatCompact = (n: number) => compact.format(n)

/** "2030-01-02" to "02/01" for the chart axis. */
export const shortDay = (day: string) => {
  const [, m, d] = day.split('-')
  return m && d ? `${d}/${m}` : day
}

/** "2030-01-02" to "02/01/2030". */
export const longDay = (day: string) => day.split('-').reverse().join('/')
