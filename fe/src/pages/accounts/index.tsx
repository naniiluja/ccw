import { useQuery } from '@tanstack/react-query'
import { PlusIcon, SearchIcon, UsersIcon } from 'lucide-react'
import { useMemo, useState } from 'react'
import { type Account, accountsQuery, accountTestsQuery } from '@/api/accounts'
import { PageHeader } from '@/components/app/page-header'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Empty,
  EmptyContent,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from '@/components/ui/empty'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Skeleton } from '@/components/ui/skeleton'
import { useIsMobile } from '@/hooks/use-mobile'
import { AddAccountDialog } from './add-account-dialog'
import { AccountList, type RowHandlers } from './account-list'
import { ModelsDialog, RemoveDialog, RenameDialog } from './account-dialogs'
import { text } from './strings'
import { useAccountMutations } from './use-account-mutations'

const ALL = '__all__'

function ListSkeleton() {
  return (
    <div role="status" aria-label={text.loading} className="flex flex-col gap-2">
      {[0, 1, 2, 3].map((i) => (
        <Skeleton key={i} className="h-14" />
      ))}
    </div>
  )
}

export default function AccountsPage() {
  const accounts = useQuery(accountsQuery)
  const tests = useQuery(accountTestsQuery)
  const mutations = useAccountMutations()
  const mobile = useIsMobile()
  const [adding, setAdding] = useState(false)
  const [provider, setProvider] = useState(ALL)
  const [search, setSearch] = useState('')
  const [testing, setTesting] = useState<string | null>(null)
  const [renaming, setRenaming] = useState<Account | null>(null)
  const [removing, setRemoving] = useState<Account | null>(null)
  const [viewing, setViewing] = useState<Account | null>(null)

  const providers = useMemo(
    () => [...new Set((accounts.data ?? []).map((a) => a.provider))].sort(),
    [accounts.data],
  )
  const shown = useMemo(() => {
    const q = search.trim().toLowerCase()
    return (accounts.data ?? []).filter(
      (a) =>
        (provider === ALL || a.provider === provider) &&
        (!q || a.label.toLowerCase().includes(q)),
    )
  }, [accounts.data, provider, search])

  const handlers: RowHandlers = {
    testing,
    onActive: (a, active) => mutations.active.mutate({ id: a.id, active }),
    onStandby: (a, standby) =>
      mutations.active.mutate({ id: a.id, active: a.isActive, standby }),
    onTest: (a) => {
      setTesting(a.id)
      mutations.test.mutate(
        { id: a.id, label: a.label },
        { onSettled: () => setTesting(null) },
      )
    },
    onModels: setViewing,
    onRename: setRenaming,
    onRemove: setRemoving,
  }

  const hasAny = (accounts.data?.length ?? 0) > 0

  return (
    <div className="mx-auto flex max-w-7xl flex-col gap-6">
      <PageHeader
        title={text.title}
        description={text.description}
        actions={
          <Button onClick={() => setAdding(true)}>
            <PlusIcon aria-hidden="true" />
            {text.add}
          </Button>
        }
      />

      {hasAny ? (
        <div className="flex flex-wrap items-center gap-2">
          <div className="relative min-w-48 flex-1 sm:max-w-xs">
            <SearchIcon
              className="pointer-events-none absolute start-2.5 top-1/2 size-4 -translate-y-1/2 text-muted-foreground"
              aria-hidden="true"
            />
            <Input
              type="search"
              className="ps-8"
              aria-label={text.search}
              placeholder={text.searchPlaceholder}
              value={search}
              onChange={(e) => setSearch(e.target.value)}
            />
          </div>
          <Select value={provider} onValueChange={setProvider}>
            <SelectTrigger aria-label={text.providerFilter} className="w-full sm:w-56">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value={ALL}>{text.allProviders}</SelectItem>
              {providers.map((p) => (
                <SelectItem key={p} value={p}>
                  {p}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
      ) : null}

      {accounts.isPending ? (
        <ListSkeleton />
      ) : accounts.isError ? (
        <Alert variant="destructive" role="alert">
          <AlertDescription className="flex flex-wrap items-center justify-between gap-2">
            <span>
              {text.loadFailed}: {accounts.error.message}
            </span>
            <Button size="sm" variant="outline" onClick={() => void accounts.refetch()}>
              {text.retry}
            </Button>
          </AlertDescription>
        </Alert>
      ) : !hasAny ? (
        <Empty className="border border-dashed">
          <EmptyHeader>
            <EmptyMedia variant="icon">
              <UsersIcon aria-hidden="true" />
            </EmptyMedia>
            <EmptyTitle>{text.emptyTitle}</EmptyTitle>
            <EmptyDescription>{text.emptyHint}</EmptyDescription>
          </EmptyHeader>
          <EmptyContent>
            <Button onClick={() => setAdding(true)}>
              <PlusIcon aria-hidden="true" />
              {text.add}
            </Button>
          </EmptyContent>
        </Empty>
      ) : shown.length === 0 ? (
        <p className="py-10 text-center text-sm text-muted-foreground">{text.noMatch}</p>
      ) : (
        <AccountList
          accounts={shown}
          tests={tests.data}
          handlers={handlers}
          cards={mobile}
        />
      )}

      {adding ? <AddAccountDialog open onOpenChange={setAdding} /> : null}
      <RenameDialog
        account={renaming}
        onClose={() => setRenaming(null)}
        onSave={(id, label) => mutations.label.mutate({ id, label })}
      />
      <RemoveDialog
        account={removing}
        onClose={() => setRemoving(null)}
        onConfirm={(a) => mutations.remove.mutate({ id: a.id, label: a.label })}
      />
      <ModelsDialog account={viewing} onClose={() => setViewing(null)} />
    </div>
  )
}
