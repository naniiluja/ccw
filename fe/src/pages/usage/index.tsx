import { ActivityIcon, TriangleAlertIcon } from 'lucide-react'
import { useCallback, useMemo, useState } from 'react'
import { useUsage, useUsageAccounts } from '@/api/usage'
import { PageHeader } from '@/components/app/page-header'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from '@/components/ui/empty'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Skeleton } from '@/components/ui/skeleton'
import { UsageChart } from './usage-chart'
import {
  type Filters,
  allValue,
  applyFilters,
  byDay,
  formatNumber,
  totalsOf,
} from './usage-model'
import { UsageTable } from './usage-table'

function Stat({ label, value }: { label: string; value: number }) {
  return (
    <Card size="sm">
      <CardHeader>
        <CardTitle className="text-xs font-normal text-muted-foreground">
          {label}
        </CardTitle>
      </CardHeader>
      <CardContent>
        <p className="text-2xl font-semibold tabular-nums">{formatNumber(value)}</p>
      </CardContent>
    </Card>
  )
}

function Loading() {
  return (
    <div role="status" aria-label="Đang tải lượng dùng" className="flex flex-col gap-4">
      <div className="grid gap-4 sm:grid-cols-3">
        {[0, 1, 2].map((i) => (
          <Skeleton key={i} className="h-24" />
        ))}
      </div>
      <Skeleton className="h-72" />
      <Skeleton className="h-48" />
    </div>
  )
}

export default function UsagePage() {
  const usage = useUsage()
  const accounts = useUsageAccounts()
  const [filters, setFilters] = useState<Filters>({
    model: allValue,
    account: allValue,
  })

  const nameOf = useCallback(
    (id: string) => accounts.data?.get(id) ?? id,
    [accounts.data],
  )
  const rows = usage.data
  const models = useMemo(
    () => [...new Set((rows ?? []).map((r) => r.model))].sort(),
    [rows],
  )
  const accountIds = useMemo(
    () => [...new Set((rows ?? []).map((r) => r.connectionId))],
    [rows],
  )
  const shown = useMemo(
    () => applyFilters(rows ?? [], filters),
    [rows, filters],
  )
  const days = useMemo(() => byDay(shown), [shown])
  const totals = useMemo(() => totalsOf(days), [days])

  const hasData = (rows?.length ?? 0) > 0
  const filterBar = hasData ? (
    <>
      <Select
        value={filters.model}
        onValueChange={(model) => setFilters((f) => ({ ...f, model }))}
      >
        <SelectTrigger aria-label="Model" className="w-44">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value={allValue}>Tất cả model</SelectItem>
          {models.map((m) => (
            <SelectItem key={m} value={m}>
              {m}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      <Select
        value={filters.account}
        onValueChange={(account) => setFilters((f) => ({ ...f, account }))}
      >
        <SelectTrigger aria-label="Tài khoản" className="w-44">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value={allValue}>Tất cả tài khoản</SelectItem>
          {accountIds.map((id) => (
            <SelectItem key={id} value={id}>
              {nameOf(id)}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </>
  ) : null

  let body
  if (usage.isPending) body = <Loading />
  else if (usage.isError) {
    body = (
      <Alert variant="destructive">
        <TriangleAlertIcon aria-hidden="true" />
        <AlertTitle>Không tải được lượng dùng</AlertTitle>
        <AlertDescription>
          <p>{usage.error.message}</p>
          <Button
            variant="outline"
            size="sm"
            className="mt-3"
            onClick={() => usage.refetch()}
          >
            Thử lại
          </Button>
        </AlertDescription>
      </Alert>
    )
  } else if (!hasData) {
    body = (
      <Card className="p-0">
        <Empty>
          <EmptyHeader>
            <EmptyMedia variant="icon">
              <ActivityIcon aria-hidden="true" />
            </EmptyMedia>
            <EmptyTitle>Chưa có dữ liệu lượng dùng</EmptyTitle>
            <EmptyDescription>
              Số liệu sẽ xuất hiện sau khi có request đi qua gateway.
            </EmptyDescription>
          </EmptyHeader>
        </Empty>
      </Card>
    )
  } else {
    body = (
      <>
        <div className="grid gap-4 sm:grid-cols-3">
          <Stat label="Token vào" value={totals.inputTokens} />
          <Stat label="Token ra" value={totals.outputTokens} />
          <Stat label="Số request" value={totals.requests} />
        </div>
        {shown.length === 0 ? (
          <Card className="p-0">
            <Empty>
              <EmptyHeader>
                <EmptyTitle>Không có dòng nào khớp bộ lọc</EmptyTitle>
                <EmptyDescription>
                  Chọn model hoặc tài khoản khác.
                </EmptyDescription>
              </EmptyHeader>
            </Empty>
          </Card>
        ) : (
          <>
            <Card size="sm">
              <CardHeader>
                <CardTitle>Token theo ngày</CardTitle>
              </CardHeader>
              <CardContent>
                <UsageChart days={days} totals={totals} />
              </CardContent>
            </Card>
            <UsageTable rows={shown} nameOf={nameOf} />
          </>
        )}
      </>
    )
  }

  return (
    <div className="flex flex-col gap-6">
      <PageHeader
        title="Lượng dùng"
        description="Số request và token theo thời gian, model và tài khoản."
        actions={<div className="flex flex-wrap gap-2">{filterBar}</div>}
      />
      {body}
    </div>
  )
}
