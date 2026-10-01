import { TriangleAlertIcon } from 'lucide-react'
import type { ReactNode } from 'react'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'

// Shared pieces of the independent blocks: each block renders its own loading
// skeleton and its own error with a retry button.

export function BlockSkeleton({
  label,
  className = 'h-24',
}: {
  label: string
  className?: string
}) {
  return (
    <div role="status" aria-label={label} className="flex flex-col gap-2">
      <Skeleton className={className} />
    </div>
  )
}

export function BlockError({
  title,
  message,
  onRetry,
}: {
  title: string
  message?: string
  onRetry: () => void
}) {
  return (
    <div
      role="alert"
      className="flex flex-col items-start gap-2 text-sm text-destructive"
    >
      <p className="flex items-center gap-2 font-medium">
        <TriangleAlertIcon aria-hidden="true" className="size-4 shrink-0" />
        {title}
      </p>
      {message ? (
        <p className="text-muted-foreground break-words">{message}</p>
      ) : null}
      <Button variant="outline" size="sm" onClick={onRetry}>
        Thử lại
      </Button>
    </div>
  )
}

export function Muted({ children }: { children: ReactNode }) {
  return <p className="text-sm text-muted-foreground">{children}</p>
}
