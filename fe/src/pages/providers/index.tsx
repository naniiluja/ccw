import { PlugZapIcon } from 'lucide-react'
import { useSearchParams } from 'react-router'
import { useAccountCounts, useProviders } from '@/api/providers'
import { PageHeader } from '@/components/app/page-header'
import { Empty, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from '@/components/ui/empty'
import { Skeleton } from '@/components/ui/skeleton'
import { InlineError } from './inline-error'
import { ProviderDetail } from './provider-detail'
import { ProviderList } from './provider-list'

export default function ProvidersPage() {
  const providers = useProviders()
  const counts = useAccountCounts()
  const [params, setParams] = useSearchParams()
  const selected = params.get('p')

  const select = (id: string) =>
    setParams((prev) => {
      const next = new URLSearchParams(prev)
      next.set('p', id)
      return next
    })

  const current = providers.data?.find((p) => p.id === selected)

  return (
    <div className="mx-auto flex w-full max-w-7xl flex-col gap-6">
      <PageHeader
        title="Nhà cung cấp"
        description="Nhà cung cấp, model và chính sách xoay vòng."
      />
      {providers.isPending ? (
        <div
          role="status"
          aria-label="Đang tải danh sách nhà cung cấp"
          className="grid grid-cols-1 gap-4 lg:grid-cols-[18rem_minmax(0,1fr)]"
        >
          <div className="flex flex-col gap-2">
            <Skeleton className="h-14" />
            <Skeleton className="h-14" />
            <Skeleton className="h-14" />
          </div>
          <Skeleton className="h-72" />
        </div>
      ) : providers.isError ? (
        <InlineError
          title="Không tải được danh sách nhà cung cấp"
          error={providers.error}
          onRetry={() => void providers.refetch()}
        />
      ) : providers.data.length === 0 ? (
        <Empty className="border">
          <EmptyHeader>
            <EmptyMedia variant="icon">
              <PlugZapIcon />
            </EmptyMedia>
            <EmptyTitle>Chưa có nhà cung cấp nào</EmptyTitle>
            <EmptyDescription>
              Máy chủ chưa khai báo nhà cung cấp nào.
            </EmptyDescription>
          </EmptyHeader>
        </Empty>
      ) : (
        <div className="grid grid-cols-1 items-start gap-4 lg:grid-cols-[18rem_minmax(0,1fr)]">
          <ProviderList
            providers={providers.data}
            counts={counts.data}
            selected={selected}
            onSelect={select}
          />
          {current ? (
            <ProviderDetail key={current.id} provider={current} />
          ) : (
            <Empty className="border">
              <EmptyHeader>
                <EmptyMedia variant="icon">
                  <PlugZapIcon />
                </EmptyMedia>
                <EmptyTitle>Chọn một nhà cung cấp</EmptyTitle>
                <EmptyDescription>
                  Chọn nhà cung cấp ở danh sách để xem model, xoay vòng và
                  định nghĩa.
                </EmptyDescription>
              </EmptyHeader>
            </Empty>
          )}
        </div>
      )}
    </div>
  )
}
