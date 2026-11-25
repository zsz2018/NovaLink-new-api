import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, test, vi } from 'vitest'

import { BindingsPanel } from '../components/bindings-panel'
import type { MonitorAccount, MonitorSnapshot, MonitorState } from '../types'

const account: MonitorAccount = {
  id: 'bee',
  site_id: 'site',
  name: 'BeeAPI',
  username: '',
  external_id: '42',
  has_password: false,
  device_id: '',
  flow_id: '',
  interval_minutes: 0,
  enabled: true,
  purchase_factor: 1,
  fee_rate: 0,
  fee_bearer: 'other',
}

function snapshot(
  accountID: string,
  id: string,
  time: number,
  quotes: [string, string][],
  complete = true
): MonitorSnapshot {
  return {
    id,
    job_id: id,
    account_id: accountID,
    created_at: time,
    complete,
    confirmed: complete,
    data: {
      account_id: '42',
      balances: [],
      topups: [],
      warnings: [],
      groups: quotes.map(([name]) => ({
        name,
        display_name: 'Shared display name',
        ratio: 1,
      })),
      models: quotes.map(([group, name]) => ({
        name,
        group,
        currency: 'USD',
        billing_unit: 'token',
        token_unit: 1_000_000,
        input: 1,
        output: 2,
        cache_read: null,
        cache_write: null,
        request_price: null,
        includes_group: false,
        notes: '',
      })),
    },
  }
}

function state(): MonitorState {
  return {
    config: {
      version: 1,
      sites: [],
      accounts: [account, { ...account, id: 'other', name: 'Other upstream' }],
      flows: [],
      bindings: [],
      scenario: {
        currency: 'USD',
        usd_exchange: 1,
        input_tokens: 1,
        output_tokens: 1,
        cache_read: 0,
        cache_write: 0,
        requests: 1,
        revenue_factor: 1,
        fee_rate: 0,
        fee_bearer: 'other',
        user_group: '',
      },
    },
    channels: [
      {
        id: 1,
        name: 'OpenAI',
        group: 'default',
        models: ' local-gpt, local-other, local-gpt ',
      },
      { id: 2, name: 'Other channel', group: 'default', models: 'elsewhere' },
    ],
    snapshots: [
      snapshot('bee', 'older', 1, [['old-code', 'old-model']]),
      snapshot('bee', 'partial', 3, [['partial-code', 'partial-model']], false),
      snapshot('other', 'other-snapshot', 4, [['other-code', 'other-model']]),
      snapshot('bee', 'newest', 2, [
        ['shop-a', 'gpt-upstream'],
        ['shop-b', 'other-quote'],
      ]),
    ],
    devices: [],
    jobs: [],
    groups: {},
    profit_rows: [],
    key_ready: true,
  }
}

describe('cost binding selectors', () => {
  test('saved groups append distinct display names while preserving codes and missing-name fallbacks', () => {
    const input = state()
    const newest = input.snapshots.find((item) => item.id === 'newest')
    if (!newest) throw new Error('Missing confirmed snapshot')
    newest.data.groups.push({
      name: 'same-code',
      display_name: 'same-code',
      ratio: 1,
    })
    input.config.bindings = ['shop-a', 'same-code', 'retired-code'].map(
      (group) => ({
        id: group,
        channel_id: 1,
        account_id: 'bee',
        upstream_group: group,
        local_model: '',
        upstream_model: '',
      })
    )
    render(<BindingsPanel state={input} onSave={vi.fn()} />)
    expect(
      screen.getByRole('cell', { name: 'shop-a (Shared display name)' })
    ).toBeVisible()
    expect(screen.getByRole('cell', { name: 'same-code' })).toBeVisible()
    expect(screen.getByRole('cell', { name: 'retired-code' })).toBeVisible()
  })
  test('searches confirmed groups by code and saves models from the selected channel and group', async () => {
    const user = userEvent.setup()
    const onSave = vi.fn().mockResolvedValue(undefined)
    render(<BindingsPanel state={state()} onSave={onSave} />)
    await user.click(screen.getByRole('button', { name: 'Add binding' }))
    const group = screen.getByRole('combobox', { name: 'Upstream group' })
    await user.click(group)
    expect(screen.getAllByRole('option')).toHaveLength(2)
    await user.type(group, 'shop-a')
    await user.click(
      screen.getByRole('option', { name: 'shop-a (Shared display name)' })
    )
    await user.click(screen.getByRole('combobox', { name: 'Local model' }))
    expect(screen.getAllByRole('option')).toHaveLength(3)
    await user.click(screen.getByRole('option', { name: 'local-gpt' }))
    await user.keyboard('{Escape}')
    await user.click(screen.getByRole('combobox', { name: 'Upstream model' }))
    expect(
      screen.queryByRole('option', { name: 'other-quote' })
    ).not.toBeInTheDocument()
    await user.click(screen.getByRole('option', { name: 'gpt-upstream' }))
    await user.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() =>
      expect(onSave).toHaveBeenCalledWith(
        expect.objectContaining({
          bindings: [
            expect.objectContaining({
              channel_id: 1,
              account_id: 'bee',
              upstream_group: 'shop-a',
              local_model: 'local-gpt',
              upstream_model: 'gpt-upstream',
            }),
          ],
        })
      )
    )
  })

  test('clears incompatible selections when switching account, channel or model scope', async () => {
    const user = userEvent.setup()
    const input = state()
    input.config.bindings = [
      {
        id: 'binding',
        channel_id: 1,
        account_id: 'bee',
        upstream_group: 'shop-a',
        local_model: 'local-gpt',
        upstream_model: 'gpt-upstream',
      },
    ]
    render(<BindingsPanel state={input} onSave={vi.fn()} />)
    await user.click(screen.getByRole('button', { name: 'Edit' }))
    const local = screen.getByRole('combobox', { name: 'Local model' })
    const upstream = screen.getByRole('combobox', { name: 'Upstream model' })
    await user.click(local)
    await user.click(screen.getByRole('option', { name: 'All channel models' }))
    await user.keyboard('{Escape}')
    expect(upstream).toBeDisabled()
    expect(upstream).toHaveValue('Channel mapping')
    await user.click(screen.getByRole('combobox', { name: 'Channel' }))
    await user.click(screen.getByRole('option', { name: 'Other channel' }))
    await user.click(local)
    expect(
      screen.queryByRole('option', { name: 'local-gpt' })
    ).not.toBeInTheDocument()
    await user.click(screen.getByRole('option', { name: 'elsewhere' }))
    await user.keyboard('{Escape}')
    await user.click(screen.getByRole('combobox', { name: 'Upstream account' }))
    await user.click(screen.getByRole('option', { name: 'Other upstream' }))
    expect(upstream).toBeDisabled()
    const group = screen.getByRole('combobox', { name: 'Upstream group' })
    expect(group).toHaveValue('')
    await user.click(group)
    expect(screen.getAllByRole('option')).toHaveLength(1)
    await user.click(
      screen.getByRole('option', { name: 'other-code (Shared display name)' })
    )
    await user.click(upstream)
    expect(screen.getByRole('option', { name: 'other-model' })).toBeVisible()
  })

  test('saves a selection of local models together without adding the whole channel', async () => {
    const user = userEvent.setup()
    const input = state()
    input.channels = input.channels.map((channel) => ({
      ...channel,
      models: `${channel.models}, untouched-model`,
    }))
    const onSave = vi.fn().mockResolvedValue(undefined)
    render(<BindingsPanel state={input} onSave={onSave} />)
    await user.click(screen.getByRole('button', { name: 'Add binding' }))
    await user.click(screen.getByRole('combobox', { name: 'Upstream group' }))
    await user.click(
      screen.getByRole('option', { name: 'shop-a (Shared display name)' })
    )
    await user.click(screen.getByRole('combobox', { name: 'Local model' }))
    await user.click(screen.getByRole('option', { name: 'local-gpt' }))
    await user.click(screen.getByRole('option', { name: 'local-other' }))
    expect(
      screen.getByRole('option', { name: 'All channel models' })
    ).toHaveAttribute('aria-selected', 'false')
    await user.keyboard('{Escape}')
    expect(
      screen.getByRole('combobox', { name: 'Upstream model' })
    ).toBeDisabled()
    await user.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(onSave).toHaveBeenCalledTimes(1))
    const saved = onSave.mock.calls[0]?.[0] as MonitorState['config']
    expect(saved.bindings).toMatchObject([
      {
        channel_id: 1,
        account_id: 'bee',
        upstream_group: 'shop-a',
        local_model: 'local-gpt',
        upstream_model: '',
      },
      {
        channel_id: 1,
        account_id: 'bee',
        upstream_group: 'shop-a',
        local_model: 'local-other',
        upstream_model: '',
      },
    ])
    expect(new Set(saved.bindings.map((binding) => binding.id)).size).toBe(2)
  })

  test('extending a saved binding preserves its explicit upstream model and adds only missing bindings', async () => {
    const user = userEvent.setup()
    const input = state()
    input.channels = input.channels.map((channel) => ({
      ...channel,
      models: `${channel.models}, local-third`,
    }))
    const original = {
      id: 'original',
      channel_id: 1,
      account_id: 'bee',
      upstream_group: 'shop-a',
      local_model: 'local-gpt',
      upstream_model: 'gpt-upstream',
    }
    const existing = {
      ...original,
      id: 'existing',
      local_model: 'local-other',
      upstream_model: '',
    }
    input.config.bindings = [original, existing]
    const onSave = vi.fn().mockResolvedValue(undefined)
    render(<BindingsPanel state={input} onSave={onSave} />)
    const edit = screen.getAllByRole('button', { name: 'Edit' }).at(0)
    expect(edit).toBeDefined()
    if (!edit) throw new Error('Missing edit action')
    await user.click(edit)
    await user.click(screen.getByRole('combobox', { name: 'Local model' }))
    await user.click(screen.getByRole('option', { name: 'local-other' }))
    await user.click(screen.getByRole('option', { name: 'local-third' }))
    await user.keyboard('{Escape}')
    await user.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(onSave).toHaveBeenCalledTimes(1))
    const saved = onSave.mock.calls[0]?.[0] as MonitorState['config']
    expect(saved.bindings).toHaveLength(3)
    expect(saved.bindings).toEqual(
      expect.arrayContaining([
        original,
        existing,
        expect.objectContaining({
          local_model: 'local-third',
          upstream_model: '',
        }),
      ])
    )
  })

  test('an empty model selection cannot silently save a whole-channel binding', async () => {
    const user = userEvent.setup()
    const onSave = vi.fn()
    render(<BindingsPanel state={state()} onSave={onSave} />)
    await user.click(screen.getByRole('button', { name: 'Add binding' }))
    await user.click(screen.getByRole('combobox', { name: 'Upstream group' }))
    await user.click(
      screen.getByRole('option', { name: 'shop-a (Shared display name)' })
    )
    const models = screen.getByRole('combobox', { name: 'Local model' })
    await user.click(models)
    await user.click(screen.getByRole('option', { name: 'All channel models' }))
    await user.keyboard('{Escape}')
    expect(screen.getByRole('button', { name: 'Save' })).toBeDisabled()
    expect(onSave).not.toHaveBeenCalled()
    await user.click(models)
    await user.click(screen.getByRole('option', { name: 'local-gpt' }))
    await user.click(screen.getByRole('option', { name: 'All channel models' }))
    expect(screen.getByRole('option', { name: 'local-gpt' })).toHaveAttribute(
      'aria-selected',
      'false'
    )
    await user.keyboard('{Escape}')
    await user.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(onSave).toHaveBeenCalledTimes(1))
    const saved = onSave.mock.calls[0]?.[0] as MonitorState['config']
    expect(saved.bindings).toMatchObject([
      { local_model: '', upstream_model: '' },
    ])
  })

  test('a missing usable snapshot keeps group selection unavailable and explains the prerequisite', async () => {
    const user = userEvent.setup()
    const input = state()
    input.snapshots = input.snapshots.filter((item) => !item.complete)
    render(<BindingsPanel state={input} onSave={vi.fn()} />)
    await user.click(screen.getByRole('button', { name: 'Add binding' }))
    expect(
      screen.getByRole('combobox', { name: 'Upstream group' })
    ).toBeDisabled()
    expect(
      screen.getByText(
        'Run a complete collection for this account to choose an upstream group.'
      )
    ).toBeVisible()
    expect(screen.getByRole('button', { name: 'Save' })).toBeDisabled()
  })
})
