import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { expect, test, vi } from 'vitest'

import { SiteDialog } from '../components/site-dialog'
import type { MonitorConfig, MonitorSite } from '../types'

const site: MonitorSite = {
  id: 'site',
  name: 'Provider',
  origin: 'https://provider.example',
  allowed_origins: [],
  currency: 'USD',
  fx_rate: 1,
}

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

test('a new site defaults to USD and saves the currency code selected from the plugin currency list', async () => {
  const user = userEvent.setup()
  const onSave = vi.fn().mockResolvedValue(undefined)
  render(<SiteDialog config={config} onSave={onSave} onClose={vi.fn()} />)

  await user.type(screen.getByRole('textbox', { name: 'Site name' }), site.name)
  await user.type(
    screen.getByRole('textbox', { name: 'Website origin' }),
    site.origin
  )

  const currency = screen.getByRole('combobox', { name: 'Site currency' })
  expect(currency).toHaveTextContent(/^USD · .+/)
  await user.click(currency)
  expect(
    screen
      .getAllByRole('option')
      .map((option) => option.textContent?.split(' · ')[0])
  ).toEqual([
    'USD',
    'CNY',
    'EUR',
    'GBP',
    'HKD',
    'JPY',
    'TWD',
    'SGD',
    'KRW',
    'AUD',
    'CAD',
    'NZD',
    'CHF',
    'THB',
    'RUB',
    'VND',
  ])
  await user.click(screen.getByRole('option', { name: /^CNY · / }))
  expect(currency).toHaveTextContent(/^CNY · .+/)
  await user.click(screen.getByRole('button', { name: 'Save' }))

  await waitFor(() => expect(onSave).toHaveBeenCalledOnce())
  expect(onSave).toHaveBeenCalledWith({
    ...config,
    sites: [expect.objectContaining({ currency: 'CNY', fx_rate: 1 })],
  })
})

test('editing a site keeps a previously saved currency outside the preset list', async () => {
  const user = userEvent.setup()
  const onSave = vi.fn().mockResolvedValue(undefined)
  const existingSite = { ...site, currency: 'INR', fx_rate: 0.012 }
  const existingConfig = { ...config, sites: [existingSite] }
  render(
    <SiteDialog
      site={existingSite}
      config={existingConfig}
      onSave={onSave}
      onClose={vi.fn()}
    />
  )

  const currency = screen.getByRole('combobox', { name: 'Site currency' })
  expect(currency).toHaveTextContent('INR')
  await user.click(currency)
  expect(
    screen.getByRole('option', { name: 'INR', selected: true })
  ).toBeVisible()
  await user.keyboard('{Escape}')
  await user.click(screen.getByRole('button', { name: 'Save' }))

  await waitFor(() => expect(onSave).toHaveBeenCalledWith(existingConfig))
})
