import { useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { accountModels, type Account } from '@/api/accounts'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
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
import { Field, FieldError, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Skeleton } from '@/components/ui/skeleton'
import { text } from './strings'

interface RenameProps {
  account: Account | null
  onClose: () => void
  onSave: (id: string, label: string) => void
}

export function RenameDialog({ account, onClose, onSave }: RenameProps) {
  return (
    <Dialog open={account !== null} onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="sm:max-w-sm">
        {account ? (
          <RenameForm key={account.id} account={account} onClose={onClose} onSave={onSave} />
        ) : null}
      </DialogContent>
    </Dialog>
  )
}

function RenameForm({
  account,
  onClose,
  onSave,
}: {
  account: Account
  onClose: () => void
  onSave: RenameProps['onSave']
}) {
  const [value, setValue] = useState(account.label)
  const [error, setError] = useState<string>()
  return (
    <form
      noValidate
      className="flex flex-col gap-4"
      onSubmit={(e) => {
        e.preventDefault()
        const label = value.trim()
        if (!label) {
          setError(text.rename.required)
          return
        }
        onSave(account.id, label)
        onClose()
      }}
    >
      <DialogHeader>
        <DialogTitle>{text.rename.title}</DialogTitle>
        <DialogDescription>{account.provider}</DialogDescription>
      </DialogHeader>
      <Field data-invalid={error ? true : undefined}>
        <FieldLabel htmlFor="rename-label">{text.rename.label}</FieldLabel>
        <Input
          id="rename-label"
          value={value}
          autoFocus
          aria-invalid={error ? true : undefined}
          onChange={(e) => {
            setValue(e.target.value)
            setError(undefined)
          }}
        />
        <FieldError>{error}</FieldError>
      </Field>
      <DialogFooter>
        <Button type="button" variant="outline" onClick={onClose}>
          {text.cancel}
        </Button>
        <Button type="submit">{text.rename.save}</Button>
      </DialogFooter>
    </form>
  )
}

interface RemoveProps {
  account: Account | null
  onClose: () => void
  onConfirm: (a: Account) => void
}

export function RemoveDialog({ account, onClose, onConfirm }: RemoveProps) {
  return (
    <AlertDialog open={account !== null} onOpenChange={(o) => !o && onClose()}>
      <AlertDialogContent>
        {account ? (
          <>
            <AlertDialogHeader>
              <AlertDialogTitle>{text.remove.title}</AlertDialogTitle>
              <AlertDialogDescription>
                {text.remove.body(account.label, account.provider)}
              </AlertDialogDescription>
            </AlertDialogHeader>
            <AlertDialogFooter>
              <AlertDialogCancel>{text.remove.cancel}</AlertDialogCancel>
              <AlertDialogAction
                variant="destructive"
                onClick={() => onConfirm(account)}
              >
                {text.remove.confirm}
              </AlertDialogAction>
            </AlertDialogFooter>
          </>
        ) : null}
      </AlertDialogContent>
    </AlertDialog>
  )
}

export function ModelsDialog({
  account,
  onClose,
}: {
  account: Account | null
  onClose: () => void
}) {
  return (
    <Dialog open={account !== null} onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="sm:max-w-md">
        {account ? <ModelList account={account} /> : null}
      </DialogContent>
    </Dialog>
  )
}

function ModelList({ account }: { account: Account }) {
  const q = useQuery({
    queryKey: ['accounts', account.id, 'models'],
    queryFn: ({ signal }) => accountModels(account.id, signal),
    retry: false,
  })
  return (
    <>
      <DialogHeader>
        <DialogTitle>{text.models.title(account.label)}</DialogTitle>
        <DialogDescription>
          {q.data ? text.models.count(q.data.length) : account.provider}
        </DialogDescription>
      </DialogHeader>
      {q.isPending ? (
        <div role="status" aria-label={text.loading} className="flex flex-col gap-2">
          <Skeleton className="h-6" />
          <Skeleton className="h-6" />
          <Skeleton className="h-6" />
        </div>
      ) : q.isError ? (
        <Alert variant="destructive" role="alert">
          <AlertDescription className="flex items-center justify-between gap-2">
            {q.error.message}
            <Button size="sm" variant="outline" onClick={() => void q.refetch()}>
              {text.retry}
            </Button>
          </AlertDescription>
        </Alert>
      ) : q.data.length === 0 ? (
        <p className="text-sm text-muted-foreground">{text.models.empty}</p>
      ) : (
        <ScrollArea className="max-h-72 rounded-md border">
          <ul className="divide-y text-sm">
            {q.data.map((id) => (
              <li key={id} className="px-3 py-2 font-mono text-xs">
                {id}
              </li>
            ))}
          </ul>
        </ScrollArea>
      )}
    </>
  )
}
