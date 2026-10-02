import { useState } from 'react'
import { toast } from 'sonner'
import {
  type PolicyState,
  useModelPolicy,
  useSavePolicy,
} from '@/api/providers'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Field,
  FieldContent,
  FieldDescription,
  FieldGroup,
  FieldLabel,
} from '@/components/ui/field'
import { Skeleton } from '@/components/ui/skeleton'
import { Spinner } from '@/components/ui/spinner'
import { Switch } from '@/components/ui/switch'
import { formatTime, messageOf } from './format'
import { InlineError } from './inline-error'

export function PolicyTab({ providerId }: { providerId: string }) {
  const query = useModelPolicy(providerId)
  if (query.isPending) {
    return (
      <div role="status" aria-label="Đang tải chính sách model" className="flex flex-col gap-3">
        <Skeleton className="h-14" />
        <Skeleton className="h-14" />
        <Skeleton className="h-9 w-40" />
      </div>
    )
  }
  if (query.isError) {
    return (
      <InlineError
        title="Không tải được chính sách model"
        error={query.error}
        onRetry={() => void query.refetch()}
      />
    )
  }
  return <PolicyForm key={JSON.stringify(query.data.policy)} providerId={providerId} state={query.data} />
}

function PolicyForm({
  providerId,
  state,
}: {
  providerId: string
  state: PolicyState
}) {
  const save = useSavePolicy(providerId)
  const [autoTest, setAutoTest] = useState(state.policy.autoTest)
  const [onlyFree, setOnlyFree] = useState(state.policy.onlyFree)
  const dirty =
    autoTest !== state.policy.autoTest || onlyFree !== state.policy.onlyFree

  const submit = (e: React.FormEvent) => {
    e.preventDefault()
    save.mutate(
      { autoTest, onlyFree },
      {
        onSuccess: () => toast.success('Đã lưu chính sách model.'),
        onError: (e) => toast.error(messageOf(e)),
      },
    )
  }

  return (
    <form onSubmit={submit} className="flex max-w-xl flex-col gap-6">
      <FieldGroup>
        <Field orientation="horizontal">
          <FieldContent>
            <FieldLabel htmlFor="pol-auto">Tự động thử model</FieldLabel>
            <FieldDescription>
              Máy chủ định kỳ thử từng model và tự bật model chạy được, tắt
              model lỗi.
            </FieldDescription>
          </FieldContent>
          <Switch id="pol-auto" checked={autoTest} onCheckedChange={setAutoTest} />
        </Field>
        <Field orientation="horizontal">
          <FieldContent>
            <FieldLabel htmlFor="pol-free">Chỉ dùng model miễn phí</FieldLabel>
            <FieldDescription>
              Chỉ giữ bật những model miễn phí khi áp dụng chính sách.
            </FieldDescription>
          </FieldContent>
          <Switch id="pol-free" checked={onlyFree} onCheckedChange={setOnlyFree} />
        </Field>
      </FieldGroup>

      <div className="flex flex-col gap-1 text-sm text-muted-foreground">
        {state.running ? (
          <Badge variant="secondary" className="w-fit">
            Đang chạy thử model
          </Badge>
        ) : null}
        {state.policy.lastRun ? (
          <p>
            Lần chạy gần nhất:{' '}
            <span className="tabular-nums">{formatTime(state.policy.lastRun)}</span>
            {state.policy.lastResult ? ` (${state.policy.lastResult})` : ''}
          </p>
        ) : null}
      </div>

      <div>
        <Button type="submit" disabled={save.isPending || !dirty}>
          {save.isPending ? <Spinner aria-hidden="true" /> : null}
          Lưu chính sách
        </Button>
      </div>
    </form>
  )
}
