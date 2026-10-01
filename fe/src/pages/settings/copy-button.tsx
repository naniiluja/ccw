import { CheckIcon, CopyIcon } from 'lucide-react'
import { useState } from 'react'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'

interface CopyButtonProps {
  text: string
  /** Accessible name, for example "Sao chép cấu hình Codex". */
  label: string
}

// CopyButton writes text to the clipboard and answers with a toast.
export function CopyButton({ text, label }: CopyButtonProps) {
  const [done, setDone] = useState(false)

  async function copy() {
    try {
      await navigator.clipboard.writeText(text)
      toast.success('Đã sao chép')
      setDone(true)
      setTimeout(() => setDone(false), 2000)
    } catch {
      toast.error('Không sao chép được. Hãy chọn và sao chép thủ công.')
    }
  }

  return (
    <Button
      type="button"
      variant="outline"
      size="sm"
      aria-label={label}
      onClick={copy}
    >
      {done ? (
        <CheckIcon aria-hidden="true" />
      ) : (
        <CopyIcon aria-hidden="true" />
      )}
      <span className="hidden sm:inline">Sao chép</span>
    </Button>
  )
}
