import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'

import type { MonitorData } from '../types'

export function SnapshotPricingStatus(props: {
  data: MonitorData
  coreComplete: boolean
}) {
  const { t } = useTranslation()
  if (!props.data.pricing_status) return null
  const labels = {
    available: t('Captured quotes available'),
    partial: t('Some upstream quotes were not captured.'),
    unavailable: props.coreComplete
      ? t('Account, balance and ratios collected; no quotes captured.')
      : t('No upstream quotes captured.'),
  }
  return (
    <Badge variant='outline' className='h-auto max-w-full whitespace-normal'>
      {labels[props.data.pricing_status]}
    </Badge>
  )
}
