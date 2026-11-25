import { zodResolver } from '@hookform/resolvers/zod'
import { useId, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import type { z } from 'zod'

import { StaticDataTable } from '@/components/data-table'
import { Dialog } from '@/components/dialog'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Combobox } from '@/components/ui/combobox'
import { Form } from '@/components/ui/form'
import { Label } from '@/components/ui/label'
import { toIntlLocale } from '@/i18n/languages'
import { handleServerError } from '@/lib/handle-server-error'

import { accountCostSchema } from '../lib/config'
import { monitorGroupLabel } from '../lib/groups'
import { latestMonitorSnapshots } from '../lib/jobs'
import {
  boundUpstreamModels,
  monitorQuoteHasPricingFacts,
} from '../lib/official-estimates'
import { formatProfitAmount } from '../lib/profit'
import { formatMonitorMessage } from '../lib/task-messages'
import type { MonitorAccount, MonitorState, SaveMonitorConfig } from '../types'
import { MonitorField, MonitorSelect } from './form-fields'
import {
  OfficialEstimateNotice,
  OfficialEstimateSource,
} from './official-estimate'

type CostDraft = z.infer<typeof accountCostSchema>

export function AccountCostDialog(props: {
  account: MonitorAccount
  state: MonitorState
  onSave: SaveMonitorConfig
  onClose: () => void
}) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const reviewID = useId()
  const bound = boundUpstreamModels(props.state, props.account.id)
  const snapshot = latestMonitorSnapshots(props.state.snapshots).get(
    props.account.id
  )
  const groups = [
    ...new Set([
      ...(snapshot?.data.groups.map((item) => item.name) ?? []),
      ...props.state.config.bindings
        .filter((item) => item.account_id === props.account.id)
        .map((item) => item.upstream_group),
      ...(props.account.official_estimate?.models.map((item) => item.group) ??
        []),
    ]),
  ].filter(Boolean)
  const form = useForm<CostDraft>({
    resolver: zodResolver(accountCostSchema),
    defaultValues: {
      enabled: Boolean(props.account.official_estimate),
      catalog_version:
        props.account.official_estimate?.catalog_version ??
        props.state.official_catalog_latest_version ??
        '',
      models: props.account.official_estimate?.models ?? [],
    },
  })
  const enabled = form.watch('enabled')
  const version = form.watch('catalog_version')
  const models = form.watch('models')
  const catalog = props.state.official_catalogs?.find(
    (item) => item.version === version
  )
  const site = props.state.config.sites.find(
    (item) => item.id === props.account.site_id
  )
  const usd = site?.currency === 'USD'
  const [group, setGroup] = useState(groups[0] ?? '')
  const [name, setName] = useState('')
  const [entryID, setEntryID] = useState('')
  const [reviewed, setReviewed] = useState(false)
  const entry = catalog?.entries.find((item) => item.id === entryID)
  const observed =
    snapshot?.data.models.filter((item) => item.group === group) ?? []
  const modelNames = [
    ...new Set([
      ...observed.map((item) => item.name),
      ...bound.filter((item) => item.group === group).map((item) => item.name),
    ]),
  ]
  const capturedQuote = snapshot?.data.models.find(
    (item) => item.name === name && item.group === group
  )
  const capturedQuoteProtected = Boolean(
    capturedQuote &&
    (capturedQuote.currency !== site?.currency ||
      monitorQuoteHasPricingFacts(capturedQuote))
  )
  const groupData = snapshot?.data.groups.find((item) => item.name === group)
  const groupCostKnown = Boolean(
    groupData && groupData.ratio !== null && !groupData.cost_unsupported
  )
  const validSelections = models.every((model) =>
    catalog?.entries.some(
      (item) => item.id === model.entry_id && item.quote.currency === 'USD'
    )
  )
  const versionOptions = (props.state.official_catalogs ?? []).map((item) => ({
    value: item.version,
    label: item.version,
  }))
  if (version && !versionOptions.some((item) => item.value === version)) {
    versionOptions.unshift({ value: version, label: version })
  }
  const save = async (values: CostDraft) => {
    if (values.enabled && (!usd || !catalog || !validSelections)) return
    try {
      await props.onSave({
        ...props.state.config,
        accounts: props.state.config.accounts.map((account) =>
          account.id === props.account.id
            ? {
                ...account,
                official_estimate: values.enabled
                  ? {
                      catalog_version: values.catalog_version,
                      models: values.models,
                    }
                  : undefined,
              }
            : account
        ),
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
        if (!open && !form.formState.isSubmitting) props.onClose()
      }}
      title={t('Cost comparison estimates')}
      description={props.account.name}
      contentClassName='sm:max-w-4xl'
      footer={
        <>
          <Button
            variant='outline'
            onClick={props.onClose}
            disabled={form.formState.isSubmitting}
          >
            {t('Cancel')}
          </Button>
          <Button
            form='monitor-account-cost-form'
            type='submit'
            disabled={
              form.formState.isSubmitting ||
              (enabled &&
                (!usd || !catalog || !models.length || !validSelections))
            }
          >
            {t('Save comparison estimates')}
          </Button>
        </>
      }
    >
      <Form {...form}>
        <form
          id='monitor-account-cost-form'
          onSubmit={form.handleSubmit(save)}
          className='flex min-w-0 flex-col gap-4'
        >
          <MonitorField
            control={form.control}
            name='enabled'
            kind='switch'
            label={t(
              'Estimate missing quotes using official reference prices × upstream ratio'
            )}
            disabled={!usd && !enabled}
          />
          {!usd && (
            <Alert role='note'>
              <AlertDescription>
                {t('Official reference estimates require a USD upstream site.')}
              </AlertDescription>
            </Alert>
          )}
          {enabled && (
            <>
              <OfficialEstimateNotice />
              <MonitorField
                control={form.control}
                name='catalog_version'
                label={t('Catalog version')}
                options={versionOptions}
                onValueChange={() => {
                  setEntryID('')
                  setReviewed(false)
                }}
              />
              {props.state.official_catalog_latest_version &&
                version !== props.state.official_catalog_latest_version && (
                  <p className='text-muted-foreground text-sm'>
                    {t(
                      'A newer official catalog is available. Select its version explicitly to update this policy.'
                    )}
                  </p>
                )}
              {!catalog && (
                <Alert variant='destructive'>
                  <AlertDescription>
                    {t('Monitor official catalog is unavailable')}
                  </AlertDescription>
                </Alert>
              )}
              <p className='text-muted-foreground text-sm'>
                {t(
                  'This account policy uses the selected official catalog and captured group ratios to estimate comparison costs.'
                )}
              </p>
              <Button
                type='button'
                variant='outline'
                disabled={!catalog || !usd}
                onClick={() => {
                  const next = [...models]
                  for (const model of bound) {
                    const captured = snapshot?.data.models.find(
                      (item) =>
                        item.name === model.name && item.group === model.group
                    )
                    if (
                      next.some(
                        (item) =>
                          item.name === model.name && item.group === model.group
                      ) ||
                      (captured &&
                        (captured.currency !== site?.currency ||
                          monitorQuoteHasPricingFacts(captured)))
                    ) {
                      continue
                    }
                    const matches =
                      catalog?.entries.filter(
                        (item) =>
                          item.model === model.name &&
                          item.quote.currency === 'USD'
                      ) ?? []
                    if (matches.length === 1) {
                      next.push({ ...model, entry_id: matches[0].id })
                    }
                  }
                  if (next.length > 500) {
                    form.setError('models', {
                      message: t('Select at most 500 estimate mappings.'),
                    })
                    return
                  }
                  form.setValue('models', next, {
                    shouldDirty: true,
                    shouldValidate: true,
                  })
                }}
              >
                {t('Match bound upstream models by exact name')}
              </Button>
              <StaticDataTable
                data={models}
                getRowKey={(model) => JSON.stringify([model.name, model.group])}
                tableProps={{ 'aria-label': t('Selected estimate mappings') }}
                emptyContent={t('No estimate mappings selected.')}
                columns={[
                  {
                    id: 'group',
                    header: t('Group'),
                    cell: (model) => (
                      <span className='break-all whitespace-normal'>
                        {model.group}
                      </span>
                    ),
                  },
                  {
                    id: 'model',
                    header: t('Upstream model'),
                    cell: (model) => (
                      <span className='break-all whitespace-normal'>
                        {model.name}
                      </span>
                    ),
                  },
                  {
                    id: 'entry',
                    header: t('Catalog entry'),
                    cell: (model) => (
                      <span className='break-all whitespace-normal'>
                        {model.entry_id}
                      </span>
                    ),
                  },
                  {
                    id: 'actions',
                    header: t('Actions'),
                    cell: (model) => (
                      <Button
                        type='button'
                        variant='ghost'
                        size='sm'
                        aria-label={t(
                          'Remove estimate mapping for {{model}} in {{group}}',
                          { model: model.name, group: model.group }
                        )}
                        onClick={() =>
                          form.setValue(
                            'models',
                            models.filter((item) => item !== model),
                            { shouldDirty: true, shouldValidate: true }
                          )
                        }
                      >
                        {t('Remove')}
                      </Button>
                    ),
                  },
                ]}
              />
              {form.formState.errors.models && (
                <p role='alert' className='text-destructive text-sm'>
                  {form.formState.errors.models.message}
                </p>
              )}
              {catalog && !validSelections && (
                <p role='alert' className='text-destructive text-sm'>
                  {t(
                    'A selected catalog entry is unavailable in this version. Remove or review the mapping.'
                  )}
                </p>
              )}
              <section
                className='flex min-w-0 flex-col gap-3 rounded-lg border p-3'
                aria-label={t('Review a model correspondence')}
              >
                <h3 className='font-medium'>
                  {t('Review a model correspondence')}
                </h3>
                <div className='grid min-w-0 gap-3 sm:grid-cols-2'>
                  <div className='flex min-w-0 flex-col gap-2'>
                    <Label>{t('Upstream group')}</Label>
                    <MonitorSelect
                      label={t('Upstream group')}
                      value={group}
                      onChange={(value) => {
                        setGroup(value)
                        setName('')
                        setReviewed(false)
                      }}
                      options={groups.map((value) => ({
                        value,
                        label: monitorGroupLabel(
                          value,
                          snapshot?.data.groups.find(
                            (item) => item.name === value
                          )?.display_name
                        ),
                      }))}
                    />
                  </div>
                  <div className='flex min-w-0 flex-col gap-2'>
                    <Label>{t('Upstream model name')}</Label>
                    <Combobox
                      aria-label={t('Upstream model name')}
                      value={name}
                      options={modelNames.map((value) => ({
                        value,
                        label: value,
                      }))}
                      allowCustomValue
                      onValueChange={(value) => {
                        setName(value ?? '')
                        setReviewed(false)
                      }}
                    />
                  </div>
                  <div className='flex min-w-0 flex-col gap-2 sm:col-span-2'>
                    <Label>{t('Official reference model')}</Label>
                    <MonitorSelect
                      label={t('Official reference model')}
                      value={entryID}
                      onChange={(value) => {
                        setEntryID(value)
                        setReviewed(false)
                      }}
                      options={(catalog?.entries ?? [])
                        .filter((item) => item.quote.currency === 'USD')
                        .map((item) => ({
                          value: item.id,
                          label: `${item.provider} · ${item.model}`,
                        }))}
                    />
                  </div>
                </div>
                {!groups.length && (
                  <p className='text-muted-foreground text-sm'>
                    {t(
                      'Collect upstream groups or add an account binding before selecting estimate mappings.'
                    )}
                  </p>
                )}
                {!groupCostKnown && (
                  <p role='status' className='text-muted-foreground text-sm'>
                    {t(
                      'The group needs a captured cash-billing ratio before estimates can enter comparisons.'
                    )}
                    {groupData?.cost_unsupported &&
                      ` ${formatMonitorMessage(groupData.cost_unsupported, t)}`}
                  </p>
                )}
                {capturedQuoteProtected && (
                  <p role='status' className='text-muted-foreground text-sm'>
                    {t(
                      'A captured quote exists for this model and group. It will not be replaced, including zero or unresolved prices.'
                    )}
                  </p>
                )}
                {entry && (
                  <section
                    className='flex min-w-0 flex-col gap-3'
                    aria-label={t('Official reference preview')}
                  >
                    <OfficialEstimateSource
                      source={{
                        kind: 'official_estimate',
                        catalog_version: version,
                        entry_id: entry.id,
                        provider: entry.provider,
                        model: entry.model,
                        source_url: entry.source_url,
                        verified_at: entry.verified_at,
                      }}
                    />
                    <p className='text-muted-foreground text-sm'>
                      {t('Per 1M tokens')} · {entry.quote.currency}
                    </p>
                    <dl className='grid grid-cols-2 gap-3 text-sm'>
                      {[
                        [t('Input'), entry.quote.input],
                        [t('Output'), entry.quote.output],
                        [t('Cache read'), entry.quote.cache_read],
                        [t('Cache write'), entry.quote.cache_write],
                      ].map(([label, value]) => (
                        <div key={String(label)}>
                          <dt className='text-muted-foreground'>{label}</dt>
                          <dd>
                            {formatProfitAmount(
                              typeof value === 'number' ? value : null,
                              'USD',
                              locale
                            )}
                          </dd>
                        </div>
                      ))}
                    </dl>
                    {entry.quote.notes && (
                      <p className='text-muted-foreground text-xs'>
                        {formatMonitorMessage(entry.quote.notes, t)}
                      </p>
                    )}
                    {entry.quote.billing_expression && (
                      <p className='text-muted-foreground text-xs'>
                        {t(
                          'Reference prices may use context tiers. The complete official expression determines comparison costs.'
                        )}
                      </p>
                    )}
                  </section>
                )}
                <div className='flex items-start gap-2'>
                  <Checkbox
                    id={reviewID}
                    checked={reviewed}
                    onCheckedChange={(value) => setReviewed(value === true)}
                  />
                  <Label htmlFor={reviewID}>
                    {t(
                      'I reviewed the upstream model and official reference match.'
                    )}
                  </Label>
                </div>
                <Button
                  type='button'
                  variant='outline'
                  disabled={
                    !usd ||
                    !group ||
                    !name ||
                    !entry ||
                    !reviewed ||
                    capturedQuoteProtected
                  }
                  onClick={() => {
                    const next = models.filter(
                      (item) => item.name !== name || item.group !== group
                    )
                    form.setValue(
                      'models',
                      [...next, { name, group, entry_id: entryID }],
                      { shouldDirty: true, shouldValidate: true }
                    )
                    setName('')
                    setReviewed(false)
                  }}
                >
                  {t('Add reviewed model')}
                </Button>
              </section>
            </>
          )}
        </form>
      </Form>
    </Dialog>
  )
}
