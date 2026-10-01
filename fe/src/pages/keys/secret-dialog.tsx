import { CheckIcon, CopyIcon, TriangleAlertIcon } from 'lucide-react'
import { useState } from 'react'
import { toast } from 'sonner'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { s } from './strings'

export interface SecretView {
  title: string
  description: string
  value: string
}

// SecretDialog shows a full key. It cannot be dismissed (Escape, a click
// outside, a close button) until the person confirms the key is saved. The
// value lives only in this dialog's props, held by the page's local state.
export function SecretDialog({
  secret,
  onClose,
}: {
  secret: SecretView
  onClose: () => void
}) {
  const [saved, setSaved] = useState(false)
  const [copied, setCopied] = useState(false)

  async function copy() {
    try {
      await navigator.clipboard.writeText(secret.value)
      setCopied(true)
      toast.success(s.copied)
    } catch {
      toast.error(s.copyFailed)
    }
  }

  return (
    <Dialog open onOpenChange={() => {}}>
      <DialogContent
        showCloseButton={false}
        onEscapeKeyDown={(e) => e.preventDefault()}
        onPointerDownOutside={(e) => e.preventDefault()}
        onInteractOutside={(e) => e.preventDefault()}
      >
        <DialogHeader>
          <DialogTitle>{secret.title}</DialogTitle>
          <DialogDescription>{secret.description}</DialogDescription>
        </DialogHeader>
        <Alert>
          <TriangleAlertIcon aria-hidden="true" />
          <AlertDescription>{s.secretWarn}</AlertDescription>
        </Alert>
        <div className="flex flex-col gap-2">
          <Label htmlFor="secret-key">{s.secretField}</Label>
          <div className="flex gap-2">
            <Input
              id="secret-key"
              readOnly
              value={secret.value}
              className="font-mono text-xs"
              autoComplete="off"
              spellCheck={false}
              onFocus={(e) => e.currentTarget.select()}
            />
            <Button type="button" variant="outline" onClick={copy}>
              {copied ? <CheckIcon aria-hidden="true" /> : <CopyIcon aria-hidden="true" />}
              {s.copy}
            </Button>
          </div>
        </div>
        <div className="flex items-center gap-2">
          <Checkbox
            id="secret-saved"
            checked={saved}
            onCheckedChange={(v) => setSaved(v === true)}
          />
          <Label htmlFor="secret-saved">{s.savedConfirm}</Label>
        </div>
        <DialogFooter>
          <Button type="button" disabled={!saved} onClick={onClose}>
            {s.close}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
