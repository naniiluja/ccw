import { ActivityIcon } from 'lucide-react'
import { useMemo } from 'react'
import { Link } from 'react-router'
import { Bar, BarChart, CartesianGrid, XAxis, YAxis } from 'recharts'
import { lastDays } from '@/api/overview'
import { useUsage } from '@/api/usage'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import {
  type ChartConfig,
  ChartContainer,
  ChartLegend,
  ChartLegendContent,
  ChartTooltip,
  ChartTooltipContent,
} from '@/components/ui/chart'
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from '@/components/ui/empty'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { BlockError, BlockSkeleton } from './block'
import { formatNumber } from './format'

const config = {
  inputTokens: { label: 'Token vào', color: 'var(--chart-1)' },
  outputTokens: { label: 'Token ra', color: 'var(--chart-3)' },
} satisfies ChartConfig

const shortDay = (day: string) => {
  const [, m, d] = day.split('-')
  return d && m ? `${d}/${m}` : day
}

const compact = new Intl.NumberFormat('vi-VN', { notation: 'compact' })

export function UsageBlock() {
  const usage = useUsage()
  const days = useMemo(() => lastDays(usage.data ?? [], 14), [usage.data])
  const totals = days.reduce(
    (t, d) => ({
      input: t.input + d.inputTokens,
      output: t.output + d.outputTokens,
      requests: t.requests + d.requests,
    }),
    { input: 0, output: 0, requests: 0 },
  )

  let body
  if (usage.isPending) {
    body = <BlockSkeleton label="Đang tải lượng dùng" className="h-64" />
  } else if (usage.isError) {
    body = (
      <BlockError
        title="Không tải được lượng dùng"
        message={usage.error.message}
        onRetry={() => void usage.refetch()}
      />
    )
  } else if (days.length === 0) {
    body = (
      <Empty>
        <EmptyHeader>
          <EmptyMedia variant="icon">
            <ActivityIcon aria-hidden="true" />
          </EmptyMedia>
          <EmptyTitle>Chưa có dữ liệu lượng dùng</EmptyTitle>
          <EmptyDescription>
            Số liệu sẽ xuất hiện sau khi có request đi qua gateway.
          </EmptyDescription>
        </EmptyHeader>
      </Empty>
    )
  } else {
    const summary = `Biểu đồ token vào và ra trong ${days.length} ngày gần nhất: ${formatNumber(
      totals.input,
    )} token vào, ${formatNumber(totals.output)} token ra, ${formatNumber(
      totals.requests,
    )} request.`
    body = (
      <figure className="flex flex-col gap-2">
        <div role="img" aria-label={summary}>
          <ChartContainer config={config} className="aspect-auto h-60 w-full">
            <BarChart data={days} margin={{ left: 4, right: 4, top: 8 }}>
              <CartesianGrid vertical={false} />
              <XAxis
                dataKey="day"
                tickLine={false}
                axisLine={false}
                tickMargin={8}
                tickFormatter={shortDay}
                minTickGap={16}
                label={{ value: 'Ngày', position: 'insideBottom', offset: -4 }}
                height={40}
              />
              <YAxis
                tickLine={false}
                axisLine={false}
                width={48}
                tickFormatter={(v: number) => compact.format(v)}
                label={{
                  value: 'Token',
                  angle: -90,
                  position: 'insideLeft',
                  offset: 8,
                  style: { textAnchor: 'middle' },
                }}
              />
              <ChartTooltip content={<ChartTooltipContent />} />
              <ChartLegend content={<ChartLegendContent />} />
              <Bar dataKey="inputTokens" stackId="t" fill="var(--color-inputTokens)" />
              <Bar
                dataKey="outputTokens"
                stackId="t"
                fill="var(--color-outputTokens)"
                radius={[3, 3, 0, 0]}
              />
            </BarChart>
          </ChartContainer>
        </div>
        <figcaption className="text-xs text-muted-foreground tabular-nums">
          {days.length} ngày gần nhất · {formatNumber(totals.requests)} request.
        </figcaption>
        <Table className="sr-only" aria-label="Bảng số liệu usage 14 ngày gần nhất">
          <TableHeader>
            <TableRow>
              <TableHead>Ngày</TableHead>
              <TableHead>Token vào</TableHead>
              <TableHead>Token ra</TableHead>
              <TableHead>Số request</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {days.map((d) => (
              <TableRow key={d.day}>
                <TableHead scope="row">{d.day}</TableHead>
                <TableCell>{formatNumber(d.inputTokens)}</TableCell>
                <TableCell>{formatNumber(d.outputTokens)}</TableCell>
                <TableCell>{formatNumber(d.requests)}</TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </figure>
    )
  }

  return (
    <Card size="sm">
      <CardHeader>
        <CardTitle className="flex items-center justify-between gap-2">
          Lượng dùng 14 ngày
          <Link
            to="/usage"
            className="text-sm font-normal text-muted-foreground hover:underline"
          >
            Xem chi tiết
          </Link>
        </CardTitle>
      </CardHeader>
      <CardContent>{body}</CardContent>
    </Card>
  )
}
