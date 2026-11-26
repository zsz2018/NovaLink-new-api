import { zodResolver } from '@hookform/resolvers/zod'
import { useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import type { z } from 'zod'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { StaticDataTable } from '@/components/data-table'
import { Dialog } from '@/components/dialog'
import { MultiSelect } from '@/components/multi-select'
import { Button } from '@/components/ui/button'
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { parseModelsList } from '@/features/channels/lib/channel-utils'
import { toIntlLocale } from '@/i18n/languages'
import { formatNumber } from '@/lib/format'
import { handleServerError } from '@/lib/handle-server-error'

import { bindingSchema, newMonitorID } from '../lib/config'
import { monitorGroupLabel } from '../lib/groups'
import { latestMonitorSnapshots } from '../lib/jobs'
import type { MonitorBinding, MonitorState, SaveMonitorConfig } from '../types'
import { MonitorField, type MonitorOption } from './form-fields'

function includeSavedOption(
  options: MonitorOption[],
  saved?: string
): MonitorOption[] {
  return saved && !options.some((option) => option.value === saved)
    ? [...options, { value: saved, label: saved }]
    : options
}

function BindingDialog(props: {
  binding?: MonitorBinding
  state: MonitorState
  onSave: SaveMonitorConfig
  onClose: () => void
}) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const binding = props.binding
  const form = useForm<z.infer<typeof bindingSchema>>({
    resolver: zodResolver(bindingSchema),
    defaultValues: {
      channel_id: binding?.channel_id ?? props.state.channels[0]?.id ?? 0,
      account_id:
        binding?.account_id ?? props.state.config.accounts[0]?.id ?? '',
      upstream_group: binding?.upstream_group ?? '',
      local_models: [binding?.local_model ?? ''],
      upstream_model: binding?.upstream_model ?? '',
    },
  })
  const accountID = form.watch('account_id')
  const channelID = form.watch('channel_id')
  const group = form.watch('upstream_group')
  const localModels = form.watch('local_models')
  const localModel = localModels.length === 1 ? localModels[0] : undefined
  const snapshot = latestMonitorSnapshots(props.state.snapshots).get(accountID)
  const channel = props.state.channels.find((item) => item.id === channelID)
  const groupOptions = includeSavedOption(
    (snapshot?.data.groups ?? []).map((item) => ({
      value: item.name,
      label: `${monitorGroupLabel(item.name, item.display_name)} · ${t(
        'Upstream ratio {{ratio}}',
        {
          ratio:
            item.ratio === null
              ? t('Unknown')
              : formatNumber(item.ratio, locale, { fullPrecision: true }),
        }
      )}`,
    })),
    binding?.account_id === accountID ? binding.upstream_group : undefined
  )
  const localOptions = includeSavedOption(
    [...new Set(parseModelsList(channel?.models ?? ''))].map((name) => ({
      value: name,
      label: name,
    })),
    binding?.channel_id === channelID ? binding.local_model : undefined
  )
  const upstreamOptions = includeSavedOption(
    [
      ...new Set(
        (snapshot?.data.models ?? [])
          .filter((item) => item.group === group)
          .map((item) => item.name)
      ),
    ].map((name) => ({ value: name, label: name })),
    binding?.account_id === accountID &&
      binding.channel_id === channelID &&
      binding.upstream_group === group &&
      binding.local_model === localModel
      ? binding.upstream_model
      : undefined
  )
  const clearUpstreamModel = () =>
    form.setValue('upstream_model', '', { shouldDirty: true })
  const submit = async (values: z.infer<typeof bindingSchema>) => {
    const { local_models, ...scope } = values
    const existing = props.state.config.bindings.filter(
      (item) => item.id !== binding?.id
    )
    const sameGroup = (item: MonitorBinding) =>
      item.channel_id === scope.channel_id &&
      item.account_id === scope.account_id &&
      item.upstream_group === scope.upstream_group
    const selectedModels = local_models.includes('')
      ? ['']
      : [...new Set(local_models)]
    const modelMappings = new Set(
      existing
        .filter(sameGroup)
        .map((item) => JSON.stringify([item.local_model, item.upstream_model]))
    )
    const next: MonitorBinding[] = []
    for (const model of selectedModels) {
      const retained = binding?.local_model === model ? binding : undefined
      const upstreamModel =
        selectedModels.length > 1 && retained && sameGroup(retained)
          ? retained.upstream_model
          : scope.upstream_model
      if (modelMappings.has(JSON.stringify([model, upstreamModel]))) continue
      const previous = selectedModels.length === 1 ? binding : retained
      next.push({
        ...scope,
        local_model: model,
        upstream_model: upstreamModel,
        id: previous?.id ?? newMonitorID(),
      })
    }
    if (!binding && !next.length) {
      props.onClose()
      return
    }
    try {
      await props.onSave({
        ...props.state.config,
        bindings: [...existing, ...next],
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
      title={t('Channel cost binding')}
      footer={
        <>
          <Button variant='outline' onClick={props.onClose}>
            {t('Cancel')}
          </Button>
          <Button
            type='submit'
            form='monitor-binding-form'
            disabled={
              form.formState.isSubmitting || !group || !localModels.length
            }
          >
            {t('Save')}
          </Button>
        </>
      }
    >
      <Form {...form}>
        <form
          id='monitor-binding-form'
          className='space-y-4'
          onSubmit={form.handleSubmit(submit)}
        >
          <MonitorField
            control={form.control}
            name='channel_id'
            label={t('Channel')}
            onValueChange={() => {
              form.setValue('local_models', [''], { shouldDirty: true })
              clearUpstreamModel()
            }}
            options={props.state.channels.map((channel) => ({
              value: String(channel.id),
              label: channel.name,
            }))}
          />
          <MonitorField
            control={form.control}
            name='account_id'
            label={t('Upstream account')}
            onValueChange={() => {
              form.setValue('upstream_group', '', { shouldDirty: true })
              clearUpstreamModel()
            }}
            options={props.state.config.accounts.map((account) => ({
              value: account.id,
              label: account.name,
            }))}
          />
          <MonitorField
            control={form.control}
            name='upstream_group'
            label={t('Upstream group')}
            options={groupOptions}
            searchable
            disabled={!groupOptions.length}
            onValueChange={clearUpstreamModel}
            description={
              groupOptions.length === 0
                ? t(
                    'Run a complete collection for this account to choose an upstream group.'
                  )
                : undefined
            }
          />
          <FormField
            control={form.control}
            name='local_models'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Local models')}</FormLabel>
                <FormControl>
                  <MultiSelect
                    options={[
                      { value: '', label: t('All channel models') },
                      ...localOptions,
                    ]}
                    selected={field.value}
                    onChange={(selected) => {
                      let next = selected
                      if (selected.includes('')) {
                        next = field.value.includes('')
                          ? selected.filter(Boolean)
                          : ['']
                      }
                      field.onChange(next)
                      clearUpstreamModel()
                    }}
                    placeholder={t('Search models...')}
                    aria-label={t('Local model')}
                    allowCreate={false}
                    maxVisibleChips={5}
                  />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
          <MonitorField
            control={form.control}
            name='upstream_model'
            label={t('Upstream model')}
            options={[
              { value: '', label: t('Channel mapping') },
              ...upstreamOptions,
            ]}
            searchable
            disabled={!group || !localModel}
            description={
              localModel
                ? undefined
                : t('Select one local model to customize its upstream mapping.')
            }
          />
        </form>
      </Form>
    </Dialog>
  )
}

export function BindingsPanel(props: {
  state: MonitorState
  onSave: SaveMonitorConfig
}) {
  const { t } = useTranslation()
  const latestByAccount = latestMonitorSnapshots(props.state.snapshots)
  const [edit, setEdit] = useState<MonitorBinding | 'new' | null>(null)
  const [remove, setRemove] = useState<MonitorBinding | null>(null)
  const [pending, setPending] = useState(false)
  const deleteBinding = async () => {
    if (!remove) return
    setPending(true)
    try {
      await props.onSave({
        ...props.state.config,
        bindings: props.state.config.bindings.filter(
          (item) => item.id !== remove.id
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
    <section className='space-y-3'>
      <div className='flex justify-end'>
        <Button
          disabled={
            !props.state.channels.length || !props.state.config.accounts.length
          }
          onClick={() => setEdit('new')}
        >
          {t('Add binding')}
        </Button>
      </div>
      <StaticDataTable
        data={props.state.config.bindings}
        getRowKey={(item) => item.id}
        emptyContent={t(
          'Bind a channel to an upstream account and group to compare its cost.'
        )}
        columns={[
          {
            id: 'channel',
            header: t('Channel'),
            cell: (item) =>
              props.state.channels.find(
                (channel) => channel.id === item.channel_id
              )?.name ?? String(item.channel_id),
          },
          {
            id: 'account',
            header: t('Upstream account'),
            cell: (item) =>
              props.state.config.accounts.find(
                (account) => account.id === item.account_id
              )?.name ?? item.account_id,
          },
          {
            id: 'group',
            header: t('Upstream group'),
            cell: (item) =>
              monitorGroupLabel(
                item.upstream_group,
                latestByAccount
                  .get(item.account_id)
                  ?.data.groups.find(
                    (group) => group.name === item.upstream_group
                  )?.display_name
              ),
          },
          {
            id: 'models',
            header: t('Model mapping'),
            cell: (item) =>
              `${item.local_model || t('All channel models')} → ${item.upstream_model || t('Channel mapping')}`,
          },
          {
            id: 'actions',
            header: t('Actions'),
            cell: (item) => (
              <div className='flex gap-2'>
                <Button
                  variant='outline'
                  size='sm'
                  onClick={() => setEdit(item)}
                >
                  {t('Edit')}
                </Button>
                <Button
                  variant='ghost'
                  size='sm'
                  onClick={() => setRemove(item)}
                >
                  {t('Delete')}
                </Button>
              </div>
            ),
          },
        ]}
      />
      {edit && (
        <BindingDialog
          binding={edit === 'new' ? undefined : edit}
          state={props.state}
          onSave={props.onSave}
          onClose={() => setEdit(null)}
        />
      )}
      <ConfirmDialog
        open={Boolean(remove)}
        onOpenChange={(open) => {
          if (!open && !pending) setRemove(null)
        }}
        title={t('Delete channel cost binding?')}
        desc={t(
          'This removes the cost association. The channel remains available.'
        )}
        destructive
        isLoading={pending}
        handleConfirm={() => void deleteBinding()}
      />
    </section>
  )
}
