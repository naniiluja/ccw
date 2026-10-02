import { LockIcon, TriangleAlertIcon } from 'lucide-react'
import type { ReactNode } from 'react'
import { isApiError } from '@/api/client'
import { type AuthMode, type Settings, useSettings } from '@/api/settings'
import { PageHeader } from '@/components/app/page-header'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { CopyButton } from './copy-button'
import { buildSnippets } from './snippets'

const readOnlyNote = 'Chỉ đọc: đổi bằng biến môi trường rồi khởi động lại ccw.'

const authModes: Record<AuthMode, { label: string; env: string }> = {
  password: { label: 'Mật khẩu (phiên đăng nhập)', env: 'CCW_PASSWORD' },
  token: { label: 'Token máy (master token)', env: 'CCW_API_TOKEN' },
  none: { label: 'Không xác thực', env: 'CCW_INSECURE_NO_AUTH' },
}

function ttlText(seconds: number): string {
  if (seconds >= 3600 && seconds % 3600 === 0) return `${seconds / 3600} giờ (${seconds} giây)`
  if (seconds >= 60 && seconds % 60 === 0) return `${seconds / 60} phút (${seconds} giây)`
  return `${seconds} giây`
}

function Unset() {
  return <span className="text-muted-foreground">Chưa đặt (dùng mặc định)</span>
}

interface RowProps {
  label: string
  env: string
  children: ReactNode
}

// Row is one setting: its name, the value in force and the variable behind it.
function Row({ label, env, children }: RowProps) {
  return (
    <div className="grid gap-1 py-3 sm:grid-cols-[12rem_minmax(0,1fr)_auto] sm:items-center sm:gap-4">
      <dt className="text-sm font-medium">{label}</dt>
      <dd className="min-w-0 text-sm break-words [overflow-wrap:anywhere]">
        {children}
      </dd>
      <dd>
        <code className="rounded bg-muted px-1.5 py-0.5 font-mono text-xs">
          {env}
        </code>
      </dd>
    </div>
  )
}

function Section({
  title,
  description,
  children,
}: {
  title: string
  description?: string
  children: ReactNode
}) {
  return (
    <Card>
      <CardHeader>
        <CardTitle>{title}</CardTitle>
        {description ? <CardDescription>{description}</CardDescription> : null}
      </CardHeader>
      <CardContent>{children}</CardContent>
    </Card>
  )
}

function Value({ value }: { value: string | number }) {
  return value === '' ? <Unset /> : <span>{value}</span>
}

function WebSearch({ data }: { data: Settings['websearch'] }) {
  return (
    <Section
      title="Tìm kiếm web"
      description="Dịch vụ ccw gọi khi client yêu cầu công cụ web search."
    >
      <dl className="divide-y">
        <Row label="Nhà cung cấp" env="CCW_SEARCH_PROVIDER">
          <Value value={data.provider} />
        </Row>
        <Row label="Model tìm kiếm" env="CCW_SEARCH_MODEL">
          <Value value={data.model} />
        </Row>
        <Row label="Số kết quả" env="CCW_SEARCH_COUNT">
          <Value value={data.count > 0 ? data.count : ''} />
        </Row>
        <Row label="URL dịch vụ" env="CCW_SEARCH_URL">
          <Value value={data.url} />
        </Row>
        <Row label="Khóa dịch vụ" env="CCW_SEARCH_KEY">
          {data.keySet ? (
            <Badge variant="secondary">Đã đặt</Badge>
          ) : (
            <Badge variant="outline">Chưa đặt</Badge>
          )}
        </Row>
      </dl>
    </Section>
  )
}

function Security({ data }: { data: Settings }) {
  const mode = authModes[data.authMode] ?? {
    label: data.authMode,
    env: 'CCW_PASSWORD',
  }
  return (
    <Section title="Phiên và bảo mật">
      <dl className="divide-y">
        <Row label="Chế độ xác thực" env={mode.env}>
          {mode.label}
          {data.authMode === 'password' ? (
            <span className="block text-xs text-muted-foreground">
              Token máy dùng biến CCW_API_TOKEN.
            </span>
          ) : null}
        </Row>
        <Row label="Thời hạn phiên" env="CCW_SESSION_TTL">
          {ttlText(data.sessionTtlSeconds)}
        </Row>
        <Row label="Múi giờ" env="CCW_TZ">
          <Value value={data.timezone} />
        </Row>
      </dl>
      <p className="mt-3 text-sm text-muted-foreground">
        Mật khẩu chỉ được đổi bằng cách chạy ccw với cờ{' '}
        <code className="rounded bg-muted px-1 py-0.5 font-mono text-xs">
          -reset-password
        </code>
        ; trang này không đổi và không hiện mật khẩu hay khóa nào.
      </p>
    </Section>
  )
}

function Endpoint({ label, url, copyLabel }: { label: string; url: string; copyLabel: string }) {
  return (
    <div className="flex flex-wrap items-center justify-between gap-2 py-3">
      <div className="min-w-0">
        <p className="text-sm font-medium">{label}</p>
        <code className="font-mono text-sm break-all">{url}</code>
      </div>
      <CopyButton text={url} label={copyLabel} />
    </div>
  )
}

function Connection() {
  const origin = window.location.origin
  const snippets = buildSnippets(origin)
  return (
    <Section
      title="Kết nối"
      description="Trỏ client vào ccw. Tạo khóa API ở trang Khóa API rồi dán vào chỗ có dấu <>."
    >
      <div className="divide-y">
        <Endpoint
          label="Base URL (/v1)"
          url={`${origin}/v1`}
          copyLabel="Sao chép base URL"
        />
        <Endpoint
          label="Máy chủ MCP"
          url={`${origin}/mcp`}
          copyLabel="Sao chép URL máy chủ MCP"
        />
      </div>
      <div className="mt-4 flex flex-col gap-4">
        {snippets.map((s) => (
          <figure key={s.id} className="rounded-lg border">
            <figcaption className="flex flex-wrap items-center justify-between gap-2 border-b px-3 py-2">
              <div className="min-w-0">
                <p className="text-sm font-medium">{s.title}</p>
                <p className="text-xs text-muted-foreground">{s.hint}</p>
              </div>
              <CopyButton text={s.code} label={`Sao chép cấu hình ${s.title}`} />
            </figcaption>
            <pre
              tabIndex={0}
              aria-label={`Cấu hình ${s.title}`}
              className="max-w-full overflow-x-auto p-3 font-mono text-xs leading-relaxed"
            >
              <code>{s.code}</code>
            </pre>
          </figure>
        ))}
      </div>
    </Section>
  )
}

function SettingsSkeleton() {
  return (
    <div
      role="status"
      aria-live="polite"
      aria-label="Đang tải cài đặt"
      className="flex flex-col gap-4"
    >
      <Skeleton className="h-56" />
      <Skeleton className="h-40" />
    </div>
  )
}

function LoadError({ error, onRetry }: { error: unknown; onRetry: () => void }) {
  const missing = isApiError(error) && error.status === 404
  const message = missing
    ? 'Máy chủ này chưa hỗ trợ trang cài đặt (không có GET /api/settings). Hãy cập nhật ccw lên bản mới hơn.'
    : error instanceof Error
      ? error.message
      : 'Đã có lỗi không xác định.'
  return (
    <Alert variant="destructive">
      <TriangleAlertIcon aria-hidden="true" />
      <AlertTitle>Không tải được cài đặt</AlertTitle>
      <AlertDescription>
        <p>{message}</p>
        <Button variant="outline" size="sm" className="mt-3" onClick={onRetry}>
          Thử lại
        </Button>
      </AlertDescription>
    </Alert>
  )
}

export default function SettingsPage() {
  const settings = useSettings()
  return (
    <div
      data-testid="settings-page"
      className="mx-auto flex w-full max-w-4xl flex-col gap-6"
    >
      <PageHeader title="Cài đặt" description="Cấu hình hiệu lực của máy chủ." />
      <p className="flex items-center gap-2 text-sm text-muted-foreground">
        <LockIcon className="size-4 shrink-0" aria-hidden="true" />
        {readOnlyNote}
      </p>
      {settings.isPending ? (
        <SettingsSkeleton />
      ) : settings.isError ? (
        <LoadError error={settings.error} onRetry={() => void settings.refetch()} />
      ) : (
        <>
          <WebSearch data={settings.data.websearch} />
          <Security data={settings.data} />
          <Connection />
        </>
      )}
    </div>
  )
}
