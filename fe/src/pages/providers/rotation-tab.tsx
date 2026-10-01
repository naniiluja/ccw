import { zodResolver } from '@hookform/resolvers/zod'
import { Controller, useForm } from 'react-hook-form'
import { toast } from 'sonner'
import { z } from 'zod'
import { isApiError } from '@/api/client'
import {
  type RotationMode,
  type RotationState,
  useRotation,
  useSaveRotation,
} from '@/api/providers'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Field,
  FieldDescription,
  FieldError,
  FieldGroup,
  FieldLabel,
  FieldLegend,
  FieldSet,
} from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { RadioGroup, RadioGroupItem } from '@/components/ui/radio-group'
import { Skeleton } from '@/components/ui/skeleton'
import { Spinner } from '@/components/ui/spinner'
import { fieldOfError, messageOf } from './format'
import { InlineError } from './inline-error'

const schema = z.object({
  mode: z.enum(['round-robin', 'fallback']),
  sticky: z
    .string()
    .trim()
    .refine((v) => /^\d+$/.test(v) && Number(v) >= 1 && Number(v) <= 1000, {
      message: 'Nhập số từ 1 đến 1000',
    }),
})

type Values = z.infer<typeof schema>
const fields = ['mode', 'sticky'] as const

export function RotationTab({ providerId }: { providerId: string }) {
  const query = useRotation(providerId)
  if (query.isPending) {
    return (
      <div role="status" aria-label="Đang tải cấu hình xoay vòng" className="flex flex-col gap-3">
        <Skeleton className="h-6 w-48" />
        <Skeleton className="h-20" />
        <Skeleton className="h-9 w-40" />
      </div>
    )
  }
  if (query.isError) {
    return (
      <InlineError
        title="Không tải được cấu hình xoay vòng"
        error={query.error}
        onRetry={() => void query.refetch()}
      />
    )
  }
  return <RotationForm providerId={providerId} state={query.data} />
}

function RotationForm({
  providerId,
  state,
}: {
  providerId: string
  state: RotationState
}) {
  const save = useSaveRotation(providerId)
  const {
    control,
    register,
    handleSubmit,
    setError,
    reset,
    formState: { errors },
  } = useForm<Values>({
    resolver: zodResolver(schema),
    defaultValues: {
      mode: state.rotation.mode,
      sticky: String(state.rotation.sticky),
    },
  })
  const serverError = errors.root?.message

  const onSubmit = (v: Values) =>
    save.mutate(
      { mode: v.mode as RotationMode, sticky: Number(v.sticky) },
      {
        onSuccess: (data) => {
          reset({
            mode: data.rotation.mode,
            sticky: String(data.rotation.sticky),
          })
          toast.success('Đã lưu cấu hình xoay vòng.')
        },
        onError: (e) => {
          const message = messageOf(e)
          const field = isApiError(e) && e.status === 400 ? fieldOfError(message, fields) : undefined
          if (field) setError(field, { message })
          else setError('root', { message })
        },
      },
    )

  return (
    <form onSubmit={handleSubmit(onSubmit)} noValidate className="flex max-w-xl flex-col gap-6">
      <FieldGroup>
        <Controller
          control={control}
          name="mode"
          render={({ field, fieldState }) => (
            <FieldSet data-invalid={fieldState.invalid}>
              <FieldLegend variant="label">Chế độ</FieldLegend>
              <RadioGroup
                value={field.value}
                onValueChange={field.onChange}
                aria-label="Chế độ xoay vòng"
              >
                <Field orientation="horizontal">
                  <RadioGroupItem value="round-robin" id="rot-rr" />
                  <FieldLabel htmlFor="rot-rr">Vòng tròn (round-robin)</FieldLabel>
                </Field>
                <Field orientation="horizontal">
                  <RadioGroupItem value="fallback" id="rot-fb" />
                  <FieldLabel htmlFor="rot-fb">Dự phòng (fallback)</FieldLabel>
                </Field>
              </RadioGroup>
              <FieldDescription>
                Vòng tròn chia request đều giữa các tài khoản. Dự phòng luôn
                dùng tài khoản đầu tiên và chỉ chuyển sang tài khoản sau khi
                nó lỗi.
              </FieldDescription>
              {fieldState.error ? <FieldError errors={[fieldState.error]} /> : null}
            </FieldSet>
          )}
        />
        <Field data-invalid={errors.sticky ? true : undefined}>
          <FieldLabel htmlFor="rot-sticky">Số request liên tiếp</FieldLabel>
          <Input
            id="rot-sticky"
            type="number"
            inputMode="numeric"
            min={1}
            max={1000}
            className="tabular-nums sm:w-40"
            aria-invalid={errors.sticky ? true : undefined}
            {...register('sticky')}
          />
          <FieldDescription>
            Một tài khoản phục vụ liên tiếp bấy nhiêu request trước khi đến
            lượt tài khoản khác (1 đến 1000).
          </FieldDescription>
          {errors.sticky ? <FieldError errors={[errors.sticky]} /> : null}
        </Field>
      </FieldGroup>

      {state.next ? (
        <p className="text-sm text-muted-foreground">
          Lượt tiếp theo thuộc tài khoản{' '}
          <code className="font-mono text-xs">{state.next}</code>
          {state.used ? (
            <span className="tabular-nums"> (đã phục vụ {state.used} request)</span>
          ) : null}
          .
        </p>
      ) : null}

      {serverError ? (
        <Alert variant="destructive">
          <AlertDescription>{serverError}</AlertDescription>
        </Alert>
      ) : null}

      <div>
        <Button type="submit" disabled={save.isPending}>
          {save.isPending ? <Spinner aria-hidden="true" /> : null}
          Lưu xoay vòng
        </Button>
      </div>
    </form>
  )
}
