import { CompassIcon } from 'lucide-react'
import { Link } from 'react-router'
import { Button } from '@/components/ui/button'
import {
  Empty,
  EmptyContent,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from '@/components/ui/empty'

export default function NotFoundPage() {
  return (
    <Empty>
      <EmptyHeader>
        <EmptyMedia variant="icon">
          <CompassIcon aria-hidden="true" />
        </EmptyMedia>
        <EmptyTitle>Không tìm thấy trang</EmptyTitle>
        <EmptyDescription>
          Đường dẫn này không tồn tại hoặc đã được chuyển đi.
        </EmptyDescription>
      </EmptyHeader>
      <EmptyContent>
        <Button asChild>
          <Link to="/">Về trang tổng quan</Link>
        </Button>
      </EmptyContent>
    </Empty>
  )
}
