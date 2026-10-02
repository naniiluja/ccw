import { useSearchParams } from 'react-router'
import { useDriftChanges } from '@/api/drift'
import { PageHeader } from '@/components/app/page-header'
import { Badge } from '@/components/ui/badge'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { ChangesTab } from './changes-tab'
import { ConfigTab } from './config-tab'
import { FieldsTab } from './fields-tab'
import { useFilters } from './filters'

const tabs = ['changes', 'fields', 'config'] as const
type Tab = (typeof tabs)[number]

export default function DriftPage() {
  const [params, setParams] = useSearchParams()
  const requested = params.get('tab')
  const tab: Tab = tabs.find((t) => t === requested) ?? 'changes'
  const filters = useFilters()
  // The unacked count is global; it comes with every list answer.
  const { data } = useDriftChanges(filters.filter)
  const unacked = data?.unacked

  return (
    <div className="flex flex-col gap-6">
      <PageHeader
        title="Thay đổi shape"
        description="Những lần nhà cung cấp đổi cấu trúc yêu cầu hoặc phản hồi."
        actions={
          unacked === undefined ? null : (
            <Badge variant={unacked > 0 ? 'destructive' : 'secondary'} className="h-7 px-3 text-sm">
              Chưa xác nhận:{' '}
              <span data-testid="unacked-count" className="font-semibold tabular-nums">
                {unacked}
              </span>
            </Badge>
          )
        }
      />
      <Tabs
        value={tab}
        onValueChange={(v) =>
          setParams(
            (prev) => {
              const next = new URLSearchParams(prev)
              if (v === 'changes') next.delete('tab')
              else next.set('tab', v)
              return next
            },
            { replace: true },
          )
        }
      >
        <TabsList className="max-w-full overflow-x-auto sm:overflow-visible">
          <TabsTrigger value="changes">Thay đổi</TabsTrigger>
          <TabsTrigger value="fields">Trường theo dõi</TabsTrigger>
          <TabsTrigger value="config">Cấu hình</TabsTrigger>
        </TabsList>
        <TabsContent value="changes" className="mt-4">
          <ChangesTab filters={filters} />
        </TabsContent>
        <TabsContent value="fields" className="mt-4">
          <FieldsTab />
        </TabsContent>
        <TabsContent value="config" className="mt-4">
          <ConfigTab />
        </TabsContent>
      </Tabs>
    </div>
  )
}
