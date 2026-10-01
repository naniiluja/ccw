import { SettingsIcon } from 'lucide-react'
import { PagePlaceholder } from '@/components/app/page-placeholder'

export default function SettingsPage() {
  return (
    <PagePlaceholder
      title="Cài đặt"
      description="Cấu hình chạy của máy chủ."
      icon={SettingsIcon}
    />
  )
}
