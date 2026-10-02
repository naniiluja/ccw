import { ArrowDownIcon, ArrowUpDownIcon, ArrowUpIcon } from 'lucide-react'
import { useMemo, useState } from 'react'
import type { UsageRow } from '@/api/usage'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { cn } from '@/lib/utils'
import {
  type Sort,
  type SortKey,
  formatNumber,
  longDay,
  sortRows,
} from './usage-model'

const columns: { key: SortKey; label: string; numeric?: boolean }[] = [
  { key: 'day', label: 'Ngày' },
  { key: 'account', label: 'Tài khoản' },
  { key: 'model', label: 'Model' },
  { key: 'inputTokens', label: 'Token vào', numeric: true },
  { key: 'outputTokens', label: 'Token ra', numeric: true },
  { key: 'requests', label: 'Số request', numeric: true },
]

export function UsageTable({
  rows,
  nameOf,
}: {
  rows: UsageRow[]
  nameOf: (id: string) => string
}) {
  const [sort, setSort] = useState<Sort>({ key: 'day', dir: 'desc' })
  const sorted = useMemo(() => sortRows(rows, sort, nameOf), [rows, sort, nameOf])

  const toggle = (key: SortKey) =>
    setSort((s) =>
      s.key === key
        ? { key, dir: s.dir === 'asc' ? 'desc' : 'asc' }
        : { key, dir: 'asc' },
    )

  return (
    <Card className="p-0">
      <Table className="min-w-[44rem]">
        <TableHeader>
          <TableRow>
            {columns.map((c) => {
              const active = sort.key === c.key
              const Icon = !active
                ? ArrowUpDownIcon
                : sort.dir === 'asc'
                  ? ArrowUpIcon
                  : ArrowDownIcon
              return (
                <TableHead
                  key={c.key}
                  aria-sort={
                    active
                      ? sort.dir === 'asc'
                        ? 'ascending'
                        : 'descending'
                      : 'none'
                  }
                  className={cn(c.numeric && 'text-right')}
                >
                  <Button
                    variant="ghost"
                    size="sm"
                    className="-mx-2"
                    onClick={() => toggle(c.key)}
                  >
                    {c.label}
                    <Icon data-icon="inline-end" aria-hidden="true" />
                  </Button>
                </TableHead>
              )
            })}
          </TableRow>
        </TableHeader>
        <TableBody>
          {sorted.map((r) => (
            <TableRow key={`${r.day}|${r.connectionId}|${r.model}`}>
              <TableCell className="tabular-nums">{longDay(r.day)}</TableCell>
              <TableCell>{nameOf(r.connectionId)}</TableCell>
              <TableCell>{r.model}</TableCell>
              <TableCell className="text-right tabular-nums">
                {formatNumber(r.inputTokens)}
              </TableCell>
              <TableCell className="text-right tabular-nums">
                {formatNumber(r.outputTokens)}
              </TableCell>
              <TableCell className="text-right tabular-nums">
                {formatNumber(r.requests)}
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </Card>
  )
}
