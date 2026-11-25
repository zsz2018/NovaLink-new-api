import { useTranslation } from 'react-i18next'

import { CopyButton } from '@/components/copy-button'
import { Dialog } from '@/components/dialog'
import { Textarea } from '@/components/ui/textarea'
import { formatTimestamp } from '@/lib/format'

import { formatMonitorMessage } from '../lib/task-messages'
import type { MonitorSnapshot } from '../types'
import { SnapshotPricingStatus } from './snapshot-pricing-status'

export function SnapshotReview(props: {
  snapshot: MonitorSnapshot
  onClose: () => void
}) {
  const { t } = useTranslation()
  const value = JSON.stringify(props.snapshot.data, null, 2)
  const usable = props.snapshot.complete && props.snapshot.confirmed
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) props.onClose()
      }}
      title={t('Collection result')}
      contentClassName='sm:max-w-5xl'
    >
      <div className='space-y-3'>
        <div className='flex items-center justify-between gap-3'>
          <p className='text-muted-foreground text-xs'>
            {formatTimestamp(props.snapshot.created_at)} ·{' '}
            {usable
              ? t('Used for cost comparisons')
              : t('Unavailable for cost comparisons')}
          </p>
          <CopyButton value={value} aria-label={t('Copy structured data')} />
        </div>
        <SnapshotPricingStatus
          data={props.snapshot.data}
          coreComplete={usable}
        />
        {props.snapshot.data.groups.some((group) => group.cost_unsupported) && (
          <ul className='space-y-1 text-sm'>
            {props.snapshot.data.groups
              .filter((group) => group.cost_unsupported)
              .map((group) => (
                <li key={group.name} className='break-words'>
                  {group.name}:{' '}
                  {formatMonitorMessage(group.cost_unsupported ?? '', t)}
                </li>
              ))}
          </ul>
        )}
        {props.snapshot.data.warnings?.length > 0 && (
          <ul className='list-disc space-y-1 pl-5 text-sm'>
            {[...new Set(props.snapshot.data.warnings)].map((warning) => (
              <li key={warning}>{formatMonitorMessage(warning, t)}</li>
            ))}
          </ul>
        )}
        <Textarea
          value={value}
          readOnly
          aria-label={t('Structured upstream data')}
          className='h-[55vh] max-h-[65vh] min-h-64 resize-none font-mono text-xs'
        />
      </div>
    </Dialog>
  )
}
