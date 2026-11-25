import { zodResolver } from '@hookform/resolvers/zod'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import type { z } from 'zod'

import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { Form } from '@/components/ui/form'
import { toIntlLocale } from '@/i18n/languages'
import { handleServerError } from '@/lib/handle-server-error'

import { newMonitorID, siteSchema } from '../lib/config'
import { monitorCurrencyOptions } from '../lib/currency-options'
import type { MonitorConfig, MonitorSite, SaveMonitorConfig } from '../types'
import { MonitorField } from './form-fields'

type SiteDraft = z.infer<typeof siteSchema>
export function SiteDialog(props: {
  site?: MonitorSite
  config: MonitorConfig
  onSave: SaveMonitorConfig
  onClose: () => void
}) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const currencyOptions = monitorCurrencyOptions(locale, props.site?.currency)
  const form = useForm<SiteDraft>({
    resolver: zodResolver(siteSchema),
    defaultValues: {
      name: props.site?.name ?? '',
      origin: props.site?.origin ?? '',
      allowed_origins: props.site?.allowed_origins.join('\n') ?? '',
      currency: props.site?.currency ?? 'USD',
      fx_rate: props.site?.fx_rate ?? 1,
    },
  })
  const submit = async (values: SiteDraft) => {
    const site: MonitorSite = {
      ...values,
      id: props.site?.id ?? newMonitorID(),
      origin: values.origin.replace(/\/$/, ''),
      allowed_origins: values.allowed_origins
        .split('\n')
        .map((line) => line.trim())
        .filter(Boolean),
    }
    const sites = [
      ...props.config.sites.filter((item) => item.id !== site.id),
      site,
    ]
    try {
      await props.onSave({ ...props.config, sites })
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
      title={props.site ? t('Edit upstream site') : t('Add upstream site')}
      footer={
        <>
          <Button variant='outline' onClick={props.onClose}>
            {t('Cancel')}
          </Button>
          <Button
            form='monitor-site-form'
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
          id='monitor-site-form'
          onSubmit={form.handleSubmit(submit)}
          className='space-y-4'
        >
          <MonitorField
            control={form.control}
            name='name'
            label={t('Site name')}
          />
          <MonitorField
            control={form.control}
            name='origin'
            label={t('Website origin')}
            description={t(
              'Use the website origin, for example https://provider.example.'
            )}
          />
          <MonitorField
            control={form.control}
            name='allowed_origins'
            kind='textarea'
            label={t('Additional allowed origins')}
            description={t(
              'One origin per line for login redirects or account APIs.'
            )}
          />
          <div className='grid gap-4 sm:grid-cols-2'>
            <MonitorField
              control={form.control}
              name='currency'
              label={t('Site currency')}
              options={currencyOptions}
            />
            <MonitorField
              control={form.control}
              name='fx_rate'
              kind='number'
              min={0}
              label={t('Site exchange rate')}
              description={t(
                'Reporting currency received for one unit of site currency.'
              )}
            />
          </div>
        </form>
      </Form>
    </Dialog>
  )
}
