import {
  ListChecksIcon,
  Loader2Icon,
  PencilIcon,
  PlayIcon,
  Trash2Icon,
} from 'lucide-react'
import type { Account, AccountTest } from '@/api/accounts'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Switch } from '@/components/ui/switch'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { text } from './strings'

export interface RowHandlers {
  testing: string | null
  onActive: (a: Account, active: boolean) => void
  onStandby: (a: Account, standby: boolean) => void
  onTest: (a: Account) => void
  onModels: (a: Account) => void
  onRename: (a: Account) => void
  onRemove: (a: Account) => void
}

function TestBadge({ test }: { test?: AccountTest }) {
  if (!test) return <Badge variant="outline">{text.tested.none}</Badge>
  return test.ok ? (
    <Badge variant="secondary" className="tabular-nums">
      {text.tested.ok(test.ms)}
    </Badge>
  ) : (
    <Badge
      variant="destructive"
      className="tabular-nums"
      title={test.message || undefined}
    >
      {text.tested.bad(test.ms)}
    </Badge>
  )
}

function ActiveSwitches({ a, h }: { a: Account; h: RowHandlers }) {
  return (
    <div className="flex items-center gap-3">
      <Switch
        checked={a.isActive}
        onCheckedChange={(v) => h.onActive(a, v)}
        aria-label={text.on(a.label)}
      />
      {a.isActive ? (
        <label className="flex items-center gap-1.5 text-xs text-muted-foreground">
          <Switch
            size="sm"
            checked={a.standby}
            onCheckedChange={(v) => h.onStandby(a, v)}
            aria-label={text.standbyOf(a.label)}
          />
          {text.standby}
        </label>
      ) : null}
    </div>
  )
}

function Actions({ a, h }: { a: Account; h: RowHandlers }) {
  const busy = h.testing === a.id
  return (
    <div className="flex items-center gap-1">
      <Button
        variant="ghost"
        size="icon-sm"
        aria-label={text.actions.test(a.label)}
        disabled={busy}
        onClick={() => h.onTest(a)}
      >
        {busy ? (
          <Loader2Icon className="animate-spin" aria-hidden="true" />
        ) : (
          <PlayIcon aria-hidden="true" />
        )}
      </Button>
      <Button
        variant="ghost"
        size="icon-sm"
        aria-label={text.actions.models(a.label)}
        onClick={() => h.onModels(a)}
      >
        <ListChecksIcon aria-hidden="true" />
      </Button>
      <Button
        variant="ghost"
        size="icon-sm"
        aria-label={text.actions.rename(a.label)}
        onClick={() => h.onRename(a)}
      >
        <PencilIcon aria-hidden="true" />
      </Button>
      <Button
        variant="ghost"
        size="icon-sm"
        aria-label={text.actions.remove(a.label)}
        onClick={() => h.onRemove(a)}
      >
        <Trash2Icon aria-hidden="true" className="text-destructive" />
      </Button>
    </div>
  )
}

function Endpoint({ a }: { a: Account }) {
  return a.baseUrl ? (
    <span
      className="block max-w-[16rem] truncate text-xs text-muted-foreground"
      title={a.baseUrl}
    >
      {text.customUrl}: {a.baseUrl}
    </span>
  ) : null
}

interface ListProps {
  accounts: Account[]
  tests: Record<string, AccountTest> | undefined
  handlers: RowHandlers
  cards: boolean
}

export function AccountList({ accounts, tests, handlers, cards }: ListProps) {
  if (cards) {
    return (
      <ul aria-label={text.list} className="flex flex-col gap-3">
        {accounts.map((a) => (
          <li key={a.id}>
            <Card size="sm">
              <CardContent className="flex flex-col gap-3">
                <div className="flex items-start justify-between gap-2">
                  <div className="min-w-0">
                    <p className="truncate font-medium">{a.label}</p>
                    <p className="text-sm text-muted-foreground">{a.provider}</p>
                    <Endpoint a={a} />
                  </div>
                  <TestBadge test={tests?.[a.id]} />
                </div>
                <ActiveSwitches a={a} h={handlers} />
                <Actions a={a} h={handlers} />
              </CardContent>
            </Card>
          </li>
        ))}
      </ul>
    )
  }
  return (
    <div className="overflow-x-auto rounded-lg border">
      <Table aria-label={text.list}>
        <TableHeader>
          <TableRow>
            <TableHead>{text.col.label}</TableHead>
            <TableHead>{text.col.provider}</TableHead>
            <TableHead>{text.col.status}</TableHead>
            <TableHead>{text.col.active}</TableHead>
            <TableHead className="text-end">{text.col.actions}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {accounts.map((a) => (
            <TableRow key={a.id}>
              <TableCell>
                <span className="font-medium">{a.label}</span>
                <Endpoint a={a} />
              </TableCell>
              <TableCell>{a.provider}</TableCell>
              <TableCell>
                <TestBadge test={tests?.[a.id]} />
              </TableCell>
              <TableCell>
                <ActiveSwitches a={a} h={handlers} />
              </TableCell>
              <TableCell className="flex justify-end">
                <Actions a={a} h={handlers} />
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </div>
  )
}
