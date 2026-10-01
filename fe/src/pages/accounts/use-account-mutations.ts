import { useMutation, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import {
  type Account,
  type AccountTest,
  accountsQuery,
  accountTestsQuery,
  deleteAccount,
  setActive,
  setLabel,
  testAccount,
} from '@/api/accounts'
import { text } from './strings'

const reason = (e: unknown) => (e instanceof Error ? e.message : text.unknownError)

// useAccountMutations holds every write of the page. Only the active switch and
// the label are optimistic: the list changes at once and returns to what it was
// when the server refuses. Each write ends by loading the list again.
export function useAccountMutations() {
  const qc = useQueryClient()
  const key = accountsQuery.queryKey

  async function patch(id: string, change: Partial<Account>) {
    await qc.cancelQueries({ queryKey: key })
    const before = qc.getQueryData<Account[]>(key)
    qc.setQueryData<Account[]>(key, (list) =>
      list?.map((a) => (a.id === id ? { ...a, ...change } : a)),
    )
    return { before }
  }
  const rollback = (ctx?: { before?: Account[] }) => {
    if (ctx?.before) qc.setQueryData(key, ctx.before)
  }
  const settle = () => qc.invalidateQueries({ queryKey: key })

  const active = useMutation({
    mutationFn: (v: { id: string; active: boolean; standby?: boolean }) =>
      setActive(v.id, v.active, v.standby),
    onMutate: (v) =>
      patch(v.id, {
        isActive: v.active,
        ...(v.standby !== undefined || !v.active
          ? { standby: v.active && v.standby === true }
          : {}),
      }),
    onError: (e, _v, ctx) => {
      rollback(ctx)
      toast.error(text.toast.activeFailed(reason(e)))
    },
    onSettled: settle,
  })

  const label = useMutation({
    mutationFn: (v: { id: string; label: string }) => setLabel(v.id, v.label),
    onMutate: (v) => patch(v.id, { label: v.label }),
    onSuccess: () => toast.success(text.toast.renamed),
    onError: (e, _v, ctx) => {
      rollback(ctx)
      toast.error(text.toast.renameFailed(reason(e)))
    },
    onSettled: settle,
  })

  const remove = useMutation({
    mutationFn: (v: { id: string; label: string }) => deleteAccount(v.id),
    onSuccess: (_d, v) => toast.success(text.toast.deleted(v.label)),
    onError: (e) => toast.error(text.toast.deleteFailed(reason(e))),
    onSettled: settle,
  })

  const test = useMutation({
    mutationFn: (v: { id: string; label: string }) => testAccount(v.id),
    onSuccess: (res, v) => {
      qc.setQueryData<Record<string, AccountTest>>(
        accountTestsQuery.queryKey,
        (m) => ({ ...m, [v.id]: res }),
      )
      if (res.ok) toast.success(text.toast.testOk(v.label, res.ms))
      else toast.error(text.toast.testBad(v.label, res.ms, res.message))
    },
    onError: (e, v) => toast.error(text.toast.testFailed(v.label, reason(e))),
    onSettled: () => qc.invalidateQueries({ queryKey: accountTestsQuery.queryKey }),
  })

  return { active, label, remove, test }
}
