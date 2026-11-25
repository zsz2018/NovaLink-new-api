import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, expect, test, vi } from 'vitest'

import { DevicesPanel } from '../components/devices-panel'
import type { MonitorDevice } from '../types'

const api = vi.hoisted(() => ({
  deleteMonitorDeviceRecord: vi.fn(),
  revokeMonitorDevice: vi.fn(),
  pairMonitorDevice: vi.fn(),
}))
vi.mock('../api', () => ({ ...api, monitorQueryKey: ['upstream-monitor'] }))
vi.mock('@/features/auth/secure-verification', () => ({
  useSecureVerification: () => ({
    requestVerification: vi.fn(),
    dialogProps: {},
  }),
  SecureVerificationDialog: () => null,
}))
const clients: QueryClient[] = []
const device: MonitorDevice = {
  id: 'revoked-browser',
  name: 'My browser',
  last_seen_at: 0,
  created_at: 1,
  revoked_at: 2,
  expires_at: 0,
}

function renderDevices(value: MonitorDevice) {
  const client = new QueryClient({
    defaultOptions: { mutations: { retry: false } },
  })
  clients.push(client)
  render(
    <QueryClientProvider client={client}>
      <DevicesPanel devices={[value]} />
    </QueryClientProvider>
  )
  return client
}

afterEach(() => {
  clients.forEach((client) => client.clear())
  clients.length = 0
  vi.clearAllMocks()
})

test('a revoked browser can be removed after confirmation and refreshes the device list', async () => {
  const user = userEvent.setup()
  api.deleteMonitorDeviceRecord.mockResolvedValue(undefined)
  const client = renderDevices(device)
  const invalidate = vi.spyOn(client, 'invalidateQueries')

  await user.click(
    screen.getByRole('button', { name: 'Delete browser record' })
  )
  expect(screen.getByRole('alertdialog')).toHaveTextContent(
    'Delete this browser record?'
  )
  expect(api.deleteMonitorDeviceRecord).not.toHaveBeenCalled()
  await user.click(screen.getByRole('button', { name: 'Continue' }))

  await waitFor(() =>
    expect(api.deleteMonitorDeviceRecord).toHaveBeenCalledWith(device.id)
  )
  expect(api.revokeMonitorDevice).not.toHaveBeenCalled()
  await waitFor(() =>
    expect(invalidate).toHaveBeenCalledWith({ queryKey: ['upstream-monitor'] })
  )
})

test('a browser must be revoked before the delete action is offered', () => {
  renderDevices({ ...device, revoked_at: 0 })
  expect(screen.getByRole('button', { name: 'Revoke browser' })).toBeEnabled()
  expect(
    screen.queryByRole('button', { name: 'Delete browser record' })
  ).not.toBeInTheDocument()
})
