import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import i18next, { createInstance } from 'i18next'
import { I18nextProvider } from 'react-i18next'
import { afterEach, describe, expect, test, vi } from 'vitest'

import en from '@/i18n/locales/en.json'
import zh from '@/i18n/locales/zh.json'
import { api } from '@/lib/api'

import { UpstreamMonitor } from '..'
import { ProfitDetailBreakdown } from '../components/profit-detail-breakdown'
import { ProfitPanel } from '../components/profit-panel'
import { SitesPanel } from '../components/sites-panel'
import {
  resolveProfitSelection,
  sortProfitRows,
  summarizeProfit,
  upstreamSelectionKey,
} from '../lib/profit'
import type {
  MonitorPriceSource,
  MonitorProfitDetail,
  MonitorProfitRow,
  MonitorState,
} from '../types'

afterEach(() => vi.restoreAllMocks())

const estimateSource: MonitorPriceSource = {
  kind: 'official_estimate',
  catalog_version: 'official-2026-11',
  entry_id: 'vendor/reference-model',
  provider: 'Verified vendor',
  model: 'Reference model',
  source_url: 'https://vendor.example/pricing',
  verified_at: '2026-11-24',
}

const baseRow: MonitorProfitRow = {
  estimated: false,
  local_group: 'standard',
  local_ratio: 1,
  channel_id: 1,
  channel_name: 'Channel A',
  account_id: 'account-a',
  site_name: 'Site A',
  upstream_group: 'base',
  upstream_ratio: 0.5,
  local_model: 'model-a',
  upstream_model: 'source-a',
  revenue: 10,
  cost: 6,
  profit: 4,
  margin: 0.4,
  status: 'ready',
  reason: '',
  snapshot_id: '',
  captured_at: 0,
}

function stateWithRows(rows: MonitorProfitRow[]): MonitorState {
  return {
    config: {
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
    },
    devices: [],
    jobs: [],
    snapshots: [],
    channels: [],
    groups: {},
    profit_rows: rows,
    key_ready: true,
  }
}

function renderProfitDetails(row: MonitorProfitRow) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: Infinity } },
  })
  return render(
    <QueryClientProvider client={client}>
      <ProfitPanel state={stateWithRows([row])} onSnapshot={vi.fn()} />
    </QueryClientProvider>
  )
}

describe('profit comparisons', () => {
  test('the empty comparison describes its purpose without requiring captured prices or manual confirmation', () => {
    render(<ProfitPanel state={stateWithRows([])} onSnapshot={vi.fn()} />)
    expect(
      screen.getByText(
        'Compare channel revenue with upstream procurement costs using captured account data and configured model correspondences.'
      )
    ).toBeVisible()
    expect(
      screen.queryByText(
        'Collect and confirm upstream prices, then bind channels to upstream accounts and groups.'
      )
    ).not.toBeInTheDocument()
  })

  test('mixed ready rows visibly identify estimated costs while keeping pending reasons and mixed totals clear', async () => {
    const translations = createInstance()
    await translations.init({
      lng: 'en',
      fallbackLng: 'en',
      keySeparator: false,
      nsSeparator: false,
      interpolation: { escapeValue: false },
      resources: { en, zhCN: zh },
    })
    const estimated = {
      ...baseRow,
      channel_id: 2,
      local_model: 'estimated-model',
      estimated: true,
      price_source: estimateSource,
    }
    const pending = {
      ...baseRow,
      channel_id: 3,
      local_model: 'pending-model',
      status: 'pending',
      cost: null,
      profit: null,
      margin: null,
      reason: 'Upstream model price is missing',
    }
    render(
      <I18nextProvider i18n={translations}>
        <ProfitPanel
          state={stateWithRows([baseRow, estimated, pending])}
          onSnapshot={vi.fn()}
        />
      </I18nextProvider>
    )
    const actual = within(screen.getByRole('row', { name: /model-a/ }))
    expect(
      actual.queryByText('Official price estimate')
    ).not.toBeInTheDocument()
    const route = within(screen.getByRole('row', { name: /estimated-model/ }))
    expect(route.getByText('Official price estimate')).toBeVisible()
    expect(route.getByText('Estimated margin')).toBeVisible()
    expect(route.getByText('40%')).toBeVisible()
    expect(
      screen.getByRole('row', { name: /pending-model/ })
    ).toHaveTextContent(pending.reason)
    const summary = within(
      screen.getByRole('region', { name: 'Scenario summary' })
    )
    expect(summary.getByText('Estimated procurement cost')).toBeVisible()
    expect(summary.getByText('12')).toBeVisible()
    expect(summary.getByText('Estimated profit')).toBeVisible()
    expect(summary.getByText('8')).toBeVisible()
    expect(
      screen.getByText(
        'Totals include official price estimates. Pending comparisons remain excluded.'
      )
    ).toBeVisible()
    await act(() => translations.changeLanguage('zhCN'))
    expect(route.getByText('官方价格估算')).toBeVisible()
    expect(route.getByText('估算利润率')).toBeVisible()
    expect(summary.getByText('估算采购成本')).toBeVisible()
    expect(summary.getByText('估算利润')).toBeVisible()
    expect(
      screen.getByText(zh.translation['Upstream model price is missing'])
    ).toBeVisible()
  })

  test('estimated profit details label calculation amounts and preserve source metadata in raw JSON and copy', async () => {
    const user = userEvent.setup()
    const clipboard = vi.spyOn(navigator.clipboard, 'writeText')
    const row: MonitorProfitRow = {
      ...baseRow,
      binding_id: 'estimate-binding',
      estimated: true,
      price_source: estimateSource,
    }
    const detail: MonitorProfitDetail = {
      row,
      scenario: stateWithRows([row]).config.scenario,
      context_tokens: 1000,
      upstream: {
        estimated: true,
        price_source: estimateSource,
        currency: 'USD',
        unit: 'token',
        base_amount: 6,
        group_ratio: 1,
        fx_rate: 1,
        factor: 1,
        fee_rate: 0,
        fee_bearer: 'self',
        fee_multiplier: 1,
        amount: 6,
      },
      components: [
        {
          kind: 'input',
          quantity: 1000,
          local_unit_price: 10,
          upstream_unit_price: 6,
          unit_profit: 4,
          unit_margin: 0.4,
          revenue: 10,
          cost: 6,
        },
      ],
      tier_samples: [
        {
          input_tokens: 1000,
          context_tokens: 1000,
          revenue: 10,
          cost: 6,
          profit: 4,
          margin: 0.4,
          status: 'ready',
          reason: '',
        },
      ],
      per_million_context: {
        revenue: 10000,
        cost: 6000,
        profit: 4000,
        margin: 0.4,
      },
    }
    vi.spyOn(api, 'get').mockResolvedValue({
      data: { success: true, data: detail },
    })
    renderProfitDetails(row)
    await user.click(screen.getByRole('button', { name: 'Profit details' }))
    const dialog = within(
      await screen.findByRole('dialog', { name: 'Profit details' })
    )
    const scenario = within(
      await dialog.findByRole('region', { name: 'Scenario result' })
    )
    expect(scenario.getByText('Estimated procurement cost')).toBeVisible()
    expect(scenario.getByText('Estimated profit')).toBeVisible()
    expect(scenario.getByText('Estimated margin')).toBeVisible()
    expect(
      dialog.getByRole('region', { name: 'Official price estimate sources' })
    ).toHaveTextContent('official-2026-11')
    expect(dialog.getByText('2026-11-24')).toBeVisible()
    expect(dialog.getByText('Verified vendor')).toBeVisible()
    expect(dialog.getByText('Reference model')).toBeVisible()
    const tiers = within(
      dialog.getByRole('table', { name: 'Profit at tier boundaries' })
    )
    expect(
      tiers.getByRole('columnheader', { name: 'Estimated procurement cost' })
    ).toBeVisible()
    expect(
      tiers.getByRole('columnheader', { name: 'Estimated profit' })
    ).toBeVisible()
    expect(
      dialog.getByRole('columnheader', {
        name: 'Estimated unit purchase price',
      })
    ).toBeVisible()
    expect(
      dialog.getByRole('columnheader', { name: 'Estimated unit profit' })
    ).toBeVisible()
    expect(
      dialog.getByRole('heading', { name: 'Estimated purchase calculation' })
    ).toBeVisible()
    const jsonTab = dialog.getByRole('tab', { name: 'JSON' })
    jsonTab.focus()
    await user.keyboard('{Enter}')
    const json = JSON.stringify(detail, null, 2)
    expect(
      dialog.getByRole('tabpanel', { name: 'JSON' }).querySelector('code')
        ?.textContent
    ).toBe(json)
    await user.click(dialog.getByRole('button', { name: 'Copy to clipboard' }))
    expect(clipboard).toHaveBeenCalledWith(json)
  })

  test('upstream estimate metadata labels pending detail amounts in both languages without suppressing the reason', async () => {
    const translations = createInstance()
    await translations.init({
      lng: 'en',
      fallbackLng: 'en',
      keySeparator: false,
      nsSeparator: false,
      interpolation: { escapeValue: false },
      resources: { en, zhCN: zh },
    })
    render(
      <I18nextProvider i18n={translations}>
        <ProfitDetailBreakdown
          detail={{
            row: {
              ...baseRow,
              status: 'pending',
              cost: null,
              profit: null,
              margin: null,
              reason: 'Upstream group ratio is unknown',
            },
            scenario: stateWithRows([]).config.scenario,
            context_tokens: 1000,
            components: [],
            upstream: {
              estimated: true,
              price_source: estimateSource,
              currency: 'USD',
              base_amount: 6,
              group_ratio: null,
              fx_rate: 1,
              factor: 1,
              fee_rate: 0,
              fee_bearer: 'self',
              fee_multiplier: 1,
              amount: null,
            },
          }}
        />
      </I18nextProvider>
    )
    const result = screen.getByRole('region', { name: 'Scenario result' })
    expect(result).toHaveTextContent('Estimated procurement cost—')
    expect(screen.getByRole('status')).toHaveTextContent(
      'Upstream group ratio is unknown'
    )
    await act(() => translations.changeLanguage('zhCN'))
    expect(result).toHaveTextContent('估算采购成本—')
    expect(within(result).getByText('估算利润')).toBeVisible()
    expect(screen.getByRole('status')).toHaveTextContent(
      zh.translation['Upstream group ratio is unknown']
    )
    expect(screen.getByText('目录版本')).toBeVisible()
    expect(screen.getByText('official-2026-11')).toBeVisible()
    expect(screen.getByText('核验日期')).toBeVisible()
    expect(screen.getByText('2026-11-24')).toBeVisible()
  })

  test('stored profit reasons change language in the table and breakdown while model, group and unknown diagnostic text stay exact', async () => {
    const translations = createInstance()
    await translations.init({
      lng: 'en',
      fallbackLng: 'en',
      nsSeparator: false,
      keySeparator: false,
      interpolation: { escapeValue: false },
      resources: { en, zhCN: zh },
    })
    const row: MonitorProfitRow = {
      ...baseRow,
      local_group: 'Saved successfully',
      local_model: '<img src=x onerror=alert(1)>',
      reason: 'Upstream billing rule or token unit is unsupported',
      status: 'pending',
      cost: null,
      profit: null,
      margin: null,
      tier_samples: [
        {
          input_tokens: 1000,
          context_tokens: 1000,
          upstream_tier: 'base',
          revenue: null,
          cost: null,
          profit: null,
          margin: null,
          status: 'pending',
          reason: 'Saved successfully',
        },
      ],
    }
    const state = stateWithRows([row])
    render(
      <I18nextProvider i18n={translations}>
        <ProfitPanel state={state} onSnapshot={vi.fn()} />
        <ProfitDetailBreakdown
          detail={{
            row,
            scenario: state.config.scenario,
            context_tokens: 1000,
            components: [],
            tier_samples: row.tier_samples,
          }}
        />
      </I18nextProvider>
    )
    expect(screen.getAllByText(row.reason)).toHaveLength(2)
    await act(async () => {
      await translations.changeLanguage('zhCN')
    })
    expect(
      screen.getAllByText(
        zh.translation[row.reason as keyof typeof zh.translation]
      )
    ).toHaveLength(2)
    expect(
      screen.getByRole('row', { name: /<img src=x onerror=alert\(1\)>/ })
    ).toHaveTextContent('Saved successfully')
    expect(screen.getAllByText('Saved successfully').length).toBeGreaterThan(0)
    expect(screen.queryByText('保存成功')).not.toBeInTheDocument()
    expect(document.querySelector('img')).toBeNull()
    await act(async () => {
      await translations.changeLanguage('en')
    })
    expect(screen.getAllByText(row.reason)).toHaveLength(2)
  })
  test('summary headings contain the units while values retain every significant digit', () => {
    render(
      <ProfitPanel
        state={stateWithRows([
          { ...baseRow, revenue: 0.085, cost: 0.034, profit: 0.051 },
        ])}
        onSnapshot={vi.fn()}
      />
    )
    const summary = within(
      screen.getByRole('region', { name: 'Scenario summary' })
    )
    const revenue = summary.getByText('Scenario net revenue').parentElement
    expect(revenue).toHaveTextContent('Scenario net revenue (USD)')
    expect(revenue?.nextElementSibling).toHaveTextContent(/^0.085$/)
    const margin = summary.getByText('Estimated gross margin').parentElement
    expect(margin).toHaveTextContent('Estimated gross margin (%)')
    expect(margin?.nextElementSibling?.textContent).not.toContain('%')
    const columns = screen.getByTestId('profit-columns')
    expect(columns).toHaveClass('lg:flex-1', 'min-h-0')
    expect(screen.getByTestId('profit-model-scroll')).toHaveClass(
      'min-h-0',
      'overflow-y-auto'
    )
  })

  test('the desktop comparison stays within its tab and refresh precedes the channel bindings drawer', async () => {
    const user = userEvent.setup()
    vi.spyOn(api, 'get').mockResolvedValue({
      data: { success: true, data: stateWithRows([baseRow]) },
    })
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false, gcTime: Infinity } },
    })
    const view = render(
      <QueryClientProvider client={client}>
        <UpstreamMonitor />
      </QueryClientProvider>
    )
    try {
      const models = await screen.findByRole('region', {
        name: 'Models and estimated profit',
      })
      const tab = screen.getByRole('tabpanel', { name: 'Profit comparison' })
      expect(tab).toHaveClass('lg:overflow-hidden')
      expect(models).toHaveClass('min-h-0', 'flex-col')
      expect(
        screen.queryByRole('heading', { name: 'Channel cost bindings' })
      ).not.toBeInTheDocument()
      const refresh = screen.getByRole('button', { name: 'Refresh' })
      const bindings = screen.getByRole('button', {
        name: 'Channel cost bindings',
      })
      expect(
        refresh.compareDocumentPosition(bindings) &
          Node.DOCUMENT_POSITION_FOLLOWING
      ).toBeTruthy()
      expect(bindings).toHaveAttribute('aria-expanded', 'false')
      await user.click(bindings)
      const drawer = await screen.findByRole('dialog', {
        name: 'Channel cost bindings',
      })
      expect(drawer).toHaveAttribute('data-side', 'right')
      expect(bindings).toHaveAttribute('aria-expanded', 'true')
      expect(
        within(drawer).getByRole('button', { name: 'Add binding' })
      ).toBeVisible()
      await user.keyboard('{Escape}')
      await waitFor(() => expect(drawer).not.toBeInTheDocument())
      expect(bindings).toHaveAttribute('aria-expanded', 'false')
      await waitFor(() => expect(bindings).toHaveFocus())
      expect(models).toBeVisible()
    } finally {
      view.unmount()
      client.clear()
    }
  })

  test('details show the matched tier, unit prices, and the same trial values in JSON', async () => {
    const user = userEvent.setup()
    const row = {
      ...baseRow,
      binding_id: 'binding-a',
      snapshot_id: 'captured-a',
    }
    const scenario = stateWithRows([row]).config.scenario
    const rates = {
      input: 2,
      output: null,
      cache_read: null,
      cache_write: null,
    }
    const detail: MonitorProfitDetail = {
      row,
      scenario,
      context_tokens: 1000,
      upstream: {
        currency: 'USD',
        unit: 'token',
        token_unit: 1000,
        tier: 'base',
        tier_mode: 'whole_request',
        tier_basis: 'context',
        base_prices: { ...rates, input_threshold: null },
        tiers: [{ ...rates, input: 3, input_threshold: 1000 }],
        base_amount: 2,
        amount: 2,
        group_ratio: 1,
        fx_rate: 1,
        factor: 1,
        fee_rate: 0,
        fee_bearer: 'self',
        fee_multiplier: 1,
      },
      components: [
        {
          kind: 'cache_read',
          quantity: 0,
          local_unit_price: 0,
          upstream_unit_price: null,
          unit_profit: null,
          unit_margin: null,
          revenue: 0,
          cost: null,
        },
      ],
      tier_samples: [
        {
          input_tokens: 1000,
          context_tokens: 1000,
          local_tier: 'standard',
          upstream_tier: 'base',
          revenue: 10,
          cost: 6,
          profit: 4,
          margin: 0.4,
          status: 'ready',
          reason: '',
          components: [
            {
              kind: 'input',
              quantity: 1000,
              local_unit_price: 10,
              upstream_unit_price: 6,
              unit_profit: 4,
              unit_margin: 0.4,
              revenue: 0.01,
              cost: 0.006,
            },
          ],
        },
        {
          input_tokens: 1001,
          context_tokens: 1001,
          local_tier: 'long_context',
          upstream_tier: 'context > 1000',
          revenue: 10,
          cost: 11,
          profit: -1,
          margin: -0.1,
          status: 'ready',
          reason: '',
          components: [
            {
              kind: 'input',
              quantity: 1001,
              local_unit_price: 10,
              upstream_unit_price: 11,
              unit_profit: -1,
              unit_margin: -0.1,
              revenue: 0.01001,
              cost: 0.011011,
            },
          ],
        },
      ],
      per_million_context: {
        revenue: 10000,
        cost: 6000,
        profit: 4000,
        margin: 0.4,
      },
    }
    const trial: MonitorProfitDetail = {
      ...detail,
      upstream: detail.upstream && {
        ...detail.upstream,
        tier: 'context > 1000',
      },
      scenario: { ...scenario, input_tokens: 1001 },
      context_tokens: 1001,
    }
    const get = vi.spyOn(api, 'get').mockImplementation(async (_, config) => {
      const params = config?.params
      const isTrial =
        params !== null &&
        typeof params === 'object' &&
        'input_tokens' in params &&
        params.input_tokens === 1001
      return { data: { success: true, data: isTrial ? trial : detail } }
    })
    renderProfitDetails(row)
    const route = within(screen.getByRole('row', { name: /model-a/ }))
    expect(route.getByRole('button', { name: 'View snapshot' })).toBeVisible()
    await user.click(route.getByRole('button', { name: 'Profit details' }))

    const dialog = within(
      await screen.findByRole('dialog', { name: 'Profit details' })
    )
    const tiers = within(
      await dialog.findByRole('table', { name: 'Captured price tiers' })
    )
    expect(tiers.getByRole('row', { name: /Base tier/ })).toHaveTextContent(
      'Matched'
    )
    expect(dialog.getByText('Per 1,000 tokens')).toBeVisible()
    const samples = within(
      dialog.getByRole('table', { name: 'Profit at tier boundaries' })
    )
    expect(
      samples.getByRole('row', { name: /standard.*Base tier/ })
    ).toHaveTextContent('40%')
    expect(
      samples.getByRole('row', { name: /long_context/ })
    ).toHaveTextContent('-10%')
    const standardPrices = within(
      dialog.getByRole('table', { name: /1,000.*standard.*Base tier/ })
    )
    const longPrices = within(
      dialog.getByRole('table', { name: /1,001.*long_context/ })
    )
    expect(
      standardPrices.getByRole('row', { name: /^Uncached input tokens/ })
    ).toHaveTextContent('6 USD')
    expect(
      longPrices.getByRole('row', { name: /^Uncached input tokens/ })
    ).toHaveTextContent('-10%')
    const priceCells = dialog.getAllByRole('row', {
      name: /^Cache read tokens/,
    })[0]
    expect(within(priceCells).getByText('0 USD')).toBeVisible()
    expect(within(priceCells).getAllByText('—')).toHaveLength(3)
    expect(
      dialog.getByRole('region', { name: 'Per 1M input-context tokens' })
    ).toHaveTextContent('40%')

    const input = dialog.getByRole('spinbutton', {
      name: 'Uncached input tokens',
    })
    await user.clear(input)
    expect(dialog.getByRole('button', { name: 'Apply' })).toBeDisabled()
    await user.type(input, '1001')
    await user.click(dialog.getByRole('button', { name: 'Apply' }))
    expect(
      await dialog.findByRole('row', { name: /> 1,000 Matched/ })
    ).toBeVisible()
    expect(get).toHaveBeenCalledWith('/api/upstream_monitor/profit/detail', {
      params: {
        binding_id: 'binding-a',
        local_group: 'standard',
        local_model: 'model-a',
        input_tokens: 1001,
      },
    })
    const json = dialog.getByRole('tab', { name: 'JSON' })
    json.focus()
    await user.keyboard('{Enter}')
    expect(json).toHaveAttribute('aria-selected', 'true')
    expect(dialog.getByRole('tabpanel', { name: 'JSON' })).toHaveTextContent(
      '"input_tokens": 1001'
    )
    await user.click(dialog.getByRole('button', { name: 'Reset' }))
    await waitFor(() =>
      expect(dialog.getByRole('tabpanel', { name: 'JSON' })).toHaveTextContent(
        '"input_tokens": 1000'
      )
    )
    expect(
      dialog.getByRole('spinbutton', { name: 'Uncached input tokens' })
    ).toHaveValue(1000)
  })

  test('details widen the dialog and reserve more desktop space for the sale tier table', async () => {
    const user = userEvent.setup()
    const row = { ...baseRow, binding_id: 'binding-a' }
    vi.spyOn(api, 'get').mockResolvedValue({
      data: {
        success: true,
        data: {
          row,
          scenario: stateWithRows([row]).config.scenario,
          context_tokens: 1000,
          components: [],
          local: {
            currency: 'USD',
            unit: 'token',
            tier: 'long_context',
            expression:
              'len <= 272000 ? tier("standard", p * 5 + c * 10 + cr * 0.2 + cc * 2.5 + cc12h * 7) : tier("long_context", p * 4 + c * 15 + cr * 0.4 + cc * 5 + cc12h * 14)',
            base_amount: 4.015,
            amount: 4.015,
            group_ratio: 1,
            fx_rate: 1,
            factor: 1,
            fee_rate: 0,
            fee_bearer: 'other',
            fee_multiplier: 1,
          },
        } satisfies MonitorProfitDetail,
      },
    })
    renderProfitDetails(row)
    await user.click(screen.getByRole('button', { name: 'Profit details' }))
    const dialog = await screen.findByRole('dialog')
    const tiers = await within(dialog).findByRole('table', {
      name: 'Expression price tiers',
    })
    expect(within(tiers).getAllByRole('columnheader')).toHaveLength(7)
    expect(
      within(tiers).getByRole('columnheader', { name: 'Cache Write (30m)' })
    ).toBeVisible()
    const standard = within(
      within(tiers).getByRole('row', { name: /^standard/ })
    )
    expect(standard.getByText('2.5 USD')).toBeVisible()
    expect(standard.getByText('Cache Write (12h): 7 USD')).toBeVisible()
    expect(dialog).toHaveClass('w-[calc(100%-2rem)]', 'sm:max-w-7xl')
    const sale = within(dialog)
      .getByRole('heading', { name: 'Local sale calculation' })
      .closest('section')
    expect(sale?.parentElement).toHaveClass('xl:grid-cols-[3fr_2fr]')
    expect(sale?.parentElement).not.toHaveClass('lg:grid-cols-2')
  })

  test('bound routes with no confirmed snapshot can display the pending reason', async () => {
    const user = userEvent.setup()
    const row: MonitorProfitRow = {
      ...baseRow,
      binding_id: 'binding-a',
      status: 'pending',
      cost: null,
      profit: null,
      margin: null,
      reason: 'Unconfirmed price',
    }
    const state = stateWithRows([row])
    vi.spyOn(api, 'get').mockResolvedValue({
      data: {
        success: true,
        data: {
          row,
          scenario: state.config.scenario,
          context_tokens: 1000,
          components: [],
        } satisfies MonitorProfitDetail,
      },
    })
    renderProfitDetails(row)
    expect(
      screen.queryByRole('button', { name: 'View snapshot' })
    ).not.toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Profit details' }))
    const dialog = within(await screen.findByRole('dialog'))
    expect(await dialog.findByText(row.reason)).toHaveAttribute(
      'role',
      'status'
    )
    expect(
      dialog.getByRole('region', { name: 'Scenario result' })
    ).toHaveTextContent('Scenario purchase cost—')
  })

  test('a failed detail request preserves the server reason and can be retried', async () => {
    const user = userEvent.setup()
    const row = { ...baseRow, binding_id: 'binding-a' }
    vi.spyOn(api, 'get')
      .mockResolvedValueOnce({
        data: {
          success: false,
          message: 'Procurement route is no longer available',
        },
      })
      .mockResolvedValue({
        data: {
          success: true,
          data: {
            row,
            scenario: stateWithRows([row]).config.scenario,
            context_tokens: 1000,
            components: [],
          } satisfies MonitorProfitDetail,
        },
      })
    renderProfitDetails(row)
    await user.click(screen.getByRole('button', { name: 'Profit details' }))
    const dialog = within(await screen.findByRole('dialog'))
    expect(
      await dialog.findByText('Procurement route is no longer available')
    ).toBeVisible()
    await user.click(dialog.getByRole('button', { name: 'Retry' }))
    expect(
      await dialog.findByRole('region', { name: 'Scenario result' })
    ).toBeVisible()
  })

  test('margin ordering keeps unknown values last in both directions without mutating source rows', () => {
    const rows = [
      { ...baseRow, margin: null },
      { ...baseRow, margin: -0.2 },
      baseRow,
    ]
    expect(
      sortProfitRows(rows, 'margin_desc').map((row) => row.margin)
    ).toEqual([0.4, -0.2, null])
    expect(sortProfitRows(rows, 'margin_asc').map((row) => row.margin)).toEqual(
      [-0.2, 0.4, null]
    )
    expect(rows[0].margin).toBeNull()
  })
  test('weights margins by revenue and excludes missing costs without converting them to zero', () => {
    const summary = summarizeProfit([
      baseRow,
      { ...baseRow, revenue: 90, cost: 81, profit: 9, margin: 0.1 },
      { ...baseRow, revenue: 100, cost: null, profit: null, margin: null },
    ])
    expect(summary).toMatchObject({
      revenue: 100,
      cost: 87,
      profit: 13,
      margin: 0.13,
      coverage: 2 / 3,
      knownCount: 2,
      totalCount: 3,
    })
  })
  test('free or unavailable revenue has no defined margin and zero cost remains valid', () => {
    expect(
      summarizeProfit([
        { ...baseRow, revenue: 0, cost: 0, profit: 0, margin: null },
      ])
    ).toMatchObject({ revenue: 0, cost: 0, margin: null, coverage: 1 })
    expect(summarizeProfit([])).toMatchObject({
      revenue: null,
      cost: null,
      margin: null,
      coverage: null,
    })
  })
  test('a changed local group falls back to one of its own upstream accounts', () => {
    const second = {
      ...baseRow,
      local_group: 'premium',
      account_id: 'account-b',
    }
    expect(
      resolveProfitSelection(
        [baseRow, second],
        'premium',
        upstreamSelectionKey(baseRow)
      )
    ).toEqual({
      localGroup: 'premium',
      upstreamKey: upstreamSelectionKey(second),
    })
    expect(resolveProfitSelection([], 'removed', 'removed')).toEqual({
      localGroup: '',
      upstreamKey: '',
    })
  })
  test('switching local groups updates selected upstream and models with keyboard access', async () => {
    const user = userEvent.setup()
    const second = {
      ...baseRow,
      local_group: 'premium',
      account_id: 'account-b',
      site_name: 'Site B',
      local_model: 'model-b',
    }
    render(
      <ProfitPanel
        state={stateWithRows([baseRow, second])}
        onSnapshot={vi.fn()}
      />
    )
    expect(screen.getByText('model-a')).toBeInTheDocument()
    const local = screen.getByRole('region', { name: 'Local groups' })
    const premium = within(local).getByRole('button', { name: /premium/ })
    premium.focus()
    await user.keyboard('{Enter}')
    expect(premium).toHaveAttribute('aria-pressed', 'true')
    expect(screen.getByText('model-b')).toBeInTheDocument()
    expect(screen.queryByText('model-a')).not.toBeInTheDocument()
    const upstream = screen.getByRole('region', { name: 'Upstream groups' })
    expect(
      within(upstream).getByRole('button', { pressed: true })
    ).toHaveTextContent('Site B')
    expect(screen.getByTestId('profit-columns')).toHaveClass(
      'lg:grid-cols-[20fr_20fr_60fr]'
    )
  })
  test('profit list omits repeated account names, channel names and the comparable badge', () => {
    const state = stateWithRows([baseRow])
    state.config.accounts = [
      {
        id: 'account-a',
        site_id: 'site-a',
        name: 'Site A',
        username: 'a',
        external_id: '',
        has_password: false,
        device_id: '',
        flow_id: '',
        interval_minutes: 0,
        enabled: true,
        purchase_factor: 1,
        fee_rate: 0,
        fee_bearer: 'other',
      },
    ]
    const view = render(<ProfitPanel state={state} onSnapshot={vi.fn()} />)
    const upstream = within(
      screen.getByRole('region', { name: 'Upstream groups' })
    )
    expect(
      upstream
        .getByRole('button', { pressed: true })
        .textContent?.match(/Site A/g)
    ).toHaveLength(1)
    const groupButton = within(upstream.getByRole('button', { pressed: true }))
    expect(groupButton.getByText('base')).toBeVisible()
    expect(groupButton.getByText('Site A · Upstream ratio 0.5')).toBeVisible()
    const route = within(screen.getByRole('row', { name: /model-a/ }))
    expect(route.queryByText('Channel A')).not.toBeInTheDocument()
    expect(route.queryByText('Comparable')).not.toBeInTheDocument()
    state.config.accounts = state.config.accounts.map((account) => ({
      ...account,
      name: 'Distinct account',
    }))
    view.rerender(<ProfitPanel state={state} onSnapshot={vi.fn()} />)
    expect(upstream.getByRole('button', { pressed: true })).toHaveTextContent(
      'Distinct account'
    )
  })

  test('group ratios and small profit amounts keep their precision when the language changes', async () => {
    const row = {
      ...baseRow,
      local_ratio: 0.085,
      upstream_ratio: 0.1234567890123456,
      revenue: 0.341275001,
      cost: 0.341275,
      profit: 0.000000001,
      margin: 0.00000000293,
    }
    if (!i18next.hasResourceBundle('fr', 'translation')) {
      i18next.addResourceBundle('fr', 'translation', {})
    }
    render(<ProfitPanel state={stateWithRows([row])} onSnapshot={vi.fn()} />)
    try {
      expect(screen.getByText('Ratio 0.085 · 1 routes')).toBeVisible()
      expect(
        screen.getByText('Site A · Upstream ratio 0.1234567890123456')
      ).toBeVisible()
      const route = within(screen.getByRole('row', { name: /model-a/ }))
      const summary = within(
        screen.getByRole('region', { name: 'Scenario summary' })
      )
      expect(summary.getByText('0.341275001')).toBeVisible()
      expect(summary.getByText('0.000000001')).toBeVisible()
      expect(route.getByText('0.000000293%')).toBeVisible()

      await act(() => i18next.changeLanguage('fr'))
      expect(screen.getByText('Ratio 0,085 · 1 routes')).toBeVisible()
      expect(
        screen.getByText('Site A · Upstream ratio 0,1234567890123456')
      ).toBeVisible()
      expect(summary.getByText('0,000000001')).toBeVisible()
    } finally {
      await act(() => i18next.changeLanguage('en'))
    }
  })

  test('the calculation details retain the full ratio and the nonzero digits of small amounts', () => {
    const row = {
      ...baseRow,
      local_ratio: 0.1234567890123456,
      revenue: 8.5e-14,
    }
    const view = render(
      <ProfitDetailBreakdown
        detail={{
          row,
          scenario: stateWithRows([row]).config.scenario,
          context_tokens: 1000,
          local: {
            currency: 'USD',
            unit: 'token',
            base_amount: 6.885e-13,
            group_ratio: row.local_ratio,
            fx_rate: 1,
            factor: 1,
            fee_rate: 0.0029999999999999996,
            fee_bearer: 'other',
            fee_multiplier: 1,
            amount: row.revenue,
          },
          components: [],
        }}
      />
    )
    expect(
      within(screen.getByRole('region', { name: 'Scenario result' })).getByText(
        '0.000000000000085 USD'
      )
    ).toBeVisible()
    expect(view.getByText('0.1234567890123456')).toBeVisible()
    expect(view.getByText('0.29999999999999996%')).toBeVisible()
  })

  test('a tiered comparison labels both sides of the margin comparison', () => {
    render(
      <ProfitPanel
        state={stateWithRows([
          {
            ...baseRow,
            local_tier: 'long<context> & cache',
            upstream_tier: 'context > 272000',
          },
          { ...baseRow, channel_id: 2, local_model: 'flat-model' },
        ])}
        onSnapshot={vi.fn()}
      />
    )
    const route = screen.getByRole('row', { name: /model-a.*source-a/ })
    expect(route).toHaveTextContent('long<context> & cache / Context > 272,000')
    expect(route).toHaveTextContent('40%')
    expect(
      screen.getByRole('row', { name: /flat-model/ })
    ).not.toHaveTextContent('Context > 272,000')
  })
  test('the model list shows all tier margins in one row and includes losses in other tiers', async () => {
    const user = userEvent.setup()
    const row = {
      ...baseRow,
      snapshot_id: 'captured-a',
      binding_id: 'binding-a',
      tier_samples: [
        {
          input_tokens: 500,
          context_tokens: 500,
          local_tier: 'short',
          upstream_tier: 'base',
          revenue: 2,
          cost: 3,
          profit: -1,
          margin: -0.5,
          status: 'ready',
          reason: '',
        },
        {
          input_tokens: 1000,
          context_tokens: 1000,
          local_tier: 'long',
          upstream_tier: 'input > 500',
          revenue: 10,
          cost: 6,
          profit: 4,
          margin: 0.4,
          status: 'ready',
          reason: '',
        },
      ],
    }
    render(<ProfitPanel state={stateWithRows([row])} onSnapshot={vi.fn()} />)
    const route = screen.getByRole('row', { name: /model-a/ })
    expect(screen.getAllByRole('row', { name: /model-a|source-a/ })).toEqual([
      route,
    ])
    expect(route).toHaveTextContent('short · -50%')
    expect(route).toHaveTextContent('long · 40%')
    expect(route).not.toHaveTextContent('USD')
    expect(
      screen.getAllByRole('button', { name: 'Profit details' })
    ).toHaveLength(1)
    expect(
      screen.getByRole('region', { name: 'Scenario summary' })
    ).toHaveTextContent('Estimated gross profit (USD)4')
    expect(
      screen.getByRole('region', { name: 'Scenario summary' })
    ).toHaveTextContent('Loss-making routes1')
    await user.click(screen.getByRole('switch', { name: 'Losses only' }))
    expect(route).toBeVisible()
  })

  test('model search and sorting share one row while the switches can wrap on narrow screens', () => {
    render(
      <ProfitPanel state={stateWithRows([baseRow])} onSnapshot={vi.fn()} />
    )
    const models = within(
      screen.getByRole('region', { name: 'Models and estimated profit' })
    )
    const search = models.getByRole('textbox', {
      name: 'Filter models or channels...',
    })
    const sort = models.getByRole('combobox', {
      name: 'Sort model comparisons',
    })
    const row = search.parentElement
    expect(row).toBe(sort.parentElement)
    expect(row).toHaveClass('grid', 'grid-cols-2', 'min-w-0', 'basis-full')
    expect(row?.parentElement).toHaveClass('flex-wrap')
    expect(models.getByRole('switch', { name: 'Losses only' })).toBeVisible()
    expect(
      models.getByRole('switch', { name: 'All collected upstream models' })
    ).toBeVisible()
  })

  test('evidence puts the timestamp above both action buttons and keeps the buttons on one line', () => {
    render(
      <ProfitPanel
        state={stateWithRows([
          {
            ...baseRow,
            snapshot_id: 'snapshot-a',
            binding_id: 'binding-a',
            captured_at: 1_795_325_169,
          },
        ])}
        onSnapshot={vi.fn()}
      />
    )
    const route = within(screen.getByRole('row', { name: /model-a/ }))
    const snapshot = route.getByRole('button', { name: 'View snapshot' })
    const details = route.getByRole('button', { name: 'Profit details' })
    const actions = snapshot.parentElement
    expect(actions).toBe(details.parentElement)
    expect(actions).toHaveClass('flex', 'flex-nowrap')
    expect(actions).not.toHaveClass('flex-wrap')
    expect(actions?.previousElementSibling).toHaveTextContent(
      /\d{4}-\d{2}-\d{2}/
    )
  })

  test('missing cost displays its reason and loss filtering excludes unknown routes', async () => {
    const user = userEvent.setup()
    render(
      <ProfitPanel
        state={stateWithRows([
          {
            ...baseRow,
            cost: null,
            profit: null,
            margin: null,
            reason: 'Unconfirmed price',
            tier_samples: [
              {
                input_tokens: 1_000,
                context_tokens: 1_000,
                upstream_tier: 'base',
                revenue: null,
                cost: null,
                profit: null,
                margin: null,
                status: 'pending',
                reason: 'Unconfirmed price',
              },
            ],
          },
          {
            ...baseRow,
            channel_id: 2,
            local_model: 'loss-model',
            cost: 12,
            profit: -2,
            margin: -0.2,
          },
        ])}
        onSnapshot={vi.fn()}
      />
    )
    const reason = screen.getByText('Unconfirmed price')
    expect(reason).toBeInTheDocument()
    expect(reason).toHaveClass('basis-full')
    expect(reason.previousElementSibling).toHaveTextContent('Base tier')
    await user.click(screen.getByRole('switch', { name: 'Losses only' }))
    expect(screen.queryByText('model-a')).not.toBeInTheDocument()
    expect(screen.getByText('loss-model')).toBeInTheDocument()
  })
  test('upstream sites show precise exchange rates before compact actions and stack with accounts on narrow screens', () => {
    const state = stateWithRows([])
    state.config.sites = [
      {
        id: 'usd',
        name: 'USD provider',
        origin: 'https://usd.example',
        allowed_origins: [],
        currency: 'USD',
        fx_rate: 7.123456789,
      },
      {
        id: 'small',
        name: 'Small rate',
        origin: 'https://small.example',
        allowed_origins: [],
        currency: 'JPY',
        fx_rate: 0.000123456789,
      },
    ]
    render(<SitesPanel state={state} onSave={vi.fn()} onRun={vi.fn()} />)
    const sites = screen.getByRole('region', { name: 'Upstream sites' })
    const accounts = screen.getByRole('region', { name: 'Upstream accounts' })

    expect(sites.nextElementSibling).toBe(accounts)
    expect(sites.parentElement).toHaveClass(
      'grid-cols-1',
      'xl:grid-cols-[minmax(0,2fr)_minmax(0,3fr)]'
    )
    const table = within(sites).getByRole('table')
    expect(table).toBeVisible()
    expect(
      within(table)
        .getAllByRole('columnheader')
        .map((header) => header.textContent)
    ).toEqual(['Site name', 'Currency', 'Site exchange rate', 'Actions'])
    expect(within(table).getByText('7.123456789')).toBeVisible()
    expect(within(table).getByText('0.000123456789')).toBeVisible()
    const actions = within(table).getByRole('columnheader', { name: 'Actions' })
    expect(actions).toHaveClass('w-px')
    for (const button of within(table).getAllByRole('button', {
      name: 'Edit',
    })) {
      expect(button.parentElement).toHaveClass('w-max', 'flex-nowrap')
      expect(button.closest('td')).toHaveClass('w-px')
    }
    expect(within(accounts).getByRole('table')).toBeVisible()
  })

  test('upstream account balances retain captured fractional precision, signs and unknown values', () => {
    const state = stateWithRows([])
    state.config.accounts = [
      {
        id: 'account',
        site_id: 'site',
        name: 'Precision account',
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
    ]
    state.snapshots = [
      {
        id: 'snapshot',
        job_id: 'job',
        account_id: 'account',
        confirmed: true,
        complete: true,
        created_at: 100,
        data: {
          account_id: '42',
          groups: [],
          models: [],
          topups: [],
          warnings: [],
          balances: [
            { kind: 'wallet', amount: 4.000123456, currency: 'USD' },
            { kind: 'available', amount: -0.00022783, currency: 'CNY' },
            { kind: 'tiny', amount: 8.5123456789e-14, currency: 'JPY' },
            { kind: 'zero', amount: 0, currency: 'USD' },
            { kind: 'unknown', amount: null, currency: 'USD' },
          ],
        },
      },
    ]

    render(<SitesPanel state={state} onSave={vi.fn()} onRun={vi.fn()} />)

    expect(screen.getByText('wallet: 4.000123456 USD')).toBeVisible()
    expect(screen.getByText('available: -0.00022783 CNY')).toBeVisible()
    expect(
      screen.getByText('tiny: 0.000000000000085123456789 JPY')
    ).toBeVisible()
    expect(screen.getByText('zero: 0 USD')).toBeVisible()
    expect(screen.getByText('unknown: — USD')).toBeVisible()
  })

  test.each([false, true])(
    'all models with estimates=%s shows labeled unconnected inventory without changing scenario totals',
    async (estimated) => {
      const user = userEvent.setup()
      const state = stateWithRows([
        baseRow,
        {
          ...baseRow,
          estimated,
          price_source: estimated ? estimateSource : undefined,
          channel_id: 2,
        },
      ])
      state.config.accounts = [
        {
          id: 'account-a',
          site_id: 'site-a',
          name: 'Account A',
          username: 'a',
          external_id: '',
          has_password: true,
          device_id: '',
          flow_id: '',
          interval_minutes: 0,
          enabled: true,
          purchase_factor: 1,
          fee_rate: 0,
          fee_bearer: 'self',
        },
      ]
      state.snapshots = [
        {
          id: 'snapshot',
          job_id: 'job',
          account_id: 'account-a',
          confirmed: true,
          created_at: 100,
          complete: true,
          data: {
            account_id: 'a',
            groups: [],
            balances: [{ amount: 60, currency: 'USD', kind: 'wallet' }],
            models: [
              {
                name: 'unconnected-model',
                group: 'base',
                currency: 'USD',
                billing_unit: 'token',
                token_unit: 1e6,
                input: 1,
                output: 2,
                cache_read: null,
                cache_write: null,
                request_price: null,
                includes_group: true,
                notes: '',
              },
            ],
            topups: [],
            warnings: [],
          },
        },
      ]
      state.snapshots[0].data.groups = [
        { name: 'base', display_name: 'Standard plan', ratio: 0.5 },
      ]
      const profit = render(<ProfitPanel state={state} onSnapshot={vi.fn()} />)
      expect(screen.queryByText(/60 USD/)).not.toBeInTheDocument()
      expect(
        screen.getByRole('button', { name: /base \(Standard plan\)/ })
      ).toBeVisible()
      await user.click(
        screen.getByRole('switch', {
          name: 'All collected upstream models',
        })
      )
      const unconnected = screen.getByRole('row', { name: /unconnected-model/ })
      expect(
        within(unconnected).queryByText('Official price estimate')
      ).not.toBeInTheDocument()
      expect(
        screen.getByRole('heading', {
          name: 'Collected models not connected to this local group',
        })
      ).toBeVisible()
      expect(screen.getByText('unconnected-model')).toBeInTheDocument()
      expect(
        screen.getByText('Not connected; excluded from profit totals')
      ).toBeInTheDocument()
      expect(
        screen.getByRole('region', { name: 'Scenario summary' })
      ).toHaveTextContent('Scenario net revenue (USD)20')
      profit.unmount()
      const confirmed = state.snapshots[0]
      state.snapshots.push({
        ...confirmed,
        id: 'partial-newer',
        confirmed: false,
        created_at: 200,
        data: {
          ...confirmed.data,
          balances: [{ amount: 999, currency: 'USD', kind: 'wallet' }],
        },
      })
      render(<SitesPanel state={state} onSave={vi.fn()} onRun={vi.fn()} />)
      const account = screen
        .getByRole('cell', { name: /^Account A · a$/ })
        .closest('tr')
      if (!account) throw new Error('Missing upstream account row')
      expect(
        within(account).getByRole('cell', { name: /wallet.*60 USD/ })
      ).toBeVisible()
      expect(account).not.toHaveTextContent('999')
    }
  )
})
