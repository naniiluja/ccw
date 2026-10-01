import { TriangleAlertIcon } from 'lucide-react'
import type { AccountQuota } from '@/api/quota'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { ResetList } from './claim'
import type { ClaimNotice } from './notices'
import { sourceText } from './format'
import { WindowMeter } from './window-meter'

interface ViewProps {
  accounts: AccountQuota[]
  onNotice: (n: ClaimNotice) => void
}

function AccountError({ message }: { message: string }) {
  return (
    <p className="flex items-start gap-2 text-sm text-destructive">
      <TriangleAlertIcon className="mt-0.5 size-4 shrink-0" aria-hidden="true" />
      <span className="min-w-0 break-words [overflow-wrap:anywhere]">{message}</span>
    </p>
  )
}

export function QuotaCards({ accounts, onNotice }: ViewProps) {
  return (
    <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
      {accounts.map((a) => (
        <Card
          key={a.connectionId}
          id={`account-${a.connectionId}`}
          className="scroll-mt-20"
          role="article"
          aria-label={`${a.provider} ${a.label}`}
          size="sm"
        >
          <CardHeader>
            <CardTitle className="flex flex-wrap items-center gap-2">
              <span className="min-w-0 break-all">{a.label}</span>
            </CardTitle>
            <div className="flex flex-wrap items-center gap-1.5">
              <Badge variant="secondary">{a.provider}</Badge>
              {a.plan ? <Badge variant="outline">{a.plan}</Badge> : null}
              <Badge variant="outline">{sourceText(a.source)}</Badge>
            </div>
          </CardHeader>
          <CardContent className="flex flex-col gap-4">
            {a.error ? <AccountError message={a.error} /> : null}
            {a.windows.map((w) => (
              <WindowMeter key={w.name} window={w} />
            ))}
            {!a.error && a.windows.length === 0 ? (
              <p className="text-sm text-muted-foreground">
                Chưa có số liệu hạn mức cho tài khoản này.
              </p>
            ) : null}
            {a.resetsError ? <AccountError message={a.resetsError} /> : null}
            <ResetList account={a} onNotice={onNotice} />
          </CardContent>
        </Card>
      ))}
    </div>
  )
}

export function QuotaTable({ accounts, onNotice }: ViewProps) {
  return (
    <Card className="p-0">
      <Table className="min-w-[56rem]">
        <TableHeader>
          <TableRow>
            <TableHead>Tài khoản</TableHead>
            <TableHead>Provider</TableHead>
            <TableHead>Gói</TableHead>
            <TableHead>Nguồn</TableHead>
            <TableHead className="w-80">Hạn mức</TableHead>
            <TableHead>Reset</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {accounts.map((a) => (
            <TableRow key={a.connectionId} id={`account-${a.connectionId}`} className="scroll-mt-20 align-top">
              <TableCell className="font-medium">{a.label}</TableCell>
              <TableCell>{a.provider}</TableCell>
              <TableCell>{a.plan ?? '—'}</TableCell>
              <TableCell>{sourceText(a.source)}</TableCell>
              <TableCell>
                <div className="flex flex-col gap-3 py-1">
                  {a.error ? <AccountError message={a.error} /> : null}
                  {a.windows.map((w) => (
                    <WindowMeter key={w.name} window={w} />
                  ))}
                </div>
              </TableCell>
              <TableCell className="min-w-56 whitespace-normal">
                <ResetList account={a} onNotice={onNotice} />
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </Card>
  )
}
