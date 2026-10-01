import { useState, type FormEvent } from 'react'
import { toast } from 'sonner'
import { useSetKeyModels, type ApiKey } from '@/api/keys'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Spinner } from '@/components/ui/spinner'
import { ModelPicker } from './model-picker'
import { s } from './strings'

export function ModelsDialog({
  apiKey,
  onClose,
}: {
  apiKey: ApiKey
  onClose: () => void
}) {
  const [models, setModels] = useState(apiKey.models)
  const save = useSetKeyModels()

  function submit(e: FormEvent) {
    e.preventDefault()
    save.mutate(
      { id: apiKey.id, models },
      {
        onSuccess: () => {
          toast.success(s.modelsSaved)
          onClose()
        },
        onError: (err) => toast.error(`${s.modelsFailed}: ${err.message}`),
      },
    )
  }

  return (
    <Dialog open onOpenChange={(o) => !o && !save.isPending && onClose()}>
      <DialogContent className="sm:max-w-lg">
        <form onSubmit={submit} className="grid gap-6">
          <DialogHeader>
            <DialogTitle>{s.modelsTitle}</DialogTitle>
            <DialogDescription>{s.modelsDesc(apiKey.name)}</DialogDescription>
          </DialogHeader>
          <ModelPicker value={models} onChange={setModels} />
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
