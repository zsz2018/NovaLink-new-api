import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import i18next, { createInstance, type i18n } from 'i18next'
import { I18nextProvider } from 'react-i18next'
import { afterEach, describe, expect, test, vi } from 'vitest'

import en from '@/i18n/locales/en.json'
import fr from '@/i18n/locales/fr.json'
import ja from '@/i18n/locales/ja.json'
import ru from '@/i18n/locales/ru.json'
import viLocale from '@/i18n/locales/vi.json'
import zhTW from '@/i18n/locales/zh-TW.json'
import zh from '@/i18n/locales/zh.json'

import { JobDetail } from '../components/job-detail'
import { JobsPanel } from '../components/jobs-panel'
import { formatMonitorMessage } from '../lib/task-messages'
import type { MonitorConfig, MonitorJobDetail } from '../types'

const api = vi.hoisted(() => ({
  getMonitorJob: vi.fn(),
  controlMonitorJob: vi.fn(),
  createMonitorJob: vi.fn(),
}))
vi.mock('../api', () => ({ ...api, monitorQueryKey: ['upstream-monitor'] }))
const clients: QueryClient[] = []
const config: MonitorConfig = {
  version: 1,
  sites: [],
  accounts: [],
  flows: [],
  bindings: [],
  scenario: {
    currency: 'USD',
    usd_exchange: 1,
    input_tokens: 1000,
    output_tokens: 100,
    cache_read: 0,
    cache_write: 0,
    requests: 1,
    revenue_factor: 1,
    fee_rate: 0,
    fee_bearer: 'self',
    user_group: '',
  },
}

function renderJob(
  result: MonitorJobDetail,
  onJobCreated = vi.fn(),
  translations = i18next
) {
  api.getMonitorJob.mockResolvedValue(result)
  api.controlMonitorJob.mockResolvedValue(undefined)
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  clients.push(client)
  render(
    <QueryClientProvider client={client}>
      <I18nextProvider i18n={translations}>
        <JobDetail
          id={result.job.id}
          config={config}
          onClose={vi.fn()}
          onJobCreated={onJobCreated}
        />
      </I18nextProvider>
    </QueryClientProvider>
  )
}
const job: MonitorJobDetail['job'] = {
  id: 'record-job',
  account_id: 'account',
  device_id: 'browser',
  flow_id: 'flow',
  mode: 'record',
  status: 'running',
  step: 1,
  message: '',
  created_at: 1,
  updated_at: 1,
  complete: false,
}
afterEach(() => {
  clients.forEach((client) => client.clear())
  clients.length = 0
  vi.clearAllMocks()
})

const taskMessageResources = {
  en,
  zhCN: zh,
  zhTW,
  fr,
  ja,
  ru,
  vi: viLocale,
}

async function taskMessagesI18n(): Promise<i18n> {
  const translations = createInstance()
  await translations.init({
    lng: 'en',
    fallbackLng: 'en',
    nsSeparator: false,
    keySeparator: false,
    interpolation: { escapeValue: false },
    resources: taskMessageResources,
  })
  return translations
}

describe('browser task messages', () => {
  test('a workflow failure localizes the installed error and action while preserving the operator step name', async () => {
    const translations = await taskMessagesI18n()
    await translations.changeLanguage('zhCN')
    const name = '<img src=x> $t(Saved successfully) {{action}}'
    const message = `步骤 2.否则.1 · ${name}（click）：The selected control is not unique`
    expect(formatMonitorMessage(message, translations.t)).toBe(
      `步骤 2.否则.1 · ${name}（点击）：所选控件不唯一`
    )
    await translations.changeLanguage('en')
    expect(formatMonitorMessage(message, translations.t)).toBe(
      `Step 2.Otherwise.1 · ${name} (Click): The selected control is not unique`
    )
    expect(
      formatMonitorMessage(`步骤 1（condition）：${message}`, translations.t)
    ).toBe(
      `Step 1 (Conditional branch): Step 2.Otherwise.1 · ${name} (Click): The selected control is not unique`
    )
    for (const unknown of [
      '步骤 1（condition）：'.repeat(4) + message,
      '步骤 1（click）：Saved successfully',
      '步骤 501（click）：The selected control is not unique',
      '步骤 1（unknown）：The selected control is not unique',
      '步骤 1（click）：The selected control is not unique\n',
    ]) {
      expect(formatMonitorMessage(unknown, translations.t)).toBe(unknown)
    }
  })
  test.each(Object.entries(taskMessageResources))(
    '%s translates fixed lifecycle and recovery messages without classifying the outcome',
    async (language, resources) => {
      const translations = await taskMessagesI18n()
      await translations.changeLanguage(language)
      for (const message of [
        'Task exceeded its 30-minute execution limit',
        'Flow draft updated in the browser sidebar',
        'Browser lease expired',
        'Browser executor restarted; interrupted task was stopped',
        'Browser worker restarted; interrupted task was stopped to prevent duplicate login',
        'Temporary response preview is unavailable; visit the page again',
        'Captured identity is unavailable; identify the requests again',
        'Source facts are unavailable; collect the data again',
      ] as const) {
        const expected = resources.translation[message]
        expect(formatMonitorMessage(message, translations.t)).toBe(expected)
        if (language !== 'en') expect(expected).not.toBe(message)
      }
      expect(
        formatMonitorMessage(
          'Executing step 500[100].1: wait_response',
          translations.t
        )
      ).toBe(
        translations.t('Executing step {{path}}: {{action}}', {
          path: '500[100].1',
          action: resources.translation['Wait for selected response'],
        })
      )
      expect(
        formatMonitorMessage('Recorded 500 page steps', translations.t)
      ).toBe(translations.t('Recorded page steps: {{number}}', { number: 500 }))
    }
  )

  test('unrecognized or out-of-bounds progress remains literal text', async () => {
    const translations = await taskMessagesI18n()
    await translations.changeLanguage('zhCN')
    for (const message of [
      '',
      'Saved successfully',
      'Task exceeded its 30-minute execution limit: upstream details',
      'Executing step 1: <img src=x onerror=alert(1)>',
      'Executing step 1: toString',
      'Executing step 501: click',
      'Executing step 0: click',
      'Executing step 1[101].1: click',
      'Executing step 1[1].1[1].1[1].1[1].1: click',
      'Executing step 1: click\n',
      'Recorded 501 page steps',
      'Recorded 1 page steps\n',
    ]) {
      expect(formatMonitorMessage(message, translations.t)).toBe(message)
    }
  })

  test('stored English list messages follow all seven interface languages and preserve unknown failures', async () => {
    const translations = await taskMessagesI18n()
    const timeout = 'Task exceeded its 30-minute execution limit'
    const draft = 'Flow draft updated in the browser sidebar'
    const unknown = 'Saved successfully'
    render(
      <I18nextProvider i18n={translations}>
        <JobsPanel
          state={{
            config,
            devices: [],
            jobs: [
              { ...job, id: 'timeout', status: 'failed', message: timeout },
              { ...job, id: 'draft', message: draft },
              {
                ...job,
                id: 'step',
                message: 'Executing login step 1.否则.2[3].1: input',
              },
              {
                ...job,
                id: 'unknown',
                status: 'failed',
                message: 'Browser flow saved',
                error: unknown,
              },
            ],
            snapshots: [],
            channels: [],
            groups: {},
            profit_rows: [],
            key_ready: true,
          }}
          onJob={vi.fn()}
          onSnapshot={vi.fn()}
        />
      </I18nextProvider>
    )
    for (const [language, resources] of Object.entries(taskMessageResources)) {
      await act(() => translations.changeLanguage(language))
      const messages = resources.translation
      expect(screen.getByText(messages[timeout])).toBeInTheDocument()
      expect(screen.getByText(messages[draft])).toBeInTheDocument()
      const path = `1.${messages.Otherwise}.2[3].1`
      expect(
        screen.getByText(
          translations.t('Executing login step {{path}}: {{action}}', {
            path,
            action: messages.Input,
          })
        )
      ).toBeInTheDocument()
      const failure = screen.getByRole('row', { name: new RegExp(unknown) })
      expect(within(failure).getByText(messages.failed)).toBeInTheDocument()
      expect(
        within(failure).queryByText(messages['Browser flow saved'])
      ).not.toBeInTheDocument()
    }
  })

  test('task detail translates fixed errors on language switch and safely displays unknown error text', async () => {
    const translations = await taskMessagesI18n()
    const error = '<img src=x onerror=alert(1)> {{name}} $t(Saved successfully)'
    renderJob(
      {
        job: {
          ...job,
          status: 'failed',
          message: 'Task exceeded its 30-minute execution limit',
          error,
        },
        recorded_steps: [],
      },
      vi.fn(),
      translations
    )
    expect(await screen.findByRole('alert')).toHaveTextContent(error)
    expect(
      screen.getByText(/Task exceeded its 30-minute execution limit/)
    ).toBeInTheDocument()
    await act(() => translations.changeLanguage('zhCN'))
    expect(screen.getByText(/任务已超过 30 分钟的执行时限/)).toBeInTheDocument()
    expect(screen.getByRole('alert').textContent).toBe(error)
    expect(screen.getByRole('alert').querySelector('img')).toBeNull()
  })
})

describe('browser task controls', () => {
  test('retry opens the newly created task instead of leaving the failed task selected', async () => {
    const user = userEvent.setup()
    const onRetry = vi.fn()
    renderJob(
      {
        job: { ...job, status: 'failed', flow_revision: 0 },
        recorded_steps: [],
      },
      onRetry
    )
    api.controlMonitorJob.mockResolvedValue({
      ...job,
      id: 'retried-task',
      status: 'queued',
    })
    await user.click(await screen.findByRole('button', { name: 'Retry task' }))
    await waitFor(() => expect(onRetry).toHaveBeenCalledWith('retried-task'))
    expect(api.controlMonitorJob).toHaveBeenCalledWith('record-job', 'retry')
  })
  test('a completed configuration starts editing the saved flow in a new task', async () => {
    const user = userEvent.setup()
    const onCreated = vi.fn()
    renderJob(
      {
        job: { ...job, status: 'completed', complete: true },
        recorded_steps: [],
      },
      onCreated
    )
    api.createMonitorJob.mockResolvedValue({
      ...job,
      id: 'edit-task',
      status: 'queued',
    })
    await user.click(
      await screen.findByRole('button', { name: 'Edit collection flow' })
    )
    await waitFor(() => expect(onCreated).toHaveBeenCalledWith('edit-task'))
    expect(api.createMonitorJob).toHaveBeenCalledWith({
      account_id: 'account',
      device_id: 'browser',
      flow_id: 'flow',
      mode: 'record',
    })
    expect(api.controlMonitorJob).not.toHaveBeenCalled()
    expect(
      screen.queryByRole('button', { name: 'Retry task' })
    ).not.toBeInTheDocument()
    expect(
      screen.queryByText(/Use the browser extension side panel/)
    ).not.toBeInTheDocument()
  })
  test('a completed task offers recollection without exposing raw responses or historical reanalysis', async () => {
    const user = userEvent.setup()
    const onCreated = vi.fn()
    renderJob(
      {
        job: {
          ...job,
          mode: 'collect',
          status: 'completed',
          complete: true,
        },
        recorded_steps: [],
      },
      onCreated
    )
    api.createMonitorJob.mockResolvedValue({
      ...job,
      id: 'collect-again',
      mode: 'collect',
      status: 'queued',
    })
    await user.click(
      await screen.findByRole('button', { name: 'Collect again' })
    )
    await waitFor(() => expect(onCreated).toHaveBeenCalledWith('collect-again'))
    expect(api.createMonitorJob).toHaveBeenCalledWith({
      account_id: 'account',
      device_id: 'browser',
      flow_id: 'flow',
      mode: 'collect',
    })
    expect(api.controlMonitorJob).not.toHaveBeenCalled()
    expect(
      screen.queryByRole('button', { name: 'Retry task' })
    ).not.toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: 'View raw response' })
    ).not.toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: 'Analyze captured responses' })
    ).not.toBeInTheDocument()
    expect(screen.queryByText(/Analysis status/)).not.toBeInTheDocument()
  })
  test('a running configuration displays recorded steps without site editing or finishing controls', async () => {
    renderJob({
      job,
      recorded_steps: [
        { action: 'input', selector: '#password', variable: 'password' },
      ],
    })
    expect(
      await screen.findByText(/Use the browser extension side panel/)
    ).toBeInTheDocument()
    expect(screen.getByText('#password')).toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: 'Finish recording' })
    ).not.toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: 'Save recorded steps to flow' })
    ).not.toBeInTheDocument()
    expect(
      screen.queryByRole('combobox', { name: 'Target flow' })
    ).not.toBeInTheDocument()
    expect(screen.queryByRole('textbox')).not.toBeInTheDocument()
    expect(api.controlMonitorJob).not.toHaveBeenCalled()
  })
  test('a completed collection with an incomplete result offers recollection and cannot be cancelled', async () => {
    renderJob({
      job: {
        ...job,
        mode: 'collect',
        status: 'completed',
        complete: false,
      },
      recorded_steps: [],
    })
    expect(
      await screen.findByRole('button', { name: 'Collect again' })
    ).toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: 'Cancel task' })
    ).not.toBeInTheDocument()
  })
  test.each(['queued', 'running', 'awaiting_user'] as const)(
    'a %s task can be cancelled after confirmation',
    async (status) => {
      const user = userEvent.setup()
      renderJob({ job: { ...job, status }, recorded_steps: [] })
      await user.click(
        await screen.findByRole('button', { name: 'Cancel task' })
      )
      await user.click(screen.getByRole('button', { name: /^Continue$/ }))
      await waitFor(() =>
        expect(api.controlMonitorJob).toHaveBeenCalledWith(job.id, 'cancel')
      )
    }
  )
})
