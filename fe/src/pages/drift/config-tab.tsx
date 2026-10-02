import { PlayIcon, TriangleAlertIcon } from 'lucide-react'
import { useState, type FormEvent } from 'react'
import { toast } from 'sonner'
import {
  useDriftReview,
  useRunDriftReview,
  useSaveDriftReview,
  useSeedDrift,
  type DriftDirection,
  type DriftReview,
} from '@/api/drift'
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
  Card,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Skeleton } from '@/components/ui/skeleton'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import { directionLabel } from './strings'

const minConfidence = 0.5
const maxConfidence = 0.99

function ReviewForm({ review }: { review: DriftReview }) {
  const save = useSaveDriftReview()
  const run = useRunDriftReview()
  const [enabled, setEnabled] = useState(review.enabled)
  const [decision, setDecision] = useState(review.decisionModel)
  const [resolver, setResolver] = useState(review.resolverModel)
  const [confidence, setConfidence] = useState(String(review.ackConfidence))
  const [error, setError] = useState('')

  const onSubmit = (e: FormEvent) => {
    e.preventDefault()
    const ackConfidence = Number(confidence)
    if (
      !Number.isFinite(ackConfidence) ||
      ackConfidence < minConfidence ||
      ackConfidence > maxConfidence
    ) {
      setError('Ngưỡng tự xác nhận phải từ 0,5 đến 0,99.')
      return
    }
    if (enabled && !decision.trim()) {
      setError('Cần model quyết định trước khi bật AI review.')
      return
    }
    setError('')
    save.mutate(
      {
        enabled,
        decisionModel: decision.trim(),
        resolverModel: resolver.trim(),
        ackConfidence,
      },
      {
        onSuccess: () => toast.success('Đã lưu cấu hình AI review'),
        onError: (err) =>
          toast.error('Không lưu được cấu hình', { description: err.message }),
      },
    )
  }

  const onRun = () =>
    run.mutate(undefined, {
      onSuccess: (r) => toast.success(`Đã xét ${r.judged ?? 0} thay đổi`),
      onError: (err) =>
        toast.error('Không chạy được AI review', { description: err.message }),
    })

  return (
    <form onSubmit={onSubmit} noValidate>
      <Card>
        <CardHeader>
          <CardTitle className="flex flex-wrap items-center gap-2">
            AI drift review
            <Badge variant={review.ready ? 'secondary' : 'outline'}>
              {review.ready ? 'Sẵn sàng' : 'Chưa sẵn sàng'}
            </Badge>
          </CardTitle>
          <CardDescription>
            Một model đánh giá nguyên nhân từng thay đổi và tự xác nhận những
            thay đổi lành tính, đủ tin cậy.
          </CardDescription>
        </CardHeader>
        <CardContent className="flex flex-col gap-4">
          {review.lastError ? (
            <Alert variant="destructive">
              <TriangleAlertIcon aria-hidden="true" />
              <AlertTitle>Lần chạy gần nhất lỗi</AlertTitle>
              <AlertDescription>{review.lastError}</AlertDescription>
            </Alert>
          ) : null}
          <div className="flex items-center gap-2">
            <Switch id="review-enabled" checked={enabled} onCheckedChange={setEnabled} />
            <Label htmlFor="review-enabled">Bật AI review</Label>
          </div>
          <div className="grid gap-4 sm:grid-cols-2">
            <div className="flex flex-col gap-1.5">
              <Label htmlFor="review-decision">Model quyết định</Label>
              <Input
                id="review-decision"
                value={decision}
                placeholder="typesafe/jev-latest"
                onChange={(e) => setDecision(e.target.value)}
              />
            </div>
            <div className="flex flex-col gap-1.5">
              <Label htmlFor="review-resolver">Model giải quyết</Label>
              <Input
                id="review-resolver"
                value={resolver}
                placeholder="Để trống nếu muốn người duyệt"
                onChange={(e) => setResolver(e.target.value)}
              />
            </div>
            <div className="flex flex-col gap-1.5">
              <Label htmlFor="review-confidence">Ngưỡng tự xác nhận (0,5 đến 0,99)</Label>
              <Input
                id="review-confidence"
                type="number"
                inputMode="decimal"
                step="0.01"
                min={minConfidence}
                max={maxConfidence}
                className="tabular-nums"
                value={confidence}
                onChange={(e) => setConfidence(e.target.value)}
              />
            </div>
          </div>
          {error ? (
            <p role="alert" className="text-sm text-destructive">
              {error}
            </p>
          ) : null}
        </CardContent>
        <CardFooter className="flex flex-wrap gap-2">
          <Button type="submit" disabled={save.isPending}>
            Lưu cấu hình
          </Button>
          <Button type="button" variant="outline" disabled={run.isPending} onClick={onRun}>
            <PlayIcon aria-hidden="true" />
            Chạy ngay
          </Button>
        </CardFooter>
      </Card>
    </form>
  )
}

function ReviewCard() {
  const query = useDriftReview()
  if (query.isPending) {
    return <Skeleton role="status" aria-label="Đang tải cấu hình" className="h-72 w-full" />
  }
  if (query.isError) {
    return (
      <Alert variant="destructive">
        <TriangleAlertIcon aria-hidden="true" />
        <AlertTitle>Không tải được cấu hình AI review</AlertTitle>
        <AlertDescription>
          <p>{query.error.message}</p>
          <Button variant="outline" size="sm" className="mt-3" onClick={() => query.refetch()}>
            Thử lại
          </Button>
        </AlertDescription>
      </Alert>
    )
  }
  const r = query.data
  // A new key remounts the form when the server's config changes.
  const key = `${r.enabled}|${r.decisionModel}|${r.resolverModel}|${r.ackConfidence}`
  return <ReviewForm key={key} review={r} />
}

// Documents are separated by a line holding only "---".
const parseDocuments = (text: string) =>
  text
    .split(/^---$/m)
    .map((d) => d.trim())
    .filter(Boolean)

function SeedCard() {
  const seed = useSeedDrift()
  const [direction, setDirection] = useState<DriftDirection>('response')
  const [provider, setProvider] = useState('')
  const [endpoint, setEndpoint] = useState('')
  const [sse, setSse] = useState(false)
  const [text, setText] = useState('')
  const [confirming, setConfirming] = useState(false)

  const documents = parseDocuments(text)
  const valid = provider.trim() !== '' && endpoint.trim() !== '' && documents.length > 0

  const confirm = () =>
    seed.mutate(
      { direction, provider: provider.trim(), endpoint: endpoint.trim(), sse, documents },
      {
        onSuccess: (r) => {
          toast.success(`Đã học ${r.learned} trường từ mẫu`)
          setText('')
        },
        onError: (err) =>
          toast.error('Không khởi tạo được mẫu', { description: err.message }),
      },
    )

  return (
    <Card>
      <CardHeader>
        <CardTitle>Khởi tạo mẫu</CardTitle>
        <CardDescription>
          Dạy hệ thống cấu trúc chuẩn từ tài liệu tham chiếu, ví dụ một bản
          ghi thật của công cụ.
        </CardDescription>
      </CardHeader>
      <CardContent className="grid gap-4 sm:grid-cols-2">
        <div className="flex flex-col gap-1.5">
          <Label htmlFor="seed-direction">Hướng</Label>
          <Select value={direction} onValueChange={(v) => setDirection(v as DriftDirection)}>
            <SelectTrigger id="seed-direction" className="w-full">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="request">{directionLabel.request}</SelectItem>
              <SelectItem value="response">{directionLabel.response}</SelectItem>
            </SelectContent>
          </Select>
        </div>
        <div className="flex flex-col gap-1.5">
          <Label htmlFor="seed-provider">Nhà cung cấp</Label>
          <Input id="seed-provider" value={provider} onChange={(e) => setProvider(e.target.value)} />
        </div>
        <div className="flex flex-col gap-1.5">
          <Label htmlFor="seed-endpoint">Endpoint</Label>
          <Input
            id="seed-endpoint"
            value={endpoint}
            placeholder="/chat/completions"
            onChange={(e) => setEndpoint(e.target.value)}
          />
        </div>
        <div className="flex items-center gap-2 sm:pt-6">
          <Switch id="seed-sse" checked={sse} onCheckedChange={setSse} />
          <Label htmlFor="seed-sse">Tài liệu là luồng SSE</Label>
        </div>
        <div className="flex flex-col gap-1.5 sm:col-span-2">
          <Label htmlFor="seed-documents">Tài liệu mẫu (JSON)</Label>
          <Textarea
            id="seed-documents"
            rows={6}
            className="font-mono text-xs"
            value={text}
            placeholder={'{"id":"…"}\n---\n{"id":"…"}'}
            onChange={(e) => setText(e.target.value)}
          />
          <p className="text-xs text-muted-foreground">
            Phân tách nhiều tài liệu bằng một dòng chỉ có ---.
          </p>
        </div>
      </CardContent>
      <CardFooter>
        <Button
          type="button"
          variant="outline"
          disabled={!valid || seed.isPending}
          onClick={() => setConfirming(true)}
        >
          Khởi tạo mẫu
        </Button>
      </CardFooter>

      <AlertDialog open={confirming} onOpenChange={setConfirming}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Khởi tạo mẫu cho {provider.trim()}?</AlertDialogTitle>
            <AlertDialogDescription>
              {documents.length} tài liệu sẽ được học làm cấu trúc chuẩn cho{' '}
              {endpoint.trim()}. Hệ thống không ghi nhận thay đổi nào khi học
              mẫu, nhưng các lần lệch sau này sẽ so với cấu trúc này. Thao tác
              không thể hoàn tác.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Hủy</AlertDialogCancel>
            <AlertDialogAction onClick={confirm}>Khởi tạo</AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </Card>
  )
}

export function ConfigTab() {
  return (
    <div className="grid gap-4 xl:grid-cols-2">
      <ReviewCard />
      <SeedCard />
    </div>
  )
}
