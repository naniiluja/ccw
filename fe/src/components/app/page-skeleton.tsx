import { Skeleton } from '@/components/ui/skeleton'

// PageSkeleton is the loading state of a page or of the whole app.
export function PageSkeleton({ fullscreen = false }: { fullscreen?: boolean }) {
  return (
    <div
      role="status"
      aria-live="polite"
      aria-label="Đang tải"
      className={fullscreen ? 'min-h-svh p-6' : undefined}
    >
      <div className="mx-auto flex max-w-7xl flex-col gap-6">
        <div className="flex flex-col gap-2">
          <Skeleton className="h-7 w-48" />
          <Skeleton className="h-4 w-80 max-w-full" />
        </div>
        <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
          <Skeleton className="h-28" />
          <Skeleton className="h-28" />
          <Skeleton className="h-28" />
        </div>
        <Skeleton className="h-64" />
      </div>
    </div>
  )
}
