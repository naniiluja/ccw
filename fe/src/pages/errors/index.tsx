import {
  CheckCircle2Icon,
  PauseIcon,
  PlayIcon,
  RefreshCwIcon,
  TriangleAlertIcon,
} from 'lucide-react'
import { useCallback, useMemo, useState } from 'react'
import { useSearchParams } from 'react-router'
import { useErrors, useErrorStats, type ErrorGroup } from '@/api/errors'
import { PageHeader } from '@/components/app/page-header'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from '@/components/ui/empty'
import { Skeleton } from '@/components/ui/skeleton'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { ErrorDetail } from './error-detail'
import { ErrorList } from './error-list'
import { FilterBar, type FilterPatch } from './filter-bar'
import { defaultLimit, readFilter } from './format'
import { GroupsTable } from './groups-table'
import { ReviewPanel } from './review-panel'
import { StatsCards } from './stats-cards'

const tabs = ['list', 'groups', 'review'] as const
type Tab = (typeof tabs)[number]

const limitStep = 50

function RetryAlert({
  title,
  message,
  onRetry,
}: {
  title: string
  message: string
  onRetry: () => void
}) {
  return (
    <Alert variant="destructive">
      <TriangleAlertIcon aria-hidden="true" />
      <AlertTitle>{title}</AlertTitle>
      <AlertDescription>
        <p>{message}</p>
        <Button variant="outline" size="sm" className="mt-3" onClick={onRetry}>
          Thử lại
        </Button>
      </AlertDescription>
    </Alert>
  )
}

function ListSkeleton() {
  return (
    <div role="status" aria-label="Đang tải danh sách" className="flex flex-col gap-2">
      {Array.from({ length: 6 }, (_, i) => (
        <Skeleton key={i} className="h-12" />
      ))}
    </div>
  )
}

function NoErrors({ filtered }: { filtered: boolean }) {
  return (
    <Empty className="border">
      <EmptyHeader>
        <EmptyMedia variant="icon">
          <CheckCircle2Icon aria-hidden="true" />
        </EmptyMedia>
        <EmptyTitle>
          {filtered ? 'Không có lỗi khớp bộ lọc' : 'Chưa có lỗi'}
        </EmptyTitle>
        <EmptyDescription>
          {filtered
            ? 'Thử nới bộ lọc hoặc chọn khoảng thời gian dài hơn.'
            : 'Chưa có lỗi nào từ nhà cung cấp được ghi lại.'}
        </EmptyDescription>
      </EmptyHeader>
    </Empty>
  )
}

export default function ErrorsPage() {
  const [params, setParams] = useSearchParams()
  const [paused, setPaused] = useState(false)
  const [providers, setProviders] = useState<string[]>([])

  const filter = useMemo(() => readFilter(params), [params])
  const rawTab = params.get('tab')
  const tab: Tab = tabs.find((t) => t === rawTab) ?? 'list'
  const openId = Number(params.get('error')) || null

  const poll = !paused
  const list = useErrors(filter, { poll })
  const stats = useErrorStats(
    { provider: filter.provider, since: filter.since },
    { poll },
  )

  // The provider filter lists every provider seen so far, so narrowing to one
  // does not remove the others from the choices.
  const seen = (stats.data ?? []).map((g) => g.provider)
  const merged = [...new Set([...providers, ...seen])]
  if (merged.length !== providers.length) setProviders(merged)

  const update = useCallback(
    (patch: Record<string, string>) => {
      setParams(
        (prev) => {
          const next = new URLSearchParams(prev)
          for (const [k, v] of Object.entries(patch)) {
            if (v) next.set(k, v)
            else next.delete(k)
          }
          return next
        },
        { replace: true },
      )
    },
    [setParams],
  )

  const onFilter = (patch: FilterPatch) => update(patch)
  const onReset = () =>
    update({ provider: '', class: '', signature: '', status: '', since: '' })
  const filtered = Boolean(
    filter.provider ||
      filter.class ||
      filter.signature ||
      filter.status ||
      filter.since,
  )
  const open = (id: number) => update({ error: String(id) })
  const pick = (g: ErrorGroup) =>
    update({ signature: g.signature, provider: g.provider, tab: '' })

  const refreshNow = () => {
    void list.refetch()
    void stats.refetch()
  }
  const updatedAt = Math.max(list.dataUpdatedAt, stats.dataUpdatedAt)
  const refreshing = list.isFetching || stats.isFetching

  const canLoadMore = (list.data?.length ?? 0) >= (filter.limit ?? defaultLimit)

  return (
    <div className="mx-auto flex max-w-7xl flex-col gap-6">
      <PageHeader
        title="Lỗi upstream"
        description="Lỗi trả về từ nhà cung cấp, được nhóm theo nội dung."
        actions={
          <>
            <Button
              variant="outline"
              size="sm"
              aria-pressed={paused}
              onClick={() => setPaused((p) => !p)}
            >
              {paused ? (
                <PlayIcon aria-hidden="true" data-icon="inline-start" />
              ) : (
                <PauseIcon aria-hidden="true" data-icon="inline-start" />
              )}
              {paused ? 'Tiếp tục làm mới' : 'Tạm dừng làm mới'}
            </Button>
            <Button
              variant="outline"
              size="sm"
              onClick={refreshNow}
              disabled={refreshing}
              aria-label="Làm mới ngay"
            >
              <RefreshCwIcon
                aria-hidden="true"
                data-icon="inline-start"
                className={refreshing ? 'animate-spin' : undefined}
              />
              <span className="hidden sm:inline">Làm mới</span>
            </Button>
          </>
        }
      />
      <p className="-mt-3 text-xs text-muted-foreground" aria-live="polite">
        {paused
          ? 'Tự động làm mới đang tạm dừng.'
          : 'Tự động làm mới mỗi 30 giây khi tab đang mở.'}
        {updatedAt ? (
          <>
            {' '}
            Cập nhật lúc{' '}
            <span className="tabular-nums">
              {new Date(updatedAt).toLocaleTimeString('vi-VN')}
            </span>
            .
          </>
        ) : null}
      </p>

      <FilterBar
        key={`${filter.status ?? ''}|${filter.signature ?? ''}`}
        filter={filter}
        providers={providers}
        onChange={onFilter}
        onReset={onReset}
      />

      <StatsCards
        groups={stats.data}
        loading={stats.isPending}
        failed={stats.isError}
      />

      <Tabs
        value={tab}
        onValueChange={(v) => update({ tab: v === 'list' ? '' : v })}
      >
        <TabsList className="w-full justify-start overflow-x-auto sm:w-fit sm:overflow-visible">
          <TabsTrigger value="list">Danh sách lỗi</TabsTrigger>
          <TabsTrigger value="groups">Nhóm theo chữ ký</TabsTrigger>
          <TabsTrigger value="review">AI review</TabsTrigger>
        </TabsList>

        <TabsContent value="list" className="flex flex-col gap-4">
          {list.isPending ? (
            <ListSkeleton />
          ) : list.isError && !list.data ? (
            <RetryAlert
              title="Không tải được danh sách lỗi"
              message={list.error.message}
              onRetry={() => void list.refetch()}
            />
          ) : list.data && list.data.length === 0 ? (
            <NoErrors filtered={filtered} />
          ) : list.data ? (
            <>
              <ErrorList errors={list.data} onOpen={open} />
              <div className="flex flex-wrap items-center justify-between gap-2 text-sm text-muted-foreground">
                <span className="tabular-nums">
                  Hiển thị {list.data.length} lỗi mới nhất
                </span>
                {canLoadMore ? (
                  <Button
                    variant="outline"
                    size="sm"
                    disabled={list.isFetching}
                    onClick={() =>
                      update({
                        limit: String((filter.limit ?? defaultLimit) + limitStep),
                      })
                    }
                  >
                    Tải thêm
                  </Button>
                ) : null}
              </div>
            </>
          ) : null}
        </TabsContent>

        <TabsContent value="groups" className="flex flex-col gap-4">
          {stats.isPending ? (
            <ListSkeleton />
          ) : stats.isError && !stats.data ? (
            <RetryAlert
              title="Không tải được thống kê"
              message={stats.error.message}
              onRetry={() => void stats.refetch()}
            />
          ) : stats.data && stats.data.length === 0 ? (
            <NoErrors filtered={filtered} />
          ) : stats.data ? (
            <GroupsTable groups={stats.data} onPick={pick} onOpen={open} />
          ) : null}
        </TabsContent>

        <TabsContent value="review">
          <ReviewPanel />
        </TabsContent>
      </Tabs>

      <ErrorDetail id={openId} onClose={() => update({ error: '' })} />
    </div>
  )
}
