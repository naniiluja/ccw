import {
  GaugeIcon,
  LayoutGridIcon,
  RefreshCwIcon,
  TableIcon,
  TriangleAlertIcon,
} from 'lucide-react'
import { useState } from 'react'
import { toast } from 'sonner'
import {
  type QuotaView,
  useQuota,
  useQuotaView,
  useRefreshQuota,
} from '@/api/quota'
import { PageHeader } from '@/components/app/page-header'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group'
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from '@/components/ui/empty'
import { Skeleton } from '@/components/ui/skeleton'
import { Spinner } from '@/components/ui/spinner'
import { cn } from '@/lib/utils'
import type { ClaimNotice } from './notices'
import { clockText } from './format'
import { QuotaCards, QuotaTable } from './quota-views'

const views: { value: QuotaView; label: string; icon: typeof TableIcon }[] = [
  { value: 'cards', label: 'Thẻ', icon: LayoutGridIcon },
  { value: 'table', label: 'Bảng', icon: TableIcon },
]

function ViewSwitch({
  value,
  onChange,
}: {
  value: QuotaView
  onChange: (v: QuotaView) => void
}) {
  return (
    <ToggleGroup
      type="single"
      variant="outline"
      size="sm"
      aria-label="Chế độ xem"
      value={value}
      // Radix reports an empty string when the pressed item is pressed again.
      onValueChange={(v) => v && onChange(v as QuotaView)}
    >
      {views.map(({ value: v, label, icon: Icon }) => (
        <ToggleGroupItem key={v} value={v}>
          <Icon aria-hidden="true" />
          {label}
        </ToggleGroupItem>
      ))}
    </ToggleGroup>
  )
}

function Loading() {
  return (
    <div
      role="status"
      aria-label="Đang tải hạn mức"
      className="grid gap-4 md:grid-cols-2 xl:grid-cols-3"
    >
      {[0, 1, 2].map((i) => (
        <Skeleton key={i} className="h-52" />
      ))}
    </div>
  )
}

export default function QuotaPage() {
  const quota = useQuota()
  const refresh = useRefreshQuota()
  const { view, setView } = useQuotaView()
  const [notice, setNotice] = useState<ClaimNotice | null>(null)

  const onRefresh = () =>
    refresh.mutate(undefined, {
      onError: () => toast.error('Không làm mới được hạn mức'),
    })

  const accounts = quota.data ?? []
  let body
  if (quota.isPending) body = <Loading />
  else if (quota.isError && !quota.data) {
    body = (
      <Alert variant="destructive">
        <TriangleAlertIcon aria-hidden="true" />
        <AlertTitle>Không tải được hạn mức</AlertTitle>
        <AlertDescription>
          <p>{quota.error.message}</p>
          <Button
            variant="outline"
            size="sm"
            className="mt-3"
            onClick={() => quota.refetch()}
          >
            Thử lại
          </Button>
        </AlertDescription>
      </Alert>
    )
  } else if (accounts.length === 0) {
    body = (
      <Card className="p-0">
        <Empty>
          <EmptyHeader>
            <EmptyMedia variant="icon">
              <GaugeIcon aria-hidden="true" />
            </EmptyMedia>
            <EmptyTitle>Chưa có tài khoản nào</EmptyTitle>
            <EmptyDescription>
              Thêm tài khoản đang hoạt động để theo dõi hạn mức tại đây.
            </EmptyDescription>
          </EmptyHeader>
        </Empty>
      </Card>
    )
  } else if (view === 'table') {
    body = <QuotaTable accounts={accounts} onNotice={setNotice} />
  } else {
    body = <QuotaCards accounts={accounts} onNotice={setNotice} />
  }

  return (
    <div className="flex flex-col gap-6">
      <PageHeader
        title="Hạn mức"
        description="Hạn mức còn lại theo từng tài khoản và lần reset kế tiếp."
        actions={
          <>
            <ViewSwitch value={view} onChange={setView} />
            <Button
              variant="outline"
              size="sm"
              onClick={onRefresh}
              disabled={refresh.isPending}
            >
              {refresh.isPending ? (
                <Spinner data-icon="inline-start" />
              ) : (
                <RefreshCwIcon data-icon="inline-start" aria-hidden="true" />
              )}
              Làm mới
            </Button>
          </>
        }
      />
      {notice ? (
        <Alert
          role="status"
          variant={notice.ok ? 'default' : 'destructive'}
          className={cn(notice.ok && 'border-emerald-600/40')}
        >
          <AlertTitle>{notice.title}</AlertTitle>
          {notice.detail ? (
            <AlertDescription>{notice.detail}</AlertDescription>
          ) : null}
        </Alert>
      ) : null}
      {quota.isError && quota.data ? (
        <Alert variant="destructive">
          <TriangleAlertIcon aria-hidden="true" />
          <AlertTitle>Không cập nhật được, đang hiện số liệu cũ</AlertTitle>
          <AlertDescription>{quota.error.message}</AlertDescription>
        </Alert>
      ) : null}
      {body}
      {quota.data ? (
        <p className="text-xs text-muted-foreground tabular-nums">
          Cập nhật lúc {clockText(quota.dataUpdatedAt)} · tự làm mới mỗi 60 giây
        </p>
      ) : null}
    </div>
  )
}
