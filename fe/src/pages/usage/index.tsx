import { ActivityIcon } from 'lucide-react'
import { PagePlaceholder } from '@/components/app/page-placeholder'

export default function UsagePage() {
  return (
    <PagePlaceholder
      title="Lượng dùng"
      description="Số request và token theo thời gian, model và khóa."
      icon={ActivityIcon}
    />
  )
}
