import { useId, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { StaticDataTable } from '@/components/data-table'
import { Button } from '@/components/ui/button'
import { toIntlLocale } from '@/i18n/languages'
import { formatLocalCurrencyAmount } from '@/lib/currency'
import { formatNumber, formatTimestamp } from '@/lib/format'
import { handleServerError } from '@/lib/handle-server-error'

import { latestMonitorSnapshots } from '../lib/jobs'
import type {
  MonitorAccount,
  MonitorSite,
  MonitorState,
  SaveMonitorConfig,
} from '../types'
import { AccountCostDialog } from './account-cost-dialog'
import { AccountDialog } from './account-dialog'
import { SiteDialog } from './site-dialog'

export function SitesPanel(props: {
  state: MonitorState
  onSave: SaveMonitorConfig
  onRun: (account: MonitorAccount) => void
}) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const sectionId = useId()
  const latestByAccount = latestMonitorSnapshots(props.state.snapshots)
  const [site, setSite] = useState<MonitorSite | 'new' | null>(null)
  const [account, setAccount] = useState<MonitorAccount | 'new' | null>(null)
  const [costAccount, setCostAccount] = useState<MonitorAccount | null>(null)
  const [remove, setRemove] = useState<{
    kind: 'site' | 'account'
    id: string
  } | null>(null)
  const [pending, setPending] = useState(false)
  const config = props.state.config
  const deleteItem = async () => {
    if (!remove) return
    setPending(true)
    try {
      const deletedAccounts = new Set(
        config.accounts
          .filter((item) =>
            remove.kind === 'site'
              ? item.site_id === remove.id
              : item.id === remove.id
          )
          .map((item) => item.id)
      )
      await props.onSave({
        ...config,
        sites: config.sites.filter(
          (item) => remove.kind !== 'site' || item.id !== remove.id
        ),
        accounts: config.accounts.filter(
          (item) => !deletedAccounts.has(item.id)
        ),
        flows: config.flows.filter(
          (item) => remove.kind !== 'site' || item.site_id !== remove.id
        ),
        bindings: config.bindings.filter(
          (item) => !deletedAccounts.has(item.account_id)
        ),
      })
      setRemove(null)
    } catch (error) {
      handleServerError(error)
    } finally {
      setPending(false)
    }
  }
  return (
    <div className='grid grid-cols-1 items-start gap-6 xl:grid-cols-[minmax(0,2fr)_minmax(0,3fr)]'>
      <section
        className='min-w-0 space-y-3'
        aria-labelledby={`${sectionId}-sites`}
      >
        <div className='flex flex-wrap items-center justify-between gap-2'>
          <h3 id={`${sectionId}-sites`} className='font-medium'>
            {t('Upstream sites')}
          </h3>
          <Button onClick={() => setSite('new')}>
            {t('Add upstream site')}
          </Button>
        </div>
        <StaticDataTable
          data={config.sites}
          getRowKey={(item) => item.id}
          emptyContent={t(
            'Add a site before configuring its accounts and flows.'
          )}
          columns={[
            {
              id: 'name',
              header: t('Site name'),
              className: 'w-full',
              cellClassName: 'wrap-anywhere whitespace-normal',
              cell: (item) => (
                <div>
                  <div className='font-medium'>{item.name}</div>
                  <div className='text-muted-foreground text-xs'>
                    {item.origin}
                  </div>
                </div>
              ),
            },
            {
              id: 'currency',
              header: t('Currency'),
              className: 'w-px',
              cell: (item) => item.currency,
            },
            {
              id: 'fx_rate',
              header: t('Site exchange rate'),
              className: 'w-px',
              cell: (item) =>
                formatNumber(item.fx_rate, locale, { fullPrecision: true }),
            },
            {
              id: 'actions',
              header: t('Actions'),
              className: 'w-px',
              cellClassName: 'w-px',
              cell: (item) => (
                <div className='flex w-max flex-nowrap gap-1'>
                  <Button
                    size='sm'
                    variant='outline'
                    onClick={() => setSite(item)}
                  >
                    {t('Edit')}
                  </Button>
                  <Button
                    size='sm'
                    variant='ghost'
                    onClick={() => setRemove({ kind: 'site', id: item.id })}
                  >
                    {t('Delete')}
                  </Button>
                </div>
              ),
            },
          ]}
        />
      </section>
      <section
        className='min-w-0 space-y-3'
        aria-labelledby={`${sectionId}-accounts`}
      >
        <div className='flex flex-wrap items-center justify-between gap-2'>
          <h3 id={`${sectionId}-accounts`} className='font-medium'>
            {t('Upstream accounts')}
          </h3>
          <Button
            disabled={!config.sites.length}
            onClick={() => setAccount('new')}
          >
            {t('Add upstream account')}
          </Button>
        </div>
        <StaticDataTable
          data={config.accounts}
          getRowKey={(item) => item.id}
          emptyContent={t('No upstream accounts configured.')}
          columns={[
            {
              id: 'name',
              header: t('Account'),
              cellClassName: 'wrap-anywhere whitespace-normal',
              cell: (item) => (
                <div>
                  <div className='font-medium'>{item.name}</div>
                  <div className='text-muted-foreground text-xs'>
                    {
                      config.sites.find((entry) => entry.id === item.site_id)
                        ?.name
                    }{' '}
                    · {item.username}
                  </div>
                </div>
              ),
            },
            {
              id: 'balances',
              header: t('Account balances'),
              cell: (item) => {
                const snapshot = latestByAccount.get(item.id)
                if (!snapshot) return t('Not collected')
                return (
                  <div className='space-y-1 text-xs tabular-nums'>
                    {snapshot.data.balances.length ? (
                      snapshot.data.balances.map((balance) => (
                        <p key={`${balance.kind}:${balance.currency}`}>
                          {balance.kind}:{' '}
                          {balance.amount === null
                            ? '—'
                            : formatLocalCurrencyAmount(balance.amount, {
                                locale,
                                showSymbol: false,
                                abbreviate: false,
                              })}{' '}
                          {balance.currency}
                        </p>
                      ))
                    ) : (
                      <p>—</p>
                    )}
                    <p className='text-muted-foreground'>
                      {formatTimestamp(snapshot.created_at)}
                    </p>
                  </div>
                )
              },
            },
            {
              id: 'status',
              header: t('Status'),
              cell: (item) => (item.enabled ? t('Enabled') : t('Disabled')),
            },
            {
              id: 'schedule',
              header: t('Schedule'),
              cell: (item) =>
                item.interval_minutes
                  ? t('Every {{minutes}} minutes', {
                      minutes: item.interval_minutes,
                    })
                  : t('Manual'),
            },
            {
              id: 'actions',
              header: t('Actions'),
              cell: (item) => (
                <div className='flex flex-wrap gap-2'>
                  <Button
                    size='sm'
                    disabled={!item.enabled || !props.state.key_ready}
                    onClick={() => props.onRun(item)}
                  >
                    {t('Run or record')}
                  </Button>
                  <Button
                    size='sm'
                    variant='outline'
                    onClick={() => setAccount(item)}
                  >
                    {t('Edit')}
                  </Button>
                  <Button
                    size='sm'
                    variant='outline'
                    onClick={() => setCostAccount(item)}
                  >
                    {t('Cost comparison estimates')}
                  </Button>
                  <Button
                    size='sm'
                    variant='ghost'
                    onClick={() => setRemove({ kind: 'account', id: item.id })}
                  >
                    {t('Delete')}
                  </Button>
                </div>
              ),
            },
          ]}
        />
      </section>
      {site && (
        <SiteDialog
          site={site === 'new' ? undefined : site}
          config={config}
          onSave={props.onSave}
          onClose={() => setSite(null)}
        />
      )}
      {account && (
        <AccountDialog
          account={account === 'new' ? undefined : account}
          state={props.state}
          onSave={props.onSave}
          onClose={() => setAccount(null)}
        />
      )}
      {costAccount && (
        <AccountCostDialog
          account={costAccount}
          state={props.state}
          onSave={props.onSave}
          onClose={() => setCostAccount(null)}
        />
      )}
      <ConfirmDialog
        open={Boolean(remove)}
        onOpenChange={(open) => {
          if (!open && !pending) setRemove(null)
        }}
        title={t('Delete upstream configuration?')}
        desc={t(
          'Deleting a site also removes its accounts, flows and channel bindings. Collection history is retained.'
        )}
        destructive
        isLoading={pending}
        handleConfirm={() => void deleteItem()}
      />
    </div>
  )
}
