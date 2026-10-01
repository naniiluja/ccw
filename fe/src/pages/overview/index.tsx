import { useQuery } from '@tanstack/react-query'
import { UsersIcon } from 'lucide-react'
import { useMemo, useState } from 'react'
import { Link } from 'react-router'
import { accountTestsQuery, accountsQuery } from '@/api/accounts'
import { useDriftChanges } from '@/api/drift'
import { useErrorStats, useErrors } from '@/api/errors'
import { lowQuotaAccounts } from '@/api/overview'
import { useQuota } from '@/api/quota'
import { PageHeader } from '@/components/app/page-header'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import {
  Empty,
  EmptyContent,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from '@/components/ui/empty'
import { LatestDriftPanel, LatestErrorsPanel, LowQuotaPanel } from './attention'
import { StatCards } from './stat-cards'
import { UsageBlock } from './usage-block'

const dayMs = 86_400_000

/** An RFC 3339 time one day back, fixed to the minute so the query key holds still. */
function sinceOneDay(): string {
  const now = Math.floor(Date.now() / 60_000) * 60_000
  return new Date(now - dayMs).toISOString().replace(/\.\d{3}Z$/, 'Z')
}

export default function OverviewPage() {
  const [since] = useState(sinceOneDay)
  const accounts = useQuery(accountsQuery)
  const tests = useQuery(accountTestsQuery)
  const quota = useQuota()
  const stats = useErrorStats({ since })
  const latestErrors = useErrors({ limit: 5 })
  const drift = useDriftChanges({ unacked: true, limit: 5 })

  const low = useMemo(() => lowQuotaAccounts(quota.data ?? []), [quota.data])
  const lowSource = { ...quota, data: quota.isSuccess ? low : undefined }
  const driftList = { ...drift, data: drift.data?.changes }

  const noAccounts = accounts.isSuccess && accounts.data.length === 0

  return (
    <div className="flex flex-col gap-6">
      <PageHeader
        title="Tổng quan"
        description="Tình trạng chung của gateway: tài khoản, lượng dùng và lỗi gần đây."
      />
      {noAccounts ? (
        <Card className="p-0">
          <Empty>
            <EmptyHeader>
              <EmptyMedia variant="icon">
                <UsersIcon aria-hidden="true" />
              </EmptyMedia>
              <EmptyTitle>Chưa có tài khoản nào</EmptyTitle>
              <EmptyDescription>
                Thêm tài khoản nhà cung cấp đầu tiên để gateway bắt đầu phục vụ request.
              </EmptyDescription>
            </EmptyHeader>
            <EmptyContent>
              <Button asChild>
                <Link to="/accounts">Thêm tài khoản đầu tiên</Link>
              </Button>
            </EmptyContent>
          </Empty>
        </Card>
      ) : null}
      <StatCards
        s={{ accounts, tests, quota, errors: stats, drift, errorsSince: since }}
      />
      <UsageBlock />
      <section aria-labelledby="attention-title" className="flex flex-col gap-3">
        <h2 id="attention-title" className="text-base font-semibold">
          Cần chú ý
        </h2>
        <div className="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-3">
          <LowQuotaPanel source={lowSource} />
          <LatestErrorsPanel source={latestErrors} />
          <LatestDriftPanel source={driftList} />
        </div>
      </section>
    </div>
  )
}
