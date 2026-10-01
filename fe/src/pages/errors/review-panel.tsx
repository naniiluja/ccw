import { BotIcon, PlayIcon, SaveIcon } from 'lucide-react'
import { useState, type FormEvent } from 'react'
import { toast } from 'sonner'
import {
  useErrorReview,
  useErrorVerdicts,
  useSaveErrorReview,
  type ErrorReviewSettings,
  type ErrorReviewState,
  type ErrorVerdict,
} from '@/api/errors'
import { useSession } from '@/api/session'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from '@/components/ui/empty'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Skeleton } from '@/components/ui/skeleton'
import { Spinner } from '@/components/ui/spinner'
import { Switch } from '@/components/ui/switch'
import { actionLabel, confidence, formatTime } from './format'

function ReviewForm({ state }: { state: ErrorReviewState }) {
  const session = useSession()
  const save = useSaveErrorReview()
  const [enabled, setEnabled] = useState(state.enabled)
  const [model, setModel] = useState(state.model)
  const [minErrors, setMinErrors] = useState(String(state.minErrors))
  const [replay, setReplay] = useState(state.replay)
  const [running, setRunning] = useState(false)

  const canWrite = session.data?.admin ?? false
  const min = Number(minErrors)
  const valid = Number.isInteger(min) && min >= 1

  const settings = (): ErrorReviewSettings => ({
    enabled,
    model: model.trim(),
    minErrors: min,
    replay,
  })

  function onSubmit(e: FormEvent) {
    e.preventDefault()
    if (!valid) return
    save.mutate(settings(), {
      onSuccess: () => toast.success('Đã lưu cấu hình AI error review'),
      onError: (err) => toast.error(err.message),
    })
  }

  function run() {
    if (!valid) return
    setRunning(true)
    save.mutate(
      { ...settings(), run: true },
      {
        onSuccess: (s) =>
          toast.success(`Đã chạy review: ${s.judged ?? 0} nhóm được kết luận`),
        onError: (err) => toast.error(err.message),
        onSettled: () => setRunning(false),
      },
    )
  }

  return (
    <form
      onSubmit={onSubmit}
      aria-label="Cấu hình AI error review"
      className="flex flex-col gap-4 rounded-lg border bg-card p-4"
    >
      <div className="flex flex-col gap-1">
        <h2 className="text-base font-medium">Cấu hình</h2>
        <p className="text-sm text-muted-foreground">
          Một model đọc từng nhóm lỗi, đề xuất nguyên nhân và có thể tự áp dụng
          biện pháp như blacklist một field.
        </p>
      </div>
      {!canWrite ? (
        <Alert>
          <AlertTitle>Chỉ xem</AlertTitle>
          <AlertDescription>
            Cần phiên quản trị để đổi cấu hình hoặc chạy review.
          </AlertDescription>
        </Alert>
      ) : null}
      {state.enabled && !state.ready ? (
        <Alert>
          <AlertTitle>Model chưa sẵn sàng</AlertTitle>
          <AlertDescription>
            Provider của model này chưa có tài khoản đang hoạt động.
          </AlertDescription>
        </Alert>
      ) : null}
      {state.lastError ? (
        <Alert variant="destructive">
          <AlertTitle>Lần chạy gần nhất gặp lỗi</AlertTitle>
          <AlertDescription>{state.lastError}</AlertDescription>
        </Alert>
      ) : null}
      <div className="grid gap-4 sm:grid-cols-2">
        <div className="flex items-center justify-between gap-3 rounded-md border p-3">
          <Label htmlFor="review-enabled">Bật AI error review</Label>
          <Switch
            id="review-enabled"
            checked={enabled}
            onCheckedChange={setEnabled}
            disabled={!canWrite}
          />
        </div>
        <div className="flex items-center justify-between gap-3 rounded-md border p-3">
          <Label htmlFor="review-replay">
            Replay request lỗi trước khi kết luận
          </Label>
          <Switch
            id="review-replay"
            checked={replay}
            onCheckedChange={setReplay}
            disabled={!canWrite}
          />
        </div>
        <div className="flex flex-col gap-1.5">
          <Label htmlFor="review-model">Model</Label>
          <Input
            id="review-model"
            placeholder="nhà-cung-cấp/tên-model"
            value={model}
            onChange={(e) => setModel(e.target.value)}
            disabled={!canWrite}
          />
        </div>
        <div className="flex flex-col gap-1.5">
          <Label htmlFor="review-min">Số lỗi tối thiểu</Label>
          <Input
            id="review-min"
            inputMode="numeric"
            value={minErrors}
            onChange={(e) => setMinErrors(e.target.value.replace(/\D/g, ''))}
            aria-invalid={!valid}
            className="tabular-nums"
            disabled={!canWrite}
          />
          <p className="text-xs text-muted-foreground">
            Nhóm lỗi cần đủ số lần này mới được xét.
          </p>
        </div>
      </div>
      <div className="flex flex-wrap gap-2">
        <Button type="submit" disabled={!canWrite || !valid || save.isPending}>
          {save.isPending && !running ? (
            <Spinner aria-label="Đang xử lý" data-icon="inline-start" />
          ) : (
            <SaveIcon aria-hidden="true" data-icon="inline-start" />
          )}
          Lưu cấu hình
        </Button>
        <Button
          type="button"
          variant="outline"
          onClick={run}
          disabled={!canWrite || !valid || !model.trim() || save.isPending}
        >
          {running ? (
            <Spinner aria-label="Đang xử lý" data-icon="inline-start" />
          ) : (
            <PlayIcon aria-hidden="true" data-icon="inline-start" />
          )}
          Chạy ngay
        </Button>
      </div>
    </form>
  )
}

function VerdictCard({ v }: { v: ErrorVerdict }) {
  const c = confidence(v)
  return (
    <li className="flex flex-col gap-2 rounded-lg border bg-card p-4">
      <div className="flex flex-wrap items-center gap-1.5">
        <Badge variant="secondary">{actionLabel(v.action)}</Badge>
        <Badge variant={v.applied ? 'default' : 'outline'}>
          {v.applied ? 'Đã áp dụng' : 'Chưa áp dụng'}
        </Badge>
        <Badge variant={c.level === 'high' ? 'default' : 'outline'}>{c.label}</Badge>
        <span className="text-xs text-muted-foreground tabular-nums">
          {formatTime(v.at)}
        </span>
      </div>
      <p className="text-sm font-medium break-words">{v.cause}</p>
      <p className="text-xs text-muted-foreground break-words">
        {v.provider} · <code>{v.signature}</code> ·{' '}
        <span className="tabular-nums">{v.errors}</span> lỗi
        {v.detail ? ` · ${v.detail}` : ''}
      </p>
      {v.reason ? <p className="text-sm break-words">{v.reason}</p> : null}
      {v.note ? (
        <p className="text-xs text-muted-foreground break-words">{v.note}</p>
      ) : null}
    </li>
  )
}

function Verdicts() {
  const verdicts = useErrorVerdicts()
  if (verdicts.isPending) {
    return <Skeleton role="status" aria-label="Đang tải kết luận" className="h-28" />
  }
  if (verdicts.isError) {
    return (
      <Alert variant="destructive">
        <AlertTitle>Không tải được kết luận</AlertTitle>
        <AlertDescription>
          <p>{verdicts.error.message}</p>
          <Button
            variant="outline"
            size="sm"
            className="mt-3"
            onClick={() => void verdicts.refetch()}
          >
            Thử lại
          </Button>
        </AlertDescription>
      </Alert>
    )
  }
  if (!verdicts.data.length) {
    return (
      <Empty className="border">
        <EmptyHeader>
          <EmptyMedia variant="icon">
            <BotIcon aria-hidden="true" />
          </EmptyMedia>
          <EmptyTitle>Chưa có kết luận nào</EmptyTitle>
          <EmptyDescription>
            Kết luận xuất hiện sau khi review xét một nhóm lỗi.
          </EmptyDescription>
        </EmptyHeader>
      </Empty>
    )
  }
  return (
    <ul aria-label="Kết luận của AI error review" className="flex flex-col gap-3">
      {verdicts.data.map((v) => (
        <VerdictCard key={v.id} v={v} />
      ))}
    </ul>
  )
}

export function ReviewPanel() {
  const review = useErrorReview()
  return (
    <div className="flex flex-col gap-6">
      {review.isPending ? (
        <Skeleton role="status" aria-label="Đang tải cấu hình" className="h-72" />
      ) : review.isError ? (
        <Alert variant="destructive">
          <AlertTitle>Không tải được cấu hình review</AlertTitle>
          <AlertDescription>
            <p>{review.error.message}</p>
            <Button
              variant="outline"
              size="sm"
              className="mt-3"
              onClick={() => void review.refetch()}
            >
              Thử lại
            </Button>
          </AlertDescription>
        </Alert>
      ) : (
        <ReviewForm state={review.data} />
      )}
      <section className="flex flex-col gap-3">
        <h2 className="text-base font-medium">Kết luận gần đây</h2>
        <Verdicts />
      </section>
    </div>
  )
}
