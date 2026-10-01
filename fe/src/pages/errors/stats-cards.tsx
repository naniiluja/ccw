import type { ErrorGroup } from '@/api/errors'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { classLabel } from './format'

interface Bar {
  key: string
  label: string
  value: number
}

function tally(groups: ErrorGroup[]) {
  const byClass = new Map<string, number>()
  const byProvider = new Map<string, number>()
  let total = 0
  for (const g of groups) {
    total += g.count
    byProvider.set(g.provider, (byProvider.get(g.provider) ?? 0) + g.count)
    for (const [c, n] of Object.entries(g.classes ?? {})) {
      byClass.set(c, (byClass.get(c) ?? 0) + n)
    }
  }
  const sorted = (m: Map<string, number>, label: (k: string) => string): Bar[] =>
    [...m]
      .map(([key, value]) => ({ key, label: label(key), value }))
      .sort((a, b) => b.value - a.value)
  return {
    total,
    classes: sorted(byClass, classLabel),
    providers: sorted(byProvider, (k) => k),
  }
}

// Bars are plain proportional rows: each one carries its number as text, so the
// chart reads without colour or a pointer.
function BarList({ label, bars }: { label: string; bars: Bar[] }) {
  const max = Math.max(1, ...bars.map((b) => b.value))
  return (
    <ul aria-label={label} className="flex flex-col gap-2">
      {bars.map((b) => (
        <li key={b.key} className="grid grid-cols-[minmax(0,8rem)_1fr_auto] items-center gap-2 text-sm">
          <span className="truncate" title={b.label}>
            {b.label}
          </span>
          <span className="h-2 rounded-full bg-muted" aria-hidden="true">
            <span
              className="block h-full rounded-full bg-primary"
              style={{ width: `${Math.max(4, (b.value / max) * 100)}%` }}
            />
          </span>
          <span className="tabular-nums text-muted-foreground">{b.value}</span>
        </li>
      ))}
    </ul>
  )
}

interface StatsCardsProps {
  groups: ErrorGroup[] | undefined
  loading: boolean
  failed: boolean
}

export function StatsCards({ groups, loading, failed }: StatsCardsProps) {
  if (loading) {
    return (
      <div className="grid gap-4 md:grid-cols-3" aria-label="Đang tải thống kê">
        <Skeleton className="h-36" />
        <Skeleton className="h-36" />
        <Skeleton className="h-36" />
      </div>
    )
  }
  if (failed || !groups) {
    return (
      <p className="text-sm text-muted-foreground">Không tải được thống kê.</p>
    )
  }
  const t = tally(groups)
  return (
    <div className="grid gap-4 md:grid-cols-3">
      <Card size="sm">
        <CardHeader>
          <CardTitle className="text-sm font-medium text-muted-foreground">
            Tổng số lỗi
          </CardTitle>
        </CardHeader>
        <CardContent>
          <p
            aria-label="Tổng số lỗi"
            className="text-3xl font-semibold tabular-nums"
          >
            {t.total}
          </p>
          <p className="mt-1 text-sm text-muted-foreground tabular-nums">
            {groups.length} nhóm chữ ký
          </p>
        </CardContent>
      </Card>
      <Card size="sm">
        <CardHeader>
          <CardTitle className="text-sm font-medium text-muted-foreground">
            Theo phân loại
          </CardTitle>
        </CardHeader>
        <CardContent>
          {t.classes.length ? (
            <BarList label="Lỗi theo phân loại" bars={t.classes} />
          ) : (
            <p className="text-sm text-muted-foreground">Chưa có dữ liệu.</p>
          )}
        </CardContent>
      </Card>
      <Card size="sm">
        <CardHeader>
          <CardTitle className="text-sm font-medium text-muted-foreground">
            Theo nhà cung cấp
          </CardTitle>
        </CardHeader>
        <CardContent>
          {t.providers.length ? (
            <BarList label="Lỗi theo nhà cung cấp" bars={t.providers} />
          ) : (
            <p className="text-sm text-muted-foreground">Chưa có dữ liệu.</p>
          )}
        </CardContent>
      </Card>
    </div>
  )
}
