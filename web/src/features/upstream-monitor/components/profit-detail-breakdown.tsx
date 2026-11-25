import { useTranslation } from 'react-i18next'

import {
  StaticDataTable,
  type StaticDataTableColumn,
} from '@/components/data-table'
import { Badge } from '@/components/ui/badge'
import { parseTiersFromExpr } from '@/features/pricing/lib/billing-expr'
import { formatBillingCondition } from '@/features/pricing/lib/billing-expression/condition-display'
import { getTierCachePricingMode } from '@/features/pricing/lib/cache-pricing'
import { toIntlLocale } from '@/i18n/languages'
import { formatNumber, formatTimestamp } from '@/lib/format'

import { formatProfitAmount, formatProfitPercent } from '../lib/profit'
import { formatMonitorMessage } from '../lib/task-messages'
import type {
  MonitorProfitComponent,
  MonitorProfitDetail,
  MonitorProfitSide,
} from '../types'
import {
  OfficialEstimateNotice,
  OfficialEstimateSource,
} from './official-estimate'

export function ProfitDetailBreakdown(props: { detail: MonitorProfitDetail }) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const detail = props.detail
  const currency = detail.scenario.currency
  const priceSource = detail.upstream?.price_source ?? detail.row.price_source
  const estimated =
    detail.row.estimated ||
    detail.upstream?.estimated ||
    priceSource?.kind === 'official_estimate'
  const costLabel = estimated
    ? t('Estimated procurement cost')
    : t('Scenario purchase cost')
  const profitLabel = estimated
    ? t('Estimated profit')
    : t('Estimated gross profit')
  const marginLabel = estimated
    ? t('Estimated margin')
    : t('Estimated gross margin')
  const money = (value: number | null | undefined) =>
    formatProfitAmount(value, currency, locale)
  const percent = (value: number | null | undefined) =>
    formatProfitPercent(value, locale)
  const kinds = {
    input: t('Uncached input tokens'),
    output: t('Output tokens'),
    cache_read: t('Cache read tokens'),
    cache_write: t('Cache write tokens'),
    request: t('Per request'),
  }
  const upstreamTier = (tier?: string) => {
    if (tier === 'base') return t('Base tier')
    const threshold = /^(context|input) > (\d+)$/.exec(tier ?? '')
    if (!threshold) return tier || '—'
    const tokens = formatNumber(Number(threshold[2]), locale)
    return threshold[1] === 'context'
      ? t('Context > {{tokens}}', { tokens })
      : t('Uncached input > {{tokens}}', { tokens })
  }
  const unitColumns: StaticDataTableColumn<MonitorProfitComponent>[] = [
    {
      id: 'kind',
      header: t('Billing component'),
      cell: (part) => kinds[part.kind],
    },
    {
      id: 'quantity',
      header: t('Quantity'),
      cell: (part) => formatNumber(part.quantity, locale),
    },
    {
      id: 'local',
      header: t('Local sale price'),
      cell: (part) => money(part.local_unit_price),
    },
    {
      id: 'upstream',
      header: estimated
        ? t('Estimated unit purchase price')
        : t('Upstream purchase price'),
      cell: (part) => money(part.upstream_unit_price),
    },
    {
      id: 'profit',
      header: estimated ? t('Estimated unit profit') : t('Unit gross profit'),
      cell: (part) => money(part.unit_profit),
    },
    {
      id: 'margin',
      header: estimated ? t('Estimated unit margin') : t('Unit gross margin'),
      cell: (part) => percent(part.unit_margin),
    },
  ]
  const seenUnits = new Set<string>()
  const unitGroups = [
    ...(detail.tier_samples ?? []).flatMap((sample) =>
      sample.components?.length
        ? [{ ...sample, components: sample.components }]
        : []
    ),
    {
      context_tokens: detail.context_tokens,
      local_tier: detail.local?.tier || detail.row.local_tier,
      upstream_tier: detail.upstream?.tier || detail.row.upstream_tier,
      components: detail.components,
    },
  ]
    .filter((group) => {
      const key = JSON.stringify([
        group.local_tier,
        group.upstream_tier,
        group.components.map((part) => [
          part.kind,
          part.local_unit_price,
          part.upstream_unit_price,
        ]),
      ])
      if (seenUnits.has(key)) return false
      seenUnits.add(key)
      return true
    })
    .sort((a, b) => a.context_tokens - b.context_tokens)

  return (
    <div className='space-y-5'>
      {detail.row.reason && (
        <p className='rounded-lg border p-3 text-sm' role='status'>
          {formatMonitorMessage(detail.row.reason, t)}
        </p>
      )}
      {estimated && (
        <section
          className='min-w-0 space-y-3 rounded-lg border p-3'
          aria-label={t('Official price estimate sources')}
        >
          <Badge
            variant='outline'
            className='h-auto max-w-full whitespace-normal'
          >
            {t('Official price estimate')}
          </Badge>
          <OfficialEstimateNotice />
          {priceSource && <OfficialEstimateSource source={priceSource} />}
        </section>
      )}
      <section
        className='space-y-3'
        aria-label={t('Profit comparison scenario')}
      >
        <h3 className='font-medium'>{t('Profit comparison scenario')}</h3>
        <dl className='grid gap-3 rounded-lg border p-3 sm:grid-cols-3'>
          {[
            [t('Uncached input tokens'), detail.scenario.input_tokens],
            [t('Output tokens'), detail.scenario.output_tokens],
            [t('Cache read tokens'), detail.scenario.cache_read],
            [t('Cache write tokens'), detail.scenario.cache_write],
            [t('Full input context'), detail.context_tokens],
            [t('Request count'), detail.scenario.requests],
          ].map(([label, quantity]) => (
            <div key={label}>
              <dt className='text-muted-foreground text-xs'>{label}</dt>
              <dd className='text-sm tabular-nums'>
                {formatNumber(Number(quantity), locale)}
              </dd>
            </div>
          ))}
        </dl>
        <p className='text-muted-foreground text-xs'>
          {t(
            'Token quantities are per request. Full input context includes uncached input, cache reads, and cache writes.'
          )}{' '}
          {t(
            'The comparison uses standard cache writes (30 minutes for GPT). Long-lived writes have zero usage in this scenario and require a separate upstream quote.'
          )}
        </p>
      </section>
      {detail.row.captured_at > 0 && (
        <p className='text-muted-foreground text-xs'>
          {t('Upstream snapshot captured at {{time}}', {
            time: formatTimestamp(detail.row.captured_at),
          })}
        </p>
      )}
      <section className='space-y-3' aria-label={t('Scenario result')}>
        <h3 className='font-medium'>{t('Scenario result')}</h3>
        <dl className='grid gap-3 sm:grid-cols-2 lg:grid-cols-4'>
          {[
            [t('Scenario net revenue'), money(detail.row.revenue)],
            [costLabel, money(detail.row.cost)],
            [profitLabel, money(detail.row.profit)],
            [marginLabel, percent(detail.row.margin)],
          ].map(([label, value]) => (
            <div className='space-y-1 rounded-lg border p-3' key={label}>
              <dt className='text-muted-foreground text-xs'>{label}</dt>
              <dd className='text-lg font-semibold tabular-nums'>{value}</dd>
            </div>
          ))}
        </dl>
        <p className='text-muted-foreground text-xs'>
          {estimated
            ? t(
                'Estimated profit = net revenue − estimated procurement cost. Estimated margin = estimated profit ÷ net revenue. Zero revenue has no defined margin.'
              )
            : t(
                'Gross profit = net revenue − purchase cost. Gross margin = gross profit ÷ net revenue. Zero revenue has no defined margin.'
              )}
        </p>
      </section>
      {!!detail.tier_samples?.length && (
        <section
          className='space-y-3'
          aria-label={t('Profit at tier boundaries')}
        >
          <h3 className='font-medium'>{t('Profit at tier boundaries')}</h3>
          <p className='text-muted-foreground text-xs'>
            {t(
              'Each row compares both sides at the same input length near a pricing threshold. Output, cache usage, and request count stay unchanged. These scenarios are alternatives and are not added together.'
            )}
          </p>
          <StaticDataTable
            data={detail.tier_samples}
            getRowKey={(sample) => sample.input_tokens}
            tableProps={{ 'aria-label': t('Profit at tier boundaries') }}
            columns={[
              {
                id: 'context',
                header: t('Full input context'),
                cell: (sample) => formatNumber(sample.context_tokens, locale),
              },
              {
                id: 'input',
                header: t('Uncached input tokens'),
                cell: (sample) => formatNumber(sample.input_tokens, locale),
              },
              {
                id: 'tiers',
                header: t('Local tier / upstream tier'),
                cell: (sample) => (
                  <div className='whitespace-normal'>
                    <p className='wrap-anywhere'>
                      {sample.local_tier || '—'} /{' '}
                      {upstreamTier(sample.upstream_tier)}
                    </p>
                    {sample.reason && (
                      <p className='text-muted-foreground'>
                        {formatMonitorMessage(sample.reason, t)}
                      </p>
                    )}
                  </div>
                ),
              },
              {
                id: 'revenue',
                header: t('Scenario net revenue'),
                cell: (sample) => money(sample.revenue),
              },
              {
                id: 'cost',
                header: costLabel,
                cell: (sample) => money(sample.cost),
              },
              {
                id: 'profit',
                header: profitLabel,
                cell: (sample) => money(sample.profit),
              },
              {
                id: 'margin',
                header: marginLabel,
                cell: (sample) => percent(sample.margin),
              },
            ]}
          />
        </section>
      )}
      <section className='space-y-3' aria-label={t('Unit price comparison')}>
        <h3 className='font-medium'>{t('Unit price comparison')}</h3>
        <p className='text-muted-foreground text-xs'>
          {t(
            'Prices below include group ratios, exchange rates, recharge factors, and fees. Token prices are per 1M tokens; request prices are per request.'
          )}
        </p>
        {detail.upstream?.tier_mode === 'graduated' && (
          <p className='text-muted-foreground text-xs'>
            {t(
              'Graduated input prices are weighted averages across the brackets used by this scenario.'
            )}
          </p>
        )}
        {unitGroups.map((group) => {
          let label = `${t('Full input context')} · ${formatNumber(group.context_tokens, locale)}`
          if (group.local_tier || group.upstream_tier) {
            label += ` · ${group.local_tier || '—'} / ${upstreamTier(group.upstream_tier)}`
          }
          return (
            <div key={label} className='space-y-2'>
              {unitGroups.length > 1 && (
                <h4 className='text-sm font-medium'>{label}</h4>
              )}
              <StaticDataTable
                data={group.components}
                getRowKey={(part) => part.kind}
                tableProps={{
                  'aria-label': `${t('Unit price comparison')} · ${label}`,
                }}
                emptyContent={t(
                  'This rule cannot be separated into independent unit prices. The complete expression remains authoritative.'
                )}
                columns={unitColumns}
              />
            </div>
          )
        })}
        <h4 className='text-sm font-medium'>
          {t('Component scenario amounts')} · {t('Full input context')}:{' '}
          {formatNumber(detail.context_tokens, locale)}
        </h4>
        <StaticDataTable
          data={detail.components}
          getRowKey={(part) => part.kind}
          emptyContent={t('No independent component amounts available.')}
          tableProps={{ 'aria-label': t('Component scenario amounts') }}
          columns={[
            {
              id: 'kind',
              header: t('Billing component'),
              cell: (part) => kinds[part.kind],
            },
            {
              id: 'revenue',
              header: t('Scenario net revenue'),
              cell: (part) => money(part.revenue),
            },
            {
              id: 'cost',
              header: costLabel,
              cell: (part) => money(part.cost),
            },
          ]}
        />
        <p className='text-muted-foreground text-xs'>
          {t(
            'Component amounts cover all requests in this scenario. Unknown prices remain unavailable even when their quantity is zero.'
          )}
        </p>
      </section>
      <section
        className='space-y-3'
        aria-label={t('Per 1M input-context tokens')}
      >
        <h3 className='font-medium'>{t('Per 1M input-context tokens')}</h3>
        <p className='text-muted-foreground text-xs'>
          {t(
            'Equivalent amounts scale the current scenario to 1M full input-context tokens with the same output and cache mix.'
          )}{' '}
          {t(
            'Scenario amounts are scaled by 1,000,000 ÷ (full input context per request × request count).'
          )}
        </p>
        <dl className='grid gap-3 rounded-lg border p-3 sm:grid-cols-2 lg:grid-cols-4'>
          {[
            [
              t('Scenario net revenue'),
              money(detail.per_million_context?.revenue),
            ],
            [costLabel, money(detail.per_million_context?.cost)],
            [profitLabel, money(detail.per_million_context?.profit)],
            [marginLabel, percent(detail.per_million_context?.margin)],
          ].map(([label, value]) => (
            <div key={label}>
              <dt className='text-muted-foreground text-xs'>{label}</dt>
              <dd className='font-medium tabular-nums'>{value}</dd>
            </div>
          ))}
        </dl>
      </section>
      <div className='grid min-w-0 gap-4 xl:grid-cols-[3fr_2fr]'>
        <ProfitSide
          title={t('Local sale calculation')}
          otherFeeBearer={t('User')}
          side={detail.local}
        />
        <ProfitSide
          title={
            estimated
              ? t('Estimated purchase calculation')
              : t('Upstream purchase calculation')
          }
          otherFeeBearer={t('Upstream site')}
          side={detail.upstream}
          estimated={estimated}
        />
      </div>
    </div>
  )
}

function ProfitSide(props: {
  title: string
  otherFeeBearer: string
  side?: MonitorProfitSide
  estimated?: boolean
}) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const side = props.side
  const tierLabel = props.estimated
    ? t('Reference price tiers')
    : t('Captured price tiers')
  const number = (value: number | null | undefined) =>
    value == null ? '—' : formatNumber(value, locale, { fullPrecision: true })
  const nativeAmount = formatProfitAmount(
    side?.base_amount,
    side?.currency ?? '',
    locale
  )
  const expressionTiers = side?.expression
    ? parseTiersFromExpr(side.expression)
    : []
  const cacheMode = getTierCachePricingMode(expressionTiers)
  let cacheWriteLabel = t('Cache write')
  if (cacheMode === 'gpt') cacheWriteLabel = t('Cache Write (30m)')
  if (cacheMode === 'claude') cacheWriteLabel = t('Cache Write (5m)')
  const nativePrice = (value: unknown) =>
    typeof value === 'number' ? `${number(value)} ${side?.currency ?? ''}` : '—'
  const capturedTiers = side?.base_prices
    ? [side.base_prices, ...(side.tiers ?? [])]
    : (side?.tiers ?? [])
  const threshold = /^(context|input) > (\d+)$/.exec(side?.tier ?? '')
  let appliedTier = side?.tier
  if (side?.tier === 'base' && capturedTiers.length) {
    appliedTier = t('Base tier')
  }
  if (threshold) {
    const tokens = number(Number(threshold[2]))
    appliedTier =
      threshold[1] === 'context'
        ? t('Context > {{tokens}}', { tokens })
        : t('Uncached input > {{tokens}}', { tokens })
  }
  const uniqueExpressionMatch =
    expressionTiers.filter((tier) => tier.label === side?.tier).length === 1
  const feeBearer = {
    self: t('Our site'),
    other: props.otherFeeBearer,
    unknown: t('Unknown'),
  }

  return (
    <section className='min-w-0 space-y-3 rounded-lg border p-3'>
      <h3 className='font-medium'>{props.title}</h3>
      {!side ? (
        <p className='text-muted-foreground text-sm'>{t('Unknown')}</p>
      ) : (
        <>
          <div className='flex flex-wrap gap-2'>
            {appliedTier && (
              <Badge
                variant='outline'
                className='wrap-anywhere whitespace-normal'
              >
                {appliedTier}
              </Badge>
            )}
            {side.unit === 'request' && (
              <Badge variant='outline'>{t('Per request')}</Badge>
            )}
            {side.unit === 'token' && (
              <Badge variant='outline'>
                {side.token_unit && side.token_unit !== 1_000_000
                  ? t('Per {{tokens}} tokens', {
                      tokens: number(side.token_unit),
                    })
                  : t('Per 1M tokens')}
              </Badge>
            )}
            {side.unit === 'expression' && (
              <Badge variant='outline'>{t('Billing expression')}</Badge>
            )}
            {side.tier_mode && (
              <Badge variant='outline'>
                {side.tier_mode === 'graduated'
                  ? t('Graduated input brackets')
                  : t('Whole-request tier')}
              </Badge>
            )}
            {side.tier_basis && (
              <Badge variant='outline'>
                {side.tier_basis === 'context'
                  ? t('Full input context')
                  : t('Uncached input tokens')}
              </Badge>
            )}
          </div>
          <dl className='grid grid-cols-2 gap-3 text-sm'>
            {[
              [t('Base scenario amount'), nativeAmount],
              [t('Applied group ratio'), number(side.group_ratio)],
              [t('Exchange rate'), number(side.fx_rate)],
              [t('Recharge factor'), number(side.factor)],
              [t('Fee rate'), formatProfitPercent(side.fee_rate, locale)],
              [t('Fee bearer'), feeBearer[side.fee_bearer]],
              [t('Fee multiplier'), number(side.fee_multiplier)],
            ].map(([label, value]) => (
              <div key={label}>
                <dt className='text-muted-foreground text-xs'>{label}</dt>
                <dd className='break-words tabular-nums'>{value}</dd>
              </div>
            ))}
          </dl>
          <p className='text-muted-foreground text-xs'>
            {t(
              'Final amount = base scenario amount × applied group ratio × exchange rate × recharge factor × fee multiplier.'
            )}
            {side.includes_group === true && (
              <>
                {' '}
                {props.estimated
                  ? t('Estimated prices already include the group ratio.')
                  : t('Captured prices already include the group ratio.')}
              </>
            )}
          </p>
          {capturedTiers.length > 0 && (
            <StaticDataTable
              data={capturedTiers}
              getRowKey={(tier) => String(tier.input_threshold)}
              tableProps={{ 'aria-label': tierLabel }}
              columns={[
                {
                  id: 'threshold',
                  header: t('Input threshold'),
                  cell: (tier) => {
                    const matched =
                      side.tier_mode === 'whole_request' &&
                      (tier.input_threshold === null
                        ? side.tier === 'base'
                        : Number(threshold?.[2]) === tier.input_threshold)
                    return (
                      <div className='space-y-1'>
                        <p>
                          {tier.input_threshold === null
                            ? t('Base tier')
                            : `> ${number(tier.input_threshold)}`}
                        </p>
                        {matched && (
                          <Badge variant='outline'>{t('Matched')}</Badge>
                        )}
                      </div>
                    )
                  },
                },
                {
                  id: 'input',
                  header: t('Input'),
                  cell: (tier) => number(tier.input),
                },
                {
                  id: 'output',
                  header: t('Output'),
                  cell: (tier) => number(tier.output),
                },
                {
                  id: 'read',
                  header: t('Cache read'),
                  cell: (tier) => number(tier.cache_read),
                },
                {
                  id: 'write',
                  header: t('Cache write'),
                  cell: (tier) => number(tier.cache_write),
                },
              ]}
            />
          )}
          {expressionTiers.length > 0 && (
            <StaticDataTable
              data={expressionTiers}
              getRowKey={(tier, index) => `${tier.label}:${index}`}
              tableProps={{ 'aria-label': t('Expression price tiers') }}
              columns={[
                {
                  id: 'tier',
                  header: t('Tier'),
                  cell: (tier) => (
                    <div className='space-y-1'>
                      <span>{tier.label}</span>
                      {uniqueExpressionMatch && tier.label === side.tier && (
                        <Badge variant='outline'>{t('Matched')}</Badge>
                      )}
                    </div>
                  ),
                },
                {
                  id: 'condition',
                  header: t('Applies to'),
                  cell: (tier) => {
                    if (tier.conditionText) {
                      return (
                        formatBillingCondition(tier.conditionText, t, locale) ??
                        tier.conditionText
                      )
                    }
                    return (
                      tier.conditions
                        .map(
                          (condition) =>
                            `${condition.var} ${condition.op} ${number(condition.value)}`
                        )
                        .join(' ∧ ') || t('Base tier')
                    )
                  },
                },
                {
                  id: 'input',
                  header: t('Input'),
                  cell: (tier) => nativePrice(tier.inputPrice),
                },
                {
                  id: 'output',
                  header: t('Output'),
                  cell: (tier) => nativePrice(tier.outputPrice),
                },
                {
                  id: 'read',
                  header: t('Cache read'),
                  cell: (tier) => nativePrice(tier.cacheReadPrice),
                },
                {
                  id: 'write',
                  header: cacheWriteLabel,
                  cell: (tier) => (
                    <div className='space-y-0.5 whitespace-normal'>
                      <p>{nativePrice(tier.cacheCreatePrice)}</p>
                      {typeof tier.cacheCreate1hPrice === 'number' && (
                        <p className='text-muted-foreground text-xs'>
                          {t('Cache Write (1h)')}:{' '}
                          {nativePrice(tier.cacheCreate1hPrice)}
                        </p>
                      )}
                      {typeof tier.cacheCreate12hPrice === 'number' && (
                        <p className='text-muted-foreground text-xs'>
                          {t('Cache Write (12h)')}:{' '}
                          {nativePrice(tier.cacheCreate12hPrice)}
                        </p>
                      )}
                    </div>
                  ),
                },
                {
                  id: 'request',
                  header: t('Per request'),
                  cell: (tier) => nativePrice(tier.fixedPrice),
                },
              ]}
            />
          )}
          {side.expression && (
            <details>
              <summary className='cursor-pointer text-sm'>
                {t('Billing expression')}
              </summary>
              <pre className='bg-muted mt-2 max-h-48 overflow-auto rounded p-2 text-xs break-all whitespace-pre-wrap'>
                {side.expression}
              </pre>
            </details>
          )}
        </>
      )}
    </section>
  )
}
