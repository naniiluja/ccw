import { FilterIcon } from 'lucide-react'
import { PagePlaceholder } from '@/components/app/page-placeholder'

export default function FiltersPage() {
  return (
    <PagePlaceholder
      title="Bộ lọc"
      description="Danh sách field bị loại khỏi request gửi đi."
      icon={FilterIcon}
    />
  )
}
