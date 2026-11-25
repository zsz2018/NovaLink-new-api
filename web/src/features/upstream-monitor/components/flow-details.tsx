import { useTranslation } from 'react-i18next'

import { StaticDataTable } from '@/components/data-table'
import { Dialog } from '@/components/dialog'

import {
  countMonitorSteps,
  monitorCollectionMode,
  monitorFlowObsolete,
  monitorFlowReady,
} from '../lib/jobs'
import type { MonitorCondition, MonitorFlow, MonitorStep } from '../types'

function ConditionDetails(props: { condition: MonitorCondition }) {
  const { t } = useTranslation()
  const states = {
    visible: t('Visible'),
    hidden: t('Hidden'),
    enabled: t('Enabled'),
    disabled: t('Disabled'),
  }
  const fields = [
    { label: t('Navigation URL'), value: props.condition.url },
    { label: t('Element selector'), value: props.condition.selector },
    { label: t('Element text'), value: props.condition.text },
    {
      label: t('Element state'),
      value: props.condition.state ? states[props.condition.state] : undefined,
    },
  ]
  return (
    <dl className='grid gap-3 text-sm sm:grid-cols-2'>
      {fields
        .filter((field) => field.value !== undefined && field.value !== '')
        .map((field) => (
          <div key={field.label} className='min-w-0 space-y-1'>
            <dt className='text-muted-foreground'>{field.label}</dt>
            <dd className='break-all whitespace-pre-wrap'>{field.value}</dd>
          </div>
        ))}
    </dl>
  )
}

export function FlowSteps(props: {
  steps: MonitorStep[]
  prefix?: string
  label?: string
}) {
  const { t } = useTranslation()
  const actions = {
    navigate: t('Navigate'),
    click: t('Click'),
    check: t('Check agreement'),
    input: t('Input'),
    select: t('Select'),
    key: t('Press key'),
    wait: t('Wait'),
    scroll: t('Scroll'),
    checkpoint: t('Manual checkpoint'),
    focus: t('Focus element'),
    assert: t('Assert condition'),
    wait_response: t('Wait for selected response'),
    condition: t('Conditional branch'),
    repeat: t('Repeat while condition matches'),
  }
  const states = {
    visible: t('Visible'),
    hidden: t('Hidden'),
    enabled: t('Enabled'),
    disabled: t('Disabled'),
  }
  const occurrences = new Map<string, number>()
  return (
    <ol
      className='min-w-0 space-y-3'
      aria-label={props.label ?? t('Flow steps')}
    >
      {props.steps.map((step, index) => {
        const number = props.prefix
          ? `${props.prefix}.${index + 1}`
          : String(index + 1)
        const signature = JSON.stringify(step)
        const occurrence = (occurrences.get(signature) ?? 0) + 1
        occurrences.set(signature, occurrence)
        const fields = [
          {
            label:
              step.action === 'navigate'
                ? t('Navigation URL')
                : t('Bound page URL'),
            value: step.url,
          },
          { label: t('Element selector'), value: step.selector },
          {
            label: t('Element state'),
            value: step.state ? states[step.state] : undefined,
          },
          { label: t('Element text'), value: step.text },
          {
            label: t('Input value or key'),
            value: step.variable ? undefined : step.value,
          },
          { label: t('Credential variable'), value: step.variable },
          { label: t('Timeout (milliseconds)'), value: step.timeout_ms },
          { label: t('Captured group context'), value: step.group },
          { label: t('Step ID'), value: step.id },
          { label: t('Request'), value: step.request_id },
          { label: t('Maximum iterations'), value: step.max_iterations },
        ]
        return (
          <li
            key={`${signature}:${occurrence}`}
            className='space-y-2 rounded-lg border p-3'
          >
            <h4 className='text-sm font-medium break-all'>
              {t('Step {{number}}', { number })} · {actions[step.action]}
              {step.name && ` · ${step.name}`}
            </h4>
            <dl className='grid gap-3 text-sm sm:grid-cols-2'>
              {fields
                .filter(
                  (field) => field.value !== undefined && field.value !== ''
                )
                .map((field) => (
                  <div key={field.label} className='min-w-0 space-y-1'>
                    <dt className='text-muted-foreground'>{field.label}</dt>
                    <dd className='break-all whitespace-pre-wrap'>
                      {field.value}
                    </dd>
                  </div>
                ))}
            </dl>
            {step.condition && (
              <div className='space-y-2'>
                <p className='text-sm font-medium'>{t('Condition')}</p>
                <ConditionDetails condition={step.condition} />
              </div>
            )}
            {Boolean(step.steps?.length) && (
              <div className='min-w-0 space-y-2 border-s ps-2 sm:ps-3'>
                <p className='text-sm font-medium'>
                  {t('When condition matches')}
                </p>
                <FlowSteps
                  steps={step.steps ?? []}
                  prefix={number}
                  label={t('When condition matches')}
                />
              </div>
            )}
            {Boolean(step.otherwise?.length) && (
              <div className='min-w-0 space-y-2 border-s ps-2 sm:ps-3'>
                <p className='text-sm font-medium'>{t('Otherwise')}</p>
                <FlowSteps
                  steps={step.otherwise ?? []}
                  prefix={number}
                  label={t('Otherwise')}
                />
              </div>
            )}
          </li>
        )
      })}
    </ol>
  )
}

export function FlowDetails(props: {
  flow: MonitorFlow
  siteName?: string
  onClose: () => void
}) {
  const { t } = useTranslation()
  const mode = monitorCollectionMode(props.flow)
  const dedicated = props.flow.extraction?.adapter !== undefined
  const fields = [
    { label: t('Upstream site'), value: props.siteName ?? props.flow.site_id },
    { label: t('Collection mode'), value: t(mode) },
    { label: t('Revision'), value: props.flow.revision },
    {
      label: t('Steps'),
      value:
        countMonitorSteps(props.flow.steps) +
        countMonitorSteps(props.flow.login?.steps ?? []),
    },
    {
      label: t('Per-page capture window (seconds)'),
      value: props.flow.capture_seconds,
    },
    {
      label: t('Selected requests'),
      value: props.flow.extraction?.requests.length ?? 0,
    },
    ...(!dedicated
      ? [
          {
            label: t('Field mappings'),
            value: props.flow.extraction?.mappings.length ?? 0,
          },
        ]
      : []),
  ]
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) props.onClose()
      }}
      title={props.flow.name}
      description={t(
        'Configure collection flows in the browser extension and review saved versions here. Flows collect account identity, balances and groups, with optional model prices and top-ups.'
      )}
      contentClassName='sm:max-w-4xl'
    >
      <div className='space-y-5'>
        <dl className='grid gap-4 text-sm sm:grid-cols-2'>
          {fields.map((field) => (
            <div key={field.label} className='min-w-0 space-y-1'>
              <dt className='text-muted-foreground'>{field.label}</dt>
              <dd className='break-words whitespace-pre-wrap'>{field.value}</dd>
            </div>
          ))}
        </dl>
        {dedicated && (
          <p className='text-muted-foreground text-sm'>
            {t(
              'The {{collector}} collector uses only responses captured by the browser from these selected requests and source pages. Collection settings are managed in the extension.',
              { collector: t(mode) }
            )}
          </p>
        )}
        {!monitorFlowReady(props.flow) && (
          <p className='text-muted-foreground text-sm'>
            {monitorFlowObsolete(props.flow)
              ? t(
                  'This flow uses obsolete collection rules and cannot run. Reconfigure it in the extension: clear the data configuration or switch collection mode, then match the data again.'
                )
              : t(
                  'Complete collection setup in the extension before collecting.'
                )}
          </p>
        )}
        {props.flow.extraction && (
          <>
            <section className='space-y-3'>
              <h3 className='font-medium'>{t('Selected requests')}</h3>
              <StaticDataTable
                data={props.flow.extraction.requests}
                getRowKey={(request) => request.id}
                emptyContent={t('No requests selected')}
                columns={[
                  {
                    id: 'id',
                    header: t('Request'),
                    cell: (request) => request.id,
                  },
                  {
                    id: 'pages',
                    header: t('Source pages'),
                    cell: (request) => (
                      <div className='max-w-sm space-y-1 break-all whitespace-pre-wrap'>
                        {(request.page_urls ?? []).map((page) => (
                          <p key={page}>{page}</p>
                        ))}
                      </div>
                    ),
                  },
                  {
                    id: 'url',
                    header: t('Request URL'),
                    cell: (request) => (
                      <p className='max-w-xl break-all whitespace-pre-wrap'>
                        {request.method} {request.url}
                        {request.query &&
                          `\n${new URLSearchParams(request.query).toString()}`}
                      </p>
                    ),
                  },
                ]}
              />
            </section>
            {!dedicated && (
              <section className='space-y-3'>
                <h3 className='font-medium'>{t('Field mappings')}</h3>
                <StaticDataTable
                  data={props.flow.extraction.mappings}
                  getRowKey={(mapping) => mapping.id}
                  emptyContent={t('No field mappings configured')}
                  columns={[
                    {
                      id: 'target',
                      header: t('Target fields'),
                      cell: (mapping) => (
                        <p className='font-mono'>{mapping.target}</p>
                      ),
                    },
                    {
                      id: 'request',
                      header: t('Request'),
                      cell: (mapping) => (
                        <div className='max-w-sm break-all whitespace-pre-wrap'>
                          <p>{mapping.request_id}</p>
                          <p className='text-muted-foreground'>
                            {mapping.root || '/'}
                          </p>
                          <p className='text-muted-foreground'>
                            {mapping.iterate}
                          </p>
                          {mapping.child_root && (
                            <p className='text-muted-foreground'>
                              {mapping.child_root}
                            </p>
                          )}
                        </div>
                      ),
                    },
                    {
                      id: 'fields',
                      header: t('Field mappings'),
                      cell: (mapping) => (
                        <dl className='max-w-xl space-y-2 whitespace-pre-wrap'>
                          {Object.entries(mapping.fields).map(
                            ([field, source]) => (
                              <div key={field} className='space-y-1'>
                                <dt className='font-mono'>{field}</dt>
                                <dd className='text-muted-foreground break-all'>
                                  {JSON.stringify(source)}
                                </dd>
                              </div>
                            )
                          )}
                          {mapping.billing && (
                            <div className='space-y-1'>
                              <dt>{t('Pricing rules')}</dt>
                              <dd className='text-muted-foreground break-all'>
                                {JSON.stringify(mapping.billing)}
                              </dd>
                            </div>
                          )}
                          {mapping.tiers && (
                            <div className='space-y-1'>
                              <dt>{t('Captured price tiers')}</dt>
                              <dd className='text-muted-foreground break-all'>
                                {JSON.stringify(mapping.tiers)}
                              </dd>
                            </div>
                          )}
                        </dl>
                      ),
                    },
                  ]}
                />
              </section>
            )}
          </>
        )}
        {props.flow.login && (
          <section className='space-y-3' aria-label={t('Login')}>
            <h3 className='font-medium'>{t('Login')}</h3>
            <p className='text-muted-foreground text-sm break-all'>
              {props.flow.login.url}
            </p>
            {props.flow.login.timeout_ms !== undefined && (
              <p className='text-sm'>
                {t('Timeout (milliseconds)')}: {props.flow.login.timeout_ms}
              </p>
            )}
            {props.flow.login.success && (
              <div className='space-y-2'>
                <h4 className='text-sm font-medium'>
                  {t('Login success condition')}
                </h4>
                <ConditionDetails condition={props.flow.login.success} />
              </div>
            )}
            <FlowSteps steps={props.flow.login.steps} />
          </section>
        )}
        <section className='space-y-3'>
          <h3 className='font-medium'>{t('Flow steps')}</h3>
          <FlowSteps steps={props.flow.steps} />
        </section>
      </div>
    </Dialog>
  )
}
