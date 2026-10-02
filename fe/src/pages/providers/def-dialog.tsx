import { zodResolver } from '@hookform/resolvers/zod'
import { Controller, useForm } from 'react-hook-form'
import { toast } from 'sonner'
import { z } from 'zod'
import { isApiError } from '@/api/client'
import { type ProviderDef, useSaveDef } from '@/api/providers'
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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Spinner } from '@/components/ui/spinner'
import { Textarea } from '@/components/ui/textarea'
import { fieldOfError, messageOf } from './format'

const schema = z.object({
  id: z
    .string()
    .trim()
    .regex(/^[a-z0-9][a-z0-9-]{0,39}$/, 'Chữ thường, số và dấu -, tối đa 40 ký tự'),
  name: z.string().trim(),
  color: z.string().trim(),
  icon: z.string().trim(),
  kind: z.string(),
  api: z.string(),
  baseUrl: z.string().trim().min(1, 'Nhập địa chỉ gốc của API'),
  modelsUrl: z.string().trim(),
  authHeader: z.string().trim(),
  authPrefix: z.string(),
  models: z.string(),
})

type Values = z.infer<typeof schema>
type Name = keyof Values
const names = Object.keys(schema.shape) as Name[]

const kinds = [
  ['apikey', 'Khóa API'],
  ['oauth-code', 'OAuth (mã xác thực)'],
  ['oauth-device', 'OAuth (thiết bị)'],
] as const

const apis = [
  ['openai', 'OpenAI Chat Completions'],
  ['anthropic', 'Anthropic Messages'],
  ['responses', 'OpenAI Responses'],
  ['typesafe', 'TypeSafe (System One)'],
] as const

function initial(base?: ProviderDef): Values {
  return {
    id: base?.id ?? '',
    name: base?.name ?? '',
    color: base?.color ?? '',
    icon: base?.icon ?? '',
    kind: base?.kind ?? 'apikey',
    api: base?.api ?? 'openai',
    baseUrl: base?.baseUrl ?? '',
    modelsUrl: base?.modelsUrl ?? '',
    authHeader: base?.authHeader ?? '',
    authPrefix: base?.authPrefix ?? '',
    models: (base?.models ?? []).join('\n'),
  }
}

interface DefDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  /** The full definition being edited (with secrets, held in memory only); absent when creating. */
  base?: ProviderDef
  onSaved: (id: string) => void
}

export function DefDialog({ open, onOpenChange, base, onSaved }: DefDialogProps) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[90svh] overflow-y-auto sm:max-w-xl">
        <DefForm base={base} onClose={() => onOpenChange(false)} onSaved={onSaved} />
      </DialogContent>
    </Dialog>
  )
}

function DefForm({
  base,
  onClose,
  onSaved,
}: {
  base?: ProviderDef
  onClose: () => void
  onSaved: (id: string) => void
}) {
  const editing = base !== undefined
  const save = useSaveDef()
  const {
    control,
    register,
    handleSubmit,
    setError,
    formState: { errors },
  } = useForm<Values>({
    resolver: zodResolver(schema),
    defaultValues: initial(base),
  })

  const onSubmit = (v: Values) => {
    const models = v.models
      .split('\n')
      .map((m) => m.trim())
      .filter(Boolean)
    // Headers and OAuth settings are not edited here; they travel with the
    // definition unchanged so a save does not drop them.
    const def: ProviderDef = {
      ...base,
      id: v.id,
      name: v.name,
      color: v.color,
      icon: v.icon,
      kind: v.kind,
      api: v.api,
      baseUrl: v.baseUrl,
      modelsUrl: v.modelsUrl,
      authHeader: v.authHeader,
      authPrefix: v.authPrefix,
      models,
    }
    save.mutate(def, {
      onSuccess: () => {
        toast.success(editing ? 'Đã lưu nhà cung cấp.' : 'Đã thêm nhà cung cấp.')
        onSaved(v.id)
        onClose()
      },
      onError: (e) => {
        const message = messageOf(e)
        const field = isApiError(e) && e.status === 400 ? fieldOfError(message, names) : undefined
        if (field) setError(field, { message })
        else setError('root', { message })
      },
    })
  }

  const text = (name: Name, label: string, hint?: string, extra?: { placeholder?: string; disabled?: boolean }) => (
    <Field data-invalid={errors[name] ? true : undefined}>
      <FieldLabel htmlFor={`def-${name}`}>{label}</FieldLabel>
      <Input
        id={`def-${name}`}
        autoComplete="off"
        spellCheck={false}
        aria-invalid={errors[name] ? true : undefined}
        {...extra}
        {...register(name)}
      />
      {hint ? <FieldDescription>{hint}</FieldDescription> : null}
      {errors[name] ? <FieldError errors={[errors[name]]} /> : null}
    </Field>
  )

  const choice = (name: 'kind' | 'api', label: string, options: readonly (readonly [string, string])[]) => (
    <Controller
      control={control}
      name={name}
      render={({ field }) => (
        <Field data-invalid={errors[name] ? true : undefined}>
          <FieldLabel htmlFor={`def-${name}`}>{label}</FieldLabel>
          <Select value={field.value} onValueChange={field.onChange}>
            <SelectTrigger id={`def-${name}`} className="w-full" aria-invalid={errors[name] ? true : undefined}>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {options.map(([value, text]) => (
                <SelectItem key={value} value={value}>
                  {text}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          {errors[name] ? <FieldError errors={[errors[name]]} /> : null}
        </Field>
      )}
    />
  )

  return (
    <form onSubmit={handleSubmit(onSubmit)} noValidate className="flex flex-col gap-6">
      <DialogHeader>
        <DialogTitle>
          {editing ? `Sửa nhà cung cấp ${base.id}` : 'Thêm nhà cung cấp tùy chỉnh'}
        </DialogTitle>
        <DialogDescription>
          Khai báo một nhà cung cấp của riêng bạn. Header và cấu hình OAuth đã
          có được giữ nguyên khi lưu.
        </DialogDescription>
      </DialogHeader>
      <FieldGroup>
        {text('id', 'Mã nhà cung cấp', 'Tiền tố của tên model, ví dụ acme/model-1.', {
          disabled: editing,
          placeholder: 'acme',
        })}
        {text('name', 'Tên hiển thị')}
        <div className="grid gap-6 sm:grid-cols-2">
          {choice('kind', 'Cách đăng nhập', kinds)}
          {choice('api', 'Kiểu API', apis)}
        </div>
        {text('baseUrl', 'Địa chỉ gốc (baseUrl)', undefined, {
          placeholder: 'https://api.example.com/v1',
        })}
        {text('modelsUrl', 'Địa chỉ danh sách model', 'Để trống để dùng baseUrl + /models; nhập none nếu không có danh sách.')}
        <div className="grid gap-6 sm:grid-cols-2">
          {text('authHeader', 'Header xác thực')}
          {text('authPrefix', 'Tiền tố xác thực')}
        </div>
        <div className="grid gap-6 sm:grid-cols-2">
          {text('color', 'Màu', 'Dạng #rrggbb.', { placeholder: '#336699' })}
          {text('icon', 'Biểu tượng (URL)', 'Địa chỉ https: hoặc data:image/.')}
        </div>
        <Field data-invalid={errors.models ? true : undefined}>
          <FieldLabel htmlFor="def-models">Model dự phòng</FieldLabel>
          <Textarea id="def-models" rows={3} spellCheck={false} {...register('models')} />
          <FieldDescription>Mỗi dòng một model, dùng khi không đọc được danh sách.</FieldDescription>
        </Field>
      </FieldGroup>
      {errors.root?.message ? (
        <Alert variant="destructive">
          <AlertDescription>{errors.root.message}</AlertDescription>
        </Alert>
      ) : null}
      <DialogFooter>
        <Button type="button" variant="outline" onClick={onClose}>
          Hủy
        </Button>
        <Button type="submit" disabled={save.isPending}>
          {save.isPending ? <Spinner aria-hidden="true" /> : null}
          Lưu
        </Button>
      </DialogFooter>
    </form>
  )
}
