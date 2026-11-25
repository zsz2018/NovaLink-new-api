import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { Dialog } from '@/components/dialog'
import { ErrorState } from '@/components/error-state'
import { LoadingState } from '@/components/loading-state'
import { Button } from '@/components/ui/button'

import {
  controlMonitorJob,
  createMonitorJob,
  getMonitorJob,
  monitorQueryKey,
} from '../api'
import { monitorJobActive } from '../lib/jobs'
import { formatMonitorMessage } from '../lib/task-messages'
import type { MonitorConfig } from '../types'
import { FlowSteps } from './flow-details'
import { SnapshotReview } from './snapshot-review'

export function JobDetail(props: {
  id: string
  config: MonitorConfig
  onClose: () => void
  onJobCreated: (id: string) => void
}) {
  const { t } = useTranslation()
  const client = useQueryClient()
  const [review, setReview] = useState(false)
  const [confirmCancel, setConfirmCancel] = useState(false)
  const detail = useQuery({
    queryKey: [...monitorQueryKey, 'job', props.id],
    queryFn: () => getMonitorJob(props.id),
    refetchInterval: (query) =>
      query.state.data?.job && monitorJobActive(query.state.data.job)
        ? 3000
        : false,
  })
  const control = useMutation({
    mutationFn: (
      action:
        | 'cancel'
        | 'resume'
        | 'retry'
        | Parameters<typeof createMonitorJob>[0]
    ) =>
      typeof action === 'string'
        ? controlMonitorJob(props.id, action)
        : createMonitorJob(action),
    onSuccess: (next, action) => {
      setConfirmCancel(false)
      void client.invalidateQueries({ queryKey: monitorQueryKey })
      if (typeof action !== 'string' || action === 'retry') {
        props.onJobCreated(next.id)
      }
    },
  })
  const result = detail.data
  const job = result?.job
  const active = Boolean(job && monitorJobActive(job))
  const account = props.config.accounts.find(
    (item) => item.id === job?.account_id
  )
  return (
    <>
      <Dialog
        open={!review && !confirmCancel}
        onOpenChange={(open) => {
          if (!open) props.onClose()
        }}
        title={t('Browser task details')}
        contentClassName='sm:max-w-5xl'
      >
        {detail.isLoading && <LoadingState />}
        {detail.isError && (
          <ErrorState
            onRetry={() => void detail.refetch()}
            title={t('Could not load browser task')}
          />
        )}
        {result && job && (
          <div className='space-y-5'>
            <div className='space-y-2'>
              <p className='font-medium'>
                {account?.name ?? job.account_id} · {t(job.status)}
              </p>
              {(job.status !== 'completed' || job.mode !== 'record') && (
                <p className='text-muted-foreground text-sm break-words'>
                  {t('Step {{number}}', { number: job.step })} ·{' '}
                  {formatMonitorMessage(job.message, t)}
                </p>
              )}
              {job.error && (
                <p
                  className='text-destructive text-sm break-words'
                  role='alert'
                >
                  {formatMonitorMessage(job.error, t)}
                </p>
              )}
            </div>
            {job.mode === 'record' && (
              <p className='text-muted-foreground text-sm'>
                {job.status === 'completed'
                  ? t(
                      'Configuration is complete. Editing starts a new browser task with the latest saved flow.'
                    )
                  : t(
                      'Use the browser extension side panel to configure, save, and finish this flow. Recorded steps below are read only.'
                    )}
              </p>
            )}
            <div className='flex flex-wrap gap-2'>
              {job.status === 'awaiting_user' && (
                <Button
                  disabled={control.isPending}
                  onClick={() => control.mutate('resume')}
                >
                  {t('Continue after manual verification')}
                </Button>
              )}
              {active && (
                <Button
                  variant='outline'
                  disabled={control.isPending}
                  onClick={() => setConfirmCancel(true)}
                >
                  {t('Cancel task')}
                </Button>
              )}
              {!active && job.status === 'completed' && (
                <Button
                  variant='outline'
                  disabled={control.isPending}
                  onClick={() =>
                    control.mutate({
                      account_id: job.account_id,
                      device_id: job.device_id,
                      flow_id: job.flow_id,
                      mode: job.mode,
                    })
                  }
                >
                  {job.mode === 'record'
                    ? t('Edit collection flow')
                    : t('Collect again')}
                </Button>
              )}
              {!active &&
                (job.status === 'failed' || job.status === 'cancelled') && (
                  <Button
                    variant='outline'
                    disabled={control.isPending}
                    onClick={() => control.mutate('retry')}
                  >
                    {t('Retry task')}
                  </Button>
                )}
              {result.snapshot && (
                <Button variant='outline' onClick={() => setReview(true)}>
                  {t('View collection result')}
                </Button>
              )}
            </div>
            {result.recorded_steps.length > 0 && (
              <section className='space-y-3'>
                <h3 className='font-medium'>{t('Recorded browser steps')}</h3>
                <FlowSteps steps={result.recorded_steps} />
              </section>
            )}
          </div>
        )}
      </Dialog>
      {review && result?.snapshot && (
        <SnapshotReview
          snapshot={result.snapshot}
          onClose={() => setReview(false)}
        />
      )}
      <ConfirmDialog
        open={confirmCancel}
        onOpenChange={(open) => {
          if (!open && !control.isPending) setConfirmCancel(false)
        }}
        title={t('Cancel browser task?')}
        desc={t(
          'The browser will stop this task and discard temporary response bodies.'
        )}
        isLoading={control.isPending}
        destructive
        handleConfirm={() => control.mutate('cancel')}
      />
    </>
  )
}
