import { CheckCheckIcon, CheckIcon, EyeIcon, TriangleAlertIcon } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useSearchParams } from 'react-router'
import { toast } from 'sonner'
import {
  useAckChanges,
  useDriftChanges,
  type DriftChange,
} from '@/api/drift'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from '@/components/ui/empty'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { ScrollArea, ScrollBar } from '@/components/ui/scroll-area'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Skeleton } from '@/components/ui/skeleton'
import { Switch } from '@/components/ui/switch'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { ChangeSheet } from './change-sheet'
import type { Filters } from './filters'
import {
  causeLabel,
  defaultLimit,
  directionLabel,
  formatTime,
  kindLabel,
  limitOptions,
  percent,
} from './strings'

const ALL = 'all'

function Verdict({ change }: { change: DriftChange }) {
  if (!change.verdict) {
    return <span className="text-muted-foreground">Chưa xét</span>
  }
  return (
    <div className="flex flex-wrap items-center gap-1.5">
      <Badge
        variant="secondary"
        title={causeLabel[change.verdict] ?? change.verdict}
        className="font-mono"
      >
        {change.verdict}
        {change.verdictConf !== undefined
          ? ` · ${percent(change.verdictConf)}`
          : ''}
      </Badge>
      {change.autoAcked ? <Badge variant="outline">Tự xác nhận</Badge> : null}
    </div>
  )
}

function ListSkeleton() {
  return (
    <div role="status" aria-label="Đang tải thay đổi" className="flex flex-col gap-2 p-4">
      {Array.from({ length: 5 }, (_, i) => (
        <Skeleton key={i} className="h-10 w-full" />
      ))}
    </div>
  )
}

// ChangesTab lists recorded shape changes, filtered through the URL.
export function ChangesTab({ filters }: { filters: Filters }) {
  const { filter, update } = filters
  const query = useDriftChanges(filter)
  const ack = useAckChanges()
  const [selected, setSelected] = useState<Set<number>>(new Set())
  const [detailId, setDetailId] = useState<number | null>(
    Number(useSearchParams()[0].get('change')) || null,
  )
  const [providerText, setProviderText] = useState(filter.provider)

  // The provider box is typed into freely; the URL follows after a pause.
  useEffect(() => {
    if (providerText === filter.provider) return
    const t = setTimeout(() => update({ provider: providerText.trim() || null }), 300)
    return () => clearTimeout(t)
  }, [providerText, filter.provider, update])

  const changes = query.data?.changes ?? []
  // A link such as /drift?change=12 opens that change's sheet once it is listed.
  const detail = changes.find((c) => c.id === detailId) ?? null
  const closeDetail = () => {
    setDetailId(null)
    update({ change: null })
  }
  const pending = changes.filter((c) => !c.acked)
  const picked = pending.filter((c) => selected.has(c.id)).map((c) => c.id)

  const doAck = (ids: number[]) =>
    ack.mutate(ids, {
      onSuccess: () => {
        setSelected(new Set())
        setDetailId(null)
      },
      onError: (e) =>
        toast.error('Không xác nhận được thay đổi', { description: e.message }),
    })

  const toggle = (id: number, on: boolean) =>
    setSelected((prev) => {
      const next = new Set(prev)
      if (on) next.add(id)
      else next.delete(id)
      return next
    })

  const allPicked = pending.length > 0 && picked.length === pending.length

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-end gap-3">
        <div className="flex w-full flex-col gap-1.5 sm:w-52">
          <Label htmlFor="drift-provider">Nhà cung cấp</Label>
          <Input
            id="drift-provider"
            value={providerText}
            placeholder="vd. groq"
            onChange={(e) => setProviderText(e.target.value)}
          />
        </div>
        <div className="flex flex-col gap-1.5">
          <Label id="drift-direction-label">Hướng</Label>
          <Select
            value={filter.direction || ALL}
            onValueChange={(v) => update({ direction: v === ALL ? null : v })}
          >
            <SelectTrigger aria-label="Hướng" className="w-40">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value={ALL}>Tất cả hướng</SelectItem>
              <SelectItem value="request">Yêu cầu</SelectItem>
              <SelectItem value="response">Phản hồi</SelectItem>
            </SelectContent>
          </Select>
        </div>
        <div className="flex flex-col gap-1.5">
          <Label>Giới hạn</Label>
          <Select
            value={String(filter.limit)}
            onValueChange={(v) =>
              update({ limit: Number(v) === defaultLimit ? null : v })
            }
          >
            <SelectTrigger aria-label="Giới hạn" className="w-28">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {limitOptions.map((n) => (
                <SelectItem key={n} value={String(n)}>
                  {n}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        <div className="flex h-9 items-center gap-2">
          <Switch
            id="drift-unacked"
            checked={filter.unacked}
            onCheckedChange={(on) => update({ unacked: on ? '1' : null })}
          />
          <Label htmlFor="drift-unacked">Chỉ chưa xác nhận</Label>
        </div>
        {picked.length > 0 ? (
          <Button
            className="sm:ml-auto"
            disabled={ack.isPending}
            onClick={() => doAck(picked)}
          >
            <CheckCheckIcon aria-hidden="true" />
            Xác nhận {picked.length} đã chọn
          </Button>
        ) : null}
      </div>

      <Card className="gap-0 overflow-hidden p-0">
        {query.isPending ? (
          <ListSkeleton />
        ) : query.isError ? (
          <div className="p-4">
            <Alert variant="destructive">
              <TriangleAlertIcon aria-hidden="true" />
              <AlertTitle>Không tải được danh sách thay đổi</AlertTitle>
              <AlertDescription>
                <p>{query.error.message}</p>
                <Button
                  variant="outline"
                  size="sm"
                  className="mt-3"
                  onClick={() => query.refetch()}
                >
                  Thử lại
                </Button>
              </AlertDescription>
            </Alert>
          </div>
        ) : changes.length === 0 ? (
          <Empty>
            <EmptyHeader>
              <EmptyMedia variant="icon">
                <CheckIcon aria-hidden="true" />
              </EmptyMedia>
              <EmptyTitle>Chưa có thay đổi nào</EmptyTitle>
              <EmptyDescription>
                {filter.provider || filter.direction || filter.unacked
                  ? 'Không có thay đổi khớp bộ lọc hiện tại.'
                  : 'Chưa nhà cung cấp nào đổi cấu trúc dữ liệu.'}
              </EmptyDescription>
            </EmptyHeader>
          </Empty>
        ) : (
          <ScrollArea className="w-full">
            <Table className="min-w-[860px]">
              <TableHeader>
                <TableRow>
                  <TableHead className="w-10">
                    <Checkbox
                      aria-label="Chọn tất cả thay đổi chưa xác nhận"
                      checked={allPicked}
                      disabled={pending.length === 0}
                      onCheckedChange={(on) =>
                        setSelected(on ? new Set(pending.map((c) => c.id)) : new Set())
                      }
                    />
                  </TableHead>
                  <TableHead>Trường</TableHead>
                  <TableHead>Nhà cung cấp</TableHead>
                  <TableHead>Kiểu cũ</TableHead>
                  <TableHead>Kiểu mới</TableHead>
                  <TableHead>Thời điểm</TableHead>
                  <TableHead>Phán quyết AI</TableHead>
                  <TableHead className="text-right">
                    <span className="sr-only">Thao tác</span>
                  </TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {changes.map((c) => (
                  <TableRow key={c.id} data-acked={c.acked || undefined}>
                    <TableCell>
                      {c.acked ? null : (
                        <Checkbox
                          aria-label={`Chọn ${c.path}`}
                          checked={selected.has(c.id)}
                          onCheckedChange={(on) => toggle(c.id, on === true)}
                        />
                      )}
                    </TableCell>
                    <TableCell className="max-w-64">
                      <div className="font-mono text-xs break-all">{c.path}</div>
                      <div className="mt-0.5 flex flex-wrap items-center gap-1.5 text-xs text-muted-foreground">
                        <span>{kindLabel[c.kind] ?? c.kind}</span>
                        <span>· {c.endpoint}</span>
                      </div>
                    </TableCell>
                    <TableCell>
                      <div>{c.provider}</div>
                      <div className="text-xs text-muted-foreground">
                        {directionLabel[c.direction]}
                      </div>
                    </TableCell>
                    <TableCell className="font-mono text-xs">
                      {c.oldType || '—'}
                    </TableCell>
                    <TableCell className="font-mono text-xs">
                      {c.newType || '—'}
                    </TableCell>
                    <TableCell className="text-xs whitespace-nowrap tabular-nums">
                      {formatTime(c.at)}
                    </TableCell>
                    <TableCell>
                      <Verdict change={c} />
                    </TableCell>
                    <TableCell>
                      <div className="flex justify-end gap-1">
                        <Button
                          variant="ghost"
                          size="icon-sm"
                          aria-label={`Xem chi tiết ${c.path}`}
                          onClick={() => setDetailId(c.id)}
                        >
                          <EyeIcon aria-hidden="true" />
                        </Button>
                        {c.acked ? (
                          <Badge variant="outline">Đã xác nhận</Badge>
                        ) : (
                          <Button
                            variant="outline"
                            size="sm"
                            aria-label={`Xác nhận ${c.path}`}
                            disabled={ack.isPending}
                            onClick={() => doAck([c.id])}
                          >
                            Xác nhận
                          </Button>
                        )}
                      </div>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
            <ScrollBar orientation="horizontal" />
          </ScrollArea>
        )}
      </Card>

      <ChangeSheet
        change={detail}
        onClose={closeDetail}
        onAck={(id) => doAck([id])}
      />
    </div>
  )
}
