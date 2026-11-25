import { useTranslation } from 'react-i18next'

import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'

import type { MonitorPriceSource } from '../types'

export function OfficialEstimateNotice() {
  const { t } = useTranslation()
  return (
    <Alert role='note'>
      <AlertDescription>
        {t(
          'Official references estimate procurement costs only when upstream quotes are missing. Captured quotes and snapshots stay unchanged. Estimates do not confirm upstream model availability.'
        )}
      </AlertDescription>
    </Alert>
  )
}

export function OfficialEstimateSource(props: { source: MonitorPriceSource }) {
  const { t } = useTranslation()
  let sourceURL: string | undefined
  try {
    const url = new URL(props.source.source_url)
    if (
      /^https:\/\//i.test(props.source.source_url) &&
      !/[\s\p{Cc}\\]/u.test(props.source.source_url) &&
      url.protocol === 'https:' &&
      url.hostname &&
      !url.username &&
      !url.password
    ) {
      sourceURL = props.source.source_url
    }
  } catch {
    // Keep invalid source metadata readable without creating a link.
  }
  return (
    <dl className='grid min-w-0 gap-3 text-sm sm:grid-cols-2'>
      {[
        [t('Provider'), props.source.provider],
        [t('Official model'), props.source.model],
        [t('Catalog version'), props.source.catalog_version],
        [t('Catalog entry'), props.source.entry_id],
        [t('Verified at'), props.source.verified_at],
      ].map(([label, value]) => (
        <div key={label} className='min-w-0 space-y-1'>
          <dt className='text-muted-foreground text-xs'>{label}</dt>
          <dd className='break-all whitespace-pre-wrap'>{value}</dd>
        </div>
      ))}
      <div className='min-w-0 space-y-1 sm:col-span-2'>
        <dt className='text-muted-foreground text-xs'>{t('Source URL')}</dt>
        <dd className='break-all whitespace-pre-wrap'>
          {sourceURL ? (
            <Button
              role='link'
              variant='link'
              size='xs'
              className='h-auto max-w-full justify-start px-0 text-left break-all whitespace-normal'
              render={
                <a href={sourceURL} target='_blank' rel='noopener noreferrer' />
              }
            >
              {props.source.source_url}
            </Button>
          ) : (
            props.source.source_url
          )}
        </dd>
      </div>
    </dl>
  )
}
