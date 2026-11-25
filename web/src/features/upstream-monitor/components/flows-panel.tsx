import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { StaticDataTable } from '@/components/data-table'
import { Button } from '@/components/ui/button'
import { handleServerError } from '@/lib/handle-server-error'

import {
  countMonitorSteps,
  monitorCollectionMode,
  monitorFlowObsolete,
  monitorFlowReady,
} from '../lib/jobs'
import type { MonitorFlow, MonitorState, SaveMonitorConfig } from '../types'
import { FlowDetails } from './flow-details'

export function FlowsPanel(props: {
  state: MonitorState
  onConfigure: (flow?: MonitorFlow) => void
  onSave: SaveMonitorConfig
}) {
  const { t } = useTranslation()
  const [view, setView] = useState<MonitorFlow | null>(null)
  const [remove, setRemove] = useState<MonitorFlow | null>(null)
  const [pending, setPending] = useState(false)
  const deleteFlow = async () => {
    if (!remove || pending) return
    setPending(true)
    try {
      const config = props.state.config
      await props.onSave({
        ...config,
        flows: config.flows.filter((flow) => flow.id !== remove.id),
        accounts: config.accounts.map((account) =>
          account.flow_id === remove.id
            ? { ...account, flow_id: '', interval_minutes: 0 }
            : account
        ),
      })
      setRemove(null)
    } catch (error) {
      handleServerError(error)
    } finally {
      setPending(false)
    }
  }
  const canConfigure =
    props.state.config.accounts.length > 0 &&
    props.state.devices.some((device) => !device.revoked_at)
  return (
    <div className='space-y-4'>
      <div className='flex flex-wrap items-center justify-between gap-4'>
        <p className='text-muted-foreground text-sm'>
          {t(
            'Configure collection flows in the browser extension and review saved versions here. Flows collect account identity, balances and groups, with optional model prices and top-ups.'
          )}
        </p>
        <Button disabled={!canConfigure} onClick={() => props.onConfigure()}>
          {t('Configure in browser extension')}
        </Button>
      </div>
      {!canConfigure && (
        <p className='text-muted-foreground text-sm'>
          {t(
            'Add an upstream account and pair a browser before configuring a flow.'
          )}
        </p>
      )}
      <StaticDataTable
        data={props.state.config.flows}
        getRowKey={(flow) => flow.id}
        emptyContent={t('No collection flows configured.')}
        columns={[
          {
            id: 'name',
            header: t('Flow name'),
            cell: (flow) => (
              <div>
                <div className='font-medium'>{flow.name}</div>
                {!monitorFlowReady(flow) && (
                  <p className='text-muted-foreground max-w-sm text-xs whitespace-normal'>
                    {monitorFlowObsolete(flow)
                      ? t(
                          'This flow uses obsolete collection rules and cannot run. Reconfigure it in the extension: clear the data configuration or switch collection mode, then match the data again.'
                        )
                      : t('Collection setup required')}
                  </p>
                )}
                <div className='text-muted-foreground text-xs'>
                  {
                    props.state.config.sites.find(
                      (site) => site.id === flow.site_id
                    )?.name
                  }
                </div>
              </div>
            ),
          },
          {
            id: 'collection-mode',
            header: t('Collection mode'),
            cell: (flow) => t(monitorCollectionMode(flow)),
          },
          {
            id: 'steps',
            header: t('Steps'),
            cell: (flow) =>
              String(
                countMonitorSteps(flow.steps) +
                  countMonitorSteps(flow.login?.steps ?? [])
              ),
          },
          {
            id: 'requests',
            header: t('Selected requests'),
            cell: (flow) => String(flow.extraction?.requests.length ?? 0),
          },
          {
            id: 'mappings',
            header: t('Field mappings'),
            cell: (flow) =>
              flow.extraction?.adapter
                ? '—'
                : String(flow.extraction?.mappings.length ?? 0),
          },
          {
            id: 'revision',
            header: t('Revision'),
            cell: (flow) => String(flow.revision),
          },
          {
            id: 'actions',
            header: t('Actions'),
            cell: (flow) => (
              <div className='flex flex-wrap gap-2'>
                <Button
                  variant='outline'
                  size='sm'
                  onClick={() => setView(flow)}
                >
                  {t('View flow')}
                </Button>
                <Button
                  variant='outline'
                  size='sm'
                  disabled={
                    !canConfigure ||
                    !props.state.config.accounts.some(
                      (account) => account.site_id === flow.site_id
                    )
                  }
                  onClick={() => props.onConfigure(flow)}
                >
                  {t('Configure in browser extension')}
                </Button>
                <Button
                  variant='destructive'
                  size='sm'
                  disabled={pending}
                  onClick={() => setRemove(flow)}
                >
                  {t('Delete')}
                </Button>
              </div>
            ),
          },
        ]}
      />
      <ConfirmDialog
        open={Boolean(remove)}
        onOpenChange={(open) => {
          if (!open && !pending) setRemove(null)
        }}
        title={t('Delete collection flow?')}
        desc={t(
          'Delete "{{name}}"? Linked accounts will be unlinked and their scheduled collection will be disabled. Existing tasks and collection history are retained.',
          { name: remove?.name ?? '' }
        )}
        confirmText={t('Delete')}
        destructive
        isLoading={pending}
        handleConfirm={() => void deleteFlow()}
      />
      {view && (
        <FlowDetails
          flow={view}
          siteName={
            props.state.config.sites.find((site) => site.id === view.site_id)
              ?.name
          }
          onClose={() => setView(null)}
        />
      )}
    </div>
  )
}
