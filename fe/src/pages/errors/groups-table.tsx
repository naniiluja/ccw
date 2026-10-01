import { SearchIcon } from 'lucide-react'
import type { ErrorGroup } from '@/api/errors'
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
import { StatusBadge } from './error-list'
import { classLabel, formatLatency, formatTime } from './format'

interface GroupsTableProps {
  groups: ErrorGroup[]
  /** Narrows the list to one signature (and its provider). */
  onPick: (g: ErrorGroup) => void
  onOpen: (id: number) => void
}

function ClassCounts({ classes }: { classes: Record<string, number> }) {
  return (
    <div className="flex flex-wrap gap-1">
      {Object.entries(classes ?? {}).map(([c, n]) => (
        <Badge key={c} variant="secondary" className="tabular-nums">
          {classLabel(c)} {n}
        </Badge>
      ))}
    </div>
  )
}

function Actions({
  g,
  onPick,
  onOpen,
}: Omit<GroupsTableProps, 'groups'> & { g: ErrorGroup }) {
  return (
    <div className="flex flex-wrap gap-1">
      <Button
        variant="outline"
        size="sm"
        aria-label={`Lọc theo chữ ký ${g.signature}`}
        onClick={() => onPick(g)}
      >
        <SearchIcon aria-hidden="true" data-icon="inline-start" />
        Lọc
      </Button>
      <Button
        variant="ghost"
        size="sm"
        aria-label={`Xem lỗi gần nhất của nhóm ${g.signature}`}
        onClick={() => onOpen(g.lastId)}
      >
        Lỗi gần nhất
      </Button>
    </div>
  )
}

export function GroupsTable({ groups, onPick, onOpen }: GroupsTableProps) {
  const mobile = useIsMobile()

  if (mobile) {
    return (
      <ul className="flex flex-col gap-3" aria-label="Nhóm lỗi theo chữ ký">
        {groups.map((g) => (
          <li
            key={`${g.provider}/${g.signature}`}
            className="flex flex-col gap-2 rounded-lg border bg-card p-3"
          >
            <div className="flex flex-wrap items-center gap-1.5">
              <span className="text-lg font-semibold tabular-nums">{g.count}</span>
              <span className="text-sm text-muted-foreground">lỗi</span>
              <StatusBadge status={g.status} />
              <Badge variant="outline">{g.provider}</Badge>
            </div>
            <p className="text-sm break-words">{g.message || g.signature}</p>
            <ClassCounts classes={g.classes} />
            <p className="text-xs text-muted-foreground tabular-nums">
              Gần nhất {formatTime(g.last)} · trung vị {formatLatency(g.medianMs)}
            </p>
            <Actions g={g} onPick={onPick} onOpen={onOpen} />
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
            <TableHead className="text-right">Số lỗi</TableHead>
            <TableHead>Nhà cung cấp</TableHead>
            <TableHead>Trạng thái</TableHead>
            <TableHead>Phân loại</TableHead>
            <TableHead>Thông báo</TableHead>
            <TableHead className="text-right">Trung vị</TableHead>
            <TableHead>Gần nhất</TableHead>
            <TableHead>
              <span className="sr-only">Thao tác</span>
            </TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {groups.map((g) => (
            <TableRow key={`${g.provider}/${g.signature}`}>
              <TableCell className="text-right font-medium tabular-nums">
                {g.count}
              </TableCell>
              <TableCell>{g.provider}</TableCell>
              <TableCell>
                <StatusBadge status={g.status} />
              </TableCell>
              <TableCell>
                <ClassCounts classes={g.classes} />
              </TableCell>
              <TableCell className="max-w-80 truncate" title={g.signature}>
                {g.message || g.signature}
              </TableCell>
              <TableCell className="text-right tabular-nums">
                {formatLatency(g.medianMs)}
              </TableCell>
              <TableCell className="whitespace-nowrap tabular-nums">
                {formatTime(g.last)}
              </TableCell>
              <TableCell>
                <Actions g={g} onPick={onPick} onOpen={onOpen} />
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </div>
  )
}
