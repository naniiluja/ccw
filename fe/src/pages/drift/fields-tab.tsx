import { ListTreeIcon, TriangleAlertIcon } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useDriftFields, type DriftField } from '@/api/drift'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
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
import { Skeleton } from '@/components/ui/skeleton'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { directionLabel, formatTime } from './strings'

interface Row extends DriftField {
  direction: string
  provider: string
  endpoint: string
}

// The observer keys a field set as "direction|provider|endpoint@variant|...".
function parse(f: DriftField): Row {
  const [direction = '', provider = '', endpoint = ''] = f.key.split('|')
  return { ...f, direction, provider, endpoint: endpoint.split('@')[0] }
}

// FieldsTab shows the fields the observer is watching, grouped by provider.
export function FieldsTab() {
  const query = useDriftFields()
  const [text, setText] = useState('')

  const groups = useMemo(() => {
    const needle = text.trim().toLowerCase()
    const byProvider = new Map<string, Row[]>()
    for (const f of query.data?.fields ?? []) {
      const row = parse(f)
      if (
        needle &&
        !row.path.toLowerCase().includes(needle) &&
        !row.provider.toLowerCase().includes(needle)
      ) {
        continue
      }
      byProvider.set(row.provider, [...(byProvider.get(row.provider) ?? []), row])
    }
    return [...byProvider.entries()].sort(([a], [b]) => a.localeCompare(b))
  }, [query.data, text])

  if (query.isPending) {
    return (
      <div role="status" aria-label="Đang tải trường theo dõi" className="flex flex-col gap-3">
        <Skeleton className="h-9 w-64" />
        <Skeleton className="h-48 w-full" />
      </div>
    )
  }
  if (query.isError) {
    return (
      <Alert variant="destructive">
        <TriangleAlertIcon aria-hidden="true" />
        <AlertTitle>Không tải được trường theo dõi</AlertTitle>
        <AlertDescription>
          <p>{query.error.message}</p>
          <Button variant="outline" size="sm" className="mt-3" onClick={() => query.refetch()}>
            Thử lại
          </Button>
        </AlertDescription>
      </Alert>
    )
  }
  if ((query.data?.fields.length ?? 0) === 0) {
    return (
      <Card className="p-0">
        <Empty>
          <EmptyHeader>
            <EmptyMedia variant="icon">
              <ListTreeIcon aria-hidden="true" />
            </EmptyMedia>
            <EmptyTitle>Chưa theo dõi trường nào</EmptyTitle>
            <EmptyDescription>
              Trường được học khi có lưu lượng đi qua, hoặc khi bạn khởi tạo mẫu ở tab Cấu hình.
            </EmptyDescription>
          </EmptyHeader>
        </Empty>
      </Card>
    )
  }

  return (
    <div className="flex flex-col gap-4">
      <div className="flex w-full flex-col gap-1.5 sm:w-72">
        <Label htmlFor="drift-field-search">Tìm trường</Label>
        <Input
          id="drift-field-search"
          value={text}
          placeholder="Đường dẫn hoặc nhà cung cấp"
          onChange={(e) => setText(e.target.value)}
        />
      </div>
      {groups.length === 0 ? (
        <p className="text-sm text-muted-foreground">Không có trường nào khớp.</p>
      ) : (
        groups.map(([provider, rows]) => (
          <Card key={provider}>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <Badge variant="secondary">{provider || 'không rõ'}</Badge>
                <span className="text-sm font-normal text-muted-foreground tabular-nums">
                  {rows.length} trường
                </span>
              </CardTitle>
            </CardHeader>
            <CardContent>
              <ScrollArea className="w-full">
                <Table className="min-w-[640px]">
                  <TableHeader>
                    <TableRow>
                      <TableHead>Đường dẫn</TableHead>
                      <TableHead>Hướng</TableHead>
                      <TableHead>Endpoint</TableHead>
                      <TableHead>Kiểu</TableHead>
                      <TableHead className="text-right">Lần thấy</TableHead>
                      <TableHead>Lần cuối</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {rows.map((r) => (
                      <TableRow key={`${r.key}#${r.path}`}>
                        <TableCell className="font-mono text-xs break-all">
                          {r.path}
                          {r.gone ? (
                            <Badge variant="destructive" className="ml-2">
                              Đã mất
                            </Badge>
                          ) : null}
                        </TableCell>
                        <TableCell>
                          {directionLabel[r.direction as 'request' | 'response'] ?? r.direction}
                        </TableCell>
                        <TableCell className="text-xs">{r.endpoint}</TableCell>
                        <TableCell className="font-mono text-xs">{r.type}</TableCell>
                        <TableCell className="text-right tabular-nums">{r.seen}</TableCell>
                        <TableCell className="text-xs whitespace-nowrap tabular-nums">
                          {r.lastAt ? formatTime(r.lastAt) : '—'}
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
                <ScrollBar orientation="horizontal" />
              </ScrollArea>
            </CardContent>
          </Card>
        ))
      )}
    </div>
  )
}
