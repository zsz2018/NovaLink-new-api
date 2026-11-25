import { useQuery } from '@tanstack/react-query'
import { useId, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { CopyButton } from '@/components/copy-button'
import { Dialog } from '@/components/dialog'
import { ErrorState } from '@/components/error-state'
import { LoadingState } from '@/components/loading-state'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { getServerErrorMessage } from '@/lib/server-error-message'

import { getMonitorProfitDetail, monitorQueryKey } from '../api'
import { formatMonitorMessage } from '../lib/task-messages'
import type { MonitorProfitDetailRequest } from '../types'
import { ProfitDetailBreakdown } from './profit-detail-breakdown'

const maxInputTokens = 100_000_000

export function ProfitDetail(props: {
  request: MonitorProfitDetailRequest
  onClose: () => void
}) {
  const { t } = useTranslation()
  const inputID = useId()
  const [draft, setDraft] = useState<string | null>(null)
  const [inputTokens, setInputTokens] = useState<number | undefined>()
  const [tab, setTab] = useState('visual')
  const request = { ...props.request, input_tokens: inputTokens }
  const query = useQuery({
    queryKey: [...monitorQueryKey, 'profit-detail', request],
    queryFn: () => getMonitorProfitDetail(request),
    refetchOnMount: 'always',
    meta: { errorToast: false },
  })
  const input =
    draft ?? String(inputTokens ?? query.data?.scenario.input_tokens ?? '')
  const inputNumber = Number(input)
  const valid = /^\d+$/.test(input) && inputNumber <= maxInputTokens
  const json = query.data ? JSON.stringify(query.data, null, 2) : ''

  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) props.onClose()
      }}
      title={t('Profit details')}
      description={`${props.request.local_model} · ${props.request.local_group}`}
      contentClassName='w-[calc(100%-2rem)] sm:max-w-7xl'
    >
      <div className='space-y-4'>
        <form
          className='flex flex-wrap items-end gap-2 rounded-lg border p-3'
          onSubmit={(event) => {
            event.preventDefault()
            if (valid) {
              setInputTokens(inputNumber)
              setDraft(null)
            }
          }}
        >
          <div className='space-y-2'>
            <Label htmlFor={inputID}>{t('Uncached input tokens')}</Label>
            <Input
              id={inputID}
              type='number'
              min={0}
              max={maxInputTokens}
              step={1}
              required
              value={input}
              onChange={(event) => setDraft(event.target.value)}
            />
          </div>
          <Button type='submit' disabled={!valid || query.isFetching}>
            {t('Apply')}
          </Button>
          <Button
            type='button'
            variant='outline'
            disabled={draft === null && inputTokens === undefined}
            onClick={() => {
              setInputTokens(undefined)
              setDraft(null)
            }}
          >
            {t('Reset')}
          </Button>
          <p className='text-muted-foreground w-full text-xs'>
            {t(
              'Preview using a different input length. The saved scenario is unchanged.'
            )}
          </p>
        </form>
        {query.isPending && <LoadingState />}
        {query.isError && (
          <ErrorState
            title={t('Could not load profit details')}
            description={formatMonitorMessage(
              getServerErrorMessage(query.error),
              t
            )}
            onRetry={() => void query.refetch()}
          />
        )}
        {query.isSuccess && (
          <Tabs value={tab} onValueChange={setTab}>
            <TabsList aria-label={t('Profit details')}>
              <TabsTrigger value='visual'>{t('Visual')}</TabsTrigger>
              <TabsTrigger value='json'>{t('JSON')}</TabsTrigger>
            </TabsList>
            <TabsContent value='visual'>
              <ProfitDetailBreakdown detail={query.data} />
            </TabsContent>
            <TabsContent value='json'>
              <div className='space-y-2'>
                <div className='flex justify-end'>
                  <CopyButton value={json} />
                </div>
                <pre className='bg-muted max-h-[60vh] overflow-auto rounded-lg p-3 text-xs'>
                  <code>{json}</code>
                </pre>
              </div>
            </TabsContent>
          </Tabs>
        )}
      </div>
    </Dialog>
  )
}
