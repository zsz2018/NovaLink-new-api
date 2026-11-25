import { act, render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createInstance } from 'i18next'
import { I18nextProvider } from 'react-i18next'
import { describe, expect, test, vi } from 'vitest'

import en from '@/i18n/locales/en.json'
import fr from '@/i18n/locales/fr.json'
import ja from '@/i18n/locales/ja.json'
import ru from '@/i18n/locales/ru.json'
import viLocale from '@/i18n/locales/vi.json'
import zhTW from '@/i18n/locales/zh-TW.json'
import zh from '@/i18n/locales/zh.json'
import { STATIC_I18N_KEYS } from '@/i18n/static-keys'

import { JobsPanel } from '../components/jobs-panel'
import { SnapshotReview } from '../components/snapshot-review'
import { formatMonitorMessage } from '../lib/task-messages'
import type { MonitorSnapshot } from '../types'

const snapshot: MonitorSnapshot = {
  id: 'snapshot',
  job_id: 'job',
  account_id: 'account',
  confirmed: true,
  complete: true,
  created_at: 1,
  data: {
    schema_version: 1,
    account_id: '42',
    balances: [
      { amount: 0, currency: 'USD', kind: 'wallet' },
      { amount: null, currency: 'USD', kind: 'bonus' },
    ],
    groups: [],
    models: [],
    topups: [],
    warnings: [],
    sources: [
      {
        request_id: 'profile',
        url: 'https://provider.example/api/self',
        method: 'GET',
        page_url: 'https://provider.example/account',
        group: '',
        captured_at: 1,
      },
    ],
  },
}

const messageResources = { en, zhCN: zh, zhTW, fr, ja, ru, vi: viLocale }
const endpointWarning =
  'NewAPI endpoint pricing is outside the supported ordinary token/request schema; excluded from procurement.'

const estimateKeys = [
  'Leave blank to bind the captured account identity when saving collection settings in the extension.',
  'Compare channel revenue with upstream procurement costs using captured account data and configured model correspondences.',
  'Configure collection flows in the browser extension and review saved versions here. Flows collect account identity, balances and groups, with optional model prices and top-ups.',
  'Select at most 500 estimate mappings.',
  'Select each upstream model and group only once.',
  'Use exact non-empty model and group names within the supported UTF-8 limits, without surrounding spaces or control characters.',
  'No upstream quotes captured.',
  'Map account identity, balances and groups before collecting',
  'Cost comparison estimates',
  'Save comparison estimates',
  'Estimate missing quotes using official reference prices × upstream ratio',
  'Official reference estimates require a USD upstream site.',
  'A newer official catalog is available. Select its version explicitly to update this policy.',
  'This account policy uses the selected official catalog and captured group ratios to estimate comparison costs.',
  'Match bound upstream models by exact name',
  'Selected estimate mappings',
  'No estimate mappings selected.',
  'Remove estimate mapping for {{model}} in {{group}}',
  'A selected catalog entry is unavailable in this version. Remove or review the mapping.',
  'Review a model correspondence',
  'Upstream model name',
  'Official reference model',
  'Collect upstream groups or add an account binding before selecting estimate mappings.',
  'The group needs a captured cash-billing ratio before estimates can enter comparisons.',
  'A captured quote exists for this model and group. It will not be replaced, including zero or unresolved prices.',
  'Official reference preview',
  'Reference prices may use context tiers. The complete official expression determines comparison costs.',
  'I reviewed the upstream model and official reference match.',
  'Add reviewed model',
  'Select at least one reviewed model for estimation.',
  'Captured quotes available',
  'Some upstream quotes were not captured.',
  'Account, balance and ratios collected; no quotes captured.',
  'Upstream group cost comparison is unsupported',
  'Monitor snapshots must contain captured prices only',
  'Official price estimate',
  'Official price estimates: {{number}}',
  'Official price estimate sources',
  'Official references estimate procurement costs only when upstream quotes are missing. Captured quotes and snapshots stay unchanged. Estimates do not confirm upstream model availability.',
  'Catalog version',
  'Catalog entry',
  'Official model',
  'Verified at',
  'Source URL',
  'Estimated procurement cost',
  'Estimated profit',
  'Estimated margin',
  'Estimated unit purchase price',
  'Estimated unit profit',
  'Estimated unit margin',
  'Estimated purchase calculation',
  'Reference price tiers',
  'Estimated prices already include the group ratio.',
  'Estimated profit = net revenue − estimated procurement cost. Estimated margin = estimated profit ÷ net revenue. Zero revenue has no defined margin.',
  'Totals include official price estimates. Pending comparisons remain excluded.',
  'Invalid monitor official estimate policy',
  'Invalid monitor official estimate price',
  'Monitor official catalog is unavailable',
  'Standard global API text-token estimate. Cache writes: 30m; expression also retains 12h. Tools, media, batch and priority prices excluded.',
  'Standard global first-party text-token estimate. Cache writes: 5m; expression also retains 1h. Tools, fast, batch, regional and media prices excluded.',
] as const

describe('structured collection result', () => {
  test('an incomplete empty-price capture keeps core errors visible without implying successful account collection', () => {
    render(
      <SnapshotReview
        snapshot={{
          ...snapshot,
          complete: false,
          confirmed: false,
          data: {
            ...snapshot.data,
            pricing_status: 'unavailable',
            warnings: ['Account identity is missing'],
          },
        }}
        onClose={vi.fn()}
      />
    )
    expect(screen.getByText('No upstream quotes captured.')).toBeVisible()
    expect(screen.getByText('Account identity is missing')).toBeVisible()
    expect(screen.getByText(/Unavailable for cost comparisons/)).toBeVisible()
    expect(
      screen.queryByText(
        'Account, balance and ratios collected; no quotes captured.'
      )
    ).not.toBeInTheDocument()
  })

  test('a complete empty-price capture reports factual pricing availability in both languages and preserves exact JSON and copy', async () => {
    const user = userEvent.setup()
    const clipboard = vi.spyOn(navigator.clipboard, 'writeText')
    const translations = createInstance()
    await translations.init({
      lng: 'en',
      fallbackLng: 'en',
      keySeparator: false,
      nsSeparator: false,
      interpolation: { escapeValue: false },
      resources: messageResources,
    })
    const factual: MonitorSnapshot = {
      ...snapshot,
      data: {
        ...snapshot.data,
        pricing_status: 'unavailable',
        groups: [
          {
            name: 'standard',
            ratio: 0.5,
            cost_unsupported: 'Subscription quota is not wallet cash',
          },
        ],
      },
    }
    render(
      <I18nextProvider i18n={translations}>
        <SnapshotReview snapshot={factual} onClose={vi.fn()} />
      </I18nextProvider>
    )
    expect(
      screen.getByText(
        'Account, balance and ratios collected; no quotes captured.'
      )
    ).toBeVisible()
    expect(
      within(screen.getByRole('list')).getByText(
        /Subscription quota is not wallet cash/
      )
    ).toBeVisible()
    expect(
      screen.queryByText(/Official price estimate/)
    ).not.toBeInTheDocument()
    const json = JSON.stringify(factual.data, null, 2)
    expect(screen.getByRole('textbox')).toHaveValue(json)
    await user.click(
      screen.getByRole('button', { name: 'Copy structured data' })
    )
    expect(clipboard).toHaveBeenCalledWith(json)
    await act(() => translations.changeLanguage('zhCN'))
    expect(screen.getByText('已采到账户/余额/倍率；未取得报价')).toBeVisible()
    expect(screen.getByRole('textbox')).toHaveValue(json)
  })

  test.each(['available', 'partial', 'unavailable'] as const)(
    'snapshot list shows factual pricing status %s without estimate badges',
    (pricingStatus) => {
      render(
        <JobsPanel
          state={{
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
            channels: [],
            groups: {},
            profit_rows: [],
            key_ready: true,
            snapshots: [
              {
                ...snapshot,
                data: { ...snapshot.data, pricing_status: pricingStatus },
              },
            ],
          }}
          onJob={vi.fn()}
          onSnapshot={vi.fn()}
        />
      )
      const labels = {
        available: 'Captured quotes available',
        partial: 'Some upstream quotes were not captured.',
        unavailable:
          'Account, balance and ratios collected; no quotes captured.',
      }
      expect(screen.getByText(labels[pricingStatus])).toBeVisible()
      expect(
        screen.queryByText(/Official price estimate/)
      ).not.toBeInTheDocument()
    }
  )

  test.each(Object.entries(messageResources))(
    '%s has registered estimate translations with intact placeholders',
    (language, resource) => {
      for (const key of estimateKeys) {
        const value = (resource.translation as Record<string, string>)[key]
        expect(value).toBeDefined()
        if (language !== 'en') expect(value).not.toBe(key)
        expect((value?.match(/\{\{\w+\}\}/g) ?? []).sort()).toEqual(
          (key.match(/\{\{\w+\}\}/g) ?? []).sort()
        )
        expect(STATIC_I18N_KEYS).toContain(key)
      }
    }
  )
  test('historical English warnings follow language changes while JSON and clipboard preserve their exact source', async () => {
    const user = userEvent.setup()
    const clipboard = vi.spyOn(navigator.clipboard, 'writeText')
    const translations = createInstance()
    await translations.init({
      lng: 'zhCN',
      fallbackLng: 'en',
      keySeparator: false,
      nsSeparator: false,
      interpolation: { escapeValue: false },
      resources: messageResources,
    })
    const model =
      '<img src=x onerror=alert(1)>:{{count}}.$t(Saved successfully)'
    const data = {
      ...snapshot.data,
      warnings: [
        endpointWarning,
        endpointWarning,
        `Unsupported billing unit: ${model}`,
        'Conditional pricing pending: 2 model quotes',
        'Saved successfully',
      ],
    }
    render(
      <I18nextProvider i18n={translations}>
        <SnapshotReview snapshot={{ ...snapshot, data }} onClose={vi.fn()} />
      </I18nextProvider>
    )
    const warnings = within(screen.getByRole('list'))
    expect(warnings.getAllByRole('listitem')).toHaveLength(4)
    expect(
      warnings.getByText(
        'NewAPI 端点计价超出支持的普通 Token/按次报价结构，已排除采购比较。'
      )
    ).toBeInTheDocument()
    expect(warnings.getByText(`不支持的计费单位：${model}`)).toBeInTheDocument()
    expect(
      warnings.getByText('条件价格待确认：2 条模型报价')
    ).toBeInTheDocument()
    expect(warnings.getByText('Saved successfully')).toBeInTheDocument()
    expect(screen.queryByRole('img')).not.toBeInTheDocument()
    const json = JSON.stringify(data, null, 2)
    expect(screen.getByRole('textbox')).toHaveValue(json)
    await user.click(
      screen.getByRole('button', {
        name: translations.t('Copy structured data'),
      })
    )
    expect(clipboard).toHaveBeenCalledWith(json)

    await act(async () => {
      await translations.changeLanguage('en')
    })
    expect(warnings.getByText(endpointWarning)).toBeInTheDocument()
    expect(
      warnings.getByText(`Unsupported billing unit: ${model}`)
    ).toBeInTheDocument()
    expect(
      warnings.getByText('Conditional pricing pending: 2 model quotes')
    ).toBeInTheDocument()
    expect(screen.getByRole('textbox')).toHaveValue(json)
  })

  test.each(Object.entries(messageResources))(
    '%s localizes fixed and bounded collection messages and preserves placeholders',
    async (language, resource) => {
      const translations = createInstance()
      await translations.init({
        lng: language,
        fallbackLng: 'en',
        keySeparator: false,
        nsSeparator: false,
        interpolation: { escapeValue: false },
        resources: messageResources,
      })
      const templates = [
        'Unsupported billing unit: {{model}}',
        'Conditional pricing pending: {{number}} model quotes',
        'Upstream display currency {{currency}} has a conversion rate of 1; the native billing expression is preserved.',
        'Step {{path}}{{name}} ({{action}}): {{message}}',
      ] as const
      const fixedMessages = [
        [
          'Monitor snapshots must contain captured prices only',
          'Monitor snapshots must contain captured prices only',
        ],
        [
          'Upstream group cost comparison is unsupported',
          'Upstream group cost comparison is unsupported',
        ],
        [
          'Map account identity, balances and groups before collecting',
          'Map account identity, balances and groups before collecting',
        ],
        [
          'Invalid monitor official estimate policy',
          'Invalid monitor official estimate policy',
        ],
        [
          'Invalid monitor official estimate price',
          'Invalid monitor official estimate price',
        ],
        [
          'Monitor official catalog is unavailable',
          'Monitor official catalog is unavailable',
        ],
        [
          'Standard global API text-token estimate. Cache writes: 30m; expression also retains 12h. Tools, media, batch and priority prices excluded.',
          'Standard global API text-token estimate. Cache writes: 30m; expression also retains 12h. Tools, media, batch and priority prices excluded.',
        ],
        [
          'Standard global first-party text-token estimate. Cache writes: 5m; expression also retains 1h. Tools, fast, batch, regional and media prices excluded.',
          'Standard global first-party text-token estimate. Cache writes: 5m; expression also retains 1h. Tools, fast, batch, regional and media prices excluded.',
        ],
        [endpointWarning, endpointWarning],
        [
          'Captured NewAPI response has duplicate JSON object keys',
          'Captured NewAPI response has duplicate JSON object keys',
        ],
        ['Network request did not finish', 'Network request did not finish'],
        [
          'The target element could not receive focus',
          'The target element could not receive focus',
        ],
        [
          '所列为模型基础报价；搜索、抓取等额外工具费用未包含在此报价中。',
          'The quote is the base model price; additional search, fetch and other tool charges are excluded.',
        ],
        [
          'Sub2API: 按请求计费的上下文或规格档位尚不能表示为单次固定价格',
          'Sub2API: Per-request context or specification tiers cannot be represented as one fixed price per call',
        ],
        [
          'Sub2API: 缓存时长或图片 Token 使用不同单价，不能合并为单一文本及缓存价格',
          'Sub2API: Different cache durations or image tokens have different prices and cannot be combined into one text and cache price',
        ],
        [
          'Sub2API: 上下文档位包含不同缓存时长的价格，不能合并为单一缓存价格',
          'Sub2API: Context tiers have different cache-duration prices and cannot be combined into one cache price',
        ],
        [
          'Conflicting captured NewAPI model quotes; pricing remains pending.',
          'Conflicting captured NewAPI model quotes; pricing remains pending.',
        ],
        [
          'Upstream billing rule or token unit is unsupported',
          'Upstream billing rule or token unit is unsupported',
        ],
      ]
      for (const [source, key] of fixedMessages) {
        expect(formatMonitorMessage(source, translations.t)).toBe(
          translations.t(key)
        )
      }
      for (const key of [
        ...fixedMessages.map(([, key]) => key),
        ...templates,
      ]) {
        const value = (resource.translation as Record<string, string>)[key]
        expect(value).toBeDefined()
        if (language !== 'en') expect(value).not.toBe(key)
        expect(value?.match(/\{\{\w+\}\}/g) ?? []).toEqual(
          key.match(/\{\{\w+\}\}/g) ?? []
        )
      }
      expect(formatMonitorMessage(endpointWarning, translations.t)).toBe(
        translations.t(endpointWarning)
      )
      expect(
        formatMonitorMessage(
          'Unsupported billing unit: gpt-image-2-a',
          translations.t
        )
      ).toBe(translations.t(templates[0], { model: 'gpt-image-2-a' }))
      expect(
        formatMonitorMessage(
          'Conditional pricing pending: 20000 model quotes',
          translations.t
        )
      ).toBe(translations.t(templates[1], { number: '20000' }))
      expect(
        formatMonitorMessage(
          '上游显示币种 CNY 的换算倍率为 1，原始计价表达式保持不变。',
          translations.t
        )
      ).toBe(translations.t(templates[2], { currency: 'CNY' }))
      const clauses = [
        [
          'Sub2API simple 模式不提供此钱包扣款成本',
          'Sub2API simple mode does not provide wallet deduction cost',
        ],
        [
          '订阅 USD 额度属于周期使用权益，不是钱包现金采购成本',
          'Subscription USD quota is a periodic usage entitlement, not a cash wallet purchase cost',
        ],
        ['按图片张数计费', 'Image-count billing'],
        ['视频计费维度', 'Video billing dimension'],
        [
          '当前适配未支持此上游计费单位',
          'This adapter does not support the upstream billing unit',
        ],
        ['订阅高峰时段倍率', 'Subscription peak-hour multiplier'],
        [
          'DeepSeek 内置峰谷规则的价格来源及字段覆盖状态未在响应中提供',
          'DeepSeek built-in peak/off-peak price source and field override state are absent from the response',
        ],
        [
          '按时区及时间段变化的模型倍率',
          'Model multiplier varies by time zone and time period',
        ],
        [
          '按推理强度变化的模型倍率',
          'Model multiplier varies by reasoning effort',
        ],
      ]
      for (const [source, key] of clauses) {
        const translation = (resource.translation as Record<string, string>)[
          key
        ]
        expect(translation).toBeDefined()
        if (language !== 'en') expect(translation).not.toBe(key)
        expect(formatMonitorMessage(`Sub2API: ${source}`, translations.t)).toBe(
          `Sub2API: ${translation}`
        )
      }
      expect(
        formatMonitorMessage(
          'Sub2API: 订阅高峰时段倍率；按推理强度变化的模型倍率',
          translations.t
        )
      ).toBe(
        `Sub2API: ${translations.t('Subscription peak-hour multiplier')}; ${translations.t('Model multiplier varies by reasoning effort')}`
      )
      for (const literal of [
        'Saved successfully',
        'Sub2API: 订阅高峰时段倍率；operator note',
        `Sub2API: ${Array(8).fill('订阅高峰时段倍率').join('；')}`,
        '上游显示币种 cny 的换算倍率为 1，原始计价表达式保持不变。',
        '上游显示币种 USD:operator 的换算倍率为 1，原始计价表达式保持不变。',
        `${endpointWarning} operator note`,
        'Unsupported billing unit: ',
        `Unsupported billing unit: ${'a'.repeat(256)}`,
        'Unsupported billing unit: model\n',
        'Conditional pricing pending: 0 model quotes',
        'Conditional pricing pending: 20001 model quotes',
        'Conditional pricing pending: 01 model quotes',
        'Conditional pricing pending: 1 model quotes\n',
      ]) {
        expect(formatMonitorMessage(literal, translations.t)).toBe(literal)
      }
    }
  )
  test('a mapped result displays read-only JSON and source metadata without a confirmation step', async () => {
    const user = userEvent.setup()
    const onClose = vi.fn()
    render(<SnapshotReview snapshot={snapshot} onClose={onClose} />)
    expect(screen.getByText(/Used for cost comparisons/)).toBeInTheDocument()
    const data = screen.getByRole('textbox', {
      name: 'Structured upstream data',
    })
    expect(data).toHaveAttribute('readonly')
    expect(data).toHaveValue(JSON.stringify(snapshot.data, null, 2))
    await user.type(data, 'overwrite')
    expect(data).toHaveValue(JSON.stringify(snapshot.data, null, 2))
    expect(
      screen.getByRole('button', { name: 'Copy structured data' })
    ).toBeEnabled()
    expect(
      screen.queryByRole('button', { name: /Confirm/ })
    ).not.toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Close' }))
    expect(onClose).toHaveBeenCalledOnce()
  })

  test('an incomplete or unconfirmed historical result remains unavailable and its warnings display as text', () => {
    render(
      <SnapshotReview
        snapshot={{
          ...snapshot,
          complete: false,
          confirmed: false,
          data: {
            ...snapshot.data,
            warnings: ['<img src=x onerror=alert(1)>'],
          },
        }}
        onClose={vi.fn()}
      />
    )
    expect(
      screen.getByText(/Unavailable for cost comparisons/)
    ).toBeInTheDocument()
    expect(screen.getByText('<img src=x onerror=alert(1)>')).toBeInTheDocument()
    expect(screen.queryByRole('img')).not.toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: /Confirm/ })
    ).not.toBeInTheDocument()
  })
})
