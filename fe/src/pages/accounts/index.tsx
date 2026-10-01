import { UsersIcon } from 'lucide-react'
import { PagePlaceholder } from '@/components/app/page-placeholder'

export default function AccountsPage() {
  return (
    <PagePlaceholder
      title="Tài khoản"
      description="Tài khoản nhà cung cấp mà gateway xoay vòng và failover."
      icon={UsersIcon}
    />
  )
}
