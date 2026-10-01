import { zodResolver } from '@hookform/resolvers/zod'
import { useEffect } from 'react'
import { Controller, useForm, useWatch } from 'react-hook-form'
import { toast } from 'sonner'
import { z } from 'zod'
import { isApiError } from '@/api/client'
import {
  type Filter,
  useFilterProviders,
  useSaveFilter,
} from '@/api/filters'
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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import {
  ALL_PROVIDERS,
  kindHints,
  kindLabels,
  kinds,
  providerLabel,
  text,
} from './strings'

const schema = z
  .object({
    provider: z.string().min(1),
    kind: z.string().min(1),
    pattern: z.string().trim().min(1, text.patternRequired),
    note: z.string().max(200, text.noteTooLong),
    enabled: z.boolean(),
  })
  .superRefine((v, ctx) => {
    if (v.kind !== 'system' || !v.pattern) return
    try {
      new RegExp(v.pattern)
    } catch {
      ctx.addIssue({
        code: 'custom',
        path: ['pattern'],
        message: text.patternBadRegex,
      })
    }
  })

type Values = z.infer<typeof schema>

const blank: Values = {
  provider: ALL_PROVIDERS,
  kind: 'field',
  pattern: '',
  note: '',
  enabled: true,
}

interface FilterFormProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  /** The rule being edited; undefined creates a new one. */
  filter?: Filter
}

// FilterForm creates or edits a rule. A refusal from the server (an unsafe or
// invalid pattern, status 400) is shown beside the pattern field.
export function FilterForm({ open, onOpenChange, filter }: FilterFormProps) {
  const providers = useFilterProviders()
  const save = useSaveFilter()
  const form = useForm<Values>({
    resolver: zodResolver(schema),
    defaultValues: blank,
  })
  const { control, register, handleSubmit, reset, setError, formState } = form

  useEffect(() => {
    if (open) {
      reset(
        filter
          ? {
              provider: filter.provider,
              kind: filter.kind,
              pattern: filter.pattern,
              note: filter.note,
              enabled: filter.enabled,
            }
          : blank,
      )
    }
  }, [open, filter, reset])

  const ids = new Set((providers.data ?? []).map((p) => p.id))
  const options = [ALL_PROVIDERS, ...ids]
  if (filter && !options.includes(filter.provider)) options.push(filter.provider)

  const submit = handleSubmit(async (values) => {
    try {
      await save.mutateAsync({
        ...values,
        pattern: values.pattern.trim(),
        id: filter?.id,
      })
      toast.success(text.saved)
      onOpenChange(false)
    } catch (e) {
      if (isApiError(e) && e.status === 400) {
        setError('pattern', { message: e.message })
      } else {
        toast.error(text.saveFailed, {
          description: isApiError(e) ? e.message : undefined,
        })
      }
    }
  })

  const kind = useWatch({ control, name: 'kind' })
  const patternError = formState.errors.pattern
  const patternDescription = patternError ? 'pattern-error' : 'pattern-hint'

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[90svh] overflow-y-auto sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{filter ? text.editTitle : text.createTitle}</DialogTitle>
          <DialogDescription>{text.formDescription}</DialogDescription>
        </DialogHeader>
        <form onSubmit={submit} noValidate className="flex flex-col gap-4">
          <FieldGroup>
            <div className="grid gap-4 sm:grid-cols-2">
              <Field>
                <FieldLabel htmlFor="filter-provider">{text.provider}</FieldLabel>
                <Controller
                  control={control}
                  name="provider"
                  render={({ field }) => (
                    <Select value={field.value} onValueChange={field.onChange}>
                      <SelectTrigger id="filter-provider" className="w-full">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        {options.map((id) => (
                          <SelectItem key={id} value={id}>
                            {providerLabel(id)}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  )}
                />
              </Field>
              <Field>
                <FieldLabel htmlFor="filter-kind">{text.kind}</FieldLabel>
                <Controller
                  control={control}
                  name="kind"
                  render={({ field }) => (
                    <Select value={field.value} onValueChange={field.onChange}>
                      <SelectTrigger id="filter-kind" className="w-full">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        {kinds.map((k) => (
                          <SelectItem key={k} value={k}>
                            {kindLabels[k]}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  )}
                />
              </Field>
            </div>
            <Field data-invalid={patternError ? true : undefined}>
                  <FieldLabel htmlFor="filter-pattern">{text.pattern}</FieldLabel>
                  <Input
                    id="filter-pattern"
                    className="font-mono"
                    autoComplete="off"
                    spellCheck={false}
                    aria-invalid={patternError ? true : undefined}
                    aria-describedby={patternDescription}
                    {...register('pattern')}
                  />
                  {patternError ? (
                    <FieldError id="pattern-error">
                      {patternError.message}
                    </FieldError>
                  ) : (
                    <FieldDescription id="pattern-hint">
                      {kindHints[kind]}
                    </FieldDescription>
                  )}
            </Field>
            <Field>
              <FieldLabel htmlFor="filter-note">{text.note}</FieldLabel>
              <Input
                id="filter-note"
                autoComplete="off"
                aria-invalid={formState.errors.note ? true : undefined}
                {...register('note')}
              />
              <FieldError errors={[formState.errors.note]} />
            </Field>
            <Field orientation="horizontal">
              <Controller
                control={control}
                name="enabled"
                render={({ field }) => (
                  <Switch
                    id="filter-enabled"
                    checked={field.value}
                    onCheckedChange={field.onChange}
                  />
                )}
              />
              <FieldLabel htmlFor="filter-enabled">{text.enabled}</FieldLabel>
            </Field>
          </FieldGroup>
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={() => onOpenChange(false)}
            >
              {text.cancel}
            </Button>
            <Button type="submit" disabled={save.isPending}>
              {save.isPending ? text.saving : text.save}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
