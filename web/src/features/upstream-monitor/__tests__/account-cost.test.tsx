import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createInstance } from 'i18next'
import { I18nextProvider } from 'react-i18next'
import { afterEach, describe, expect, test, vi } from 'vitest'

import en from '@/i18n/locales/en.json'
import zh from '@/i18n/locales/zh.json'
import { api } from '@/lib/api'

import { UpstreamMonitor } from '..'
import { AccountCostDialog } from '../components/account-cost-dialog'
import { AccountDialog } from '../components/account-dialog'
import { OfficialEstimateSource } from '../components/official-estimate'
import { accountCostSchema } from '../lib/config'
import { monitorQuoteHasPricingFacts } from '../lib/official-estimates'
import type { MonitorOfficialCatalog, MonitorState } from '../types'

const catalog: MonitorOfficialCatalog = {
  version: 'official-v1',
  verified_at: '2026-11-24',
  entries: ['model-a', 'model-b', 'catalog-only'].map((model) => ({
    id: `vendor:${model}`,
    provider: 'vendor',
    model,
    source_url: 'https://vendor.example/pricing',
    verified_at: '2026-11-24',
    quote: {
      name: '',
      group: '',
      currency: 'USD',
      billing_unit: 'token',
      token_unit: 1_000_000,
      input: 2,
      output: 8,
      cache_read: 0.2,
      cache_write: null,
      request_price: null,
      includes_group: false,
      notes: '',
    },
  })),
}
function costState(): MonitorState {
  return {
    config: {
      version: 1,
      sites: [
        {
          id: 'site',
          name: 'Site',
          origin: 'https://provider.example',
          allowed_origins: [],
          currency: 'USD',
          fx_rate: 1,
        },
      ],
      accounts: [
        {
          id: 'account',
          site_id: 'site',
          name: 'Account',
          username: 'operator',
          external_id: '42',
          has_password: false,
          device_id: '',
          flow_id: '',
          interval_minutes: 0,
          enabled: true,
          purchase_factor: 1,
          fee_rate: 0,
          fee_bearer: 'self',
        },
      ],
      flows: [],
      bindings: [
        {
          id: 'mapped',
          channel_id: 1,
          account_id: 'account',
          upstream_group: 'base',
          local_model: 'local-sale-alias',
          upstream_model: '',
        },
        {
          id: 'explicit',
          channel_id: 1,
          account_id: 'account',
          upstream_group: 'premium',
          local_model: 'another-sale-name',
          upstream_model: 'model-b',
        },
      ],
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
    },
    channels: [
      {
        id: 1,
        name: 'Channel',
        group: 'default',
        models: 'local-sale-alias,another-sale-name',
        model_mapping: '{"local-sale-alias":"model-a"}',
      },
    ],
    devices: [],
    jobs: [],
    groups: {},
    key_ready: true,
    snapshots: [
      {
        id: 'capture',
        job_id: 'job',
        account_id: 'account',
        created_at: 1,
        confirmed: true,
        complete: true,
        data: {
          pricing_status: 'unavailable',
          account_id: '42',
          balances: [{ amount: 10, currency: 'USD', kind: 'wallet' }],
          groups: [{ name: 'base', ratio: 0.5 }],
          models: [],
          topups: [],
          warnings: [],
        },
      },
    ],
    profit_rows: [],
    official_catalogs: [catalog],
    official_catalog_latest_version: catalog.version,
  }
}
const enableLabel =
  'Estimate missing quotes using official reference prices × upstream ratio'
afterEach(() => vi.restoreAllMocks())

describe('account cost comparison estimates', () => {
  test('identity-only captured rows can match official references while keeping the original quote fields null', async () => {
    const user = userEvent.setup()
    const state = costState()
    state.snapshots[0].data.models = [
      {
        ...catalog.entries[0].quote,
        name: 'model-a',
        group: 'base',
        input: null,
        output: null,
        cache_read: null,
        cache_write: null,
        includes_group: null,
      },
    ]
    render(
      <AccountCostDialog
        account={state.config.accounts[0]}
        state={state}
        onSave={vi.fn()}
        onClose={vi.fn()}
      />
    )
    await user.click(screen.getByRole('switch', { name: enableLabel }))
    await user.click(
      screen.getByRole('button', {
        name: 'Match bound upstream models by exact name',
      })
    )
    expect(
      within(
        screen.getByRole('table', { name: 'Selected estimate mappings' })
      ).getByRole('row', { name: /model-a/ })
    ).toBeVisible()
    expect(state.snapshots[0].data.models[0].input).toBeNull()
    expect(state.snapshots[0].data.models[0].output).toBeNull()
  })

  test.each([
    { label: 'free captured price', fields: { input: 0 } },
    { label: 'partial price', fields: { output: 2 } },
    { label: 'pending rule', fields: { pricing_pending: true } },
    { label: 'unsupported unit', fields: { billing_unit: 'unsupported' } },
    { label: 'request metadata', fields: { billing_unit: 'request' } },
    { label: 'native expression', fields: { billing_expression: 'p * 2' } },
    {
      label: 'captured pricing note',
      fields: { notes: 'Manual price confirmation needed' },
    },
  ])(
    '$label is a captured pricing fact that cannot be replaced',
    ({ fields }) => {
      expect(
        monitorQuoteHasPricingFacts({
          ...catalog.entries[0].quote,
          input: null,
          output: null,
          cache_read: null,
          cache_write: null,
          ...fields,
        })
      ).toBe(true)
    }
  )

  test.each([
    'javascript:alert(1)',
    'http://vendor.example/prices',
    '//vendor.example/prices',
    'https://user:secret@vendor.example/prices',
    'invalid URL',
  ])(
    'official source %s is displayed as text without an unsafe link',
    (sourceURL) => {
      render(
        <OfficialEstimateSource
          source={{
            kind: 'official_estimate',
            catalog_version: 'official-v1',
            entry_id: 'vendor:model-a',
            provider: '<img src=x onerror=alert(1)>',
            model: 'model-a',
            source_url: sourceURL,
            verified_at: '2026-11-24',
          }}
        />
      )
      expect(screen.getByText(sourceURL)).toBeVisible()
      expect(screen.queryByRole('link')).not.toBeInTheDocument()
      expect(screen.queryByRole('img')).not.toBeInTheDocument()
      expect(screen.getByText('<img src=x onerror=alert(1)>')).toBeVisible()
    }
  )

  test.each([
    { name: ' model-a', group: 'base' },
    { name: 'model\u0000a', group: 'base' },
    { name: '模'.repeat(86), group: 'base' },
    { name: 'model-a', group: '组'.repeat(43) },
  ])(
    'invalid identity $name / $group produces a translated mapping error without normalizing the name',
    (identity) => {
      const result = accountCostSchema.safeParse({
        enabled: true,
        catalog_version: catalog.version,
        models: [{ ...identity, entry_id: 'vendor:model-a' }],
      })
      expect(result.success).toBe(false)
      if (!result.success) {
        expect(result.error.issues[0].message).toBe(
          'Use exact non-empty model and group names within the supported UTF-8 limits, without surrounding spaces or control characters.'
        )
      }
    }
  )

  test('duplicate selections and more than 500 mappings produce meaningful policy validation errors', () => {
    const model = { name: 'model-a', group: 'base', entry_id: 'vendor:model-a' }
    const duplicate = accountCostSchema.safeParse({
      enabled: true,
      catalog_version: catalog.version,
      models: [model, model],
    })
    expect(duplicate.success).toBe(false)
    if (!duplicate.success) {
      expect(duplicate.error.issues[0].message).toBe(
        'Select each upstream model and group only once.'
      )
    }
    const tooMany = accountCostSchema.safeParse({
      enabled: true,
      catalog_version: catalog.version,
      models: Array.from({ length: 501 }, (_, index) => ({
        ...model,
        name: `model-${index}`,
      })),
    })
    expect(tooMany.success).toBe(false)
    if (!tooMany.success) {
      expect(tooMany.error.issues[0].message).toBe(
        'Select at most 500 estimate mappings.'
      )
    }
    expect(
      accountCostSchema.safeParse({
        enabled: false,
        catalog_version: '',
        models: [{ ...model, name: ' invalid ' }],
      }).success
    ).toBe(true)
  })

  test('estimates default off and exact matching uses bound upstream names and groups without changing captured data', async () => {
    const user = userEvent.setup()
    const state = costState()
    const original = JSON.stringify(state.snapshots)
    const save = vi.fn().mockResolvedValue(undefined)
    render(
      <AccountCostDialog
        account={state.config.accounts[0]}
        state={state}
        onSave={save}
        onClose={vi.fn()}
      />
    )
    const enable = screen.getByRole('switch', { name: enableLabel })
    expect(enable).toHaveAttribute('aria-checked', 'false')
    expect(
      screen.queryByRole('button', {
        name: 'Match bound upstream models by exact name',
      })
    ).not.toBeInTheDocument()
    await user.click(enable)
    await user.click(
      screen.getByRole('button', {
        name: 'Match bound upstream models by exact name',
      })
    )
    const table = within(
      screen.getByRole('table', { name: 'Selected estimate mappings' })
    )
    expect(table.getByRole('row', { name: /model-a/ })).toHaveTextContent(
      'base'
    )
    expect(table.getByRole('row', { name: /model-b/ })).toHaveTextContent(
      'premium'
    )
    expect(table.queryByText('catalog-only')).not.toBeInTheDocument()
    expect(table.queryByText('local-sale-alias')).not.toBeInTheDocument()
    await user.click(
      screen.getByRole('button', { name: 'Save comparison estimates' })
    )
    await waitFor(() => expect(save).toHaveBeenCalledOnce())
    expect(save.mock.calls[0][0].accounts[0].official_estimate).toEqual({
      catalog_version: 'official-v1',
      models: [
        { name: 'model-a', group: 'base', entry_id: 'vendor:model-a' },
        { name: 'model-b', group: 'premium', entry_id: 'vendor:model-b' },
      ],
    })
    expect(save.mock.calls[0][0].flows).toEqual(state.config.flows)
    expect(JSON.stringify(state.snapshots)).toBe(original)
  })

  test('captured zero and pending quotes are preserved when matching missing bound models', async () => {
    const user = userEvent.setup()
    const state = costState()
    state.snapshots[0].data.models = [
      {
        ...catalog.entries[0].quote,
        name: 'model-a',
        group: 'base',
        input: 0,
        output: 0,
      },
    ]
    state.snapshots[0].data.pricing_status = 'partial'
    const save = vi.fn().mockResolvedValue(undefined)
    render(
      <AccountCostDialog
        account={state.config.accounts[0]}
        state={state}
        onSave={save}
        onClose={vi.fn()}
      />
    )
    await user.click(screen.getByRole('switch', { name: enableLabel }))
    await user.click(
      screen.getByRole('button', {
        name: 'Match bound upstream models by exact name',
      })
    )
    const table = within(
      screen.getByRole('table', { name: 'Selected estimate mappings' })
    )
    expect(
      table.queryByRole('row', { name: /model-a/ })
    ).not.toBeInTheDocument()
    expect(table.getByRole('row', { name: /model-b/ })).toBeVisible()
    await user.click(
      screen.getByRole('button', { name: 'Save comparison estimates' })
    )
    await waitFor(() => expect(save).toHaveBeenCalledOnce())
    expect(save.mock.calls[0][0].accounts[0].official_estimate.models).toEqual([
      { name: 'model-b', group: 'premium', entry_id: 'vendor:model-b' },
    ])
    expect(state.snapshots[0].data.models[0].input).toBe(0)
  })

  test('manual aliases require explicit review and display pinned official source and USD units in both languages', async () => {
    const translations = createInstance()
    await translations.init({
      lng: 'en',
      fallbackLng: 'en',
      keySeparator: false,
      nsSeparator: false,
      interpolation: { escapeValue: false },
      resources: { en, zhCN: zh },
    })
    const user = userEvent.setup()
    const state = costState()
    const save = vi.fn().mockResolvedValue(undefined)
    render(
      <I18nextProvider i18n={translations}>
        <AccountCostDialog
          account={state.config.accounts[0]}
          state={state}
          onSave={save}
          onClose={vi.fn()}
        />
      </I18nextProvider>
    )
    await user.click(screen.getByRole('switch', { name: enableLabel }))
    expect(
      screen.getByText(
        'Official references estimate procurement costs only when upstream quotes are missing. Captured quotes and snapshots stay unchanged. Estimates do not confirm upstream model availability.'
      )
    ).toBeVisible()
    expect(
      screen.getByText(
        'This account policy uses the selected official catalog and captured group ratios to estimate comparison costs.'
      )
    ).toBeVisible()
    await user.type(
      screen.getByRole('combobox', { name: 'Upstream model name' }),
      'reviewed-alias'
    )
    await user.click(
      screen.getByRole('combobox', { name: 'Official reference model' })
    )
    await user.click(screen.getByRole('option', { name: 'vendor · model-a' }))
    expect(
      screen.getByRole('button', { name: 'Add reviewed model' })
    ).toBeDisabled()
    await user.click(
      screen.getByRole('checkbox', {
        name: 'I reviewed the upstream model and official reference match.',
      })
    )
    expect(
      screen.getByRole('region', { name: 'Official reference preview' })
    ).toHaveTextContent('USD')
    expect(
      screen.getByRole('region', { name: 'Official reference preview' })
    ).toHaveTextContent('Per 1M tokens')
    expect(
      within(
        screen.getByRole('region', { name: 'Official reference preview' })
      ).getByText('official-v1')
    ).toBeVisible()
    expect(screen.getByText('2026-11-24')).toBeVisible()
    expect(
      screen.getByRole('link', { name: 'https://vendor.example/pricing' })
    ).toHaveAttribute('rel', 'noopener noreferrer')
    await user.click(screen.getByRole('button', { name: 'Add reviewed model' }))
    await act(() => translations.changeLanguage('zhCN'))
    expect(
      screen.getByRole('switch', { name: '缺价时按官方基准价×上游倍率估算' })
    ).toHaveAttribute('aria-checked', 'true')
    expect(
      screen.getByText(
        '官方参考价格仅在缺少上游报价时估算采购成本，不改变已采集报价或快照。估算不代表上游支持该模型。'
      )
    ).toBeVisible()
    expect(
      screen.getByRole('row', { name: /reviewed-alias/ })
    ).toHaveTextContent('vendor:model-a')
    await user.click(screen.getByRole('button', { name: '保存比较估算设置' }))
    await waitFor(() => expect(save).toHaveBeenCalledOnce())
    expect(save.mock.calls[0][0].accounts[0].official_estimate.models).toEqual([
      { name: 'reviewed-alias', group: 'base', entry_id: 'vendor:model-a' },
    ])
  })

  test('a saved version stays pinned when a newer catalog exists and missing pinned catalogs block enabled saves but allow disabling', async () => {
    const user = userEvent.setup()
    const state = costState()
    const account = {
      ...state.config.accounts[0],
      official_estimate: {
        catalog_version: 'official-v1',
        models: [
          { name: 'model-a', group: 'base', entry_id: 'vendor:model-a' },
        ],
      },
    }
    state.config.accounts = [account]
    state.official_catalogs?.push({ ...catalog, version: 'official-v2' })
    state.official_catalog_latest_version = 'official-v2'
    const save = vi.fn().mockResolvedValue(undefined)
    const view = render(
      <AccountCostDialog
        account={account}
        state={state}
        onSave={save}
        onClose={vi.fn()}
      />
    )
    expect(
      screen.getByRole('combobox', { name: 'Catalog version' })
    ).toHaveTextContent('official-v1')
    expect(
      screen.getByText(
        'A newer official catalog is available. Select its version explicitly to update this policy.'
      )
    ).toBeVisible()
    await user.click(
      screen.getByRole('button', { name: 'Save comparison estimates' })
    )
    await waitFor(() => expect(save).toHaveBeenCalledOnce())
    expect(
      save.mock.calls[0][0].accounts[0].official_estimate.catalog_version
    ).toBe('official-v1')
    view.unmount()
    render(
      <AccountCostDialog
        account={account}
        state={{ ...state, official_catalogs: [] }}
        onSave={save}
        onClose={vi.fn()}
      />
    )
    expect(
      screen.getByText('Monitor official catalog is unavailable')
    ).toBeVisible()
    expect(
      screen.getByRole('button', { name: 'Save comparison estimates' })
    ).toBeDisabled()
    await user.click(screen.getByRole('switch', { name: enableLabel }))
    await user.click(
      screen.getByRole('button', { name: 'Save comparison estimates' })
    )
    await waitFor(() => expect(save).toHaveBeenCalledTimes(2))
    expect(save.mock.calls[1][0].accounts[0].official_estimate).toBeUndefined()
  })

  test('non-USD account sites cannot enable official comparison estimates', () => {
    const state = costState()
    state.config.sites[0].currency = 'CNY'
    render(
      <AccountCostDialog
        account={state.config.accounts[0]}
        state={state}
        onSave={vi.fn()}
        onClose={vi.fn()}
      />
    )
    expect(screen.getByRole('switch', { name: enableLabel })).toHaveAttribute(
      'aria-disabled',
      'true'
    )
    expect(
      screen.getByText(
        'Official reference estimates require a USD upstream site.'
      )
    ).toBeVisible()
  })

  test('ordinary account editing retains an existing comparison policy', async () => {
    const user = userEvent.setup()
    const state = costState()
    const account = {
      ...state.config.accounts[0],
      official_estimate: {
        catalog_version: 'official-v1',
        models: [
          { name: 'model-a', group: 'base', entry_id: 'vendor:model-a' },
        ],
      },
    }
    state.config.accounts = [account]
    const save = vi.fn().mockResolvedValue(undefined)
    render(
      <AccountDialog
        account={account}
        state={state}
        onSave={save}
        onClose={vi.fn()}
      />
    )
    expect(
      screen.getByText(
        'Leave blank to bind the captured account identity when saving collection settings in the extension.'
      )
    ).toBeVisible()
    await user.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(save).toHaveBeenCalledOnce())
    expect(save.mock.calls[0][0].accounts[0].official_estimate).toEqual(
      account.official_estimate
    )
  })

  test('saving account estimates immediately refetches comparisons without recollecting or changing the captured snapshot', async () => {
    const user = userEvent.setup()
    const before = costState()
    const row = {
      estimated: false,
      binding_id: 'mapped',
      local_group: 'default',
      local_ratio: 1,
      channel_id: 1,
      channel_name: 'Channel',
      account_id: 'account',
      site_name: 'Site',
      upstream_group: 'base',
      upstream_ratio: 0.5,
      local_model: 'local-sale-alias',
      upstream_model: 'model-a',
      revenue: 10,
      cost: null,
      profit: null,
      margin: null,
      status: 'pending',
      reason: 'Upstream model price is missing',
      snapshot_id: 'capture',
      captured_at: 1,
    }
    before.profit_rows = [row]
    const after = {
      ...before,
      profit_rows: [
        {
          ...row,
          estimated: true,
          status: 'ready',
          reason: '',
          cost: 1,
          profit: 9,
          margin: 0.9,
        },
      ],
    }
    const get = vi
      .spyOn(api, 'get')
      .mockResolvedValue({ data: { success: true, data: before } })
    const put = vi.spyOn(api, 'put').mockImplementation(async () => {
      get.mockResolvedValue({ data: { success: true, data: after } })
      return { data: { success: true } }
    })
    const post = vi.spyOn(api, 'post')
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false, gcTime: Infinity } },
    })
    const view = render(
      <QueryClientProvider client={client}>
        <UpstreamMonitor />
      </QueryClientProvider>
    )
    try {
      await screen.findByText('Upstream model price is missing')
      const sitesTab = screen.getByRole('tab', { name: 'Sites and accounts' })
      sitesTab.focus()
      await user.keyboard('{Enter}')
      await user.click(
        screen.getByRole('button', { name: 'Cost comparison estimates' })
      )
      await user.click(screen.getByRole('switch', { name: enableLabel }))
      await user.click(
        screen.getByRole('button', {
          name: 'Match bound upstream models by exact name',
        })
      )
      await user.click(
        screen.getByRole('button', { name: 'Save comparison estimates' })
      )
      await waitFor(() => expect(put).toHaveBeenCalledOnce())
      await waitFor(() =>
        expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
      )
      expect(
        client.getQueryData<MonitorState>(['upstream-monitor'])?.profit_rows[0]
          .estimated
      ).toBe(true)
      expect(
        client.getQueryData<MonitorState>(['upstream-monitor'])?.snapshots
      ).toEqual(before.snapshots)
      expect(post).not.toHaveBeenCalled()
    } finally {
      view.unmount()
      client.clear()
    }
  })
})
