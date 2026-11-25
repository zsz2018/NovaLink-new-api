import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { t } from 'i18next'
import { useEffect, useId } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { z } from 'zod'

import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { Form } from '@/components/ui/form'

import {
  createMonitorJob,
  getMonitorBrowserSetup,
  monitorQueryKey,
} from '../api'
import { monitorSiteAuthorized } from '../lib/browser-setup'
import { monitorFlowReady } from '../lib/jobs'
import type { MonitorAccount, MonitorFlow, MonitorState } from '../types'
import { MonitorField } from './form-fields'

const runSchema = z
  .object({
    account_id: z.string().min(1, { error: () => t('This field is required') }),
    device_id: z.string().min(1, { error: () => t('This field is required') }),
    flow_id: z.string(),
    mode: z.enum(['record', 'collect']),
  })
  .refine((value) => value.mode === 'record' || Boolean(value.flow_id), {
    path: ['flow_id'],
    error: () => t('Choose a saved collection flow'),
  })
export function RunDialog(props: {
  account?: MonitorAccount
  flow?: MonitorFlow
  mode?: 'record' | 'collect'
  state: MonitorState
  onClose: () => void
  onCreated: (id: string) => void
}) {
  const { t } = useTranslation()
  const client = useQueryClient()
  const accounts = props.state.config.accounts.filter(
    (account) => !props.flow || account.site_id === props.flow.site_id
  )
  const initialAccount = props.account ?? accounts[0]
  const initialMode = props.mode ?? 'collect'
  const initialFlows = props.state.config.flows.filter(
    (flow) =>
      flow.site_id === initialAccount?.site_id &&
      (initialMode === 'record' || monitorFlowReady(flow))
  )
  const initialFlow = initialFlows.find(
    (flow) => flow.id === (props.flow?.id ?? initialAccount?.flow_id)
  )
  const form = useForm<z.infer<typeof runSchema>>({
    resolver: zodResolver(runSchema),
    defaultValues: {
      account_id: initialAccount?.id ?? '',
      device_id:
        initialAccount?.device_id ||
        props.state.devices.find((device) => !device.revoked_at)?.id ||
        '',
      flow_id:
        initialMode === 'record'
          ? (props.flow?.id ?? '')
          : (initialFlow?.id ?? initialFlows[0]?.id ?? ''),
      mode: initialMode,
    },
  })
  const setupHintID = useId()
  const accountID = form.watch('account_id')
  const deviceID = form.watch('device_id')
  const flowID = form.watch('flow_id')
  const mode = form.watch('mode')
  const account = props.state.config.accounts.find(
    (item) => item.id === accountID
  )
  const site = props.state.config.sites.find(
    (item) => item.id === account?.site_id
  )
  const setupQuery = useQuery({
    queryKey: [...monitorQueryKey, 'browser-setup'],
    queryFn: getMonitorBrowserSetup,
    enabled: mode === 'record' && Boolean(deviceID),
    refetchInterval: 2000,
    refetchIntervalInBackground: true,
    staleTime: 0,
    gcTime: 0,
    retry: false,
    meta: { errorToast: false },
  })
  const setup = setupQuery.isError ? undefined : setupQuery.data?.[deviceID]
  const authorized = monitorSiteAuthorized(site, setup)
  const setupBlocked = mode === 'record' && !setup?.sidebar_open
  let setupHint = t(
    'Waiting for browser status. Open the extension side panel or update the extension if no status appears.'
  )
  if (setup && !setup.sidebar_open) {
    setupHint = t(
      'Open the selected browser’s extension side panel before starting configuration.'
    )
  } else if (setup?.sidebar_open && authorized) {
    setupHint = t(
      'Side panel detected. This site is already authorized; you can start configuration.'
    )
  } else if (setup?.sidebar_open) {
    setupHint = t(
      'Side panel detected. Start the task, then authorize this site in the side panel.'
    )
  }
  const flows = props.state.config.flows.filter(
    (flow) =>
      flow.site_id === account?.site_id &&
      (mode === 'record' || monitorFlowReady(flow))
  )
  useEffect(() => {
    if (
      flowID &&
      !props.state.config.flows.some(
        (flow) =>
          flow.id === flowID &&
          flow.site_id === account?.site_id &&
          (mode === 'record' || monitorFlowReady(flow))
      )
    ) {
      form.setValue('flow_id', '')
    }
  }, [account?.site_id, flowID, form, mode, props.state.config.flows])
  const mutation = useMutation({
    mutationFn: (values: z.infer<typeof runSchema>) => createMonitorJob(values),
    onSuccess: (job) => {
      void client.invalidateQueries({ queryKey: monitorQueryKey })
      props.onCreated(job.id)
      props.onClose()
    },
  })
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open && !mutation.isPending) props.onClose()
      }}
      title={
        props.mode === 'record'
          ? t('Configure flow in browser extension')
          : t('Run browser task')
      }
      description={props.account?.name}
      footer={
        <>
          <Button
            variant='outline'
            disabled={mutation.isPending}
            onClick={props.onClose}
          >
            {t('Cancel')}
          </Button>
          <Button
            form='monitor-run-form'
            type='submit'
            disabled={mutation.isPending || setupBlocked}
            aria-describedby={mode === 'record' ? setupHintID : undefined}
          >
            {props.mode === 'record'
              ? t('Start configuration task')
              : t('Start task')}
          </Button>
        </>
      }
    >
      <Form {...form}>
        <form
          id='monitor-run-form'
          onSubmit={form.handleSubmit((values) => {
            if (!setupBlocked) mutation.mutate(values)
          })}
          className='space-y-4'
        >
          {!props.account && (
            <MonitorField
              control={form.control}
              name='account_id'
              label={t('Upstream account')}
              options={accounts.map((item) => ({
                value: item.id,
                label: item.name,
              }))}
            />
          )}
          <MonitorField
            control={form.control}
            name='device_id'
            label={t('Browser')}
            options={props.state.devices
              .filter((device) => !device.revoked_at)
              .map((device) => ({ value: device.id, label: device.name }))}
          />
          <MonitorField
            control={form.control}
            name='flow_id'
            label={t('Collection flow')}
            options={[
              ...(mode === 'record'
                ? [
                    {
                      value: '',
                      label: t('Create a new flow in the extension'),
                    },
                  ]
                : []),
              ...flows.map((flow) => ({ value: flow.id, label: flow.name })),
            ]}
          />
          {!props.mode && (
            <MonitorField
              control={form.control}
              name='mode'
              label={t('Task mode')}
              options={[
                { value: 'collect', label: t('Execute flow and collect') },
                { value: 'record', label: t('Configure in browser extension') },
              ]}
            />
          )}
          {mode === 'collect' && flows.length === 0 && (
            <p className='text-muted-foreground text-sm'>
              {t(
                'Complete collection setup in the extension before collecting.'
              )}
            </p>
          )}
          {mode === 'record' && (
            <p
              id={setupHintID}
              role='status'
              aria-live='polite'
              className='text-muted-foreground text-sm'
            >
              {setupHint}
            </p>
          )}
        </form>
      </Form>
    </Dialog>
  )
}
