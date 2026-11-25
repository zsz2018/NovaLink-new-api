import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, test, vi } from 'vitest'

import { AccountDialog } from '../components/account-dialog'
import { SettingsPanel } from '../components/settings-panel'
import type { MonitorAccount, MonitorConfig, MonitorState } from '../types'

const account: MonitorAccount = {
  id: 'account',
  site_id: 'site',
  name: 'Upstream account',
  username: 'operator',
  external_id: '42',
  has_password: false,
  device_id: '',
  flow_id: '',
  interval_minutes: 0,
  enabled: true,
  purchase_factor: 1,
  fee_rate: 0.05,
  fee_bearer: 'unknown',
}
const config: MonitorConfig = {
  version: 1,
  sites: [
    {
      id: 'site',
      name: 'Provider',
      origin: 'https://provider.example',
      allowed_origins: [],
      currency: 'USD',
      fx_rate: 1,
    },
  ],
  accounts: [account],
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
    fee_rate: 0.02,
    fee_bearer: 'unknown',
    user_group: '',
  },
}
const state: MonitorState = {
  config,
  devices: [],
  jobs: [],
  snapshots: [],
  channels: [],
  groups: {},
  profit_rows: [],
  key_ready: true,
}

test('cost settings save the chosen reporting currency and local user group', async () => {
  const user = userEvent.setup()
  const onSave = vi.fn().mockResolvedValue(undefined)
  const settings: MonitorConfig = {
    ...config,
    scenario: { ...config.scenario, fee_bearer: 'self', usd_exchange: 7 },
  }
  render(
    <SettingsPanel
      config={settings}
      groups={{ premium: 0.085, default: 1 }}
      onSave={onSave}
    />
  )

  await user.click(screen.getByRole('combobox', { name: 'Reporting currency' }))
  await user.click(screen.getByRole('option', { name: /^CNY · / }))
  const group = screen.getByRole('combobox', { name: 'Pricing user group' })
  expect(group).toHaveTextContent('Not selected')
  await user.click(group)
  expect(screen.getByRole('option', { name: 'default' })).toBeVisible()
  await user.click(screen.getByRole('option', { name: 'premium' }))
  await user.click(screen.getByRole('button', { name: 'Save cost settings' }))

  await waitFor(() =>
    expect(onSave).toHaveBeenCalledWith({
      ...settings,
      scenario: {
        ...settings.scenario,
        currency: 'CNY',
        user_group: 'premium',
      },
    })
  )
})

test('a saved group missing from the local list remains selected until cleared', async () => {
  const user = userEvent.setup()
  const onSave = vi.fn().mockResolvedValue(undefined)
  const settings: MonitorConfig = {
    ...config,
    scenario: { ...config.scenario, fee_bearer: 'self', user_group: 'legacy' },
  }
  render(<SettingsPanel config={settings} groups={{}} onSave={onSave} />)

  const group = screen.getByRole('combobox', { name: 'Pricing user group' })
  expect(group).toHaveTextContent('legacy')
  await user.click(group)
  expect(
    screen.getByRole('option', { name: 'legacy', selected: true })
  ).toBeVisible()
  await user.click(screen.getByRole('option', { name: 'Not selected' }))
  await user.click(screen.getByRole('button', { name: 'Save cost settings' }))

  await waitFor(() =>
    expect(onSave).toHaveBeenCalledWith({
      ...settings,
      scenario: { ...settings.scenario, user_group: '' },
    })
  )
})

describe('recharge fee responsibility', () => {
  test('an unconfirmed upstream fee requires choosing our site or the upstream site before saving', async () => {
    const user = userEvent.setup()
    const onSave = vi.fn().mockResolvedValue(undefined)
    render(
      <AccountDialog
        account={account}
        state={state}
        onSave={onSave}
        onClose={vi.fn()}
      />
    )

    await user.click(screen.getByRole('button', { name: 'Save' }))
    expect(
      await screen.findByText('Choose who pays the fee')
    ).toBeInTheDocument()
    expect(onSave).not.toHaveBeenCalled()

    await user.click(
      screen.getByRole('combobox', { name: 'Upstream fee bearer' })
    )
    expect(
      screen.getAllByRole('option').map((option) => option.textContent)
    ).toEqual(['Our site', 'Upstream site'])
    await user.click(screen.getByRole('option', { name: 'Upstream site' }))
    await user.click(screen.getByRole('button', { name: 'Save' }))

    await waitFor(() => expect(onSave).toHaveBeenCalledOnce())
    expect(onSave.mock.calls[0][0].accounts[0]).toMatchObject({
      fee_bearer: 'other',
      fee_rate: 0.05,
    })
  })

  test('an unconfirmed collection fee requires choosing our site or the user before saving', async () => {
    const user = userEvent.setup()
    const onSave = vi.fn().mockResolvedValue(undefined)
    render(
      <SettingsPanel config={config} groups={state.groups} onSave={onSave} />
    )

    await user.click(screen.getByRole('button', { name: 'Save cost settings' }))
    expect(
      await screen.findByText('Choose who pays the fee')
    ).toBeInTheDocument()
    expect(onSave).not.toHaveBeenCalled()

    await user.click(
      screen.getByRole('combobox', { name: 'Collection fee bearer' })
    )
    expect(
      screen.getAllByRole('option').map((option) => option.textContent)
    ).toEqual(['Our site', 'User'])
    await user.click(screen.getByRole('option', { name: 'User' }))
    await user.click(screen.getByRole('button', { name: 'Save cost settings' }))

    await waitFor(() => expect(onSave).toHaveBeenCalledOnce())
    expect(onSave.mock.calls[0][0].scenario).toMatchObject({
      fee_bearer: 'other',
      fee_rate: 0.02,
    })
  })
})
