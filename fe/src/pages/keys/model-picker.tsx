import { TriangleAlertIcon } from 'lucide-react'
import { useId, useMemo, useState } from 'react'
import { useModelIds } from '@/api/keys'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Skeleton } from '@/components/ui/skeleton'
import { s } from './strings'

// ModelPicker is a multi-select over the models the gateway serves. An empty
// selection means the key may call every model.
export function ModelPicker({
  value,
  onChange,
}: {
  value: string[]
  onChange: (next: string[]) => void
}) {
  const uid = useId()
  const [filter, setFilter] = useState('')
  const models = useModelIds()

  // A model already on the key stays listed even when no provider serves it now.
  const options = useMemo(() => {
    const all = new Set([...(models.data ?? []), ...value])
    return [...all].sort()
  }, [models.data, value])
  const shown = options.filter((m) =>
    m.toLowerCase().includes(filter.trim().toLowerCase()),
  )

  const toggle = (id: string, on: boolean) =>
    onChange(on ? [...value, id] : value.filter((m) => m !== id))

  return (
    <div className="flex flex-col gap-2" role="group" aria-label={s.pickerLabel}>
      <Input
        type="search"
        aria-label={s.pickerSearch}
        placeholder={s.pickerSearch}
        value={filter}
        onChange={(e) => setFilter(e.target.value)}
      />
      <div className="flex items-center justify-between gap-2 text-xs text-muted-foreground">
        <span aria-live="polite">
          {value.length === 0 ? s.pickerHelpAll : s.pickerSelected(value.length)}
        </span>
        {value.length > 0 ? (
          <Button type="button" variant="ghost" size="xs" onClick={() => onChange([])}>
            {s.pickerClear}
          </Button>
        ) : null}
      </div>
      {models.isPending ? (
        <div role="status" aria-label={s.pickerLoading} className="flex flex-col gap-2">
          <Skeleton className="h-6" />
          <Skeleton className="h-6" />
          <Skeleton className="h-6" />
        </div>
      ) : null}
      {models.isError ? (
        <Alert variant="destructive">
          <TriangleAlertIcon aria-hidden="true" />
          <AlertDescription>
            <p>{s.pickerFailed}</p>
            <Button
              type="button"
              variant="outline"
              size="sm"
              className="mt-2"
              onClick={() => void models.refetch()}
            >
              {s.retry}
            </Button>
          </AlertDescription>
        </Alert>
      ) : null}
      {!models.isPending ? (
        <div className="max-h-56 overflow-y-auto rounded-md border p-1">
          {shown.length === 0 ? (
            <p className="p-2 text-sm text-muted-foreground">{s.pickerEmpty}</p>
          ) : (
            <ul>
              {shown.map((m) => {
                const id = `${uid}-${m}`
                return (
                  <li key={m} className="flex items-center gap-2 rounded px-2 py-1.5 hover:bg-muted">
                    <Checkbox
                      id={id}
                      checked={value.includes(m)}
                      onCheckedChange={(v) => toggle(m, v === true)}
                    />
                    <Label htmlFor={id} className="min-w-0 flex-1 font-mono text-xs break-all">
                      {m}
                    </Label>
                  </li>
                )
              })}
            </ul>
          )}
        </div>
      ) : null}
    </div>
  )
}
