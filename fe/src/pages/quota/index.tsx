import { GaugeIcon } from 'lucide-react'
import { PagePlaceholder } from '@/components/app/page-placeholder'

export default function QuotaPage() {
  return (
    <PagePlaceholder
      title="Hạn mức"
      description="Hạn mức còn lại theo từng tài khoản và lần reset kế tiếp."
      icon={GaugeIcon}
    />
  )
}
