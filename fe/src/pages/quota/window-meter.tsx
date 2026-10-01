import { TriangleAlertIcon } from 'lucide-react'
import type { QuotaWindow } from '@/api/quota'
import { Progress } from '@/components/ui/progress'
import { cn } from '@/lib/utils'
import { clampPct, levelOf, resetText } from './format'

const barColor = {
  ok: '',
  high: '[&>div]:bg-amber-500',
  critical: '[&>div]:bg-destructive',
} as const

// WindowMeter shows one quota window. The level is also written in words, so
// the warning does not depend on the colour of the bar.
export function WindowMeter({ window: w }: { window: QuotaWindow }) {
  if (w.unlimited) {
    return (
      <div className="flex items-center justify-between gap-3 text-sm">
        <span className="font-medium">{w.name}</span>
        <span className="text-muted-foreground">Không giới hạn</span>
      </div>
    )
  }
  const pct = clampPct(w.usedPct)
  const level = levelOf(w)
  const reset = resetText(w.resetAt)
  const amount = w.used && w.limit ? `${w.used} / ${w.limit}` : ''
  return (
    <div className="flex flex-col gap-1.5 text-sm">
      <div className="flex items-baseline justify-between gap-3">
        <span className="font-medium">{w.name}</span>
        <span className="tabular-nums">{pct}%</span>
      </div>
      <Progress
        value={pct}
        aria-valuenow={pct}
        aria-label={`${w.name}: đã dùng ${pct}%`}
        className={cn(barColor[level])}
      />
      <div className="flex flex-wrap items-center justify-between gap-x-3 gap-y-0.5 text-xs text-muted-foreground">
        <span className="tabular-nums">{[amount, reset].filter(Boolean).join(' · ')}</span>
        {level !== 'ok' ? (
          <span
            className={cn(
              'inline-flex items-center gap-1 font-medium',
              level === 'critical' ? 'text-destructive' : 'text-amber-600 dark:text-amber-400',
            )}
          >
            <TriangleAlertIcon className="size-3" aria-hidden="true" />
            {level === 'critical' ? 'Sắp hết' : 'Gần ngưỡng'}
          </span>
        ) : null}
      </div>
    </div>
  )
}
