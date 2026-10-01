import { Bar, BarChart, CartesianGrid, XAxis, YAxis } from 'recharts'
import {
  type ChartConfig,
  ChartContainer,
  ChartLegend,
  ChartLegendContent,
  ChartTooltip,
  ChartTooltipContent,
} from '@/components/ui/chart'
import {
  type DayTotal,
  type Totals,
  formatCompact,
  formatNumber,
  longDay,
  shortDay,
} from './usage-model'

// The two series use the app chart tokens, so light and dark follow the theme.
const config = {
  inputTokens: { label: 'Token vào', color: 'var(--chart-1)' },
  outputTokens: { label: 'Token ra', color: 'var(--chart-3)' },
} satisfies ChartConfig

export function UsageChart({
  days,
  totals,
}: {
  days: DayTotal[]
  totals: Totals
}) {
  const summary = `Biểu đồ token vào và ra theo ngày, ${totals.days} ngày: ${formatNumber(
    totals.inputTokens,
  )} token vào, ${formatNumber(totals.outputTokens)} token ra, ${formatNumber(
    totals.requests,
  )} request.`
  return (
    <figure className="flex flex-col gap-2">
      <div role="img" aria-label={summary}>
        <ChartContainer config={config} className="aspect-auto h-64 w-full sm:h-72">
          <BarChart data={days} margin={{ left: 4, right: 4, top: 8 }}>
            <CartesianGrid vertical={false} />
            <XAxis
              dataKey="day"
              tickLine={false}
              axisLine={false}
              tickMargin={8}
              tickFormatter={shortDay}
              minTickGap={16}
            />
            <YAxis
              tickLine={false}
              axisLine={false}
              width={48}
              tickFormatter={formatCompact}
              label={{
                value: 'Token',
                angle: -90,
                position: 'insideLeft',
                offset: 8,
                style: { textAnchor: 'middle' },
              }}
            />
            <ChartTooltip
              content={
                <ChartTooltipContent
                  labelFormatter={(_, p) => longDay(String(p?.[0]?.payload?.day ?? ''))}
                  formatter={(value, name, item) => (
                    <div className="flex w-full items-center justify-between gap-4">
                      <span className="flex items-center gap-1.5 text-muted-foreground">
                        <span
                          className="size-2 rounded-[2px]"
                          style={{ background: item.color }}
                          aria-hidden="true"
                        />
                        {config[name as keyof typeof config]?.label ?? name}
                      </span>
                      <span className="font-mono font-medium tabular-nums">
                        {formatNumber(Number(value))}
                      </span>
                    </div>
                  )}
                />
              }
            />
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
        {totals.days} ngày · {formatNumber(totals.requests)} request. Cột xếp
        chồng token vào và token ra của mỗi ngày.
      </figcaption>
    </figure>
  )
}
