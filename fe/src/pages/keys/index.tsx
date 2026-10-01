import { KeyRoundIcon } from 'lucide-react'
import { PagePlaceholder } from '@/components/app/page-placeholder'

export default function KeysPage() {
  return (
    <PagePlaceholder
      title="Khóa API"
      description="Khóa API, giới hạn RPM và danh sách model được phép."
      icon={KeyRoundIcon}
    />
  )
}
