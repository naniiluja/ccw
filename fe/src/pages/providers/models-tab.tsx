import {
  columnFilteringFeature,
  createColumnHelper,
  createFilteredRowModel,
  createPaginatedRowModel,
  createSortedRowModel,
  filterFn_includesString,
  rowPaginationFeature,
  rowSortingFeature,
  sortFn_alphanumeric,
  sortFn_basic,
  sortFn_text,
  tableFeatures,
  useTable,
} from '@tanstack/react-table'
import {
  ArrowDownIcon,
  ArrowUpDownIcon,
  ArrowUpIcon,
  BoxesIcon,
  PlayIcon,
  PowerIcon,
  PowerOffIcon,
  SearchIcon,
  Trash2Icon,
} from 'lucide-react'
import { useMemo, useState } from 'react'
import { toast } from 'sonner'
import {
  type ModelTestResult,
  type ProviderModel,
  useDeleteModels,
  useModelTable,
  useSetModelsActive,
  useTestModel,
} from '@/api/providers'
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
import { Checkbox } from '@/components/ui/checkbox'
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from '@/components/ui/empty'
import {
  InputGroup,
  InputGroupAddon,
  InputGroupInput,
} from '@/components/ui/input-group'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Skeleton } from '@/components/ui/skeleton'
import { Spinner } from '@/components/ui/spinner'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { formatTime, messageOf } from './format'
import { InlineError } from './inline-error'

const features = tableFeatures({
  columnFilteringFeature,
  filteredRowModel: createFilteredRowModel(),
  filterFns: { includesString: filterFn_includesString },
  rowSortingFeature,
  sortedRowModel: createSortedRowModel(),
  sortFns: {
    text: sortFn_text,
    alphanumeric: sortFn_alphanumeric,
    basic: sortFn_basic,
  },
  rowPaginationFeature,
  paginatedRowModel: createPaginatedRowModel(),
})

const helper = createColumnHelper<typeof features, ProviderModel>()

const columns = helper.columns([
  helper.accessor('model', {
    id: 'model',
    header: 'Model',
    sortFn: 'text',
    filterFn: 'includesString',
  }),
  helper.accessor('active', {
    id: 'active',
    header: 'Trạng thái',
    sortFn: 'basic',
    filterFn: (row, columnId, value) =>
      value === 'all' || row.getValue<boolean>(columnId) === (value === 'on'),
  }),
  helper.accessor('lastSeen', {
    id: 'lastSeen',
    header: 'Thấy lần cuối',
    sortFn: 'text',
  }),
])

const EMPTY: ProviderModel[] = []
const pageSize = 10

type Result = ModelTestResult & { model: string }

function SortHeader({
  label,
  name,
  sorted,
  onToggle,
}: {
  label: string
  name: string
  sorted: false | 'asc' | 'desc'
  onToggle: () => void
}) {
  const Icon =
    sorted === 'asc' ? ArrowUpIcon : sorted === 'desc' ? ArrowDownIcon : ArrowUpDownIcon
  return (
    <Button
      variant="ghost"
      size="sm"
      className="-ml-2"
      aria-label={`Sắp xếp theo ${name}`}
      onClick={onToggle}
    >
      {label}
      <Icon aria-hidden="true" />
    </Button>
  )
}

function TestBadge({ m }: { m: ProviderModel }) {
  if (!m.testAt) return <span className="text-muted-foreground">Chưa thử</span>
  return (
    <span className="inline-flex flex-wrap items-center gap-1.5">
      <Badge variant={m.testOk ? 'secondary' : 'destructive'}>
        {m.testOk ? 'Thành công' : 'Lỗi'}
      </Badge>
      {m.testMs ? (
        <span className="text-xs text-muted-foreground tabular-nums">
          {m.testMs} ms
        </span>
      ) : null}
    </span>
  )
}

function Skeletons() {
  return (
    <div
      role="status"
      aria-label="Đang tải danh sách model"
      className="flex flex-col gap-2"
    >
      <Skeleton className="h-9 w-64 max-w-full" />
      <Skeleton className="h-10" />
      <Skeleton className="h-10" />
      <Skeleton className="h-10" />
    </div>
  )
}

export function ModelsTab({ providerId }: { providerId: string }) {
  const query = useModelTable(providerId)
  if (query.isPending) return <Skeletons />
  if (query.isError) {
    return (
      <InlineError
        title="Không tải được bảng model"
        error={query.error}
        onRetry={() => void query.refetch()}
      />
    )
  }
  return <ModelsTable providerId={providerId} models={query.data.models ?? EMPTY} />
}

function ModelsTable({
  providerId,
  models,
}: {
  providerId: string
  models: ProviderModel[]
}) {
  const [selected, setSelected] = useState<Set<string>>(new Set())
  const [confirmDelete, setConfirmDelete] = useState(false)
  const [results, setResults] = useState<Result[]>([])
  const [testing, setTesting] = useState<Set<string>>(new Set())

  const setActive = useSetModelsActive(providerId)
  const del = useDeleteModels(providerId)
  const test = useTestModel(providerId)

  const table = useTable({
    features,
    columns,
    data: models,
    initialState: { pagination: { pageIndex: 0, pageSize } },
  })

  const filteredRows = table.getFilteredRowModel().rows
  const rows = table.getRowModel().rows
  const pageCount = Math.max(1, table.getPageCount())
  const pageIndex = table.state.pagination.pageIndex
  const nameFilter = (table.getColumn('model')?.getFilterValue() as string) ?? ''
  const statusFilter = (table.getColumn('active')?.getFilterValue() as string) ?? 'all'

  const chosen = useMemo(
    () => models.filter((m) => selected.has(m.model)).map((m) => m.model),
    [models, selected],
  )
  const allFilteredChosen =
    filteredRows.length > 0 && filteredRows.every((r) => selected.has(r.original.model))

  const toggle = (model: string, on: boolean) =>
    setSelected((prev) => {
      const next = new Set(prev)
      if (on) next.add(model)
      else next.delete(model)
      return next
    })

  const toggleAll = (on: boolean) =>
    setSelected((prev) => {
      const next = new Set(prev)
      for (const r of filteredRows) {
        if (on) next.add(r.original.model)
        else next.delete(r.original.model)
      }
      return next
    })

  const switchSelected = (active: boolean) =>
    setActive.mutate(
      { models: chosen, active },
      {
        onSuccess: (r) => {
          toast.success(
            `${active ? 'Đã bật' : 'Đã tắt'} ${r.updated} model.`,
          )
          setSelected(new Set())
        },
        onError: (e) => toast.error(messageOf(e)),
      },
    )

  const deleteSelected = () =>
    del.mutate(chosen, {
      onSuccess: (r) => {
        toast.success(`Đã xóa ${r.deleted} model.`)
        setSelected(new Set())
        setConfirmDelete(false)
      },
      onError: (e) => toast.error(messageOf(e)),
    })

  const runTests = async (list: string[]) => {
    for (const model of list) {
      setTesting((p) => new Set(p).add(model))
      let result: Result
      try {
        result = { ...(await test.mutateAsync(model)), model }
      } catch (e) {
        result = { ok: false, status: 0, ms: 0, message: messageOf(e), model }
      }
      setResults((prev) => [result, ...prev.filter((r) => r.model !== model)])
      setTesting((p) => {
        const next = new Set(p)
        next.delete(model)
        return next
      })
    }
  }

  if (models.length === 0) {
    return (
      <Empty className="border">
        <EmptyHeader>
          <EmptyMedia variant="icon">
            <BoxesIcon />
          </EmptyMedia>
          <EmptyTitle>Chưa có model nào</EmptyTitle>
          <EmptyDescription>
            Nhà cung cấp này chưa có model. Thêm một tài khoản để máy chủ đọc
            danh sách model.
          </EmptyDescription>
        </EmptyHeader>
      </Empty>
    )
  }

  const busy = setActive.isPending || del.isPending

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-center gap-2">
        <InputGroup className="w-full sm:w-64">
          <InputGroupAddon>
            <SearchIcon aria-hidden="true" />
          </InputGroupAddon>
          <InputGroupInput
            aria-label="Lọc model"
            placeholder="Lọc theo tên model"
            value={nameFilter}
            onChange={(e) => {
              table.getColumn('model')?.setFilterValue(e.target.value)
              table.resetPageIndex()
            }}
          />
        </InputGroup>
        <Select
          value={statusFilter}
          onValueChange={(v) => {
            table.getColumn('active')?.setFilterValue(v)
            table.resetPageIndex()
          }}
        >
          <SelectTrigger aria-label="Lọc trạng thái" className="w-36">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">Tất cả</SelectItem>
            <SelectItem value="on">Đang bật</SelectItem>
            <SelectItem value="off">Đang tắt</SelectItem>
          </SelectContent>
        </Select>
        <span className="text-sm text-muted-foreground tabular-nums sm:ml-auto">
          {filteredRows.length} / {models.length} model
        </span>
      </div>

      {chosen.length > 0 ? (
        <div
          role="toolbar"
          aria-label="Thao tác với model đã chọn"
          className="flex flex-wrap items-center gap-2 rounded-lg border bg-muted/50 px-3 py-2"
        >
          <span className="text-sm tabular-nums">Đã chọn {chosen.length}</span>
          <Button variant="outline" size="sm" disabled={busy} onClick={() => switchSelected(true)}>
            <PowerIcon aria-hidden="true" />
            Bật đã chọn
          </Button>
          <Button variant="outline" size="sm" disabled={busy} onClick={() => switchSelected(false)}>
            <PowerOffIcon aria-hidden="true" />
            Tắt đã chọn
          </Button>
          <Button variant="outline" size="sm" disabled={testing.size > 0} onClick={() => void runTests(chosen)}>
            <PlayIcon aria-hidden="true" />
            Thử đã chọn
          </Button>
          <Button variant="destructive" size="sm" disabled={busy} onClick={() => setConfirmDelete(true)}>
            <Trash2Icon aria-hidden="true" />
            Xóa đã chọn
          </Button>
        </div>
      ) : null}

      <Table>
        <TableHeader>
          <TableRow>
            <TableHead className="w-10">
              <Checkbox
                aria-label="Chọn tất cả model đang lọc"
                checked={allFilteredChosen}
                onCheckedChange={(v) => toggleAll(v === true)}
              />
            </TableHead>
            <TableHead>
              <SortHeader
                label="Model"
                name="model"
                sorted={table.getColumn('model')?.getIsSorted() ?? false}
                onToggle={() => table.getColumn('model')?.toggleSorting()}
              />
            </TableHead>
            <TableHead>
              <SortHeader
                label="Trạng thái"
                name="trạng thái"
                sorted={table.getColumn('active')?.getIsSorted() ?? false}
                onToggle={() => table.getColumn('active')?.toggleSorting()}
              />
            </TableHead>
            <TableHead className="hidden md:table-cell">Lần thử cuối</TableHead>
            <TableHead className="hidden lg:table-cell">
              <SortHeader
                label="Thấy lần cuối"
                name="thời điểm thấy"
                sorted={table.getColumn('lastSeen')?.getIsSorted() ?? false}
                onToggle={() => table.getColumn('lastSeen')?.toggleSorting()}
              />
            </TableHead>
            <TableHead className="w-24 text-right">Thử</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {rows.length === 0 ? (
            <TableRow>
              <TableCell colSpan={6} className="h-24 text-center text-muted-foreground">
                Không có model nào khớp bộ lọc.
              </TableCell>
            </TableRow>
          ) : (
            rows.map((row) => {
              const m = row.original
              return (
                <TableRow key={row.id} data-state={selected.has(m.model) ? 'selected' : undefined}>
                  <TableCell>
                    <Checkbox
                      aria-label={`Chọn ${m.model}`}
                      checked={selected.has(m.model)}
                      onCheckedChange={(v) => toggle(m.model, v === true)}
                    />
                  </TableCell>
                  <TableCell className="max-w-0 min-w-40 md:max-w-none">
                    <span data-testid="model-name" className="font-mono text-xs break-all">
                      {m.model}
                    </span>
                    {m.stale ? (
                      <Badge variant="outline" className="ml-2">
                        Cũ
                      </Badge>
                    ) : null}
                    <div className="mt-1 text-xs md:hidden">
                      <TestBadge m={m} />
                    </div>
                  </TableCell>
                  <TableCell>
                    <Badge variant={m.active ? 'secondary' : 'outline'}>
                      {m.active ? 'Đang bật' : 'Đang tắt'}
                    </Badge>
                  </TableCell>
                  <TableCell className="hidden md:table-cell">
                    <TestBadge m={m} />
                  </TableCell>
                  <TableCell className="hidden text-muted-foreground tabular-nums lg:table-cell">
                    {formatTime(m.lastSeen)}
                  </TableCell>
                  <TableCell className="text-right">
                    <Button
                      variant="outline"
                      size="sm"
                      aria-label={`Thử ${m.model}`}
                      disabled={testing.has(m.model)}
                      onClick={() => void runTests([m.model])}
                    >
                      {testing.has(m.model) ? (
                        <Spinner aria-hidden="true" />
                      ) : (
                        <PlayIcon aria-hidden="true" />
                      )}
                    </Button>
                  </TableCell>
                </TableRow>
              )
            })
          )}
        </TableBody>
      </Table>

      <nav
        aria-label="Phân trang model"
        className="flex flex-wrap items-center justify-between gap-2"
      >
        <span className="text-sm text-muted-foreground tabular-nums">
          Trang {pageIndex + 1} / {pageCount}
        </span>
        <div className="flex gap-2">
          <Button
            variant="outline"
            size="sm"
            disabled={!table.getCanPreviousPage()}
            onClick={() => table.previousPage()}
          >
            Trang trước
          </Button>
          <Button
            variant="outline"
            size="sm"
            disabled={!table.getCanNextPage()}
            onClick={() => table.nextPage()}
          >
            Trang sau
          </Button>
        </div>
      </nav>

      {results.length > 0 ? (
        <section
          role="status"
          aria-label="Kết quả thử model"
          className="flex flex-col gap-2 rounded-lg border p-3"
        >
          <h3 className="text-sm font-medium">Kết quả thử model</h3>
          <ul className="flex flex-col gap-2">
            {results.map((r) => (
              <li key={r.model} className="flex flex-wrap items-center gap-2 text-sm">
                <span className="font-mono text-xs break-all">{r.model}</span>
                <Badge variant={r.ok ? 'secondary' : 'destructive'}>
                  {r.ok ? 'Thành công' : 'Lỗi'}
                </Badge>
                <span className="text-xs text-muted-foreground tabular-nums">
                  {r.ms} ms
                </span>
                {r.message ? (
                  <span className="min-w-0 break-words text-muted-foreground">
                    {r.message}
                  </span>
                ) : null}
              </li>
            ))}
          </ul>
        </section>
      ) : null}

      <AlertDialog open={confirmDelete} onOpenChange={setConfirmDelete}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Xóa {chosen.length} model?</AlertDialogTitle>
            <AlertDialogDescription>
              Các model đã chọn bị xóa khỏi bảng. Lần đọc danh sách sau có thể
              thêm lại những model nhà cung cấp còn công bố.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Hủy</AlertDialogCancel>
            <AlertDialogAction
              variant="destructive"
              onClick={(e) => {
                e.preventDefault()
                deleteSelected()
              }}
            >
              Xóa model
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}
