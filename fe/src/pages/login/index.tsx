import { zodResolver } from '@hookform/resolvers/zod'
import { useQueryClient } from '@tanstack/react-query'
import { Loader2Icon, ShieldCheckIcon } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useForm } from 'react-hook-form'
import { Navigate, useNavigate, useSearchParams } from 'react-router'
import { z } from 'zod'
import { isApiError } from '@/api/client'
import {
  isSignedIn,
  safeRedirect,
  sessionQuery,
  signIn,
  useSession,
} from '@/api/session'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
} from '@/components/ui/card'
import { Field, FieldError, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'

const schema = z.object({
  password: z.string().min(1, 'Nhập mật khẩu để tiếp tục.'),
})
type Values = z.infer<typeof schema>

// useCountdown counts whole seconds down to zero; start(n) restarts it at n.
function useCountdown() {
  const [left, setLeft] = useState(0)
  useEffect(() => {
    if (left <= 0) return
    const timer = setTimeout(() => setLeft(left - 1), 1000)
    return () => clearTimeout(timer)
  }, [left])
  return [left, setLeft] as const
}

export default function LoginPage() {
  const [params] = useSearchParams()
  const target = safeRedirect(params.get('redirect'))
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const session = useSession()
  const [failure, setFailure] = useState<string | null>(null)
  const [left, setLeft] = useCountdown()
  const form = useForm<Values>({
    resolver: zodResolver(schema),
    defaultValues: { password: '' },
  })

  if (session.data && isSignedIn(session.data)) {
    return <Navigate to={target} replace />
  }

  async function onSubmit({ password }: Values) {
    setFailure(null)
    try {
      await signIn(password)
      await queryClient.invalidateQueries({ queryKey: sessionQuery.queryKey })
      await navigate(target, { replace: true })
    } catch (e) {
      if (isApiError(e) && e.status === 429) {
        setLeft(e.retryAfter ?? 60)
        setFailure('Đăng nhập sai quá nhiều lần.')
      } else if (isApiError(e) && e.status === 401) {
        setFailure('Mật khẩu không đúng. Hãy thử lại.')
      } else {
        setFailure(e instanceof Error ? e.message : 'Đăng nhập thất bại.')
      }
    }
  }

  const blocked = left > 0
  const { isSubmitting, errors } = form.formState

  return (
    <main className="grid min-h-svh place-items-center bg-muted/40 p-4">
      <Card className="w-full max-w-sm">
        <CardHeader>
          <div className="mb-2 flex size-9 items-center justify-center rounded-lg bg-primary text-primary-foreground">
            <ShieldCheckIcon className="size-5" aria-hidden="true" />
          </div>
          <h1 className="text-lg font-semibold">Đăng nhập</h1>
          <CardDescription>
            Nhập mật khẩu quản trị để vào bảng điều khiển ccw.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <form
            onSubmit={form.handleSubmit(onSubmit)}
            noValidate
            className="flex flex-col gap-4"
          >
            <Field data-invalid={errors.password ? true : undefined}>
              <FieldLabel htmlFor="password">Mật khẩu</FieldLabel>
              <Input
                id="password"
                type="password"
                autoComplete="current-password"
                autoFocus
                aria-invalid={errors.password ? true : undefined}
                {...form.register('password')}
              />
              <FieldError errors={[errors.password]} />
            </Field>
            {failure ? (
              <Alert variant="destructive" role="alert">
                <AlertDescription>
                  {failure}
                  {blocked ? ` Thử lại sau ${left} giây.` : null}
                </AlertDescription>
              </Alert>
            ) : null}
            <Button type="submit" disabled={isSubmitting || blocked}>
              {isSubmitting ? (
                <Loader2Icon className="animate-spin" aria-hidden="true" />
              ) : null}
              {isSubmitting ? 'Đang đăng nhập' : 'Đăng nhập'}
            </Button>
          </form>
        </CardContent>
      </Card>
    </main>
  )
}
