import { CopyIcon, EyeOffIcon, ScissorsIcon } from 'lucide-react'
import { toast } from 'sonner'
import { useError } from '@/api/errors'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { ScrollArea } from '@/components/ui/scroll-area'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { Skeleton } from '@/components/ui/skeleton'
import { ClassBadge, StatusBadge } from './error-list'
import { formatLatency, formatTime, isClipped } from './format'

async function copy(text: string, what: string) {
  try {
    await navigator.clipboard.writeText(text)
    toast.success(`Đã sao chép ${what}`)
  } catch {
    toast.error('Không sao chép được, hãy chọn và sao chép thủ công')
  }
}

function Fact({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="min-w-0">
      <dt className="text-xs text-muted-foreground">{label}</dt>
      <dd className="mt-0.5 text-sm break-words">{children}</dd>
    </div>
  )
}

interface BodyProps {
  title: string
  /** Lower-case name used in the copy button and the clipped notice. */
  noun: 'request' | 'response'
  text: string
}

function Body({ title, noun, text }: BodyProps) {
  const clipped = isClipped(text)
  const Noun = noun === 'request' ? 'Request' : 'Response'
  return (
    <section aria-label={title} className="flex flex-col gap-2">
      <div className="flex items-center justify-between gap-2">
        <h3 className="text-sm font-medium">{title}</h3>
        <Button
          variant="outline"
          size="xs"
          disabled={!text}
          aria-label={`Sao chép ${noun}`}
          onClick={() => void copy(text, noun)}
        >
          <CopyIcon aria-hidden="true" data-icon="inline-start" />
          Sao chép
        </Button>
      </div>
      {clipped ? (
        <p className="flex items-center gap-1.5 text-xs text-amber-700 dark:text-amber-400">
          <ScissorsIcon aria-hidden="true" className="size-3.5" />
          {Noun} đã bị cắt ở 64 KiB, phần cuối không được lưu.
        </p>
      ) : null}
      <ScrollArea className="max-h-72 rounded-md border bg-muted/40">
        <pre className="max-h-72 overflow-auto p-3 font-mono text-xs break-all whitespace-pre-wrap">
          {text || '(trống)'}
        </pre>
      </ScrollArea>
    </section>
  )
}

interface ErrorDetailProps {
  id: number | null
  onClose: () => void
}

export function ErrorDetail({ id, onClose }: ErrorDetailProps) {
  const detail = useError(id)
  const e = detail.data
  const hidden = e && !e.reqBody && !e.respBody

  return (
    <Sheet open={id !== null} onOpenChange={(open) => !open && onClose()}>
      <SheetContent className="w-full overflow-y-auto data-[side=right]:w-full data-[side=right]:sm:max-w-2xl">
        <SheetHeader>
          <SheetTitle>Chi tiết lỗi #{id}</SheetTitle>
          <SheetDescription>
            Request và response được lưu để điều tra, tối đa 64 KiB mỗi chiều.
          </SheetDescription>
        </SheetHeader>
        <div className="flex flex-col gap-4 px-4 pb-6">
          {detail.isPending ? (
            <div role="status" aria-label="Đang tải chi tiết" className="flex flex-col gap-3">
              <Skeleton className="h-16" />
              <Skeleton className="h-40" />
            </div>
          ) : detail.isError ? (
            <Alert variant="destructive">
              <AlertTitle>Không tải được chi tiết lỗi</AlertTitle>
              <AlertDescription>
                <p>{detail.error.message}</p>
                <Button
                  variant="outline"
                  size="sm"
                  className="mt-3"
                  onClick={() => void detail.refetch()}
                >
                  Thử lại
                </Button>
              </AlertDescription>
            </Alert>
          ) : e ? (
            <>
              <div className="flex flex-wrap items-center gap-1.5">
                <StatusBadge status={e.status} />
                <ClassBadge value={e.class} />
              </div>
              <p className="text-sm break-words">{e.message}</p>
              <dl className="grid grid-cols-2 gap-3 tabular-nums">
                <Fact label="Thời gian">{formatTime(e.at)}</Fact>
                <Fact label="Độ trễ">{formatLatency(e.latencyMs)}</Fact>
                <Fact label="Nhà cung cấp">{e.provider}</Fact>
                <Fact label="Model">{e.model || 'Không rõ'}</Fact>
                <Fact label="Tài khoản">{e.connectionId || 'Không rõ'}</Fact>
                <Fact label="Client">{e.client || 'Không rõ'}</Fact>
                <Fact label="Endpoint">{e.endpoint}</Fact>
                <Fact label="Quota còn lại">
                  {e.quotaLeft < 0 ? 'Không rõ' : `${Math.round(e.quotaLeft * 100)}%`}
                </Fact>
                <Fact label="Chữ ký">
                  <code className="text-xs">{e.signature}</code>
                </Fact>
              </dl>
              {hidden ? (
                <Alert>
                  <EyeOffIcon aria-hidden="true" />
                  <AlertTitle>Nội dung đã được ẩn</AlertTitle>
                  <AlertDescription>
                    Máy chủ đã ẩn nội dung request và response vì chúng có thể
                    chứa dữ liệu của client khác. Chỉ chủ sở hữu hoặc client gây
                    ra lỗi mới xem được.
                  </AlertDescription>
                </Alert>
              ) : (
                <>
                  <Body title="Request đã gửi" noun="request" text={e.reqBody ?? ''} />
                  <Body title="Response nhận về" noun="response" text={e.respBody ?? ''} />
                </>
              )}
              {e.headers ? (
                <section aria-label="Tiêu đề phản hồi" className="flex flex-col gap-2">
                  <h3 className="text-sm font-medium">Tiêu đề phản hồi</h3>
                  <pre className="overflow-auto rounded-md border bg-muted/40 p-3 font-mono text-xs break-all whitespace-pre-wrap">
                    {e.headers}
                  </pre>
                  <p className="text-xs text-muted-foreground">
                    Máy chủ chỉ giữ vài tiêu đề an toàn; Authorization, Cookie và
                    khóa API không bao giờ được lưu.
                  </p>
                </section>
              ) : null}
            </>
          ) : null}
        </div>
      </SheetContent>
    </Sheet>
  )
}
