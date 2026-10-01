import { useState, type FormEvent } from 'react'
import { toast } from 'sonner'
import { useCreateKey } from '@/api/keys'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Field, FieldDescription, FieldGroup, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Spinner } from '@/components/ui/spinner'
import { ModelPicker } from './model-picker'
import { s } from './strings'

export function CreateKeyDialog({
  onOpenChange,
  onCreated,
}: {
  onOpenChange: (open: boolean) => void
  /** Receives the full key and the name it was created under. */
  onCreated: (secret: string, name: string) => void
}) {
  const [name, setName] = useState('')
  const [models, setModels] = useState<string[]>([])
  const create = useCreateKey()

  function submit(e: FormEvent) {
    e.preventDefault()
    const finalName = name.trim() || 'key'
    create.mutate(
      { name: finalName, models, onSecret: (key) => onCreated(key, finalName) },
      {
        onError: (err) => toast.error(`${s.createFailed}: ${err.message}`),
      },
    )
  }

  return (
    <Dialog open onOpenChange={(o) => !create.isPending && onOpenChange(o)}>
      <DialogContent className="sm:max-w-lg">
        <form onSubmit={submit} className="grid gap-6">
          <DialogHeader>
            <DialogTitle>{s.createTitle}</DialogTitle>
            <DialogDescription>{s.createDesc}</DialogDescription>
          </DialogHeader>
          <FieldGroup>
            <Field>
              <FieldLabel htmlFor="key-name">{s.nameLabel}</FieldLabel>
              <Input
                id="key-name"
                value={name}
                maxLength={80}
                placeholder={s.namePlaceholder}
                autoComplete="off"
                onChange={(e) => setName(e.target.value)}
              />
              <FieldDescription>{s.nameHelp}</FieldDescription>
            </Field>
            <Field>
              <FieldLabel>{s.pickerLabel}</FieldLabel>
              <ModelPicker value={models} onChange={setModels} />
            </Field>
          </FieldGroup>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
              {s.cancel}
            </Button>
            <Button type="submit" disabled={create.isPending}>
              {create.isPending ? <Spinner aria-hidden="true" /> : null}
              {create.isPending ? s.creating : s.submitCreate}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
