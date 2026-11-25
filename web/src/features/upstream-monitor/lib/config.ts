import { t } from 'i18next'
import { z } from 'zod'

import { validMonitorEstimateIdentity } from './official-estimates'

const origin = z
  .string()
  .url({ error: () => t('Enter a valid URL') })
  .refine(
    (value) => {
      const url = new URL(value)
      return (
        ['https:', 'http:'].includes(url.protocol) &&
        !url.username &&
        !url.password &&
        url.pathname === '/' &&
        !url.search &&
        !url.hash
      )
    },
    { error: () => t('Enter a website origin without a path or credentials') }
  )

export const siteSchema = z.object({
  name: z
    .string()
    .trim()
    .min(1, { error: () => t('This field is required') }),
  origin,
  allowed_origins: z.string().refine(
    (value) =>
      value
        .split('\n')
        .filter((line) => line.trim())
        .every((line) => origin.safeParse(line.trim()).success),
    { error: () => t('Enter one valid origin per line') }
  ),
  currency: z
    .string()
    .trim()
    .min(1, { error: () => t('This field is required') }),
  fx_rate: z
    .number({ error: () => t('Enter a valid number') })
    .positive({ error: () => t('Enter a number greater than zero') }),
})
export const accountSchema = z
  .object({
    name: z
      .string()
      .trim()
      .min(1, { error: () => t('This field is required') }),
    site_id: z.string().min(1, { error: () => t('This field is required') }),
    username: z.string().min(1, { error: () => t('This field is required') }),
    external_id: z.string(),
    password: z.string(),
    clear_password: z.boolean(),
    device_id: z.string(),
    flow_id: z.string(),
    interval_minutes: z
      .number({ error: () => t('Enter a valid number') })
      .int({ error: () => t('Enter a whole number') })
      .min(0, { error: () => t('Enter a non-negative number') }),
    enabled: z.boolean(),
    purchase_factor: z
      .number({ error: () => t('Enter a valid number') })
      .positive({ error: () => t('Enter a number greater than zero') }),
    fee_rate: z
      .number({ error: () => t('Enter a valid number') })
      .min(0, { error: () => t('Enter a non-negative number') })
      .max(1, { error: () => t('Enter a fraction between zero and one') }),
    fee_bearer: z.enum(['self', 'other'], {
      error: () => t('Choose who pays the fee'),
    }),
  })
  .refine(
    (value) =>
      value.interval_minutes === 0 || Boolean(value.device_id && value.flow_id),
    {
      path: ['interval_minutes'],
      error: () => t('Choose a browser and flow before enabling a schedule'),
    }
  )
export const accountCostSchema = z
  .object({
    enabled: z.boolean(),
    catalog_version: z.string(),
    models: z.array(
      z.object({ name: z.string(), group: z.string(), entry_id: z.string() })
    ),
  })
  .superRefine((value, context) => {
    if (!value.enabled) return
    let message: string | undefined
    if (
      !validMonitorEstimateIdentity(value.catalog_version, 255) ||
      !value.models.length
    ) {
      message = t('Select at least one reviewed model for estimation.')
    } else if (value.models.length > 500) {
      message = t('Select at most 500 estimate mappings.')
    } else if (
      value.models.some(
        (model) =>
          !validMonitorEstimateIdentity(model.name, 255) ||
          !validMonitorEstimateIdentity(model.group, 128) ||
          !validMonitorEstimateIdentity(model.entry_id, 255)
      )
    ) {
      message = t(
        'Use exact non-empty model and group names within the supported UTF-8 limits, without surrounding spaces or control characters.'
      )
    } else if (
      new Set(
        value.models.map((model) => JSON.stringify([model.name, model.group]))
      ).size !== value.models.length
    ) {
      message = t('Select each upstream model and group only once.')
    }
    if (message) context.addIssue({ code: 'custom', path: ['models'], message })
  })

export const bindingSchema = z
  .object({
    channel_id: z
      .number({ error: () => t('Enter a valid number') })
      .int({ error: () => t('Enter a whole number') })
      .positive({ error: () => t('Enter a number greater than zero') }),
    account_id: z.string().min(1, { error: () => t('This field is required') }),
    upstream_group: z
      .string()
      .trim()
      .min(1, { error: () => t('This field is required') }),
    local_models: z
      .array(z.string())
      .min(1, { error: () => t('No models selected') }),
    upstream_model: z.string(),
  })
  .refine(
    (value) =>
      !value.upstream_model ||
      (value.local_models.length === 1 && Boolean(value.local_models[0])),
    {
      path: ['local_models'],
      error: () =>
        t('Select one local model to customize its upstream mapping.'),
    }
  )
export const settingsSchema = z.object({
  scenario: z.object({
    currency: z
      .string()
      .trim()
      .min(1, { error: () => t('This field is required') }),
    usd_exchange: z
      .number({ error: () => t('Enter a valid number') })
      .positive({ error: () => t('Enter a number greater than zero') }),
    input_tokens: z
      .number({ error: () => t('Enter a valid number') })
      .int({ error: () => t('Enter a whole number') })
      .min(0, { error: () => t('Enter a non-negative number') }),
    output_tokens: z
      .number({ error: () => t('Enter a valid number') })
      .int({ error: () => t('Enter a whole number') })
      .min(0, { error: () => t('Enter a non-negative number') }),
    cache_read: z
      .number({ error: () => t('Enter a valid number') })
      .int({ error: () => t('Enter a whole number') })
      .min(0, { error: () => t('Enter a non-negative number') }),
    cache_write: z
      .number({ error: () => t('Enter a valid number') })
      .int({ error: () => t('Enter a whole number') })
      .min(0, { error: () => t('Enter a non-negative number') }),
    requests: z
      .number({ error: () => t('Enter a valid number') })
      .int({ error: () => t('Enter a whole number') })
      .positive({ error: () => t('Enter a number greater than zero') }),
    revenue_factor: z
      .number({ error: () => t('Enter a valid number') })
      .positive({ error: () => t('Enter a number greater than zero') }),
    fee_rate: z
      .number({ error: () => t('Enter a valid number') })
      .min(0, { error: () => t('Enter a non-negative number') })
      .max(1, { error: () => t('Enter a fraction between zero and one') }),
    fee_bearer: z.enum(['self', 'other'], {
      error: () => t('Choose who pays the fee'),
    }),
    user_group: z.string(),
  }),
})

export function newMonitorID(): string {
  return crypto.randomUUID()
}
