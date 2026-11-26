import { useTranslation } from 'react-i18next'

import { CopyButton } from '@/components/copy-button'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'

export function EncryptionKeyNotice(props: { ready: boolean }) {
  const { t } = useTranslation()
  if (props.ready) return null

  const generateCommand = 'openssl rand -base64 32'
  const recreateCommand =
    'docker compose up -d --no-deps --force-recreate new-api'
  const recreateDevCommand =
    'docker compose -f docker-compose.dev.yml up -d --no-deps --force-recreate new-api'

  return (
    <Alert
      variant='destructive'
      className='max-h-[50%] shrink-0 overflow-y-auto'
    >
      <AlertTitle>{t('Credential encryption is not configured')}</AlertTitle>
      <AlertDescription>
        {t(
          'An administrator must configure the upstream monitor encryption key before saving credentials or running tasks.'
        )}
        <details className='text-foreground mt-2 min-w-0'>
          <summary className='focus-visible:ring-ring cursor-pointer rounded-sm font-medium focus-visible:ring-2 focus-visible:outline-none'>
            {t('How to configure the encryption key')}
          </summary>
          <ol className='mt-3 list-decimal space-y-3 pl-5'>
            <li>
              {t(
                'For first-time setup, generate a random 32-byte Base64 key on the server:'
              )}
              <div className='bg-muted mt-1 flex min-w-0 items-start gap-2 rounded-md p-2'>
                <code className='min-w-0 flex-1 self-center break-all'>
                  {generateCommand}
                </code>
                <CopyButton value={generateCommand} className='size-7' />
              </div>
            </li>
            <li>
              {t(
                'In docker-compose.yml or docker-compose.dev.yml, find the new-api service environment section. Uncomment UPSTREAM_MONITOR_KEY and replace the placeholder with the generated key:'
              )}
              <code className='bg-muted mt-1 block rounded-md p-2 break-all'>
                {'- UPSTREAM_MONITOR_KEY=<BASE64_KEY>'}
              </code>
            </li>
            <li>
              {t(
                'Recreate the backend container to apply the environment variable; restarting the existing container does not load changes:'
              )}
              <div className='bg-muted mt-1 flex min-w-0 items-start gap-2 rounded-md p-2'>
                <code className='min-w-0 flex-1 self-center break-all'>
                  {recreateCommand}
                </code>
                <CopyButton value={recreateCommand} className='size-7' />
              </div>
              <div className='mt-2'>
                {t('For docker-compose.dev.yml, use:')}
              </div>
              <div className='bg-muted mt-1 flex min-w-0 items-start gap-2 rounded-md p-2'>
                <code className='min-w-0 flex-1 self-center break-all'>
                  {recreateDevCommand}
                </code>
                <CopyButton value={recreateDevCommand} className='size-7' />
              </div>
              <div className='mt-2'>
                {t(
                  'Replace new-api in these commands if your backend service has a different name. Refresh this page after the backend starts.'
                )}
              </div>
            </li>
          </ol>
          <div className='mt-3'>
            {t(
              'For a direct executable deployment, set UPSTREAM_MONITOR_KEY in the backend environment or in .env in its working directory, then restart the backend.'
            )}
          </div>
          <div className='mt-3'>
            {t(
              'Keep the key backed up separately from the database and use the same key on every backend node. If credentials are already saved, restore the original key instead of generating a replacement.'
            )}
          </div>
        </details>
      </AlertDescription>
    </Alert>
  )
}
