import { KeyRoundIcon, PlusIcon, TriangleAlertIcon } from 'lucide-react'
import { useState } from 'react'
import { toast } from 'sonner'
import { useDeleteKey, useKeys, useRevealKey, type ApiKey } from '@/api/keys'
import { PageHeader } from '@/components/app/page-header'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
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
import {
  Empty,
  EmptyContent,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from '@/components/ui/empty'
import { Skeleton } from '@/components/ui/skeleton'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { CreateKeyDialog } from './create-dialog'
import { keyStatus, type KeyStatus } from './format'
import { KeyList, type KeyAction } from './key-list'
import { LimitsDialog } from './limits-dialog'
import { ModelsDialog } from './models-dialog'
import { SecretDialog, type SecretView } from './secret-dialog'
import { s } from './strings'
import { UsageDialog } from './usage-dialog'

type Filter = 'all' | KeyStatus
const filterOrder: Filter[] = ['all', 'active', 'disabled', 'expired']

type Target = { action: KeyAction; key: ApiKey } | null

function LoadingState() {
  return (
    <div role="status" aria-label={s.loading} className="flex flex-col gap-3">
      <Skeleton className="h-10 w-80 max-w-full" />
      <Skeleton className="h-16" />
      <Skeleton className="h-16" />
      <Skeleton className="h-16" />
    </div>
  )
}

export default function KeysPage() {
  const keys = useKeys()
  const [filter, setFilter] = useState<Filter>('all')
  const [creating, setCreating] = useState(false)
  const [target, setTarget] = useState<Target>(null)
  // The only place a full key is held: this state, cleared when the dialog closes.
  const [secret, setSecret] = useState<SecretView | null>(null)
  const reveal = useRevealKey()
  const remove = useDeleteKey()

  const list = keys.data ?? []
  const counts: Record<Filter, number> = { all: list.length, active: 0, disabled: 0, expired: 0 }
  for (const k of list) counts[keyStatus(k)]++
  const shown = filter === 'all' ? list : list.filter((k) => keyStatus(k) === filter)

  const closeTarget = () => setTarget(null)

  function confirmReveal(k: ApiKey) {
    reveal.mutate(
      {
        id: k.id,
        onSecret: (value) =>
          setSecret({
            title: s.secretTitleReveal,
            description: s.secretDesc(k.name),
            value,
          }),
      },
      { onError: (err) => toast.error(`${s.revealFailed}: ${err.message}`) },
    )
    closeTarget()
  }

  function confirmDelete(k: ApiKey) {
    remove.mutate(
      { id: k.id },
      {
        onSuccess: () => toast.success(s.deleted(k.name)),
        onError: (err) => toast.error(`${s.deleteFailed}: ${err.message}`),
      },
    )
    closeTarget()
  }

  const dialogKey = target?.key
  let body
  if (keys.isPending) {
    body = <LoadingState />
  } else if (keys.isError) {
    body = (
      <Alert variant="destructive">
        <TriangleAlertIcon aria-hidden="true" />
        <AlertTitle>{s.loadFailed}</AlertTitle>
        <AlertDescription>
          <p>{keys.error.message}</p>
          <Button variant="outline" size="sm" className="mt-3" onClick={() => void keys.refetch()}>
            {s.retry}
          </Button>
        </AlertDescription>
      </Alert>
    )
  } else if (list.length === 0) {
    body = (
      <Empty className="rounded-lg border border-dashed">
        <EmptyHeader>
          <EmptyMedia variant="icon">
            <KeyRoundIcon aria-hidden="true" />
          </EmptyMedia>
          <EmptyTitle>{s.emptyTitle}</EmptyTitle>
          <EmptyDescription>{s.emptyBody}</EmptyDescription>
        </EmptyHeader>
        <EmptyContent>
          <Button onClick={() => setCreating(true)}>
            <PlusIcon aria-hidden="true" />
            {s.createFirst}
          </Button>
        </EmptyContent>
      </Empty>
    )
  } else {
    body = (
      <div className="flex flex-col gap-4">
        <Tabs value={filter} onValueChange={(v) => setFilter(v as Filter)}>
          <div className="overflow-x-auto sm:overflow-visible">
            <TabsList aria-label={s.filterLabel}>
              {filterOrder.map((f) => (
                <TabsTrigger key={f} value={f}>
                  {s.filters[f]}
                  <span className="ml-1.5 text-xs tabular-nums text-muted-foreground">
                    {counts[f]}
                  </span>
                </TabsTrigger>
              ))}
            </TabsList>
          </div>
        </Tabs>
        {shown.length === 0 ? (
          <p className="py-8 text-center text-sm text-muted-foreground">{s.emptyFilter}</p>
        ) : (
          <KeyList keys={shown} onAction={(action, key) => setTarget({ action, key })} />
        )}
      </div>
    )
  }

  return (
    <div className="mx-auto flex max-w-7xl flex-col gap-6">
      <PageHeader
        title={s.title}
        description={s.description}
        actions={
          <Button onClick={() => setCreating(true)}>
            <PlusIcon aria-hidden="true" />
            {s.create}
          </Button>
        }
      />
      {body}

      {creating ? (
        <CreateKeyDialog
          onOpenChange={setCreating}
          onCreated={(value, name) => {
            setSecret({
              title: s.secretTitleNew,
              description: s.secretDesc(name),
              value,
            })
            setCreating(false)
          }}
        />
      ) : null}
      {secret ? <SecretDialog secret={secret} onClose={() => setSecret(null)} /> : null}

      {dialogKey && target?.action === 'usage' ? (
        <UsageDialog apiKey={dialogKey} onClose={closeTarget} />
      ) : null}
      {dialogKey && target?.action === 'models' ? (
        <ModelsDialog apiKey={dialogKey} onClose={closeTarget} />
      ) : null}
      {dialogKey && target?.action === 'limits' ? (
        <LimitsDialog apiKey={dialogKey} onClose={closeTarget} />
      ) : null}

      <AlertDialog
        open={dialogKey !== undefined && target?.action === 'reveal'}
        onOpenChange={(o) => !o && closeTarget()}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{s.revealTitle(dialogKey?.name ?? '')}</AlertDialogTitle>
            <AlertDialogDescription>{s.revealBody}</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{s.cancel}</AlertDialogCancel>
            <AlertDialogAction onClick={() => dialogKey && confirmReveal(dialogKey)}>
              {s.revealConfirm}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      <AlertDialog
        open={dialogKey !== undefined && target?.action === 'delete'}
        onOpenChange={(o) => !o && closeTarget()}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{s.deleteTitle(dialogKey?.name ?? '')}</AlertDialogTitle>
            <AlertDialogDescription>{s.deleteBody}</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{s.cancel}</AlertDialogCancel>
            <AlertDialogAction
              variant="destructive"
              onClick={() => dialogKey && confirmDelete(dialogKey)}
            >
              {s.deleteConfirm}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}
