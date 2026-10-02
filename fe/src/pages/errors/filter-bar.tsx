import { FilterXIcon, SearchIcon } from 'lucide-react'
import { useState, type FormEvent } from 'react'
import { errorClasses, type ErrorFilter } from '@/api/errors'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  classLabel,
  defaultLimit,
  pageSizes,
  sinceFor,
  sinceRanges,
} from './format'

export type FilterPatch = Partial<
  Record<'provider' | 'class' | 'signature' | 'status' | 'since' | 'limit', string>
>

interface FilterBarProps {
  filter: ErrorFilter
  providers: string[]
  /** Sets or (with an empty string) clears URL filters. */
  onChange: (patch: FilterPatch) => void
  onReset: () => void
}

const ALL = 'all'

function Labelled({
  label,
  htmlFor,
  children,
  className,
}: {
  label: string
  htmlFor?: string
  children: React.ReactNode
  className?: string
}) {
  return (
    <div className={`flex min-w-0 flex-col gap-1.5 ${className ?? ''}`}>
      <Label htmlFor={htmlFor} className="text-xs text-muted-foreground">
        {label}
      </Label>
      {children}
    </div>
  )
}

export function FilterBar({
  filter,
  providers,
  onChange,
  onReset,
}: FilterBarProps) {
  const [status, setStatus] = useState(filter.status ? String(filter.status) : '')
  const [signature, setSignature] = useState(filter.signature ?? '')

  const options = [...new Set([...providers, filter.provider ?? ''])]
    .filter(Boolean)
    .sort()
  const sinceValue = filter.since ? 'custom' : ALL
  const dirty =
    filter.provider ||
    filter.class ||
    filter.signature ||
    filter.status ||
    filter.since

  function submit(e: FormEvent) {
    e.preventDefault()
    onChange({ status: status.trim(), signature: signature.trim() })
  }

  return (
    <form
      onSubmit={submit}
      aria-label="Bộ lọc lỗi"
      className="grid grid-cols-1 items-end gap-3 sm:grid-cols-2 lg:grid-cols-4 xl:grid-cols-[repeat(6,minmax(0,1fr))_auto]"
    >
      <Labelled label="Nhà cung cấp">
        <Select
          value={filter.provider ?? ALL}
          onValueChange={(v) => onChange({ provider: v === ALL ? '' : v })}
        >
          <SelectTrigger aria-label="Nhà cung cấp" className="w-full">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value={ALL}>Tất cả</SelectItem>
            {options.map((p) => (
              <SelectItem key={p} value={p}>
                {p}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </Labelled>
      <Labelled label="Phân loại">
        <Select
          value={filter.class ?? ALL}
          onValueChange={(v) => onChange({ class: v === ALL ? '' : v })}
        >
          <SelectTrigger aria-label="Phân loại" className="w-full">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value={ALL}>Tất cả</SelectItem>
            {errorClasses.map((c) => (
              <SelectItem key={c} value={c}>
                {classLabel(c)}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </Labelled>
      <Labelled label="Khoảng thời gian">
        <Select
          value={sinceValue}
          onValueChange={(v) => {
            const range = sinceRanges.find((r) => r.value === v)
            if (range) onChange({ since: range.ms ? sinceFor(range.ms) : '' })
          }}
        >
          <SelectTrigger aria-label="Khoảng thời gian" className="w-full">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {sinceRanges.map((r) => (
              <SelectItem key={r.value} value={r.value}>
                {r.label}
              </SelectItem>
            ))}
            <SelectItem value="custom" disabled>
              Mốc thời gian đã chọn
            </SelectItem>
          </SelectContent>
        </Select>
      </Labelled>
      <Labelled label="Mã trạng thái" htmlFor="errors-status">
        <Input
          id="errors-status"
          inputMode="numeric"
          pattern="[0-9]*"
          placeholder="429"
          value={status}
          onChange={(e) => setStatus(e.target.value.replace(/\D/g, ''))}
          onBlur={() => status !== String(filter.status ?? '') && onChange({ status })}
          className="tabular-nums"
        />
      </Labelled>
      <Labelled label="Chữ ký" htmlFor="errors-signature" className="sm:col-span-2 lg:col-span-1 xl:col-span-2">
        <Input
          id="errors-signature"
          placeholder="Một phần chữ ký"
          value={signature}
          onChange={(e) => setSignature(e.target.value)}
          onBlur={() =>
            signature.trim() !== (filter.signature ?? '') &&
            onChange({ signature: signature.trim() })
          }
        />
      </Labelled>
      <Labelled label="Số dòng">
        <Select
          value={String(filter.limit ?? defaultLimit)}
          onValueChange={(v) => onChange({ limit: v })}
        >
          <SelectTrigger aria-label="Số dòng" className="w-full">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {[...new Set([...pageSizes, filter.limit ?? defaultLimit])]
              .sort((a, b) => a - b)
              .map((n) => (
                <SelectItem key={n} value={String(n)}>
                  {n} dòng
                </SelectItem>
              ))}
          </SelectContent>
        </Select>
      </Labelled>
      <div className="flex gap-2 sm:col-span-2 lg:col-span-4 xl:col-span-1">
        <Button type="submit" variant="secondary">
          <SearchIcon aria-hidden="true" data-icon="inline-start" />
          Áp dụng
        </Button>
        <Button
          type="button"
          variant="ghost"
          disabled={!dirty}
          onClick={() => {
            setStatus('')
            setSignature('')
            onReset()
          }}
        >
          <FilterXIcon aria-hidden="true" data-icon="inline-start" />
          Xóa bộ lọc
        </Button>
      </div>
    </form>
  )
}
