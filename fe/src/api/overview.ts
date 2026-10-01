import type { Account, AccountTest } from './accounts'
import type { ErrorGroup } from './errors'
import type { AccountQuota } from './quota'
import type { UsageRow } from './usage'

// Pure aggregation for the overview page. Nothing here reads the network: the
// page feeds it the data of the existing hooks.

/** An account is "gần hết quota" from this used percentage on. */
export const lowQuotaPct = 75

export interface LowQuota {
  account: AccountQuota
  /** The highest used percentage over the account's limited windows. */
  usedPct: number
  /** Name of the window that has that percentage. */
  window: string
}

/** The accounts whose fullest window is at or above the threshold, fullest first. */
export function lowQuotaAccounts(
  accounts: AccountQuota[],
  threshold = lowQuotaPct,
): LowQuota[] {
  const found: LowQuota[] = []
  for (const account of accounts) {
    let top: LowQuota | undefined
    for (const w of account.windows ?? []) {
      if (w.unlimited || !Number.isFinite(w.usedPct)) continue
      if (!top || w.usedPct > top.usedPct) {
        top = { account, usedPct: w.usedPct, window: w.name }
      }
    }
    if (top && top.usedPct >= threshold) found.push(top)
  }
  return found.sort(
    (a, b) =>
      b.usedPct - a.usedPct || a.account.label.localeCompare(b.account.label),
  )
}

/** How many of the accounts failed their last test. Accounts never tested do not count. */
export function failingAccounts(
  accounts: Account[],
  tests: Record<string, AccountTest> | undefined,
): number {
  if (!tests) return 0
  return accounts.filter((a) => tests[a.id] && !tests[a.id].ok).length
}

/** The number of errors over every group. */
export const errorTotal = (groups: ErrorGroup[]) =>
  groups.reduce((sum, g) => sum + g.count, 0)

export interface DayTotal {
  day: string
  inputTokens: number
  outputTokens: number
  requests: number
}

/** The latest `count` days that have usage, summed per day, oldest first. */
export function lastDays(rows: UsageRow[], count = 14): DayTotal[] {
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
  return [...days.values()]
    .sort((a, b) => a.day.localeCompare(b.day))
    .slice(-count)
}
