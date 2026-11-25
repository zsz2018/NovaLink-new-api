import { api } from '@/lib/api'
import { requireServerSuccess } from '@/lib/server-error-message'

import type {
  MonitorBrowserSetup,
  MonitorConfig,
  MonitorJob,
  MonitorJobDetail,
  MonitorProfitDetail,
  MonitorProfitDetailRequest,
  MonitorState,
} from './types'

export const monitorQueryKey = ['upstream-monitor'] as const
const base = '/api/upstream_monitor'

export async function getMonitorState(): Promise<MonitorState> {
  const response = await api.get<{ success: boolean; data: MonitorState }>(
    `${base}/state`
  )
  return requireServerSuccess(response.data).data
}

export async function getMonitorBrowserSetup(): Promise<
  Record<string, MonitorBrowserSetup>
> {
  const response = await api.get<{
    success: boolean
    data: { browser_setup: Record<string, MonitorBrowserSetup> }
  }>(`${base}/browser_setup`)
  return requireServerSuccess(response.data).data.browser_setup
}

export async function getMonitorProfitDetail(
  request: MonitorProfitDetailRequest
): Promise<MonitorProfitDetail> {
  const response = await api.get<{
    success: boolean
    data: MonitorProfitDetail
  }>(`${base}/profit/detail`, { params: request })
  return requireServerSuccess(response.data).data
}

export async function saveMonitorConfig(config: MonitorConfig): Promise<void> {
  const response = await api.put(`${base}/config`, config)
  requireServerSuccess(response.data)
}

export async function pairMonitorDevice(
  name: string,
  proofToken: string
): Promise<{ device_id: string; code: string; expires_at: number }> {
  const response = await api.post(
    `${base}/devices/pair`,
    { name },
    {
      headers: { 'X-Security-Proof': proofToken },
    }
  )
  return requireServerSuccess(response.data).data
}

export async function revokeMonitorDevice(id: string): Promise<void> {
  const response = await api.delete(`${base}/devices/${encodeURIComponent(id)}`)
  requireServerSuccess(response.data)
}

export async function deleteMonitorDeviceRecord(id: string): Promise<void> {
  const response = await api.delete(
    `${base}/devices/${encodeURIComponent(id)}/record`
  )
  requireServerSuccess(response.data)
}

export async function createMonitorJob(input: {
  account_id: string
  device_id: string
  flow_id: string
  mode: 'record' | 'collect'
}): Promise<MonitorJob> {
  const response = await api.post(`${base}/jobs`, input)
  return requireServerSuccess(response.data).data
}

export async function getMonitorJob(id: string): Promise<MonitorJobDetail> {
  const response = await api.get(`${base}/jobs/${encodeURIComponent(id)}`)
  return requireServerSuccess(response.data).data
}

export async function controlMonitorJob(
  id: string,
  action: 'cancel' | 'resume' | 'retry'
): Promise<MonitorJob> {
  const response = await api.post(
    `${base}/jobs/${encodeURIComponent(id)}/control`,
    { action }
  )
  return requireServerSuccess(response.data).data
}
