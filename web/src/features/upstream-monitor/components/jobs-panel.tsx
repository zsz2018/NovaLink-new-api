import { useTranslation } from 'react-i18next'

import { StaticDataTable } from '@/components/data-table'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { formatTimestamp } from '@/lib/format'

import { formatMonitorMessage } from '../lib/task-messages'
import type { MonitorState } from '../types'
import { SnapshotPricingStatus } from './snapshot-pricing-status'

export function JobsPanel(props: {
  state: MonitorState
  onJob: (id: string) => void
  onSnapshot: (id: string) => void
}) {
  const { t } = useTranslation()
  return (
    <div className='space-y-6'>
      <section className='space-y-3'>
        <h3 className='font-medium'>{t('Browser collection tasks')}</h3>
        <StaticDataTable
          data={props.state.jobs}
          getRowKey={(job) => job.id}
          emptyContent={t(
            'Start a collection or recording from an upstream account.'
          )}
          columns={[
            {
              id: 'account',
              header: t('Account / task'),
              cell: (job) => (
                <div>
                  <p>
                    {props.state.config.accounts.find(
                      (account) => account.id === job.account_id
                    )?.name ?? job.account_id}
                  </p>
                  <p className='text-muted-foreground text-xs'>
                    {job.mode === 'record' ? t('Recording') : t('Collection')}
                  </p>
                </div>
              ),
            },
            {
              id: 'status',
              header: t('Status'),
              cell: (job) => (
                <div className='space-y-1'>
                  <Badge
                    variant={
                      job.status === 'failed' ? 'destructive' : 'outline'
                    }
                  >
                    {t(job.status)}
                  </Badge>
                  <p className='text-muted-foreground max-w-xs text-xs break-words'>
                    {formatMonitorMessage(job.error || job.message, t)}
                  </p>
                </div>
              ),
            },
            {
              id: 'time',
              header: t('Updated'),
              cell: (job) => formatTimestamp(job.updated_at),
            },
            {
              id: 'details',
              header: t('Actions'),
              cell: (job) => (
                <Button
                  variant='outline'
                  size='sm'
                  onClick={() => props.onJob(job.id)}
                >
                  {t('Task details')}
                </Button>
              ),
            },
          ]}
        />
      </section>
      <section className='space-y-3'>
        <h3 className='font-medium'>{t('Structured collection snapshots')}</h3>
        <StaticDataTable
          data={props.state.snapshots}
          getRowKey={(snapshot) => snapshot.id}
          emptyContent={t(
            'Start a collection or recording from an upstream account.'
          )}
          columns={[
            {
              id: 'account',
              header: t('Account'),
              cell: (snapshot) =>
                props.state.config.accounts.find(
                  (account) => account.id === snapshot.account_id
                )?.name ?? snapshot.account_id,
            },
            {
              id: 'status',
              header: t('Status'),
              cell: (snapshot) => (
                <div className='flex flex-wrap gap-2'>
                  <Badge variant='outline'>
                    {snapshot.confirmed && snapshot.complete
                      ? t('Used for cost comparisons')
                      : t('Unavailable for cost comparisons')}
                  </Badge>
                  <SnapshotPricingStatus
                    data={snapshot.data}
                    coreComplete={snapshot.complete && snapshot.confirmed}
                  />
                </div>
              ),
            },
            {
              id: 'time',
              header: t('Collected'),
              cell: (snapshot) => formatTimestamp(snapshot.created_at),
            },
            {
              id: 'details',
              header: t('Actions'),
              cell: (snapshot) => (
                <div className='flex gap-2'>
                  <Button
                    variant='outline'
                    size='sm'
                    onClick={() => props.onSnapshot(snapshot.id)}
                  >
                    {t('View collection result')}
                  </Button>
                  <Button
                    variant='ghost'
                    size='sm'
                    onClick={() => props.onJob(snapshot.job_id)}
                  >
                    {t('Source task')}
                  </Button>
                </div>
              ),
            },
          ]}
        />
      </section>
    </div>
  )
}
