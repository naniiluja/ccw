import { TriangleAlertIcon } from 'lucide-react'
import { isRouteErrorResponse, useRouteError } from 'react-router'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'

function messageOf(error: unknown): string {
  if (isRouteErrorResponse(error)) return `${error.status} ${error.statusText}`
  if (error instanceof Error) return error.message
  return 'Đã có lỗi không xác định.'
}

interface RouteErrorProps {
  error?: unknown
  onRetry?: () => void
}

// RouteError is the router's errorElement and the error state of the session
// check. It names the problem and offers a retry; it never shows a stack.
export function RouteError({ error, onRetry }: RouteErrorProps) {
  const fromRouter = useRouteError()
  const message = messageOf(error ?? fromRouter)
  return (
    <div className="mx-auto grid min-h-svh max-w-lg place-items-center p-6">
      <Alert variant="destructive">
        <TriangleAlertIcon aria-hidden="true" />
        <AlertTitle>Không tải được trang</AlertTitle>
        <AlertDescription>
          <p>{message}</p>
          <Button
            variant="outline"
            size="sm"
            className="mt-3"
            onClick={onRetry ?? (() => window.location.reload())}
          >
            Thử lại
          </Button>
        </AlertDescription>
      </Alert>
    </div>
  )
}
