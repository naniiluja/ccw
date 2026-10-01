import { BarChartIcon, TriangleAlertIcon } from 'lucide-react'
import { useMemo } from 'react'
import { Bar, BarChart, CartesianGrid, XAxis, YAxis } from 'recharts'
import { useKeyUsage, type ApiKey, type KeyUsage } from '@/api/keys'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  ChartContainer,
  ChartTooltip,
  ChartTooltipContent,
  type ChartConfig,
} from '@/components/ui/chart'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
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
  TableCaption,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { formatNumber } from './format'
import { s } from './strings'

const config = {
  requests: { label: s.usageRequests, color: 'var(--chart-1)' },
} satisfies ChartConfig

function UsageBody({ usage }: { usage: KeyUsage }) {
  const rows = useMemo(
    () => [...usage.rows].sort((a, b) => b.day.localeCompare(a.day) || a.model.localeCompare(b.model)),
    [usage.rows],
  )
  const perDay = useMemo(() => {
    const byDay = new Map<string, number>()
    for (const r of usage.rows) byDay.set(r.day, (byDay.get(r.day) ?? 0) + r.requests)
    return [...byDay]
      .map(([day, requests]) => ({ day, requests }))
      .sort((a, b) => a.day.localeCompare(b.day))
  }, [usage.rows])
  const total = usage.rows.reduce(
    (t, r) => ({
      requests: t.requests + r.requests,
      input: t.input + r.inputTokens,
      output: t.output + r.outputTokens,
    }),
    { requests: 0, input: 0, output: 0 },
  )

  if (rows.length === 0) {
    return (
      <Empty>
        <EmptyHeader>
          <EmptyMedia variant="icon">
            <BarChartIcon aria-hidden="true" />
          </EmptyMedia>
          <EmptyTitle>{s.usageEmpty}</EmptyTitle>
          <EmptyDescription>{s.usageEmptyBody}</EmptyDescription>
        </EmptyHeader>
      </Empty>
    )
  }

  const stats = [
    [s.usageMinute, usage.rpm > 0 ? `${formatNumber(usage.lastMinute)} / ${formatNumber(usage.rpm)}` : formatNumber(usage.lastMinute)],
    [`${s.usageRequests} (${s.usageTotal})`, formatNumber(total.requests)],
    [s.usageInput, formatNumber(total.input)],
    [s.usageOutput, formatNumber(total.output)],
  ]

  return (
    <div className="flex min-w-0 flex-col gap-4">
      <dl className="grid grid-cols-2 gap-3 sm:grid-cols-4">
        {stats.map(([label, value]) => (
          <div key={label} className="rounded-lg border p-3">
            <dt className="text-xs text-muted-foreground">{label}</dt>
            <dd className="mt-1 text-lg font-semibold tabular-nums">{value}</dd>
          </div>
        ))}
      </dl>
      <ChartContainer
        config={config}
        className="h-48 w-full"
        role="img"
        aria-label={s.usageChart(perDay.length)}
      >
        <BarChart data={perDay} accessibilityLayer>
          <CartesianGrid vertical={false} />
          <XAxis dataKey="day" tickLine={false} axisLine={false} tickFormatter={(d: string) => d.slice(5)} />
          <YAxis width={40} tickLine={false} axisLine={false} allowDecimals={false} />
          <ChartTooltip content={<ChartTooltipContent />} />
          <Bar dataKey="requests" fill="var(--color-requests)" radius={4} />
        </BarChart>
      </ChartContainer>
      <div className="max-h-64 overflow-y-auto rounded-md border">
        <Table>
          <TableCaption className="sr-only">{s.usageCaption}</TableCaption>
          <TableHeader>
            <TableRow>
              <TableHead>{s.colDay}</TableHead>
              <TableHead>{s.colModel}</TableHead>
              <TableHead className="text-right">{s.usageRequests}</TableHead>
              <TableHead className="text-right">{s.usageInput}</TableHead>
              <TableHead className="text-right">{s.usageOutput}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {rows.map((r) => (
              <TableRow key={`${r.day}|${r.model}`}>
                <TableCell className="tabular-nums">{r.day}</TableCell>
                <TableCell className="font-mono text-xs break-all">{r.model}</TableCell>
                <TableCell className="text-right tabular-nums">{formatNumber(r.requests)}</TableCell>
                <TableCell className="text-right tabular-nums">{formatNumber(r.inputTokens)}</TableCell>
                <TableCell className="text-right tabular-nums">{formatNumber(r.outputTokens)}</TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </div>
    </div>
  )
}

export function UsageDialog({
  apiKey,
  onClose,
}: {
  apiKey: ApiKey
  onClose: () => void
}) {
  const usage = useKeyUsage(apiKey.id)
  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="max-h-[90svh] overflow-y-auto sm:max-w-3xl">
        <DialogHeader>
          <DialogTitle>{s.usageTitle(apiKey.name)}</DialogTitle>
          <DialogDescription>{s.usageDesc}</DialogDescription>
        </DialogHeader>
        {usage.isPending ? (
          <div role="status" aria-label={s.loading} className="flex flex-col gap-3">
            <Skeleton className="h-16" />
            <Skeleton className="h-48" />
          </div>
        ) : usage.isError ? (
          <Alert variant="destructive">
            <TriangleAlertIcon aria-hidden="true" />
            <AlertDescription>
              <p>{`${s.usageFailed}: ${usage.error.message}`}</p>
              <Button variant="outline" size="sm" className="mt-2" onClick={() => void usage.refetch()}>
                {s.retry}
              </Button>
            </AlertDescription>
          </Alert>
        ) : (
          <UsageBody usage={usage.data} />
        )}
      </DialogContent>
    </Dialog>
  )
}
