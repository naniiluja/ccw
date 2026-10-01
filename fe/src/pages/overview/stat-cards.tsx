import {
  GaugeIcon,
  GitCompareArrowsIcon,
  type LucideIcon,
  TriangleAlertIcon,
  UsersIcon,
} from 'lucide-react'
import type { ReactNode } from 'react'
import { Link } from 'react-router'
import type { Account, AccountTest } from '@/api/accounts'
import { failingAccounts, errorTotal, lowQuotaAccounts } from '@/api/overview'
import type { ErrorGroup } from '@/api/errors'
import type { AccountQuota } from '@/api/quota'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { BlockError, BlockSkeleton } from './block'
import { formatNumber } from './format'

interface Source<T> {
  data: T | undefined
  isPending: boolean
  isError: boolean
  error: Error | null
  refetch: () => unknown
}

interface StatProps {
  id: string
  label: string
  icon: LucideIcon
  to: string
  state: Pick<Source<unknown>, 'isPending' | 'isError' | 'error' | 'refetch'>
  value: number
  hint: ReactNode
  tone?: 'default' | 'warn'
}

function Stat({ id, label, icon: Icon, to, state, value, hint, tone }: StatProps) {
  return (
    <div data-testid={`stat-${id}`} className="min-w-0">
      <Card size="sm" className="h-full">
        <CardHeader>
          <CardTitle className="flex items-center gap-2 text-sm font-medium text-muted-foreground">
            <Icon aria-hidden="true" className="size-4" />
            {label}
          </CardTitle>
        </CardHeader>
        <CardContent>
          {state.isPending ? (
            <BlockSkeleton label={`Đang tải ${label.toLowerCase()}`} className="h-14" />
          ) : state.isError ? (
            <BlockError
              title={`Không tải được ${label.toLowerCase()}`}
              message={state.error?.message}
              onRetry={() => void state.refetch()}
            />
          ) : (
            <Link
              to={to}
              className="group block rounded-md outline-none focus-visible:ring-3 focus-visible:ring-ring/50"
            >
              <p
                className={`text-3xl font-semibold tabular-nums ${
                  tone === 'warn' && value > 0 ? 'text-destructive' : ''
                }`}
              >
                {formatNumber(value)}
              </p>
              <p className="mt-1 text-sm text-muted-foreground group-hover:underline">
                {hint}
              </p>
            </Link>
          )}
        </CardContent>
      </Card>
    </div>
  )
}

export interface StatSources {
  accounts: Source<Account[]>
  tests: Source<Record<string, AccountTest>>
  quota: Source<AccountQuota[]>
  errors: Source<ErrorGroup[]>
  drift: Source<{ unacked: number }>
  errorsSince: string
}

export function StatCards({ s }: { s: StatSources }) {
  const accounts = s.accounts.data ?? []
  const failing = failingAccounts(accounts, s.tests.data)
  const low = lowQuotaAccounts(s.quota.data ?? []).length
  const errors = errorTotal(s.errors.data ?? [])
  const unacked = s.drift.data?.unacked ?? 0
  return (
    <div className="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-4">
      <Stat
        id="accounts"
        label="Tài khoản"
        icon={UsersIcon}
        to="/accounts"
        state={s.accounts}
        value={accounts.length}
        hint={failing > 0 ? `${failing} đang lỗi` : 'Không có tài khoản lỗi'}
      />
      <Stat
        id="quota"
        label="Gần hết quota"
        icon={GaugeIcon}
        to="/quota"
        state={s.quota}
        value={low}
        tone="warn"
        hint={low > 0 ? 'Dùng từ 75% trở lên' : 'Quota còn thoải mái'}
      />
      <Stat
        id="errors"
        label="Lỗi 24 giờ qua"
        icon={TriangleAlertIcon}
        to={`/errors?since=${encodeURIComponent(s.errorsSince)}`}
        state={s.errors}
        value={errors}
        tone="warn"
        hint={errors > 0 ? 'Xem chi tiết lỗi' : 'Không có lỗi'}
      />
      <Stat
        id="drift"
        label="Drift chưa xác nhận"
        icon={GitCompareArrowsIcon}
        to="/drift?unacked=1"
        state={s.drift}
        value={unacked}
        tone="warn"
        hint={unacked > 0 ? 'Cần xem xét' : 'Đã xác nhận hết'}
      />
    </div>
  )
}
