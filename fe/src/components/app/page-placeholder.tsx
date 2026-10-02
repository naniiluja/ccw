import type { LucideIcon } from 'lucide-react'
import { PageHeader } from '@/components/app/page-header'
import { Card } from '@/components/ui/card'
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from '@/components/ui/empty'

interface PagePlaceholderProps {
  title: string
  description: string
  icon: LucideIcon
}

// PagePlaceholder stands in for a feature page until its task fills it in.
export function PagePlaceholder({
  title,
  description,
  icon: Icon,
}: PagePlaceholderProps) {
  return (
    <div className="flex flex-col gap-6">
      <PageHeader title={title} description={description} />
      <Card className="p-0">
        <Empty>
          <EmptyHeader>
            <EmptyMedia variant="icon">
              <Icon aria-hidden="true" />
            </EmptyMedia>
            <EmptyTitle>Trang đang được xây dựng</EmptyTitle>
            <EmptyDescription>
              Nội dung của mục này sẽ có ở bản cập nhật tiếp theo.
            </EmptyDescription>
          </EmptyHeader>
        </Empty>
      </Card>
    </div>
  )
}
