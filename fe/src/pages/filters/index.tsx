import {
  FilterIcon,
  PencilIcon,
  PlusIcon,
  Trash2Icon,
  TriangleAlertIcon,
} from 'lucide-react'
import { useMemo, useState } from 'react'
import { toast } from 'sonner'
import { isApiError } from '@/api/client'
import {
  type Filter,
  useDeleteFilter,
  useFilterProviders,
  useFilters,
  useToggleFilter,
} from '@/api/filters'
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
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Empty,
  EmptyContent,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from '@/components/ui/empty'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Skeleton } from '@/components/ui/skeleton'
import { Switch } from '@/components/ui/switch'
import { FilterForm } from './filter-form'
import { ALL_PROVIDERS, kindLabels, kinds, providerLabel, text } from './strings'

const ANY = 'all'

function LoadingRows() {
  return (
    <div role="status" aria-label={text.loading} className="flex flex-col gap-3">
      <Skeleton className="h-9 w-full sm:w-96" />
      <Skeleton className="h-40" />
      <Skeleton className="h-40" />
    </div>
  )
}

interface RuleRowProps {
  filter: Filter
  onToggle: (f: Filter) => void
  onEdit: (f: Filter) => void
  onDelete: (f: Filter) => void
}

function RuleRow({ filter, onToggle, onEdit, onDelete }: RuleRowProps) {
  return (
    <li className="flex flex-wrap items-center gap-x-3 gap-y-2 px-4 py-3">
      <Switch
        checked={filter.enabled}
        onCheckedChange={() => onToggle(filter)}
        aria-label={text.enableRule(filter.pattern)}
      />
      <Badge variant="secondary">{kindLabels[filter.kind] ?? filter.kind}</Badge>
      <div className="min-w-0 flex-1 basis-40">
        <code
          className={`block font-mono text-sm break-all ${filter.enabled ? '' : 'text-muted-foreground line-through'}`}
        >
          {filter.pattern}
        </code>
        {filter.note ? (
          <p className="mt-0.5 text-xs text-muted-foreground">{filter.note}</p>
        ) : null}
      </div>
      <div className="ml-auto flex items-center gap-1">
        <Button
          variant="ghost"
          size="icon-sm"
          aria-label={text.editRule(filter.pattern)}
          onClick={() => onEdit(filter)}
        >
          <PencilIcon aria-hidden="true" />
        </Button>
        <Button
          variant="ghost"
          size="icon-sm"
          aria-label={text.deleteRule(filter.pattern)}
          onClick={() => onDelete(filter)}
        >
          <Trash2Icon aria-hidden="true" />
        </Button>
      </div>
    </li>
  )
}

export default function FiltersPage() {
  const filters = useFilters()
  const providers = useFilterProviders()
  const toggle = useToggleFilter()
  const remove = useDeleteFilter()
  const [provider, setProvider] = useState(ANY)
  const [kind, setKind] = useState(ANY)
  const [formOpen, setFormOpen] = useState(false)
  const [editing, setEditing] = useState<Filter | undefined>()
  const [deleting, setDeleting] = useState<Filter | undefined>()

  const all = filters.data
  const providerIds = useMemo(() => {
    const ids = new Set<string>([ALL_PROVIDERS])
    for (const p of providers.data ?? []) ids.add(p.id)
    for (const f of all ?? []) ids.add(f.provider)
    return [...ids]
  }, [providers.data, all])

  const groups = useMemo(() => {
    const by = new Map<string, Filter[]>()
    for (const f of all ?? []) {
      if (provider !== ANY && f.provider !== provider) continue
      if (kind !== ANY && f.kind !== kind) continue
      by.set(f.provider, [...(by.get(f.provider) ?? []), f])
    }
    // The rules for every provider come first, the rest in name order.
    return [...by.entries()].sort(([a], [b]) =>
      a === ALL_PROVIDERS ? -1 : b === ALL_PROVIDERS ? 1 : a.localeCompare(b),
    )
  }, [all, provider, kind])

  const openCreate = () => {
    setEditing(undefined)
    setFormOpen(true)
  }
  const openEdit = (f: Filter) => {
    setEditing(f)
    setFormOpen(true)
  }
  const onToggle = (f: Filter) =>
    toggle.mutate(f, {
      onError: (e) =>
        toast.error(text.toggleFailed, {
          description: isApiError(e) ? e.message : undefined,
        }),
    })
  const confirmDelete = () => {
    if (!deleting) return
    remove.mutate(deleting.id, {
      onSuccess: () => toast.success(text.deleted),
      onError: (e) =>
        toast.error(text.deleteFailed, {
          description: isApiError(e) ? e.message : undefined,
        }),
    })
  }

  const filtering = provider !== ANY || kind !== ANY

  return (
    <div className="mx-auto flex max-w-5xl flex-col gap-6">
      <PageHeader
        title={text.title}
        description={text.description}
        actions={
          <Button onClick={openCreate}>
            <PlusIcon aria-hidden="true" />
            {text.add}
          </Button>
        }
      />

      {filters.isPending ? (
        <LoadingRows />
      ) : filters.isError ? (
        <Alert variant="destructive">
          <TriangleAlertIcon aria-hidden="true" />
          <AlertTitle>{text.loadFailedTitle}</AlertTitle>
          <AlertDescription>
            <p>{filters.error.message}</p>
            <Button
              variant="outline"
              size="sm"
              className="mt-3"
              onClick={() => void filters.refetch()}
            >
              {text.retry}
            </Button>
          </AlertDescription>
        </Alert>
      ) : all && all.length === 0 ? (
        <Empty className="border">
          <EmptyHeader>
            <EmptyMedia variant="icon">
              <FilterIcon aria-hidden="true" />
            </EmptyMedia>
            <EmptyTitle>{text.emptyTitle}</EmptyTitle>
            <EmptyDescription>{text.emptyBody}</EmptyDescription>
          </EmptyHeader>
          <EmptyContent>
            <Button onClick={openCreate}>
              <PlusIcon aria-hidden="true" />
              {text.add}
            </Button>
          </EmptyContent>
        </Empty>
      ) : (
        <>
          <div className="flex flex-col gap-2 sm:flex-row">
            <Select value={provider} onValueChange={setProvider}>
              <SelectTrigger
                aria-label={text.filterProvider}
                className="w-full sm:w-56"
              >
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value={ANY}>{text.allProviders}</SelectItem>
                {providerIds.map((id) => (
                  <SelectItem key={id} value={id}>
                    {providerLabel(id)}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <Select value={kind} onValueChange={setKind}>
              <SelectTrigger
                aria-label={text.filterKind}
                className="w-full sm:w-48"
              >
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value={ANY}>{text.allKinds}</SelectItem>
                {kinds.map((k) => (
                  <SelectItem key={k} value={k}>
                    {kindLabels[k]}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          {groups.length === 0 ? (
            <Empty className="border">
              <EmptyHeader>
                <EmptyTitle>{text.noMatchTitle}</EmptyTitle>
                <EmptyDescription>{text.noMatchBody}</EmptyDescription>
              </EmptyHeader>
              {filtering ? (
                <EmptyContent>
                  <Button
                    variant="outline"
                    onClick={() => {
                      setProvider(ANY)
                      setKind(ANY)
                    }}
                  >
                    {text.clearFilters}
                  </Button>
                </EmptyContent>
              ) : null}
            </Empty>
          ) : (
            groups.map(([id, rules]) => (
              <section
                key={id}
                aria-labelledby={`group-${id}`}
                className="rounded-lg border bg-card text-card-foreground"
              >
                <header className="flex items-center justify-between gap-2 border-b px-4 py-3">
                  <h2 id={`group-${id}`} className="text-sm font-medium">
                    {providerLabel(id)}
                  </h2>
                  <span className="text-xs text-muted-foreground tabular-nums">
                    {text.rules(rules.length)}
                  </span>
                </header>
                <ul className="divide-y">
                  {rules.map((f) => (
                    <RuleRow
                      key={f.id}
                      filter={f}
                      onToggle={onToggle}
                      onEdit={openEdit}
                      onDelete={setDeleting}
                    />
                  ))}
                </ul>
              </section>
            ))
          )}
        </>
      )}

      <FilterForm open={formOpen} onOpenChange={setFormOpen} filter={editing} />

      <AlertDialog
        open={deleting !== undefined}
        onOpenChange={(open) => {
          if (!open) setDeleting(undefined)
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{text.deleteTitle}</AlertDialogTitle>
            <AlertDialogDescription>{text.deleteBody}</AlertDialogDescription>
          </AlertDialogHeader>
          <code className="rounded-md bg-muted px-3 py-2 font-mono text-sm break-all">
            {deleting?.pattern}
          </code>
          <AlertDialogFooter>
            <AlertDialogCancel>{text.cancel}</AlertDialogCancel>
            <AlertDialogAction variant="destructive" onClick={confirmDelete}>
              {text.delete}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}
