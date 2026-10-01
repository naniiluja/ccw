import type { ProviderInfo } from '@/api/providers'
import { Badge } from '@/components/ui/badge'
import { providerName } from './format'
import { cn } from '@/lib/utils'

interface ProviderListProps {
  providers: ProviderInfo[]
  /** Accounts per provider id; undefined while loading or when it failed. */
  counts?: Record<string, number>
  selected: string | null
  onSelect: (id: string) => void
}

export function ProviderList({
  providers,
  counts,
  selected,
  onSelect,
}: ProviderListProps) {
  return (
    <nav aria-label="Danh sách nhà cung cấp">
      <ul className="flex gap-2 overflow-x-auto pb-1 lg:flex-col lg:overflow-visible lg:pb-0">
        {providers.map((p) => {
          const n = counts?.[p.id]
          return (
            <li key={p.id} className="shrink-0 lg:shrink">
              <button
                type="button"
                aria-pressed={selected === p.id}
                onClick={() => onSelect(p.id)}
                className={cn(
                  'flex w-full min-w-44 items-center gap-3 rounded-lg border bg-card px-3 py-2.5 text-left text-sm transition-colors outline-none hover:bg-muted focus-visible:ring-3 focus-visible:ring-ring/50',
                  selected === p.id && 'border-primary bg-muted',
                )}
              >
                <span
                  aria-hidden="true"
                  className="size-2.5 shrink-0 rounded-full bg-muted-foreground/40"
                  style={p.color ? { backgroundColor: p.color } : undefined}
                />
                <span className="flex min-w-0 flex-1 flex-col">
                  <span className="truncate font-medium">{providerName(p)}</span>
                  <span className="text-xs text-muted-foreground tabular-nums">
                    {n === undefined ? '—' : `${n} tài khoản`}
                  </span>
                </span>
                {p.declared ? <Badge variant="outline">Tùy chỉnh</Badge> : null}
              </button>
            </li>
          )
        })}
      </ul>
    </nav>
  )
}
