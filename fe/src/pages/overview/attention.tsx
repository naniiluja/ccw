import type { ReactNode } from 'react'
import { Link } from 'react-router'
import type { DriftChange } from '@/api/drift'
import type { UpstreamError } from '@/api/errors'
import { type LowQuota } from '@/api/overview'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { BlockError, BlockSkeleton, Muted } from './block'
import { formatTime } from './format'

interface Source<T> {
  data: T | undefined
  isPending: boolean
  isError: boolean
  error: Error | null
  refetch: () => unknown
}

interface PanelProps<T> {
  title: string
  allTo: string
  source: Source<T[]>
  empty: string
  render: (item: T, index: number) => ReactNode
}

function Panel<T>({ title, allTo, source, empty, render }: PanelProps<T>) {
  let body: ReactNode
  if (source.isPending) {
    body = <BlockSkeleton label={`Đang tải ${title.toLowerCase()}`} className="h-32" />
  } else if (source.isError) {
    body = (
      <BlockError
        title={`Không tải được ${title.toLowerCase()}`}
        message={source.error?.message}
        onRetry={() => void source.refetch()}
      />
    )
  } else if (!source.data?.length) {
    body = <Muted>{empty}</Muted>
  } else {
    body = (
      <ul className="flex flex-col divide-y">
        {source.data.map((item, i) => render(item, i))}
      </ul>
    )
  }
  return (
    <Card size="sm" className="min-w-0">
      <CardHeader>
        <CardTitle className="flex items-center justify-between gap-2">
          {title}
          <Link
            to={allTo}
            className="text-sm font-normal text-muted-foreground hover:underline"
          >
            Xem tất cả
          </Link>
        </CardTitle>
      </CardHeader>
      <CardContent>{body}</CardContent>
    </Card>
  )
}

const itemClass =
  'block rounded-md py-2 outline-none hover:bg-muted/50 focus-visible:ring-3 focus-visible:ring-ring/50'

export function LowQuotaPanel({ source }: { source: Source<LowQuota[]> }) {
  return (
    <Panel
      title="Tài khoản quota thấp"
      allTo="/quota"
      source={source}
      empty="Không có tài khoản nào gần hết quota."
      render={(q, i) => (
        <li key={q.account.connectionId}>
          <Link to={`/quota#account-${q.account.connectionId}`} data-testid={`low-quota-item-${i}`} className={itemClass}>
            <span className="flex items-center justify-between gap-2">
              <span className="truncate font-medium">{q.account.label}</span>
              <Badge variant={q.usedPct >= 90 ? 'destructive' : 'secondary'}>
                <span className="tabular-nums">{Math.round(q.usedPct)}%</span>
              </Badge>
            </span>
            <span className="block truncate text-xs text-muted-foreground">
              {q.account.provider} · cửa sổ {q.window}
            </span>
          </Link>
        </li>
      )}
    />
  )
}

export function LatestErrorsPanel({ source }: { source: Source<UpstreamError[]> }) {
  return (
    <Panel
      title="Lỗi mới nhất"
      allTo="/errors"
      source={source}
      empty="Chưa ghi nhận lỗi upstream nào."
      render={(e) => (
        <li key={e.id}>
          <Link to={`/errors?error=${e.id}`} className={itemClass}>
            <span className="flex items-center justify-between gap-2">
              <span className="truncate font-medium">{e.message || e.signature}</span>
              <Badge variant="outline" className="tabular-nums">
                {e.status === 0 ? 'Mạng' : e.status}
              </Badge>
            </span>
            <span className="block truncate text-xs text-muted-foreground">
              {e.provider} · {e.model} · {formatTime(e.at)}
            </span>
          </Link>
        </li>
      )}
    />
  )
}

export function LatestDriftPanel({ source }: { source: Source<DriftChange[]> }) {
  return (
    <Panel
      title="Thay đổi drift mới nhất"
      allTo="/drift?unacked=1"
      source={source}
      empty="Không có thay đổi drift nào chưa xác nhận."
      render={(c) => (
        <li key={c.id}>
          <Link to={`/drift?unacked=1&change=${c.id}`} className={itemClass}>
            <span className="flex items-center justify-between gap-2">
              <span className="truncate font-mono text-sm font-medium">{c.path}</span>
              <Badge variant="secondary">{c.kind}</Badge>
            </span>
            <span className="block truncate text-xs text-muted-foreground">
              {c.provider} · {c.endpoint} · {formatTime(c.at)}
            </span>
          </Link>
        </li>
      )}
    />
  )
}
