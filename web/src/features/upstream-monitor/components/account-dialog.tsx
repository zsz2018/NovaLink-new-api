import { zodResolver } from '@hookform/resolvers/zod'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import type { z } from 'zod'

import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { Form } from '@/components/ui/form'
import { handleServerError } from '@/lib/handle-server-error'

import { accountSchema, newMonitorID } from '../lib/config'
import { monitorFlowReady } from '../lib/jobs'
import type { MonitorAccount, MonitorState, SaveMonitorConfig } from '../types'
import { MonitorField } from './form-fields'

type AccountDraft = z.infer<typeof accountSchema>
export function AccountDialog(props: {
  account?: MonitorAccount
  state: MonitorState
  onSave: SaveMonitorConfig
  onClose: () => void
}) {
  const { t } = useTranslation()
  const config = props.state.config
  const account = props.account
  const form = useForm<AccountDraft>({
    resolver: zodResolver(accountSchema),
    defaultValues: {
      name: account?.name ?? '',
      site_id: account?.site_id ?? config.sites[0]?.id ?? '',
      username: account?.username ?? '',
      external_id: account?.external_id ?? '',
      password: '',
      clear_password: false,
      device_id: account?.device_id ?? '',
      flow_id:
        config.flows.find(
          (flow) => flow.id === account?.flow_id && monitorFlowReady(flow)
        )?.id ?? '',
      interval_minutes: account?.interval_minutes ?? 0,
      enabled: account?.enabled ?? true,
      purchase_factor: account?.purchase_factor ?? 1,
      fee_rate: account?.fee_rate ?? 0,
      fee_bearer:
        account?.fee_bearer === 'unknown'
          ? undefined
          : (account?.fee_bearer ?? 'self'),
    },
  })
  const siteID = form.watch('site_id')
  const submit = async (values: AccountDraft) => {
    const next: MonitorAccount = {
      ...account,
      ...values,
      id: account?.id ?? newMonitorID(),
      has_password: account?.has_password ?? false,
    }
    try {
      await props.onSave({
        ...config,
        accounts: [
          ...config.accounts.filter((item) => item.id !== next.id),
          next,
        ],
      })
      props.onClose()
    } catch (error) {
      handleServerError(error)
    }
  }
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) props.onClose()
      }}
      title={account ? t('Edit upstream account') : t('Add upstream account')}
      contentClassName='sm:max-w-3xl'
      footer={
        <>
          <Button variant='outline' onClick={props.onClose}>
            {t('Cancel')}
          </Button>
          <Button
            form='monitor-account-form'
            type='submit'
            disabled={form.formState.isSubmitting}
          >
            {t('Save')}
          </Button>
        </>
      }
    >
      <Form {...form}>
        <form
          id='monitor-account-form'
          onSubmit={form.handleSubmit(submit)}
          className='space-y-4'
        >
          <div className='grid gap-4 sm:grid-cols-2'>
            <MonitorField
              control={form.control}
              name='name'
              label={t('Account name')}
            />
            <MonitorField
              control={form.control}
              name='site_id'
              label={t('Upstream site')}
              options={config.sites.map((site) => ({
                value: site.id,
                label: site.name,
              }))}
            />
            <MonitorField
              control={form.control}
              name='username'
              label={t('Login username')}
            />
            <MonitorField
              control={form.control}
              name='external_id'
              label={t('Expected upstream account ID')}
              description={t(
                'Leave blank to bind the captured account identity when saving collection settings in the extension.'
              )}
            />
            <MonitorField
              control={form.control}
              name='password'
              kind='password'
              label={t('Login password')}
              description={
                account?.has_password
                  ? t('A password is stored. Leave blank to keep it.')
                  : t('The server encrypts this password.')
              }
            />
            <MonitorField
              control={form.control}
              name='clear_password'
              kind='switch'
              label={t('Clear stored password')}
            />
            <MonitorField
              control={form.control}
              name='device_id'
              label={t('Scheduled browser')}
              options={[
                { value: '', label: t('Not selected') },
                ...props.state.devices
                  .filter((device) => !device.revoked_at)
                  .map((device) => ({ value: device.id, label: device.name })),
              ]}
            />
            <MonitorField
              control={form.control}
              name='flow_id'
              label={t('Scheduled flow')}
              options={[
                { value: '', label: t('Not selected') },
                ...config.flows
                  .filter(
                    (flow) => flow.site_id === siteID && monitorFlowReady(flow)
                  )
                  .map((flow) => ({ value: flow.id, label: flow.name })),
              ]}
            />
            <MonitorField
              control={form.control}
              name='interval_minutes'
              kind='number'
              min={0}
              step='1'
              label={t('Collection interval (minutes)')}
              description={t(
                'Set to zero to disable scheduling. Offline browsers wait for a connection.'
              )}
            />
            <MonitorField
              control={form.control}
              name='enabled'
              kind='switch'
              label={t('Account enabled')}
            />
            <MonitorField
              control={form.control}
              name='purchase_factor'
              kind='number'
              min={0}
              label={t('Recharge cost factor')}
              description={[
                t(
                  'Factor = recharge payment / credited face value (including bonuses), using the same currency.'
                ),
                t(
                  'Examples: pay 100, receive 100: enter 1; pay 80, receive 100: enter 0.8; pay 100, receive 120: enter 0.833333.'
                ),
                t(
                  'Convert currencies using the site exchange rate first. Exclude fees entered separately below to avoid counting them twice.'
                ),
              ].join(' ')}
            />
            <MonitorField
              control={form.control}
              name='fee_rate'
              kind='number'
              min={0}
              max={1}
              label={t('Upstream fee rate')}
              description={t(
                'Use a fraction: 0.05 means 5%. Added to cost only when our site bears the fee; do not include fees already counted in the recharge cost factor.'
              )}
            />
            <MonitorField
              control={form.control}
              name='fee_bearer'
              label={t('Upstream fee bearer')}
              options={[
                { value: 'self', label: t('Our site') },
                { value: 'other', label: t('Upstream site') },
              ]}
            />
          </div>
        </form>
      </Form>
    </Dialog>
  )
}
