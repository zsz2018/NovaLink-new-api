import { parseModelsList } from '@/features/channels/lib/channel-utils'

import type { MonitorModelPrice, MonitorState } from '../types'

export function validMonitorEstimateIdentity(
  value: string,
  maxBytes: number
): boolean {
  return Boolean(
    value &&
    value.trim() === value &&
    new TextEncoder().encode(value).length <= maxBytes &&
    !/[\p{Cc}\uD800-\uDFFF]/u.test(value) &&
    !value.toLowerCase().includes('[redacted]')
  )
}

export function monitorQuoteHasPricingFacts(price: MonitorModelPrice): boolean {
  return !(
    (!price.billing_unit || price.billing_unit === 'token') &&
    [
      price.input,
      price.output,
      price.cache_read,
      price.cache_write,
      price.request_price,
    ].every((value) => value == null) &&
    !price.billing_expression &&
    !price.notes?.trim() &&
    !price.tiers?.length &&
    !price.tier_mode &&
    !price.tier_basis &&
    !price.pricing_pending &&
    !('price_source' in price && price.price_source)
  )
}

export function boundUpstreamModels(
  state: MonitorState,
  accountID: string
): { name: string; group: string }[] {
  const models = new Map<string, { name: string; group: string }>()
  for (const binding of state.config.bindings) {
    if (binding.account_id !== accountID || !binding.upstream_group) continue
    if (binding.upstream_model) {
      const model = {
        name: binding.upstream_model,
        group: binding.upstream_group,
      }
      models.set(JSON.stringify([model.name, model.group]), model)
      continue
    }
    const channel = state.channels.find(
      (item) => item.id === binding.channel_id
    )
    let mapping: Record<string, unknown> = {}
    if (channel?.model_mapping) {
      try {
        const parsed: unknown = JSON.parse(channel.model_mapping)
        if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
          continue
        }
        mapping = parsed as Record<string, unknown>
      } catch {
        // A malformed channel mapping cannot establish an upstream model name.
        continue
      }
    }
    const names = binding.local_model
      ? [binding.local_model]
      : parseModelsList(channel?.models ?? '')
    for (const localName of names) {
      const mapped = Object.hasOwn(mapping, localName)
        ? mapping[localName]
        : localName
      if (typeof mapped !== 'string' || !mapped.trim()) continue
      const model = { name: mapped, group: binding.upstream_group }
      models.set(JSON.stringify([model.name, model.group]), model)
    }
  }
  return [...models.values()]
}
