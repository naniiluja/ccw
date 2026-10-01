import { zodResolver } from '@hookform/resolvers/zod'
import { useMemo } from 'react'
import { useForm } from 'react-hook-form'
import { toast } from 'sonner'
import { z } from 'zod'
import { isApiError } from '@/api/client'
import { maxKeyRPM, useSetKeyLimits, type ApiKey } from '@/api/keys'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  Field,
  FieldDescription,
  FieldError,
  FieldGroup,
  FieldLabel,
} from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Spinner } from '@/components/ui/spinner'
import { toLocalInput } from './format'
import { s } from './strings'

interface LimitsForm {
  rpm: string
  expires: string
}

// A form schema built per key: an expiry the key already has is accepted
// unchanged even when it has passed, so the RPM of an expired key can be edited.
function limitsSchema(initialExpires: string) {
  return z.object({
    rpm: z
      .string()
      .trim()
      .regex(/^\d+$/, s.rpmInvalid)
      .refine((v) => Number(v) <= maxKeyRPM, s.rpmInvalid),
    expires: z.string().superRefine((v, ctx) => {
      if (v === '' || v === initialExpires) return
      const t = new Date(v).getTime()
      if (Number.isNaN(t)) ctx.addIssue({ code: 'custom', message: s.expiresInvalid })
      else if (t <= Date.now()) ctx.addIssue({ code: 'custom', message: s.expiresPast })
    }),
  })
}

export function LimitsDialog({
  apiKey,
  onClose,
}: {
  apiKey: ApiKey
  onClose: () => void
}) {
  const initialExpires = useMemo(() => toLocalInput(apiKey.expiresAt), [apiKey.expiresAt])
  const schema = useMemo(() => limitsSchema(initialExpires), [initialExpires])
  const save = useSetKeyLimits()
  const form = useForm<LimitsForm>({
    resolver: zodResolver(schema),
    defaultValues: { rpm: String(apiKey.rpm), expires: initialExpires },
  })
  const { errors } = form.formState

  function submit(v: LimitsForm) {
    // Keep the exact stored time when the field was not touched.
    const expiresAt =
      v.expires === ''
        ? ''
        : v.expires === initialExpires
          ? apiKey.expiresAt
          : new Date(v.expires).toISOString()
    save.mutate(
      { id: apiKey.id, rpm: Number(v.rpm), expiresAt },
      {
        onSuccess: () => {
          toast.success(s.limitsSaved)
          onClose()
        },
        onError: (err) => {
          // The server names the field it refused ("rpm: ...", "expiresAt: ...").
          if (isApiError(err) && err.message.startsWith('rpm')) {
            form.setError('rpm', { message: err.message })
          } else if (isApiError(err) && err.message.startsWith('expiresAt')) {
            form.setError('expires', { message: err.message })
          } else {
            form.setError('root', { message: `${s.limitsFailed}: ${err.message}` })
          }
        },
      },
    )
  }

  return (
    <Dialog open onOpenChange={(o) => !o && !save.isPending && onClose()}>
      <DialogContent>
        <form onSubmit={form.handleSubmit(submit)} noValidate className="grid gap-6">
          <DialogHeader>
            <DialogTitle>{s.limitsTitle}</DialogTitle>
            <DialogDescription>{s.limitsDesc(apiKey.name)}</DialogDescription>
          </DialogHeader>
          {errors.root ? (
            <Alert variant="destructive">
              <AlertDescription>{errors.root.message}</AlertDescription>
            </Alert>
          ) : null}
          <FieldGroup>
            <Field data-invalid={errors.rpm ? true : undefined}>
              <FieldLabel htmlFor="limit-rpm">{s.rpmLabel}</FieldLabel>
              <Input
                id="limit-rpm"
                inputMode="numeric"
                autoComplete="off"
                className="tabular-nums"
                aria-invalid={errors.rpm ? true : undefined}
                {...form.register('rpm')}
              />
              {errors.rpm ? (
                <FieldError>{errors.rpm.message}</FieldError>
              ) : (
                <FieldDescription>{s.rpmHelp}</FieldDescription>
              )}
            </Field>
            <Field data-invalid={errors.expires ? true : undefined}>
              <FieldLabel htmlFor="limit-expires">{s.expiresLabel}</FieldLabel>
              <div className="flex gap-2">
                <Input
                  id="limit-expires"
                  type="datetime-local"
                  aria-invalid={errors.expires ? true : undefined}
                  {...form.register('expires')}
                />
                <Button
                  type="button"
                  variant="outline"
                  onClick={() => form.setValue('expires', '', { shouldValidate: true })}
                >
                  {s.expiresClear}
                </Button>
              </div>
              {errors.expires ? (
                <FieldError>{errors.expires.message}</FieldError>
              ) : (
                <FieldDescription>{s.expiresHelp}</FieldDescription>
              )}
            </Field>
          </FieldGroup>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={onClose}>
              {s.cancel}
            </Button>
            <Button type="submit" disabled={save.isPending}>
              {save.isPending ? <Spinner aria-hidden="true" /> : null}
              {save.isPending ? s.saving : s.save}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
