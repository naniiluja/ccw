import { ChevronRightIcon } from 'lucide-react'
import type { UpstreamError } from '@/api/errors'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { useIsMobile } from '@/hooks/use-mobile'
import { classLabel, formatLatency, formatStatus, formatTime } from './format'

export function ClassBadge({ value }: { value: string }) {
  const hard = value === 'server' || value === 'network' || value === 'timeout'
  return (
    <Badge variant={hard ? 'destructive' : 'secondary'}>{classLabel(value)}</Badge>
  )
}

export function StatusBadge({ status }: { status: number }) {
  return (
    <Badge variant="outline" className="tabular-nums">
      {formatStatus(status)}
    </Badge>
  )
}

interface ErrorListProps {
  errors: UpstreamError[]
  onOpen: (id: number) => void
}

function OpenButton({ id, onOpen }: { id: number; onOpen: (id: number) => void }) {
  return (
    <Button
      variant="ghost"
      size="icon-sm"
      aria-label={`Xem chi tiết lỗi #${id}`}
      onClick={() => onOpen(id)}
    >
      <ChevronRightIcon aria-hidden="true" />
    </Button>
  )
}

export function ErrorList({ errors, onOpen }: ErrorListProps) {
  const mobile = useIsMobile()

  if (mobile) {
    return (
      <ul className="flex flex-col gap-3" aria-label="Danh sách lỗi">
        {errors.map((e) => (
          <li
            key={e.id}
            className="flex items-start gap-2 rounded-lg border bg-card p-3"
          >
            <div className="flex min-w-0 flex-1 flex-col gap-2">
              <div className="flex flex-wrap items-center gap-1.5">
                <StatusBadge status={e.status} />
                <ClassBadge value={e.class} />
                <span className="text-xs text-muted-foreground tabular-nums">
                  {formatTime(e.at)}
                </span>
              </div>
              <p className="text-sm break-words">{e.message}</p>
              <p className="truncate text-xs text-muted-foreground">
                {e.provider} · {e.model || 'không rõ model'} ·{' '}
                <span className="tabular-nums">{formatLatency(e.latencyMs)}</span>
              </p>
            </div>
            <OpenButton id={e.id} onOpen={onOpen} />
          </li>
        ))}
      </ul>
    )
  }

  return (
    <div className="rounded-lg border">
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>Thời gian</TableHead>
            <TableHead>Nhà cung cấp</TableHead>
            <TableHead>Model</TableHead>
            <TableHead>Trạng thái</TableHead>
            <TableHead>Phân loại</TableHead>
            <TableHead className="text-right">Độ trễ</TableHead>
            <TableHead>Thông báo</TableHead>
            <TableHead className="w-10">
              <span className="sr-only">Chi tiết</span>
            </TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {errors.map((e) => (
            <TableRow key={e.id}>
              <TableCell className="whitespace-nowrap tabular-nums">
                {formatTime(e.at)}
              </TableCell>
              <TableCell>{e.provider}</TableCell>
              <TableCell className="max-w-48 truncate" title={e.model}>
                {e.model}
              </TableCell>
              <TableCell>
                <StatusBadge status={e.status} />
              </TableCell>
              <TableCell>
                <ClassBadge value={e.class} />
              </TableCell>
              <TableCell className="text-right tabular-nums">
                {formatLatency(e.latencyMs)}
              </TableCell>
              <TableCell className="max-w-80 truncate" title={e.message}>
                {e.message}
              </TableCell>
              <TableCell>
                <OpenButton id={e.id} onOpen={onOpen} />
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </div>
  )
}
