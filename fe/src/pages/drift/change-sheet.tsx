import type { DriftChange } from '@/api/drift'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { ScrollArea, ScrollBar } from '@/components/ui/scroll-area'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import {
  causeLabel,
  directionLabel,
  formatTime,
  kindLabel,
  percent,
} from './strings'

interface ChangeSheetProps {
  change: DriftChange | null
  onClose: () => void
  onAck: (id: number) => void
}

const typeBadge = (type: string) =>
  type ? (
    <Badge variant="outline" className="font-mono">
      {type}
    </Badge>
  ) : (
    <span className="text-muted-foreground">—</span>
  )

// ChangeSheet shows one change: the old and new type side by side, the AI
// verdict and the sample document the change was seen in.
export function ChangeSheet({ change, onClose, onAck }: ChangeSheetProps) {
  return (
    <Sheet open={change !== null} onOpenChange={(open) => !open && onClose()}>
      <SheetContent className="w-full gap-0 sm:max-w-xl">
        {change ? (
          <>
            <SheetHeader>
              <SheetTitle className="font-mono break-all">
                {change.path}
              </SheetTitle>
              <SheetDescription>
                {change.provider} · {directionLabel[change.direction]} ·{' '}
                {change.endpoint}
                {change.event ? ` · ${change.event}` : ''}
              </SheetDescription>
            </SheetHeader>
            <div className="flex min-h-0 flex-1 flex-col gap-5 overflow-y-auto px-4 pb-4">
              <section aria-label="So sánh cũ và mới">
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>Thuộc tính</TableHead>
                      <TableHead>Cũ</TableHead>
                      <TableHead>Mới</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    <TableRow>
                      <TableCell>Kiểu dữ liệu</TableCell>
                      <TableCell>{typeBadge(change.oldType)}</TableCell>
                      <TableCell>{typeBadge(change.newType)}</TableCell>
                    </TableRow>
                    <TableRow>
                      <TableCell>Loại thay đổi</TableCell>
                      <TableCell colSpan={2}>
                        {kindLabel[change.kind] ?? change.kind}
                      </TableCell>
                    </TableRow>
                    <TableRow>
                      <TableCell>Thời điểm</TableCell>
                      <TableCell colSpan={2} className="tabular-nums">
                        {formatTime(change.at)}
                      </TableCell>
                    </TableRow>
                    {change.client ? (
                      <TableRow>
                        <TableCell>Client</TableCell>
                        <TableCell colSpan={2}>{change.client}</TableCell>
                      </TableRow>
                    ) : null}
                  </TableBody>
                </Table>
              </section>

              {change.verdict ? (
                <section
                  aria-label="Phán quyết AI"
                  className="flex flex-col gap-2 rounded-lg border p-3"
                >
                  <div className="flex flex-wrap items-center gap-2">
                    <Badge variant="secondary">
                      {causeLabel[change.verdict] ?? change.verdict}
                    </Badge>
                    {change.verdictConf !== undefined ? (
                      <span className="text-xs text-muted-foreground tabular-nums">
                        Độ tin cậy {percent(change.verdictConf)}
                      </span>
                    ) : null}
                    {change.autoAcked ? (
                      <Badge variant="outline">Tự xác nhận</Badge>
                    ) : null}
                  </div>
                  {change.verdictBy ? (
                    <p className="text-xs text-muted-foreground">
                      Bởi {change.verdictBy}
                      {change.verdictAt ? ` · ${formatTime(change.verdictAt)}` : ''}
                    </p>
                  ) : null}
                  {change.verdictNote ? (
                    <p className="text-sm">{change.verdictNote}</p>
                  ) : null}
                </section>
              ) : null}

              <section aria-label="Dữ liệu mẫu" className="flex flex-col gap-2">
                <h3 className="text-sm font-medium">Dữ liệu mẫu</h3>
                {change.sample ? (
                  <ScrollArea className="h-64 rounded-lg border bg-muted/40">
                    <pre className="p-3 font-mono text-xs break-all whitespace-pre-wrap">
                      {change.sample}
                    </pre>
                    <ScrollBar orientation="horizontal" />
                  </ScrollArea>
                ) : (
                  <p className="text-sm text-muted-foreground">
                    Không có dữ liệu mẫu. Chỉ quản trị viên xem được mẫu.
                  </p>
                )}
              </section>
            </div>
            {!change.acked ? (
              <SheetFooter>
                <Button onClick={() => onAck(change.id)}>Xác nhận thay đổi</Button>
              </SheetFooter>
            ) : null}
          </>
        ) : null}
      </SheetContent>
    </Sheet>
  )
}
