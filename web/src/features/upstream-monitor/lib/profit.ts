import { formatLocalCurrencyAmount } from '@/lib/currency'
import { formatNumber } from '@/lib/format'

import type { MonitorProfitRow } from '../types'

export function formatProfitAmount(
  value: number | null | undefined,
  currency: string,
  locale: Intl.LocalesArgument
): string {
  if (value == null) return '—'
  const amount = formatLocalCurrencyAmount(value, {
    locale,
    showSymbol: false,
    fullPrecision: true,
  })
  return `${amount} ${currency}`
}

export function formatProfitPercent(
  value: number | null | undefined,
  locale: Intl.LocalesArgument
): string {
  if (value == null) return '—'
  return formatNumber(value, locale, { style: 'percent', fullPrecision: true })
}

export function upstreamSelectionKey(
  row: Pick<MonitorProfitRow, 'account_id' | 'upstream_group'>
): string {
  return JSON.stringify([row.account_id, row.upstream_group])
}

export function resolveProfitSelection(
  rows: MonitorProfitRow[],
  localGroup: string,
  upstreamKey: string
): { localGroup: string; upstreamKey: string } {
  const group = rows.some((row) => row.local_group === localGroup)
    ? localGroup
    : (rows[0]?.local_group ?? '')
  const candidates = rows.filter((row) => row.local_group === group)
  let key = ''
  if (candidates.some((row) => upstreamSelectionKey(row) === upstreamKey)) {
    key = upstreamKey
  } else if (candidates[0]) key = upstreamSelectionKey(candidates[0])
  return { localGroup: group, upstreamKey: key }
}

export function sortProfitRows(
  rows: MonitorProfitRow[],
  order: string
): MonitorProfitRow[] {
  const field = order.startsWith('cost') ? 'cost' : 'margin'
  const direction = order.endsWith('asc') ? 1 : -1
  return [...rows].sort((left, right) => {
    const leftValue = left[field]
    const rightValue = right[field]
    if (leftValue === null || !Number.isFinite(leftValue)) {
      return rightValue === null || !Number.isFinite(rightValue) ? 0 : 1
    }
    if (rightValue === null || !Number.isFinite(rightValue)) return -1
    return (leftValue - rightValue) * direction
  })
}

export function hasLossMakingTier(row: MonitorProfitRow): boolean {
  return [row, ...(row.tier_samples ?? [])].some(
    (sample) => sample.profit !== null && sample.profit < 0
  )
}

export function summarizeProfit(rows: MonitorProfitRow[]) {
  const known = rows.filter((row) =>
    [row.revenue, row.cost, row.profit].every(
      (value) => value !== null && Number.isFinite(value)
    )
  )
  const revenue = known.reduce((sum, row) => sum + (row.revenue ?? 0), 0)
  const cost = known.reduce((sum, row) => sum + (row.cost ?? 0), 0)
  const profit = known.reduce((sum, row) => sum + (row.profit ?? 0), 0)
  return {
    revenue: known.length ? revenue : null,
    cost: known.length ? cost : null,
    profit: known.length ? profit : null,
    margin: known.length && revenue > 0 ? profit / revenue : null,
    coverage: rows.length ? known.length / rows.length : null,
    lossCount: rows.filter(hasLossMakingTier).length,
    estimatedCount: known.filter((row) => row.estimated).length,
    knownCount: known.length,
    totalCount: rows.length,
  }
}
