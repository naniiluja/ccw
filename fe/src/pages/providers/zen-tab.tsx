import { HistoryIcon } from 'lucide-react'
import { useZenSessions } from '@/api/providers'
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from '@/components/ui/empty'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { formatDuration, formatTime } from './format'
import { InlineError } from './inline-error'

export function ZenTab() {
  const query = useZenSessions()
  if (query.isPending) {
    return (
      <div role="status" aria-label="Đang tải phiên Zen" className="flex flex-col gap-2">
        <Skeleton className="h-10" />
        <Skeleton className="h-10" />
      </div>
    )
  }
  if (query.isError) {
    return (
      <InlineError
        title="Không tải được phiên Zen"
        error={query.error}
        onRetry={() => void query.refetch()}
      />
    )
  }
  const { sessions, ttlSeconds, maxSessions } = query.data
  if (sessions.length === 0) {
    return (
      <Empty className="border">
        <EmptyHeader>
          <EmptyMedia variant="icon">
            <HistoryIcon />
          </EmptyMedia>
          <EmptyTitle>Chưa có phiên nào</EmptyTitle>
          <EmptyDescription>
            Phiên xuất hiện khi một hội thoại của người gọi được gắn với phiên
            upstream của Zen.
          </EmptyDescription>
        </EmptyHeader>
      </Empty>
    )
  }
  return (
    <div className="flex flex-col gap-3">
      <p className="text-sm text-muted-foreground tabular-nums">
        {sessions.length} / {maxSessions} phiên, giữ tối đa{' '}
        {formatDuration(ttlSeconds)} sau lần dùng cuối.
      </p>
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>Người gọi</TableHead>
            <TableHead className="hidden sm:table-cell">Phiên upstream</TableHead>
            <TableHead className="hidden md:table-cell">Nguồn</TableHead>
            <TableHead className="text-right">Lượt dùng</TableHead>
            <TableHead className="hidden md:table-cell">Dùng lần cuối</TableHead>
            <TableHead className="text-right">Hết hạn sau</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {sessions.map((s) => (
            <TableRow key={s.id}>
              <TableCell className="font-mono text-xs break-all">{s.caller}</TableCell>
              <TableCell className="hidden font-mono text-xs break-all sm:table-cell">
                {s.id}
              </TableCell>
              <TableCell className="hidden md:table-cell">{s.source}</TableCell>
              <TableCell className="text-right tabular-nums">{s.uses}</TableCell>
              <TableCell className="hidden tabular-nums md:table-cell">
                {formatTime(s.lastUsed)}
              </TableCell>
              <TableCell className="text-right tabular-nums">
                {formatDuration(s.expiresInSeconds)}
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </div>
  )
}
