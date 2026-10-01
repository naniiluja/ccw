import { TriangleAlertIcon } from 'lucide-react'
import { PagePlaceholder } from '@/components/app/page-placeholder'

export default function ErrorsPage() {
  return (
    <PagePlaceholder
      title="Lỗi upstream"
      description="Lỗi trả về từ nhà cung cấp, được nhóm theo nội dung."
      icon={TriangleAlertIcon}
    />
  )
}
