import { useState } from 'react'
import { toast } from 'sonner'
import { RotateCcwIcon } from 'lucide-react'
import {
  type AccountQuota,
  type QuotaReset,
  useClaimReset,
} from '@/api/quota'
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
import { Button } from '@/components/ui/button'
import { blockedReason } from './format'

import { type ClaimNotice, noticeOf, noticeOfError } from './notices'

interface Pending {
  account: AccountQuota
  reset: QuotaReset
}

// ResetList lists the resets of one account with a claim button each. Nothing
// renders when the account has no resets.
export function ResetList({
  account,
  onNotice,
}: {
  account: AccountQuota
  onNotice: (n: ClaimNotice) => void
}) {
  const [pending, setPending] = useState<Pending | null>(null)
  const claim = useClaimReset()
  if (!account.resets?.length) return null

  const confirm = () => {
    if (!pending) return
    const { account: a, reset } = pending
    claim.mutate(
      { connectionId: a.connectionId, resetId: reset.id },
      {
        onSuccess: (r) => {
          const n = noticeOf(r)
          onNotice(n)
          toast[n.ok ? 'success' : 'warning'](
            n.ok ? 'Đã gửi claim reset' : 'Claim reset chưa có hiệu lực',
          )
        },
        onError: (e) => {
          onNotice(noticeOfError(e))
          toast.error('Claim reset không thành công')
        },
      },
    )
  }

  return (
    <>
      <ul className="flex flex-col gap-2" aria-label="Lượt reset">
        {account.resets.map((r) => (
          <li
            key={r.id}
            className="flex flex-wrap items-center justify-between gap-2 rounded-lg border p-2.5 text-sm"
          >
            <div className="min-w-0">
              <p className="font-medium">{r.title}</p>
              <p className="text-xs text-muted-foreground tabular-nums">
                Còn {r.left}/{r.total} lượt
                {!r.usable && r.blocked ? ` · ${blockedReason(r.blocked)}` : ''}
              </p>
            </div>
            <Button
              variant="outline"
              size="sm"
              disabled={!r.usable || claim.isPending}
              onClick={() => setPending({ account, reset: r })}
            >
              <RotateCcwIcon data-icon="inline-start" aria-hidden="true" />
              Claim reset
            </Button>
          </li>
        ))}
      </ul>
      <AlertDialog
        open={pending !== null}
        onOpenChange={(open) => {
          if (!open) setPending(null)
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Dùng một lượt reset?</AlertDialogTitle>
            <AlertDialogDescription>
              {pending
                ? `${pending.reset.title} của ${pending.account.label} sẽ xóa ${
                    pending.reset.clears.length
                      ? pending.reset.clears.join(', ')
                      : 'giới hạn hiện tại'
                  }. Thao tác này tiêu một lượt (còn ${pending.reset.left}/${pending.reset.total}) và không thể hoàn tác.`
                : ''}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Hủy</AlertDialogCancel>
            <AlertDialogAction variant="destructive" onClick={confirm}>
              Xác nhận claim
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  )
}
