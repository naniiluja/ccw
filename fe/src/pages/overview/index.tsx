import { LayoutDashboardIcon } from 'lucide-react'
import { PagePlaceholder } from '@/components/app/page-placeholder'

export default function OverviewPage() {
  return (
    <PagePlaceholder
      title="Tổng quan"
      description="Tình trạng chung của gateway: tài khoản, lượng dùng và lỗi gần đây."
      icon={LayoutDashboardIcon}
    />
  )
}
