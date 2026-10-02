import { TriangleAlertIcon } from 'lucide-react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { messageOf } from './format'

interface InlineErrorProps {
  title: string
  error: unknown
  onRetry?: () => void
}

// InlineError is the error state of a section: what failed, the server's
// message, and a retry.
export function InlineError({ title, error, onRetry }: InlineErrorProps) {
  return (
    <Alert variant="destructive">
      <TriangleAlertIcon aria-hidden="true" />
      <AlertTitle>{title}</AlertTitle>
      <AlertDescription>
        <p>{messageOf(error)}</p>
        {onRetry ? (
          <Button variant="outline" size="sm" className="mt-3" onClick={onRetry}>
            Thử lại
          </Button>
        ) : null}
      </AlertDescription>
    </Alert>
  )
}
