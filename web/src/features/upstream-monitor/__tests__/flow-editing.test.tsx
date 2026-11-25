import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { toast } from 'sonner'
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'

import { AccountDialog } from '../components/account-dialog'
import { FlowDetails, FlowSteps } from '../components/flow-details'
import { FlowsPanel } from '../components/flows-panel'
import { RunDialog } from '../components/run-dialog'
import { monitorSiteAuthorized } from '../lib/browser-setup'
import { accountSchema } from '../lib/config'
import { monitorFlowObsolete, monitorFlowReady } from '../lib/jobs'
import type {
  MonitorConfig,
  MonitorExtraction,
  MonitorFlow,
  MonitorState,
} from '../types'

const api = vi.hoisted(() => ({
  createMonitorJob: vi.fn(),
  getMonitorBrowserSetup: vi.fn(),
}))
vi.mock('../api', () => ({ ...api, monitorQueryKey: ['upstream-monitor'] }))

const config: MonitorConfig = {
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
      has_password: true,
      device_id: 'browser',
      flow_id: '',
      interval_minutes: 0,
      enabled: true,
      purchase_factor: 1,
      fee_rate: 0,
      fee_bearer: 'self',
    },
  ],
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
const state: MonitorState = {
  config,
  devices: [
    {
      id: 'browser',
      name: 'Browser',
      created_at: 1,
      expires_at: 9999999999,
      last_seen_at: 1,
      revoked_at: 0,
    },
  ],
  jobs: [],
  snapshots: [],
  channels: [],
  groups: {},
  profit_rows: [],
  key_ready: true,
}
const flow: MonitorFlow & { extraction: MonitorExtraction } = {
  id: 'flow',
  site_id: 'site',
  name: 'Login',
  revision: 3,
  capture_seconds: 15,
  extraction: {
    version: 1,
    requests: [
      {
        id: 'profile',
        url: 'https://provider.example/api/self',
        method: 'GET',
        page_urls: ['https://provider.example/dashboard'],
      },
      {
        id: 'prices',
        url: 'https://provider.example/api/prices',
        method: 'GET',
        page_urls: ['https://provider.example/pricing?group=default'],
      },
    ],
    mappings: [
      {
        id: 'identity',
        request_id: 'profile',
        target: 'account_id',
        root: '',
        iterate: 'single',
        fields: { value: { kind: 'path', path: '/data/id' } },
      },
      {
        id: 'balance',
        request_id: 'profile',
        target: 'balances',
        root: '/data',
        iterate: 'single',
        fields: {
          amount: { kind: 'path', path: '/balance' },
          currency: {
            kind: 'path',
            path: '/currency',
            translate: { dollar: 'USD' },
          },
        },
      },
      {
        id: 'groups',
        request_id: 'prices',
        target: 'groups',
        root: '/groups',
        iterate: 'entries',
        fields: { name: { kind: 'key' }, ratio: { kind: 'path', path: '' } },
      },
      {
        id: 'models',
        request_id: 'prices',
        target: 'models',
        root: '/models',
        iterate: 'array',
        fields: {
          name: { kind: 'path', path: '/name' },
          input: { kind: 'path', path: '/input' },
        },
      },
    ],
  },
  steps: [
    { action: 'navigate', url: 'https://provider.example/login' },
    { action: 'focus', selector: '#search' },
  ],
}
const clients: QueryClient[] = []
beforeEach(() => {
  api.getMonitorBrowserSetup.mockResolvedValue({
    browser: { sidebar_open: true, granted_origins: [] },
  })
})
afterEach(() => {
  clients.forEach((client) => client.clear())
  clients.length = 0
  vi.clearAllMocks()
})

function renderTask(mode: 'record' | 'collect', selectedFlow?: MonitorFlow) {
  const client = new QueryClient({
    defaultOptions: { mutations: { retry: false } },
  })
  clients.push(client)
  render(
    <QueryClientProvider client={client}>
      <RunDialog
        account={mode === 'collect' ? config.accounts[0] : undefined}
        mode={mode}
        flow={selectedFlow}
        state={
          selectedFlow
            ? { ...state, config: { ...config, flows: [selectedFlow] } }
            : state
        }
        onClose={vi.fn()}
        onCreated={vi.fn()}
      />
    </QueryClientProvider>
  )
}

describe('browser flow handoff', () => {
  test.each([undefined, 'newapi', 'sub2api'] as const)(
    '%s flow descriptions separate core account data from optional prices and keep setup in the extension',
    (adapter) => {
      const saved = {
        ...flow,
        extraction: {
          ...flow.extraction,
          adapter,
          mappings: adapter
            ? []
            : flow.extraction.mappings.filter(
                (mapping) => mapping.target !== 'models'
              ),
        },
      }
      render(<FlowDetails flow={saved} onClose={vi.fn()} />)
      expect(
        screen.getByText(
          'Configure collection flows in the browser extension and review saved versions here. Flows collect account identity, balances and groups, with optional model prices and top-ups.'
        )
      ).toBeVisible()
      expect(screen.queryByRole('textbox')).not.toBeInTheDocument()
      expect(screen.queryByRole('combobox')).not.toBeInTheDocument()
      expect(monitorFlowReady(saved)).toBe(true)
    }
  )

  test('saved multi-page login shows conditions, exact page bindings and read-only variable references', () => {
    const credentialPage = 'https://provider.example/login/password'
    render(
      <FlowDetails
        flow={{
          ...flow,
          login: {
            url: 'https://provider.example/login',
            timeout_ms: 30000,
            success: { url: 'https://provider.example/account' },
            steps: [
              {
                action: 'input',
                selector: '#login-account',
                variable: 'username',
              },
              {
                action: 'condition',
                condition: { selector: '#terms', state: 'visible' },
                steps: [
                  { action: 'check', selector: '#agree', url: credentialPage },
                ],
                otherwise: [{ action: 'checkpoint', text: 'Review manually' }],
              },
              {
                action: 'input',
                selector: '#login-password',
                variable: 'password',
                url: credentialPage,
              },
              { action: 'click', selector: '#login-submit' },
            ],
          },
        }}
        onClose={vi.fn()}
      />
    )
    const login = within(screen.getByRole('region', { name: 'Login' }))
    expect(login.getByText('#login-account')).toBeVisible()
    expect(login.getByText('#login-password')).toBeVisible()
    expect(login.getByText('username')).toBeVisible()
    expect(login.getByText('password')).toBeVisible()
    expect(
      login.getByRole('heading', { name: 'Login success condition' })
    ).toBeVisible()
    expect(login.getByText('https://provider.example/account')).toBeVisible()
    expect(login.getAllByText(credentialPage)).toHaveLength(2)
    expect(
      within(
        login.getByRole('list', { name: 'When condition matches' })
      ).getByText('#agree')
    ).toBeVisible()
    expect(
      within(login.getByRole('list', { name: 'Otherwise' })).getByText(
        'Review manually'
      )
    ).toBeVisible()
    expect(login.queryByRole('checkbox')).not.toBeInTheDocument()
    expect(login.queryByRole('textbox')).not.toBeInTheDocument()
  })

  test('workflow count includes login and both branches while repeat counts defined steps once', async () => {
    const nested: MonitorFlow = {
      ...flow,
      login: {
        url: 'https://provider.example/login',
        steps: [{ action: 'input', selector: '#email', variable: 'username' }],
      },
      steps: [
        {
          action: 'repeat',
          name: 'Read more',
          max_iterations: 20,
          condition: { selector: '#next', state: 'enabled' },
          steps: [
            {
              action: 'condition',
              condition: { selector: '#ready', state: 'visible' },
              steps: [{ action: 'wait_response', request_id: 'prices' }],
              otherwise: [
                { action: 'assert', selector: '#loading', state: 'hidden' },
              ],
            },
          ],
        },
      ],
    }
    render(
      <FlowsPanel
        state={{ ...state, config: { ...config, flows: [nested] } }}
        onConfigure={vi.fn()}
        onSave={vi.fn()}
      />
    )
    const row = screen.getByRole('row', { name: /Login/ })
    expect(within(row).getByRole('cell', { name: '5' })).toBeVisible()
    await userEvent
      .setup()
      .click(within(row).getByRole('button', { name: 'View flow' }))
    const dialog = within(screen.getByRole('dialog'))
    expect(
      dialog.getByRole('heading', {
        name: /Repeat while condition matches.*Read more/,
      })
    ).toBeVisible()
    expect(dialog.getByText('20')).toBeVisible()
    expect(
      dialog.getByRole('heading', { name: /Wait for selected response/ })
    ).toBeVisible()
    expect(
      dialog.getByRole('heading', { name: /Assert condition/ })
    ).toBeVisible()
  })

  test('nested step metadata wraps long selectors and keeps explicit zero timeout visible', () => {
    const selector = `semantic={"label":"${'模型'.repeat(200)}"}`
    render(
      <FlowSteps
        steps={[
          {
            action: 'condition',
            condition: { selector: '#ready', state: 'visible' },
            steps: [{ action: 'focus', selector, timeout_ms: 0 }],
          },
        ]}
      />
    )
    expect(screen.getByText(selector)).toHaveClass(
      'break-all',
      'whitespace-pre-wrap'
    )
    expect(screen.getByText('0')).toBeVisible()
    expect(
      screen.getByRole('heading', { name: /Step 1.1.*Focus element/ })
    ).toBeVisible()
  })

  test('saved flow details are read only and configuration is handed to the extension', async () => {
    const user = userEvent.setup()
    const onConfigure = vi.fn()
    render(
      <FlowsPanel
        state={{ ...state, config: { ...config, flows: [flow] } }}
        onConfigure={onConfigure}
        onSave={vi.fn()}
      />
    )
    expect(
      screen.queryByRole('button', { name: 'Add collection flow' })
    ).not.toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: 'Edit' })
    ).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Delete' })).toBeEnabled()
    await user.click(screen.getByRole('button', { name: 'View flow' }))
    const dialog = screen.getByRole('dialog')
    expect(
      within(dialog).getByText('GET https://provider.example/api/self')
    ).toBeInTheDocument()
    expect(
      within(dialog).getByText('GET https://provider.example/api/prices')
    ).toBeInTheDocument()
    expect(
      within(dialog).getByText('{"kind":"path","path":"/data/id"}')
    ).toBeInTheDocument()
    expect(within(dialog).getByText('account_id')).toBeInTheDocument()
    expect(
      within(dialog).getByText(
        '{"kind":"path","path":"/currency","translate":{"dollar":"USD"}}'
      )
    ).toBeVisible()
    expect(
      within(dialog).getByText('https://provider.example/dashboard')
    ).toBeInTheDocument()
    expect(
      within(dialog).getByText('https://provider.example/pricing?group=default')
    ).toBeInTheDocument()
    expect(within(dialog).getByText('#search')).toBeInTheDocument()
    expect(within(dialog).queryByRole('textbox')).not.toBeInTheDocument()
    expect(
      within(dialog).queryByRole('button', { name: 'Save flow' })
    ).not.toBeInTheDocument()
    await user.click(within(dialog).getByRole('button', { name: 'Close' }))
    const configure = screen.getAllByRole('button', {
      name: 'Configure in browser extension',
    })
    await user.click(configure[1])
    expect(onConfigure).toHaveBeenCalledWith(flow)
  })

  test.each([
    ['newapi', 'NewAPI'],
    ['sub2api', 'Sub2API'],
  ] as const)(
    '%s saved flows show the collection mode and browser source bindings without generic mapping requirements',
    async (adapter, label) => {
      const dedicated: MonitorFlow = {
        ...flow,
        extraction: { ...flow.extraction, adapter, mappings: [] },
      }
      render(
        <FlowsPanel
          state={{ ...state, config: { ...config, flows: [dedicated] } }}
          onConfigure={vi.fn()}
          onSave={vi.fn()}
        />
      )
      const row = within(screen.getByRole('row', { name: /Login/ }))
      expect(row.getByRole('cell', { name: label })).toBeVisible()
      expect(
        row.queryByText('Collection setup required')
      ).not.toBeInTheDocument()
      await userEvent
        .setup()
        .click(row.getByRole('button', { name: 'View flow' }))

      const dialog = within(screen.getByRole('dialog'))
      expect(dialog.getByText('Collection mode')).toBeVisible()
      expect(dialog.getByText(label)).toBeVisible()
      expect(
        dialog.getByText(
          `The ${label} collector uses only responses captured by the browser from these selected requests and source pages. Collection settings are managed in the extension.`
        )
      ).toBeVisible()
      expect(
        dialog.getByText('GET https://provider.example/api/self')
      ).toBeVisible()
      expect(
        dialog.getByText('https://provider.example/dashboard')
      ).toBeVisible()
      expect(
        dialog.getByText('GET https://provider.example/api/prices')
      ).toBeVisible()
      expect(
        dialog.getByText('https://provider.example/pricing?group=default')
      ).toBeVisible()
      expect(
        dialog.queryByText(
          'Complete collection setup in the extension before collecting.'
        )
      ).not.toBeInTheDocument()
      expect(
        dialog.queryByText('No field mappings configured')
      ).not.toBeInTheDocument()
      expect(
        dialog.queryByRole('heading', { name: 'Field mappings' })
      ).not.toBeInTheDocument()
      expect(dialog.queryByRole('textbox')).not.toBeInTheDocument()
      expect(dialog.queryByRole('combobox')).not.toBeInTheDocument()
    }
  )

  test('ordinary flow details preserve nested rows, parent fields, arithmetic, tiers and native expressions', () => {
    const ordinary = structuredClone(flow)
    const mapping = ordinary.extraction.mappings[3]
    mapping.root = '/groups'
    mapping.iterate = 'nested_array'
    mapping.child_root = '/models'
    mapping.fields.group = { kind: 'parent', path: '/name' }
    mapping.fields.input = {
      kind: 'path',
      path: '/input',
      fallback_path: '/input_price',
      multiply: { kind: 'parent', path: '/ratio' },
      divide: { kind: 'path', source: 'profile', path: '/data/quota_per_unit' },
      scale: 1_000_000,
    }
    mapping.fields.billing_expression = {
      kind: 'path',
      path: '/billing_expression',
    }
    mapping.tiers = {
      path: '/tiers',
      mode: 'whole_request',
      basis: 'context',
      fields: {
        input_threshold: { kind: 'path', path: '/threshold' },
        input: { kind: 'path', path: '/input' },
      },
    }
    render(<FlowDetails flow={ordinary} onClose={vi.fn()} />)

    const dialog = within(screen.getByRole('dialog'))
    expect(dialog.getByText('Ordinary')).toBeVisible()
    const models = within(dialog.getByRole('row', { name: /models.*prices/ }))
    expect(models.getByText('nested_array')).toBeVisible()
    expect(models.getByText('/models')).toBeVisible()
    expect(models.getByText(JSON.stringify(mapping.fields.group))).toBeVisible()
    expect(models.getByText(JSON.stringify(mapping.fields.input))).toBeVisible()
    expect(
      models.getByText(JSON.stringify(mapping.fields.billing_expression))
    ).toBeVisible()
    expect(models.getByText(JSON.stringify(mapping.tiers))).toBeVisible()
    expect(dialog.queryByRole('textbox')).not.toBeInTheDocument()
    expect(dialog.queryByRole('combobox')).not.toBeInTheDocument()
  })

  test('ordinary flow details show saved price-field billing conditions read-only', () => {
    const ordinary = structuredClone(flow)
    const mapping = ordinary.extraction.mappings[3]
    delete mapping.fields.billing_unit
    mapping.billing = {
      rules: [
        {
          match: 'all',
          conditions: [{ path: '/base_image_per_call', op: 'gt', value: 0 }],
          unit: 'request',
          request_price: { kind: 'path', path: '/base_image_per_call' },
        },
      ],
    }
    render(<FlowDetails flow={ordinary} onClose={vi.fn()} />)

    const dialog = within(screen.getByRole('dialog'))
    const models = within(dialog.getByRole('row', { name: /models.*prices/ }))
    expect(models.getByText(JSON.stringify(mapping.billing))).toBeVisible()
    expect(models.getByText('Pricing rules')).toBeVisible()
    expect(dialog.queryByRole('textbox')).not.toBeInTheDocument()
    expect(dialog.queryByRole('combobox')).not.toBeInTheDocument()
    expect(monitorFlowReady(ordinary)).toBe(true)
    expect(monitorFlowObsolete(ordinary)).toBe(false)
  })

  test.each([
    { missing: 'models', ready: true },
    { missing: 'account_id', ready: false },
    { missing: 'balances', ready: false },
    { missing: 'groups', ready: false },
  ])(
    'a factual flow missing $missing mappings is ready=$ready without an estimate policy',
    ({ missing, ready }) => {
      const factual = {
        ...flow,
        extraction: {
          ...flow.extraction,
          mappings: flow.extraction.mappings.filter(
            (mapping) => mapping.target !== missing
          ),
        },
      }
      expect(monitorFlowReady(factual)).toBe(ready)
      if (ready) {
        render(<FlowDetails flow={factual} onClose={vi.fn()} />)
        expect(
          screen.queryByText('Official reference estimate policy')
        ).not.toBeInTheDocument()
        expect(screen.queryByRole('combobox')).not.toBeInTheDocument()
      }
    }
  )

  test.each([
    { adapter: undefined, mappings: flow.extraction.mappings, ready: true },
    { adapter: undefined, mappings: [], ready: false },
    { adapter: 'newapi', mappings: [], ready: true },
    { adapter: 'sub2api', mappings: [], ready: true },
    { adapter: 'newapi', mappings: flow.extraction.mappings, ready: false },
    { adapter: 'sub2api', mappings: flow.extraction.mappings, ready: false },
  ] satisfies {
    adapter: MonitorExtraction['adapter']
    mappings: MonitorExtraction['mappings']
    ready: boolean
  }[])(
    'collection readiness for $adapter with $mappings.length mappings is $ready and always requires selected source pages',
    ({ adapter, mappings, ready }) => {
      const extraction: MonitorExtraction = {
        ...flow.extraction,
        adapter,
        mappings,
      }
      expect(monitorFlowReady({ ...flow, extraction })).toBe(ready)
      expect(
        monitorFlowReady({
          ...flow,
          extraction: { ...extraction, requests: [] },
        })
      ).toBe(false)
      expect(
        monitorFlowReady({
          ...flow,
          extraction: {
            ...extraction,
            requests: [{ ...extraction.requests[0], page_urls: [] }],
          },
        })
      ).toBe(false)
    }
  )

  test.each([
    ['extraction', 'model_coverage', 'all_selected'],
    ['mapping', 'key_path', '/name'],
    ['mapping', 'expand', { path: '/groups' }],
    ['mapping', 'where', { path: '/price', op: 'present' }],
    ['mapping', 'filters', []],
    ['mapping', 'billing_overrides', []],
    ['field', 'values', { tokens: 'token' }],
    ['field', 'kind', 'auto'],
    ['field', 'kind', 'item'],
    ['field', 'multiply', { kind: 'auto' }],
  ])(
    'a saved flow with removed %s.%s rules cannot be selected for collection',
    (level, key, value) => {
      const obsolete = structuredClone(flow)
      const mapping = obsolete.extraction.mappings[3]
      if (level === 'extraction') {
        Object.assign(obsolete.extraction, { [key]: value })
      }
      if (level === 'mapping') Object.assign(mapping, { [key]: value })
      if (level === 'field') {
        Object.assign(mapping.fields.input, { [key]: value })
      }
      expect(monitorFlowReady(obsolete)).toBe(false)
    }
  )

  test('an obsolete saved flow offers reconfiguration in the extension and keeps deletion optional', async () => {
    const obsolete = structuredClone(flow)
    Object.assign(obsolete.extraction.mappings[3].fields.input, {
      kind: 'auto',
    })
    const onSave = vi.fn()
    const onConfigure = vi.fn()
    render(
      <FlowsPanel
        state={{ ...state, config: { ...config, flows: [obsolete] } }}
        onConfigure={onConfigure}
        onSave={onSave}
      />
    )
    const row = within(screen.getByRole('row', { name: /Login/ }))
    expect(
      row.getByText(
        'This flow uses obsolete collection rules and cannot run. Reconfigure it in the extension: clear the data configuration or switch collection mode, then match the data again.'
      )
    ).toBeVisible()
    expect(
      row.getByRole('button', { name: 'Configure in browser extension' })
    ).toBeEnabled()
    expect(row.getByRole('button', { name: 'Delete' })).toBeEnabled()
    expect(onSave).not.toHaveBeenCalled()
    const user = userEvent.setup()
    await user.click(
      row.getByRole('button', { name: 'Configure in browser extension' })
    )
    expect(onConfigure).toHaveBeenCalledWith(obsolete)
    await user.click(row.getByRole('button', { name: 'Delete' }))
    expect(screen.getByRole('alertdialog')).toBeVisible()
    expect(onSave).not.toHaveBeenCalled()
  })

  test('deleting a flow requires confirmation and unlinks only its accounts while disabling their schedules', async () => {
    const user = userEvent.setup()
    const otherFlow = { ...flow, id: 'other-flow', name: 'Other flow' }
    const currentConfig: MonitorConfig = {
      ...config,
      flows: [flow, otherFlow],
      accounts: [
        { ...config.accounts[0], flow_id: flow.id, interval_minutes: 30 },
        {
          ...config.accounts[0],
          id: 'other-account',
          flow_id: otherFlow.id,
          interval_minutes: 60,
        },
      ],
    }
    const onSave = vi.fn().mockResolvedValue(undefined)
    const renderPanel = (value: MonitorConfig) => (
      <FlowsPanel
        state={{ ...state, config: value }}
        onConfigure={vi.fn()}
        onSave={onSave}
      />
    )
    const rendered = render(renderPanel(currentConfig))
    const row = screen.getByRole('row', { name: /Login/ })
    await user.click(within(row).getByRole('button', { name: 'Delete' }))
    const confirmation = screen.getByRole('alertdialog')
    expect(confirmation).toHaveTextContent('Delete collection flow?')
    expect(confirmation).toHaveTextContent(flow.name)
    expect(confirmation).toHaveTextContent('scheduled collection')
    expect(onSave).not.toHaveBeenCalled()
    await user.click(
      within(confirmation).getByRole('button', { name: 'Cancel' })
    )
    expect(onSave).not.toHaveBeenCalled()
    await user.click(within(row).getByRole('button', { name: 'Delete' }))
    await user.click(
      within(screen.getByRole('alertdialog')).getByRole('button', {
        name: 'Delete',
      })
    )
    await waitFor(() => expect(onSave).toHaveBeenCalledOnce())
    expect(onSave).toHaveBeenCalledWith({
      ...currentConfig,
      flows: [otherFlow],
      accounts: [
        { ...currentConfig.accounts[0], flow_id: '', interval_minutes: 0 },
        currentConfig.accounts[1],
      ],
    })
    await waitFor(() =>
      expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument()
    )
    rendered.rerender(renderPanel(onSave.mock.calls[0][0]))
    expect(screen.queryByRole('row', { name: /Login/ })).not.toBeInTheDocument()
    expect(screen.getByRole('row', { name: /Other flow/ })).toBeInTheDocument()
    expect(currentConfig.flows).toEqual([flow, otherFlow])
  })

  test('a failed flow deletion keeps the confirmation available for retry and blocks duplicate submissions while saving', async () => {
    const user = userEvent.setup()
    let rejectSave!: (error: Error) => void
    const saving = new Promise<void>((_resolve, reject) => {
      rejectSave = reject
    })
    const onSave = vi
      .fn()
      .mockReturnValueOnce(saving)
      .mockResolvedValue(undefined)
    const errorToast = vi.spyOn(toast, 'error').mockReturnValue('fixture-toast')
    try {
      render(
        <FlowsPanel
          state={{
            ...state,
            config: { ...config, accounts: [], flows: [flow] },
          }}
          onConfigure={vi.fn()}
          onSave={onSave}
        />
      )
      await user.click(screen.getByRole('button', { name: 'Delete' }))
      const confirmation = screen.getByRole('alertdialog')
      await user.click(
        within(confirmation).getByRole('button', { name: 'Delete' })
      )
      expect(
        within(confirmation).getByRole('button', { name: 'Delete' })
      ).toBeDisabled()
      expect(
        within(confirmation).getByRole('button', { name: 'Cancel' })
      ).toBeDisabled()
      expect(onSave).toHaveBeenCalledOnce()
      await act(async () => {
        rejectSave(new Error('Fixture save failed'))
      })
      expect(errorToast).toHaveBeenCalledOnce()
      expect(confirmation).toBeVisible()
      expect(
        within(confirmation).getByRole('button', { name: 'Delete' })
      ).toBeEnabled()
      await user.click(
        within(confirmation).getByRole('button', { name: 'Delete' })
      )
      await waitFor(() =>
        expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument()
      )
      expect(onSave).toHaveBeenCalledTimes(2)
    } finally {
      errorToast.mockRestore()
    }
  })

  test('first site setup requires an open sidebar and responds to updated browser status', async () => {
    api.getMonitorBrowserSetup.mockResolvedValue({
      browser: { sidebar_open: false, granted_origins: [] },
    })
    api.createMonitorJob.mockResolvedValue({ id: 'configuration' })
    const user = userEvent.setup()
    renderTask('record')
    const start = screen.getByRole('button', {
      name: 'Start configuration task',
    })
    expect(start).toBeDisabled()
    expect(
      await screen.findByText(
        'Open the selected browser’s extension side panel before starting configuration.'
      )
    ).toBeVisible()
    await user.click(start)
    expect(api.createMonitorJob).not.toHaveBeenCalled()
    await act(async () => {
      clients[0].setQueryData(['upstream-monitor', 'browser-setup'], {
        browser: { sidebar_open: true, granted_origins: [] },
      })
    })
    await waitFor(() => expect(start).toBeEnabled())
    expect(
      screen.getByText(
        'Side panel detected. Start the task, then authorize this site in the side panel.'
      )
    ).toBeVisible()
    await user.click(start)
    await waitFor(() => expect(api.createMonitorJob).toHaveBeenCalledOnce())
  })

  test('authorized sites still require the sidebar for every configuration and disable start when it closes', async () => {
    const grantedOrigins = ['https://provider.example/*']
    api.getMonitorBrowserSetup.mockResolvedValue({
      browser: { sidebar_open: false, granted_origins: grantedOrigins },
    })
    const user = userEvent.setup()
    renderTask('record', flow)
    const start = screen.getByRole('button', {
      name: 'Start configuration task',
    })
    expect(
      await screen.findByText(
        'Open the selected browser’s extension side panel before starting configuration.'
      )
    ).toBeVisible()
    expect(start).toBeDisabled()
    await user.click(start)
    expect(api.createMonitorJob).not.toHaveBeenCalled()
    await act(async () => {
      clients[0].setQueryData(['upstream-monitor', 'browser-setup'], {
        browser: { sidebar_open: true, granted_origins: grantedOrigins },
      })
    })
    await waitFor(() => expect(start).toBeEnabled())
    expect(
      screen.getByText(
        'Side panel detected. This site is already authorized; you can start configuration.'
      )
    ).toBeVisible()
    await act(async () => {
      clients[0].setQueryData(['upstream-monitor', 'browser-setup'], {
        browser: { sidebar_open: false, granted_origins: grantedOrigins },
      })
    })
    await waitFor(() => expect(start).toBeDisabled())
    expect(api.createMonitorJob).not.toHaveBeenCalled()
  })

  test('missing browser status keeps first setup disabled without claiming the sidebar is closed', async () => {
    api.getMonitorBrowserSetup.mockResolvedValue({})
    renderTask('record')
    await screen.findByText(
      'Waiting for browser status. Open the extension side panel or update the extension if no status appears.'
    )
    expect(
      screen.getByRole('button', { name: 'Start configuration task' })
    ).toBeDisabled()
    expect(api.createMonitorJob).not.toHaveBeenCalled()
  })

  test('site authorization requires every configured origin and respects Chrome host permissions', () => {
    const site = {
      ...config.sites[0],
      origin: 'https://api.vendor.example:444',
      allowed_origins: ['https://login.vendor.example'],
    }
    for (const patterns of [
      ['https://*.vendor.example/*'],
      ['*://*.vendor.example/*'],
      ['<all_urls>'],
    ]) {
      expect(
        monitorSiteAuthorized(site, {
          sidebar_open: false,
          granted_origins: patterns,
        })
      ).toBe(true)
    }
    for (const patterns of [
      ['https://api.vendor.example:444/*'],
      ['http://*.vendor.example/*'],
      ['https://*.vendor.example.evil/*'],
      ['https://api.vendor.example:443/*', 'https://login.vendor.example/*'],
    ]) {
      expect(
        monitorSiteAuthorized(site, {
          sidebar_open: true,
          granted_origins: patterns,
        })
      ).toBe(false)
    }
  })

  test('a new configuration task can start without an existing flow', async () => {
    const user = userEvent.setup()
    api.createMonitorJob.mockResolvedValue({ id: 'new-task' })
    renderTask('record')
    expect(
      await within(screen.getByRole('dialog')).findByText(
        'Side panel detected. Start the task, then authorize this site in the side panel.'
      )
    ).toBeVisible()
    const selector = screen.getByRole('combobox', { name: 'Collection flow' })
    await user.click(selector)
    await user.click(
      screen.getByRole('option', { name: 'Create a new flow in the extension' })
    )
    expect(selector).toHaveTextContent('Create a new flow in the extension')
    await user.click(selector)
    expect(
      screen.getByRole('option', { name: 'Create a new flow in the extension' })
    ).toHaveAttribute('aria-selected', 'true')
    await user.keyboard('{Escape}')
    await user.click(
      screen.getByRole('button', { name: 'Start configuration task' })
    )
    await waitFor(() =>
      expect(api.createMonitorJob).toHaveBeenCalledWith({
        account_id: 'account',
        device_id: 'browser',
        flow_id: '',
        mode: 'record',
      })
    )
  })

  test('switching from a saved flow to a new flow displays the choice and submits an empty flow ID', async () => {
    const user = userEvent.setup()
    api.createMonitorJob.mockResolvedValue({ id: 'new-task' })
    renderTask('record', flow)
    const selector = screen.getByRole('combobox', { name: 'Collection flow' })
    expect(selector).toHaveTextContent('Login')
    await user.click(selector)
    await user.click(
      screen.getByRole('option', { name: 'Create a new flow in the extension' })
    )
    expect(selector).toHaveTextContent('Create a new flow in the extension')
    await user.click(
      screen.getByRole('button', { name: 'Start configuration task' })
    )
    await waitFor(() =>
      expect(api.createMonitorJob).toHaveBeenCalledWith({
        account_id: 'account',
        device_id: 'browser',
        flow_id: '',
        mode: 'record',
      })
    )
  })

  test('collection cannot start until a saved flow is selected', async () => {
    const user = userEvent.setup()
    renderTask('collect')
    expect(
      screen.getByRole('combobox', { name: 'Collection flow' })
    ).toHaveTextContent('Select an option')
    await user.click(screen.getByRole('button', { name: 'Start task' }))
    expect(
      await screen.findByText('Choose a saved collection flow')
    ).toBeInTheDocument()
    expect(api.createMonitorJob).not.toHaveBeenCalled()
  })

  test.each([undefined, 'newapi', 'sub2api'] as const)(
    'a saved %s flow starts collection without opening the sidebar or asking for per-run confirmation',
    async (adapter) => {
      const user = userEvent.setup()
      api.getMonitorBrowserSetup.mockResolvedValue({})
      api.createMonitorJob.mockResolvedValue({ id: 'collected-task' })
      renderTask(
        'collect',
        adapter
          ? {
              ...flow,
              extraction: { ...flow.extraction, adapter, mappings: [] },
            }
          : flow
      )
      expect(
        screen.getByRole('combobox', { name: 'Collection flow' })
      ).toHaveTextContent('Login')
      await user.click(screen.getByRole('button', { name: 'Start task' }))
      await waitFor(() =>
        expect(api.createMonitorJob).toHaveBeenCalledWith({
          account_id: 'account',
          device_id: 'browser',
          flow_id: 'flow',
          mode: 'collect',
        })
      )
      expect(api.getMonitorBrowserSetup).not.toHaveBeenCalled()
      expect(
        screen.queryByRole('button', { name: /Confirm/ })
      ).not.toBeInTheDocument()
    }
  )

  test('an older flow without field mappings cannot be used for collection', async () => {
    const user = userEvent.setup()
    renderTask('collect', { ...flow, extraction: undefined })
    expect(
      screen.getByRole('combobox', { name: 'Collection flow' })
    ).toHaveTextContent('Select an option')
    expect(
      screen.getByText(
        'Complete collection setup in the extension before collecting.'
      )
    ).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Start task' }))
    expect(
      await screen.findByText('Choose a saved collection flow')
    ).toBeInTheDocument()
    expect(api.createMonitorJob).not.toHaveBeenCalled()
  })

  test('an old scheduled flow requires a mapped replacement before account settings can be saved', async () => {
    const user = userEvent.setup()
    const onSave = vi.fn()
    const account = {
      ...config.accounts[0],
      flow_id: flow.id,
      interval_minutes: 10,
    }
    render(
      <AccountDialog
        account={account}
        state={{
          ...state,
          config: {
            ...config,
            accounts: [account],
            flows: [{ ...flow, extraction: undefined }],
          },
        }}
        onSave={onSave}
        onClose={vi.fn()}
      />
    )
    expect(
      screen.getByRole('combobox', { name: 'Scheduled flow' })
    ).toHaveTextContent('Not selected')
    await user.click(screen.getByRole('button', { name: 'Save' }))
    expect(
      await screen.findByText(
        'Choose a browser and flow before enabling a schedule'
      )
    ).toBeInTheDocument()
    expect(onSave).not.toHaveBeenCalled()
  })

  test('scheduling requires both a browser and flow, while manual accounts may omit them', () => {
    const account = {
      name: 'Account',
      site_id: 'site',
      username: 'user',
      external_id: '',
      password: '',
      clear_password: false,
      device_id: '',
      flow_id: '',
      interval_minutes: 0,
      enabled: true,
      purchase_factor: 1,
      fee_rate: 0,
      fee_bearer: 'self',
    }
    expect(accountSchema.safeParse(account).success).toBe(true)
    expect(
      accountSchema.safeParse({ ...account, interval_minutes: 10 }).success
    ).toBe(false)
  })
})
