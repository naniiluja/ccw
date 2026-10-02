import {
  BarChart3Icon,
  EyeIcon,
  GaugeIcon,
  ListChecksIcon,
  Trash2Icon,
} from 'lucide-react'
import { toast } from 'sonner'
import { useSetKeyActive, type ApiKey } from '@/api/keys'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Switch } from '@/components/ui/switch'
import {
  Table,
  TableBody,
  TableCaption,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { formatNumber, formatTime, keyStatus, type KeyStatus } from './format'
import { s } from './strings'

export type KeyAction = 'usage' | 'models' | 'limits' | 'reveal' | 'delete'

interface ListProps {
  keys: ApiKey[]
  onAction: (action: KeyAction, key: ApiKey) => void
}

const statusVariant: Record<KeyStatus, 'default' | 'secondary' | 'destructive'> = {
  active: 'default',
  disabled: 'secondary',
  expired: 'destructive',
}

function StatusBadge({ k }: { k: ApiKey }) {
  const st = keyStatus(k)
  return <Badge variant={statusVariant[st]}>{s.status[st]}</Badge>
}

function ActiveSwitch({ k }: { k: ApiKey }) {
  const setActive = useSetKeyActive()
  return (
    <Switch
      checked={k.enabled}
      aria-label={s.toggleLabel(k.name)}
      onCheckedChange={(active) =>
        setActive.mutate(
          { id: k.id, active },
          {
            onError: (err) => toast.error(s.toggleFailed(k.name, err.message)),
          },
        )
      }
    />
  )
}

function ModelsSummary({ k }: { k: ApiKey }) {
  if (k.models.length === 0) return <span className="text-muted-foreground">{s.allModels}</span>
  return (
    <span title={k.models.join('\n')} className="tabular-nums">
      {s.modelsCount(k.models.length)}
    </span>
  )
}

function Actions({ k, onAction }: { k: ApiKey; onAction: ListProps['onAction'] }) {
  const items: [KeyAction, string, typeof EyeIcon, boolean?][] = [
    ['usage', s.usageLabel(k.name), BarChart3Icon],
    ['models', s.modelsLabel(k.name), ListChecksIcon],
    ['limits', s.limitsLabel(k.name), GaugeIcon],
    ['reveal', s.revealLabel(k.name), EyeIcon],
    ['delete', s.deleteLabel(k.name), Trash2Icon, true],
  ]
  return (
    <div className="flex items-center gap-1">
      {items.map(([action, label, Icon, danger]) => (
        <Button
          key={action}
          type="button"
          variant="ghost"
          size="icon-sm"
          aria-label={label}
          title={label}
          className={danger ? 'text-destructive hover:text-destructive' : undefined}
          onClick={() => onAction(action, k)}
        >
          <Icon aria-hidden="true" />
        </Button>
      ))}
    </div>
  )
}

function KeyTable({ keys, onAction }: ListProps) {
  return (
    <div className="hidden rounded-lg border md:block">
      <Table>
        <TableCaption className="sr-only">{s.tableLabel}</TableCaption>
        <TableHeader>
          <TableRow>
            <TableHead>{s.col.name}</TableHead>
            <TableHead>{s.col.active}</TableHead>
            <TableHead>{s.col.models}</TableHead>
            <TableHead>{s.col.expires}</TableHead>
            <TableHead className="text-right">{s.col.rpm}</TableHead>
            <TableHead>{s.col.lastUsed}</TableHead>
            <TableHead>{s.col.created}</TableHead>
            <TableHead>
              <span className="sr-only">{s.col.actions}</span>
            </TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {keys.map((k) => (
            <TableRow key={k.id}>
              <TableCell>
                <div className="font-medium">{k.name}</div>
                <code className="text-xs text-muted-foreground">{k.masked}</code>
              </TableCell>
              <TableCell>
                <div className="flex items-center gap-2">
                  <ActiveSwitch k={k} />
                  <StatusBadge k={k} />
                </div>
              </TableCell>
              <TableCell>
                <ModelsSummary k={k} />
              </TableCell>
              <TableCell>{formatTime(k.expiresAt, s.never)}</TableCell>
              <TableCell className="text-right tabular-nums">
                {k.rpm > 0 ? formatNumber(k.rpm) : s.noRpm}
              </TableCell>
              <TableCell>{formatTime(k.lastUsed, s.unused)}</TableCell>
              <TableCell>{formatTime(k.createdAt, '')}</TableCell>
              <TableCell>
                <Actions k={k} onAction={onAction} />
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </div>
  )
}

function KeyCards({ keys, onAction }: ListProps) {
  return (
    <ul className="flex flex-col gap-3 md:hidden" aria-label={s.tableLabel}>
      {keys.map((k) => (
        <li key={k.id} className="rounded-lg border p-4">
          <div className="flex items-start justify-between gap-3">
            <div className="min-w-0">
              <div className="truncate font-medium">{k.name}</div>
              <code className="text-xs break-all text-muted-foreground">{k.masked}</code>
            </div>
            <ActiveSwitch k={k} />
          </div>
          <dl className="mt-3 grid grid-cols-2 gap-x-3 gap-y-2 text-sm">
            <Row label="Trạng thái">
              <StatusBadge k={k} />
            </Row>
            <Row label={s.col.models}>
              <ModelsSummary k={k} />
            </Row>
            <Row label={s.col.rpm}>
              <span className="tabular-nums">{k.rpm > 0 ? formatNumber(k.rpm) : s.noRpm}</span>
            </Row>
            <Row label={s.col.expires}>{formatTime(k.expiresAt, s.never)}</Row>
            <Row label={s.col.lastUsed}>{formatTime(k.lastUsed, s.unused)}</Row>
            <Row label={s.col.created}>{formatTime(k.createdAt, '')}</Row>
          </dl>
          <div className="mt-3 border-t pt-2">
            <Actions k={k} onAction={onAction} />
          </div>
        </li>
      ))}
    </ul>
  )
}

function Row({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="min-w-0">
      <dt className="text-xs text-muted-foreground">{label}</dt>
      <dd>{children}</dd>
    </div>
  )
}

// KeyList is a table from 768px up and a stack of cards below it.
export function KeyList(props: ListProps) {
  return (
    <>
      <KeyTable {...props} />
      <KeyCards {...props} />
    </>
  )
}
