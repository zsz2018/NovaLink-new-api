import { zodResolver } from '@hookform/resolvers/zod'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import type { z } from 'zod'

import { Button } from '@/components/ui/button'
import { Form } from '@/components/ui/form'
import { toIntlLocale } from '@/i18n/languages'
import { handleServerError } from '@/lib/handle-server-error'

import { settingsSchema } from '../lib/config'
import { monitorCurrencyOptions } from '../lib/currency-options'
import type { MonitorConfig, MonitorState, SaveMonitorConfig } from '../types'
import { MonitorField } from './form-fields'

type SettingsDraft = z.infer<typeof settingsSchema>
export function SettingsPanel(props: {
  config: MonitorConfig
  groups: MonitorState['groups']
  onSave: SaveMonitorConfig
}) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const currencyOptions = monitorCurrencyOptions(
    locale,
    props.config.scenario.currency
  )
  const groups = new Set(Object.keys(props.groups).filter(Boolean))
  const savedGroup = props.config.scenario.user_group
  if (savedGroup) groups.add(savedGroup)
  const groupOptions = [
    { value: '', label: t('Not selected') },
    ...[...groups].sort().map((group) => ({ value: group, label: group })),
  ]
  const form = useForm<SettingsDraft>({
    resolver: zodResolver(settingsSchema),
    defaultValues: {
      scenario: {
        ...props.config.scenario,
        fee_bearer:
          props.config.scenario.fee_bearer === 'unknown'
            ? undefined
            : props.config.scenario.fee_bearer,
      },
    },
  })
  const submit = async (values: SettingsDraft) => {
    try {
      await props.onSave({
        ...props.config,
        scenario: values.scenario,
      })
      form.reset({
        scenario: values.scenario,
      })
    } catch (error) {
      handleServerError(error)
    }
  }
  const feeOptions = [
    { value: 'self', label: t('Our site') },
    { value: 'other', label: t('User') },
  ]
  return (
    <Form {...form}>
      <form onSubmit={form.handleSubmit(submit)} className='space-y-6'>
        <section className='space-y-4 rounded-xl border p-4'>
          <h3 className='font-medium'>{t('Profit comparison scenario')}</h3>
          <p className='text-muted-foreground text-sm'>
            {t(
              'Every procurement route is evaluated with the same quantities. Totals describe this comparison scenario, not historical operating profit.'
            )}
          </p>
          <div className='grid gap-4 sm:grid-cols-2 lg:grid-cols-3'>
            <MonitorField
              control={form.control}
              name='scenario.currency'
              label={t('Reporting currency')}
              options={currencyOptions}
            />
            <MonitorField
              control={form.control}
              name='scenario.usd_exchange'
              kind='number'
              min={0}
              label={t('USD exchange rate')}
              description={t(
                'Reporting currency for one USD of local model pricing.'
              )}
            />
            <MonitorField
              control={form.control}
              name='scenario.user_group'
              label={t('Pricing user group')}
              options={groupOptions}
              description={t(
                'Optional user group for special local group ratios.'
              )}
            />
            <MonitorField
              control={form.control}
              name='scenario.input_tokens'
              kind='number'
              min={0}
              step='1'
              label={t('Uncached input tokens')}
            />
            <MonitorField
              control={form.control}
              name='scenario.output_tokens'
              kind='number'
              min={0}
              step='1'
              label={t('Output tokens')}
            />
            <MonitorField
              control={form.control}
              name='scenario.cache_read'
              kind='number'
              min={0}
              step='1'
              label={t('Cache read tokens')}
            />
            <MonitorField
              control={form.control}
              name='scenario.cache_write'
              kind='number'
              min={0}
              step='1'
              label={t('Cache write tokens')}
            />
            <MonitorField
              control={form.control}
              name='scenario.requests'
              kind='number'
              min={1}
              step='1'
              label={t('Request count')}
            />
            <MonitorField
              control={form.control}
              name='scenario.revenue_factor'
              kind='number'
              min={0}
              label={t('Revenue factor')}
              description={t(
                'Actual receipts divided by issued face value, before the fee below.'
              )}
            />
            <MonitorField
              control={form.control}
              name='scenario.fee_rate'
              kind='number'
              min={0}
              max={1}
              label={t('Collection fee rate')}
              description={t('Use a fraction: 0.02 means 2%.')}
            />
            <MonitorField
              control={form.control}
              name='scenario.fee_bearer'
              label={t('Collection fee bearer')}
              options={feeOptions}
            />
          </div>
        </section>
        <Button type='submit' disabled={form.formState.isSubmitting}>
          {t('Save cost settings')}
        </Button>
      </form>
    </Form>
  )
}
