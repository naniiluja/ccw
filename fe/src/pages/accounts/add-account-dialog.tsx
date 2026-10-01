import { useQuery, useQueryClient } from '@tanstack/react-query'
import { KeyRoundIcon, LogInIcon, MonitorSmartphoneIcon, Loader2Icon } from 'lucide-react'
import { useState } from 'react'
import { useForm } from 'react-hook-form'
import { toast } from 'sonner'
import {
  accountProvidersQuery,
  accountsQuery,
  createAccount,
  type ProviderInfo,
} from '@/api/accounts'
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
import { Field, FieldDescription, FieldError, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { RadioGroup, RadioGroupItem } from '@/components/ui/radio-group'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Skeleton } from '@/components/ui/skeleton'
import { OAuthFlow } from './oauth-flow'
import { text } from './strings'

const t = text.dialog
const CUSTOM = '__custom__'

type Kind = 'api' | 'oauth' | 'device'

const isDevice = (p: ProviderInfo) => p.id === 'github' || p.flow === 'device'
const inKind = (p: ProviderInfo, kind: Kind) =>
  kind === 'device' ? isDevice(p) : kind === 'oauth' ? p.setup === 'oauth' && !isDevice(p) : p.setup !== 'oauth'
const nameOf = (p: ProviderInfo) => p.id

const kinds: { id: Kind; icon: typeof KeyRoundIcon }[] = [
  { id: 'api', icon: KeyRoundIcon },
  { id: 'oauth', icon: LogInIcon },
  { id: 'device', icon: MonitorSmartphoneIcon },
]

export function AddAccountDialog({
  open,
  onOpenChange,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[90svh] overflow-y-auto sm:max-w-md">
        <Wizard onClose={() => onOpenChange(false)} />
      </DialogContent>
    </Dialog>
  )
}

function Wizard({ onClose }: { onClose: () => void }) {
  const qc = useQueryClient()
  const providers = useQuery(accountProvidersQuery)
  const [step, setStep] = useState<'kind' | 'form'>('kind')
  const [kind, setKind] = useState<Kind>('api')
  const [provider, setProvider] = useState('')

  const options = (providers.data ?? []).filter((p) => inKind(p, kind))
  const done = () => {
    toast.success(text.toast.added)
    void qc.invalidateQueries({ queryKey: accountsQuery.queryKey })
    onClose()
  }

  return (
    <>
      <DialogHeader>
        <DialogTitle>{t.title}</DialogTitle>
        <DialogDescription>{step === 'kind' ? t.pick : t.kinds[kind].hint}</DialogDescription>
      </DialogHeader>
      {step === 'kind' ? (
        <div className="flex flex-col gap-4">
          {/* Card-style radio group, following the structure of @originui/comp-163. */}
          <RadioGroup
            aria-label={t.kindGroup}
            className="grid-cols-1 sm:grid-cols-3"
            value={kind}
            onValueChange={(v) => {
              setKind(v as Kind)
              setProvider('')
            }}
          >
            {kinds.map(({ id, icon: Icon }) => (
              <label
                key={id}
                className="relative flex cursor-pointer flex-col items-center gap-2 rounded-md border border-input px-2 py-3 text-center shadow-xs outline-none transition-[color,box-shadow] has-data-[state=checked]:border-primary/50 has-data-[state=checked]:bg-primary/5 has-focus-visible:border-ring has-focus-visible:ring-[3px] has-focus-visible:ring-ring/50"
              >
                <RadioGroupItem value={id} className="sr-only" />
                <Icon className="size-5 opacity-60" aria-hidden="true" />
                <span className="text-xs font-medium leading-none">{t.kinds[id].title}</span>
                <span className="text-xs text-muted-foreground">{t.kinds[id].hint}</span>
              </label>
            ))}
          </RadioGroup>
          <DialogFooter>
            <Button variant="outline" onClick={onClose}>
              {text.cancel}
            </Button>
            <Button onClick={() => setStep('form')}>{t.next}</Button>
          </DialogFooter>
        </div>
      ) : (
        <div className="flex flex-col gap-4">
          <Field>
            <FieldLabel htmlFor="add-provider">{t.provider}</FieldLabel>
            {providers.isPending ? (
              <Skeleton className="h-9" />
            ) : (
              <Select value={provider} onValueChange={setProvider}>
                <SelectTrigger id="add-provider" className="w-full">
                  <SelectValue placeholder={t.pickProvider} />
                </SelectTrigger>
                <SelectContent>
                  {options.map((p) => (
                    <SelectItem key={p.id} value={p.id}>
                      {nameOf(p)}
                    </SelectItem>
                  ))}
                  {kind === 'api' ? (
                    <SelectItem value={CUSTOM}>{t.custom}</SelectItem>
                  ) : null}
                </SelectContent>
              </Select>
            )}
            {providers.isError ? <FieldError>{t.providersFailed}</FieldError> : null}
            {providers.isSuccess && options.length === 0 && kind !== 'api' ? (
              <FieldDescription>{t.noProviders}</FieldDescription>
            ) : null}
          </Field>
          {provider && kind === 'api' ? (
            <KeyForm
              key={provider}
              provider={provider}
              info={options.find((p) => p.id === provider)}
              onDone={done}
            />
          ) : null}
          {provider && kind !== 'api' ? (
            <OAuthFlow key={provider} provider={provider} onDone={done} />
          ) : null}
          <DialogFooter>
            <Button variant="outline" onClick={() => setStep('kind')}>
              {t.back}
            </Button>
          </DialogFooter>
        </div>
      )}
    </>
  )
}

interface Values {
  customId: string
  api: string
  baseUrl: string
  accountId: string
  secret: string
  label: string
}

type FieldName = keyof Values

// fieldFor picks the field that the server's 400 message is about.
function fieldFor(message: string): FieldName | null {
  const m = message.toLowerCase()
  if (m.includes('account id')) return 'accountId'
  if (m.includes('provider id')) return 'customId'
  if (m.includes('base url')) return 'baseUrl'
  if (m.startsWith('api')) return 'api'
  if (m.includes('key')) return 'secret'
  return null
}

function KeyForm({
  provider,
  info,
  onDone,
}: {
  provider: string
  info?: ProviderInfo
  onDone: () => void
}) {
  const custom = provider === CUSTOM
  const setup = custom ? 'custom' : (info?.setup ?? 'key')
  const needsKey = setup === 'key' || setup === 'account'
  const form = useForm<Values>({
    defaultValues: { customId: '', api: 'openai', baseUrl: '', accountId: '', secret: '', label: '' },
  })
  const { errors, isSubmitting } = form.formState
  const [failure, setFailure] = useState<string>()

  async function onSubmit(v: Values) {
    setFailure(undefined)
    const secret = v.secret
    // The key leaves the form state as it is sent, whatever the answer is.
    form.setValue('secret', '')
    try {
      await createAccount({
        provider: custom ? v.customId.trim() : provider,
        label: v.label.trim(),
        secret,
        account_id: setup === 'account' ? v.accountId.trim() : undefined,
        base_url: custom || setup === 'key' ? v.baseUrl.trim() : undefined,
        api: custom ? v.api : undefined,
      })
      onDone()
    } catch (e) {
      const msg = e instanceof Error ? e.message : text.unknownError
      const field = fieldFor(msg)
      if (field) form.setError(field, { message: msg })
      else setFailure(msg)
    }
  }

  return (
    <form noValidate onSubmit={form.handleSubmit(onSubmit)} className="flex flex-col gap-4">
      {custom ? (
        <>
          <Field data-invalid={errors.customId ? true : undefined}>
            <FieldLabel htmlFor="add-custom-id">{t.customId}</FieldLabel>
            <Input
              id="add-custom-id"
              autoComplete="off"
              aria-invalid={errors.customId ? true : undefined}
              {...form.register('customId', { required: t.customId })}
            />
            <FieldDescription>{t.customIdHint}</FieldDescription>
            <FieldError errors={[errors.customId]} />
          </Field>
          <Field>
            <FieldLabel htmlFor="add-custom-api">{t.customApi}</FieldLabel>
            <select
              id="add-custom-api"
              className="h-9 rounded-md border border-input bg-transparent px-2 text-sm"
              {...form.register('api')}
            >
              <option value="openai">OpenAI</option>
              <option value="anthropic">Anthropic</option>
              <option value="responses">Responses</option>
            </select>
          </Field>
        </>
      ) : null}
      {setup === 'account' ? (
        <Field data-invalid={errors.accountId ? true : undefined}>
          <FieldLabel htmlFor="add-account-id">{t.accountId}</FieldLabel>
          <Input
            id="add-account-id"
            autoComplete="off"
            aria-invalid={errors.accountId ? true : undefined}
            {...form.register('accountId', { required: t.accountIdRequired })}
          />
          <FieldError errors={[errors.accountId]} />
        </Field>
      ) : null}
      {needsKey || custom ? (
        <Field data-invalid={errors.secret ? true : undefined}>
          <FieldLabel htmlFor="add-secret">{t.secret}</FieldLabel>
          <Input
            id="add-secret"
            type="password"
            autoComplete="off"
            aria-invalid={errors.secret ? true : undefined}
            {...form.register('secret', { required: custom ? false : t.secretRequired })}
          />
          <FieldError errors={[errors.secret]} />
        </Field>
      ) : (
        <FieldDescription>{t.noKey}</FieldDescription>
      )}
      {custom || setup === 'key' ? (
        <Field data-invalid={errors.baseUrl ? true : undefined}>
          <FieldLabel htmlFor="add-base-url">
            {custom ? t.baseUrl : t.baseUrlOptional}
          </FieldLabel>
          <Input
            id="add-base-url"
            type="url"
            inputMode="url"
            autoComplete="off"
            placeholder="https://"
            aria-invalid={errors.baseUrl ? true : undefined}
            {...form.register('baseUrl', { required: custom ? t.baseUrlRequired : false })}
          />
          <FieldError errors={[errors.baseUrl]} />
        </Field>
      ) : null}
      <Field>
        <FieldLabel htmlFor="add-label">{t.label}</FieldLabel>
        <Input id="add-label" autoComplete="off" {...form.register('label')} />
      </Field>
      {failure ? (
        <Alert variant="destructive" role="alert">
          <AlertDescription>{failure}</AlertDescription>
        </Alert>
      ) : null}
      <Button type="submit" disabled={isSubmitting}>
        {isSubmitting ? <Loader2Icon className="animate-spin" aria-hidden="true" /> : null}
        {isSubmitting ? t.submitting : t.submit}
      </Button>
    </form>
  )
}
