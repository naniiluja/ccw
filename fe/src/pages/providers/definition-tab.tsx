import { PencilIcon, PlusIcon, Trash2Icon } from 'lucide-react'
import { useState } from 'react'
import { useSearchParams } from 'react-router'
import { toast } from 'sonner'
import {
  type ProviderDef,
  type ProviderInfo,
  fetchProviderDef,
  useDeleteDef,
  useProviderDefs,
} from '@/api/providers'
import { Alert, AlertDescription } from '@/components/ui/alert'
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
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { Spinner } from '@/components/ui/spinner'
import { messageOf } from './format'
import { InlineError } from './inline-error'
import { DefDialog } from './def-dialog'

const kindLabel: Record<string, string> = {
  apikey: 'Khóa API',
  'oauth-code': 'OAuth (mã xác thực)',
  'oauth-device': 'OAuth (thiết bị)',
}

function Row({ label, value }: { label: string; value?: string }) {
  return (
    <div className="flex flex-col gap-0.5 sm:flex-row sm:gap-4">
      <dt className="w-44 shrink-0 text-muted-foreground">{label}</dt>
      <dd className="min-w-0 break-all">{value || '—'}</dd>
    </div>
  )
}

export function DefinitionTab({ provider }: { provider: ProviderInfo }) {
  const defs = useProviderDefs()
  const del = useDeleteDef()
  const [, setParams] = useSearchParams()
  const [dialog, setDialog] = useState<{ base?: ProviderDef } | null>(null)
  const [loadingEdit, setLoadingEdit] = useState(false)
  const [confirmDelete, setConfirmDelete] = useState(false)
  const [deleteError, setDeleteError] = useState<string | null>(null)

  const select = (id: string | null) =>
    setParams((prev) => {
      const next = new URLSearchParams(prev)
      if (id) next.set('p', id)
      else next.delete('p')
      return next
    })

  if (defs.isPending) {
    return <Skeleton role="status" aria-label="Đang tải định nghĩa" className="h-40" />
  }
  if (defs.isError) {
    return (
      <InlineError
        title="Không tải được định nghĩa nhà cung cấp"
        error={defs.error}
        onRetry={() => void defs.refetch()}
      />
    )
  }

  const def = defs.data.find((d) => d.id === provider.id && !d.builtin)

  const edit = async () => {
    setLoadingEdit(true)
    try {
      setDialog({ base: await fetchProviderDef(provider.id) })
    } catch (e) {
      toast.error(messageOf(e))
    } finally {
      setLoadingEdit(false)
    }
  }

  const remove = () =>
    del.mutate(provider.id, {
      onSuccess: () => {
        toast.success(`Đã xóa nhà cung cấp ${provider.id}.`)
        setConfirmDelete(false)
        select(null)
      },
      onError: (e) => setDeleteError(messageOf(e)),
    })

  return (
    <div className="flex flex-col gap-4">
      {def ? (
        <>
          <dl className="flex flex-col gap-2 text-sm">
            <Row label="Mã" value={def.id} />
            <Row label="Tên hiển thị" value={def.name} />
            <Row label="Cách đăng nhập" value={kindLabel[def.kind] ?? def.kind} />
            <Row label="Kiểu API" value={def.api} />
            <Row label="Địa chỉ gốc" value={def.baseUrl} />
            <Row label="Địa chỉ danh sách model" value={def.modelsUrl} />
            <Row label="Header xác thực" value={def.authHeader} />
            <Row
              label="Header cố định"
              value={def.headers ? Object.keys(def.headers).join(', ') : undefined}
            />
            <Row label="Model dự phòng" value={def.models?.join(', ')} />
          </dl>
          <div className="flex flex-wrap gap-2">
            <Button variant="outline" disabled={loadingEdit} onClick={() => void edit()}>
              {loadingEdit ? <Spinner aria-hidden="true" /> : <PencilIcon aria-hidden="true" />}
              Sửa định nghĩa
            </Button>
            <Button
              variant="destructive"
              onClick={() => {
                setDeleteError(null)
                setConfirmDelete(true)
              }}
            >
              <Trash2Icon aria-hidden="true" />
              Xóa nhà cung cấp
            </Button>
          </div>
        </>
      ) : (
        <Alert>
          <AlertDescription>
            Đây là nhà cung cấp dựng sẵn nên không thể sửa hoặc xóa. Bạn vẫn
            có thể thêm nhà cung cấp tùy chỉnh của riêng mình.
          </AlertDescription>
        </Alert>
      )}
      <div>
        <Button variant={def ? 'ghost' : 'outline'} onClick={() => setDialog({})}>
          <PlusIcon aria-hidden="true" />
          Thêm nhà cung cấp tùy chỉnh
        </Button>
      </div>

      {dialog ? (
        <DefDialog
          open
          base={dialog.base}
          onOpenChange={(o) => {
            if (!o) setDialog(null)
          }}
          onSaved={(id) => select(id)}
        />
      ) : null}

      <AlertDialog open={confirmDelete} onOpenChange={setConfirmDelete}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Xóa nhà cung cấp {provider.id}?</AlertDialogTitle>
            <AlertDialogDescription>
              Định nghĩa bị xóa khỏi máy chủ. Nhà cung cấp còn tài khoản thì
              không xóa được.
            </AlertDialogDescription>
          </AlertDialogHeader>
          {deleteError ? (
            <Alert variant="destructive">
              <AlertDescription>{deleteError}</AlertDescription>
            </Alert>
          ) : null}
          <AlertDialogFooter>
            <AlertDialogCancel>Hủy</AlertDialogCancel>
            <AlertDialogAction
              variant="destructive"
              disabled={del.isPending}
              onClick={(e) => {
                e.preventDefault()
                remove()
              }}
            >
              Xóa
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}
