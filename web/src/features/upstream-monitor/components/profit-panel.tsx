import { Link2 } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { StaticDataTable } from '@/components/data-table'
import { EmptyState } from '@/components/empty-state'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { parseGroupsList } from '@/features/channels/lib/channel-utils'
import { toIntlLocale } from '@/i18n/languages'
import { formatLocalCurrencyAmount } from '@/lib/currency'
import { formatNumber, formatTimestamp } from '@/lib/format'
import { cn } from '@/lib/utils'

import { monitorGroupLabel } from '../lib/groups'
import { latestMonitorSnapshots } from '../lib/jobs'
import {
  formatProfitPercent,
  hasLossMakingTier,
  resolveProfitSelection,
  sortProfitRows,
  summarizeProfit,
  upstreamSelectionKey,
} from '../lib/profit'
import { formatMonitorMessage } from '../lib/task-messages'
import type {
  MonitorProfitDetailRequest,
  MonitorProfitRow,
  MonitorState,
} from '../types'
import { MonitorSelect } from './form-fields'
import { ProfitDetail } from './profit-detail'

export const profitColumnsClassName =
  'grid min-h-0 gap-2 lg:flex-1 lg:grid-cols-[20fr_20fr_60fr] lg:grid-rows-[minmax(0,1fr)]'

export function ProfitPanel(props: {
  state: MonitorState
  onSnapshot: (id: string) => void
  onBind?: (group: string) => void
}) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const [localGroup, setLocalGroup] = useState('')
  const [upstreamKey, setUpstreamKey] = useState('')
  const [search, setSearch] = useState('')
  const [lossOnly, setLossOnly] = useState(false)
  const [allModels, setAllModels] = useState(false)
  const [order, setOrder] = useState('margin_desc')
  const [detail, setDetail] = useState<MonitorProfitDetailRequest | null>(null)
  const rows = props.state.profit_rows ?? []
  const selection = resolveProfitSelection(rows, localGroup, upstreamKey)
  const groupRows = rows.filter(
    (row) => row.local_group === selection.localGroup
  )
  const selectedRows = groupRows.filter(
    (row) => upstreamSelectionKey(row) === selection.upstreamKey
  )
  const filtered = selectedRows.filter(
    (row) =>
      `${row.local_model} ${row.upstream_model} ${row.channel_name}`
        .toLowerCase()
        .includes(search.toLowerCase()) &&
      (!lossOnly || hasLossMakingTier(row))
  )
  const summary = summarizeProfit(groupRows)
  const localGroups = [...new Set(rows.map((row) => row.local_group))]
  const upstreams = [
    ...new Map(
      groupRows.map((row) => [upstreamSelectionKey(row), row])
    ).entries(),
  ]
  const currency = props.state.config.scenario.currency
  const money = (value: number | null) =>
    value === null
      ? '—'
      : formatLocalCurrencyAmount(value, {
          locale,
          showSymbol: false,
          fullPrecision: true,
        })
  const percent = (value: number | null) =>
    value === null
      ? '—'
      : formatNumber(value, locale, {
          style: 'percent',
          fullPrecision: true,
          showSymbol: false,
        })
  const selectedAccount = props.state.config.accounts.find(
    (account) => account.id === selectedRows[0]?.account_id
  )
  const latestByAccount = latestMonitorSnapshots(props.state.snapshots)
  const latest = latestByAccount.get(selectedAccount?.id ?? '')
  const unconnectedModels =
    latest?.data.models.filter(
      (model) =>
        model.group === selectedRows[0]?.upstream_group &&
        !selectedRows.some((row) => row.upstream_model === model.name) &&
        model.name.toLowerCase().includes(search.toLowerCase())
    ) ?? []
  return (
    <div className='flex min-h-0 flex-col gap-2 lg:h-full'>
      <section
        className='grid shrink-0 grid-cols-2 gap-2 sm:grid-cols-3 xl:grid-cols-6'
        aria-label={t('Scenario summary')}
      >
        {[
          {
            label: t('Scenario net revenue'),
            unit: currency,
            value: money(summary.revenue),
          },
          {
            label:
              summary.estimatedCount > 0
                ? t('Estimated procurement cost')
                : t('Scenario purchase cost'),
            unit: currency,
            value: money(summary.cost),
          },
          {
            label:
              summary.estimatedCount > 0
                ? t('Estimated profit')
                : t('Estimated gross profit'),
            unit: currency,
            value: money(summary.profit),
          },
          {
            label:
              summary.estimatedCount > 0
                ? t('Estimated margin')
                : t('Estimated gross margin'),
            unit: '%',
            value: percent(summary.margin),
          },
          {
            label: t('Cost data coverage'),
            unit: '%',
            value: percent(summary.coverage),
          },
          {
            label: t('Loss-making routes'),
            value: formatNumber(summary.lossCount, locale),
          },
        ].map((item) => (
          <div
            key={item.label}
            className='min-w-0 space-y-1 rounded-xl border px-3 py-2'
          >
            <p className='text-muted-foreground text-xs'>
              <span>{item.label}</span>
              {item.unit && (
                <>
                  {' '}
                  <span>({item.unit})</span>
                </>
              )}
            </p>
            <p className='font-semibold wrap-anywhere tabular-nums'>
              {item.value}
            </p>
          </div>
        ))}
      </section>
      {summary.estimatedCount > 0 && (
        <Alert role='note' className='shrink-0'>
          <AlertDescription className='flex flex-wrap items-center gap-2'>
            <Badge
              variant='outline'
              className='h-auto max-w-full whitespace-normal'
            >
              {t('Official price estimates: {{number}}', {
                number: summary.estimatedCount,
              })}
            </Badge>
            {t(
              'Totals include official price estimates. Pending comparisons remain excluded.'
            )}
          </AlertDescription>
        </Alert>
      )}
      {!rows.length ? (
        <EmptyState
          title={t('No procurement comparisons yet')}
          description={t(
            'Compare channel revenue with upstream procurement costs using captured account data and configured model correspondences.'
          )}
        />
      ) : (
        <div className={profitColumnsClassName} data-testid='profit-columns'>
          <section
            className='flex min-h-0 min-w-0 flex-col rounded-xl border'
            aria-label={t('Local groups')}
          >
            <h3 className='flex shrink-0 items-center justify-between gap-2 border-b px-3 py-2 text-sm font-medium'>
              <span>{t('Local groups')}</span>
              <Badge variant='secondary' className='tabular-nums'>
                {formatNumber(localGroups.length, locale)}
              </Badge>
            </h3>
            <div className='max-h-72 min-h-0 space-y-1 overflow-y-auto overscroll-contain p-2 lg:max-h-none lg:flex-1'>
              {localGroups.map((group) => (
                <div
                  key={group}
                  className={cn(
                    'flex min-w-0 items-center rounded-md',
                    selection.localGroup === group && 'bg-secondary'
                  )}
                >
                  <Button
                    variant='ghost'
                    className='h-auto min-w-0 flex-1 justify-start px-3 py-2 text-left whitespace-normal'
                    aria-pressed={selection.localGroup === group}
                    onClick={() => {
                      setLocalGroup(group)
                      setUpstreamKey('')
                    }}
                  >
                    <span className='min-w-0'>
                      <span className='block break-all'>{group}</span>
                      <span className='text-muted-foreground block text-xs'>
                        {t('Ratio {{ratio}} · {{count}} routes', {
                          ratio: formatNumber(
                            rows.find((row) => row.local_group === group)
                              ?.local_ratio,
                            locale,
                            { fullPrecision: true }
                          ),
                          count: rows.filter((row) => row.local_group === group)
                            .length,
                        })}
                      </span>
                    </span>
                  </Button>
                  {props.onBind && (
                    <Tooltip>
                      <TooltipTrigger
                        render={
                          <Button
                            variant='ghost'
                            size='icon-sm'
                            className='mr-1 shrink-0'
                            aria-label={t('Bind channels in {{group}}', {
                              group,
                            })}
                            disabled={
                              !props.state.config.accounts.length ||
                              !props.state.channels.some((channel) =>
                                parseGroupsList(channel.group).includes(group)
                              )
                            }
                            onClick={() => props.onBind?.(group)}
                          >
                            <Link2 className='size-4' />
                          </Button>
                        }
                      />
                      <TooltipContent>{t('Bind')}</TooltipContent>
                    </Tooltip>
                  )}
                </div>
              ))}
            </div>
          </section>
          <section
            className='flex min-h-0 min-w-0 flex-col rounded-xl border'
            aria-label={t('Upstream groups')}
          >
            <h3 className='flex shrink-0 items-center justify-between gap-2 border-b px-3 py-2 text-sm font-medium'>
              <span>{t('Upstream groups')}</span>
              <Badge variant='secondary' className='tabular-nums'>
                {formatNumber(upstreams.length, locale)}
              </Badge>
            </h3>
            <div className='max-h-72 min-h-0 space-y-1 overflow-y-auto overscroll-contain p-2 lg:max-h-none lg:flex-1'>
              {upstreams.map(([key, row]) => {
                const accountName =
                  props.state.config.accounts.find(
                    (account) => account.id === row.account_id
                  )?.name ?? row.account_id
                const showAccountName =
                  accountName && accountName.trim() !== row.site_name.trim()
                return (
                  <Button
                    key={key}
                    variant={
                      selection.upstreamKey === key ? 'secondary' : 'ghost'
                    }
                    className='h-auto w-full justify-start px-3 py-2 text-left whitespace-normal'
                    aria-pressed={selection.upstreamKey === key}
                    onClick={() => setUpstreamKey(key)}
                  >
                    <span className='min-w-0'>
                      <span className='block break-all'>
                        {row.account_id
                          ? monitorGroupLabel(
                              row.upstream_group,
                              latestByAccount
                                .get(row.account_id)
                                ?.data.groups.find(
                                  (group) => group.name === row.upstream_group
                                )?.display_name
                            )
                          : t('Missing upstream binding')}
                      </span>
                      {showAccountName && (
                        <span className='text-muted-foreground block text-xs break-all'>
                          {accountName}
                        </span>
                      )}
                      <span className='text-muted-foreground block text-xs'>
                        {row.site_name && `${row.site_name} · `}
                        {t('Upstream ratio {{ratio}}', {
                          ratio: formatNumber(row.upstream_ratio, locale, {
                            fullPrecision: true,
                          }),
                        })}
                      </span>
                    </span>
                  </Button>
                )
              })}
            </div>
          </section>
          <section
            className='flex min-h-0 min-w-0 flex-col gap-2 rounded-xl border p-2'
            aria-label={t('Models and estimated profit')}
          >
            <h3 className='shrink-0 text-sm font-medium'>
              {t('Models and estimated profit')}
            </h3>
            <div className='flex shrink-0 flex-wrap items-center gap-2'>
              <div className='grid min-w-0 flex-1 basis-full grid-cols-2 gap-2 sm:min-w-80 sm:basis-auto'>
                <Input
                  value={search}
                  onChange={(event) => setSearch(event.target.value)}
                  placeholder={t('Filter models or channels...')}
                  aria-label={t('Filter models or channels...')}
                />
                <MonitorSelect
                  value={order}
                  onChange={setOrder}
                  label={t('Sort model comparisons')}
                  options={[
                    { value: 'margin_desc', label: t('Highest margin first') },
                    { value: 'margin_asc', label: t('Lowest margin first') },
                    { value: 'cost_asc', label: t('Lowest cost first') },
                    { value: 'cost_desc', label: t('Highest cost first') },
                  ]}
                />
              </div>
              <div className='flex items-center gap-2'>
                <Switch
                  id='monitor-loss-only'
                  checked={lossOnly}
                  onCheckedChange={setLossOnly}
                />
                <Label htmlFor='monitor-loss-only'>{t('Losses only')}</Label>
              </div>
              <div className='flex items-center gap-2'>
                <Switch
                  id='monitor-all-models'
                  checked={allModels}
                  onCheckedChange={setAllModels}
                />
                <Label htmlFor='monitor-all-models'>
                  {t('All collected upstream models')}
                </Label>
              </div>
            </div>
            <div
              className='min-h-0 space-y-3 overflow-y-auto overscroll-contain lg:flex-1'
              data-testid='profit-model-scroll'
            >
              <StaticDataTable
                data={sortProfitRows(filtered, order)}
                getRowKey={(row) =>
                  JSON.stringify([
                    row.channel_id,
                    row.local_model,
                    row.account_id,
                    row.upstream_group,
                    row.binding_id,
                  ])
                }
                emptyContent={t('No models match these filters.')}
                tableClassName='[&_tbody>tr]:h-auto'
                columns={[
                  {
                    id: 'model',
                    header: t('Model and source'),
                    className: 'w-32',
                    cellClassName: 'min-w-28 whitespace-normal',
                    cell: (row) => (
                      <div className='flex flex-wrap items-baseline gap-x-2 wrap-anywhere'>
                        <span className='font-medium'>{row.local_model}</span>
                        {row.estimated && (
                          <Badge
                            variant='outline'
                            className='h-auto max-w-full whitespace-normal'
                          >
                            {t('Official price estimate')}
                          </Badge>
                        )}
                        {row.upstream_model &&
                          row.upstream_model !== row.local_model && (
                            <span className='text-muted-foreground text-xs'>
                              {t('Upstream')}: {row.upstream_model}
                            </span>
                          )}
                      </div>
                    ),
                  },
                  {
                    id: 'margin',
                    header: t('Estimated gross margin'),
                    cellClassName: 'whitespace-normal',
                    cell: (row) => <ProfitTierMargins row={row} />,
                  },
                  {
                    id: 'source',
                    header: t('Evidence'),
                    cellClassName: 'whitespace-normal',
                    cell: (row) => (
                      <div className='space-y-1'>
                        <div className='flex flex-wrap items-center gap-1'>
                          <span className='text-muted-foreground text-xs'>
                            {row.captured_at
                              ? formatTimestamp(row.captured_at)
                              : t('Not collected')}
                          </span>
                          {row.captured_at > 0 &&
                            Date.now() - row.captured_at * 1000 > 86400000 && (
                              <Badge variant='outline'>
                                {t('Stale snapshot')}
                              </Badge>
                            )}
                        </div>
                        {(row.snapshot_id || row.binding_id) && (
                          <div className='flex flex-nowrap items-center gap-2'>
                            {row.snapshot_id && (
                              <Button
                                variant='outline'
                                size='sm'
                                onClick={() =>
                                  props.onSnapshot(row.snapshot_id)
                                }
                              >
                                {t('View snapshot')}
                              </Button>
                            )}
                            {row.binding_id && (
                              <Button
                                variant='outline'
                                size='sm'
                                onClick={() =>
                                  setDetail({
                                    binding_id: row.binding_id ?? '',
                                    local_group: row.local_group,
                                    local_model: row.local_model,
                                  })
                                }
                              >
                                {t('Profit details')}
                              </Button>
                            )}
                          </div>
                        )}
                      </div>
                    ),
                  },
                ]}
              />
              {allModels && !lossOnly && (
                <section className='space-y-2'>
                  <h4 className='text-sm font-medium'>
                    {t('Collected models not connected to this local group')}
                  </h4>
                  <StaticDataTable
                    data={unconnectedModels}
                    getRowKey={(model) => `${model.group}:${model.name}`}
                    emptyContent={t(
                      'No additional upstream models in the confirmed snapshot.'
                    )}
                    columns={[
                      {
                        id: 'name',
                        header: t('Upstream model'),
                        cell: (model) => (
                          <span className='break-all'>{model.name}</span>
                        ),
                      },
                      {
                        id: 'unit',
                        header: t('Billing unit'),
                        cell: (model) => (
                          <span>
                            {model.billing_unit} · {model.currency}
                          </span>
                        ),
                      },
                      {
                        id: 'status',
                        header: t('Status'),
                        cell: () => (
                          <Badge variant='outline'>
                            {t('Not connected; excluded from profit totals')}
                          </Badge>
                        ),
                      },
                    ]}
                  />
                </section>
              )}
            </div>
          </section>
        </div>
      )}
      {detail && (
        <ProfitDetail request={detail} onClose={() => setDetail(null)} />
      )}
    </div>
  )
}

function ProfitTierMargins(props: { row: MonitorProfitRow }) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const samples = props.row.tier_samples ?? []
  const localTiers = new Set(samples.map((sample) => sample.local_tier))
  const upstreamTiers = new Set(samples.map((sample) => sample.upstream_tier))
  const showLocalOnly =
    samples.length > 1 &&
    localTiers.size === samples.length &&
    !localTiers.has('') &&
    !localTiers.has(undefined)
  const showUpstreamOnly =
    !showLocalOnly &&
    samples.length > 1 &&
    upstreamTiers.size === samples.length &&
    !upstreamTiers.has('') &&
    !upstreamTiers.has(undefined)
  return (
    <div className='flex flex-wrap items-center gap-1.5 text-xs tabular-nums'>
      {props.row.estimated && (
        <span className='text-muted-foreground basis-full'>
          {t('Estimated margin')}
        </span>
      )}
      {(samples.length ? samples : [props.row]).map((sample) => {
        let upstream = sample.upstream_tier
        const threshold = /^(context|input) > (\d+)$/.exec(upstream ?? '')
        if (upstream === 'base') upstream = t('Base tier')
        if (threshold) {
          const tokens = formatNumber(Number(threshold[2]), locale)
          upstream =
            threshold[1] === 'context'
              ? t('Context > {{tokens}}', { tokens })
              : t('Uncached input > {{tokens}}', { tokens })
        }
        const fullLabel = [sample.local_tier, upstream]
          .filter(Boolean)
          .join(' / ')
        let label = fullLabel
        if (showLocalOnly && sample.local_tier) label = sample.local_tier
        else if (showUpstreamOnly && upstream) label = upstream
        if (
          'context_tokens' in sample &&
          (!label ||
            samples.some(
              (other) =>
                other !== sample &&
                other.local_tier === sample.local_tier &&
                other.upstream_tier === sample.upstream_tier
            ))
        ) {
          const context = formatNumber(sample.context_tokens, locale)
          label = label ? `${label} · ${context}` : context
        }
        const reason = sample.reason
          ? formatMonitorMessage(sample.reason, t)
          : undefined
        return (
          <Badge
            key={'input_tokens' in sample ? sample.input_tokens : 'scenario'}
            title={[
              fullLabel,
              'context_tokens' in sample
                ? `${t('Full input context')}: ${formatNumber(sample.context_tokens, locale)}`
                : '',
              reason,
            ]
              .filter(Boolean)
              .join('\n')}
            variant={
              sample.profit !== null && sample.profit < 0
                ? 'destructive'
                : 'outline'
            }
            className='block h-auto max-w-full min-w-0 wrap-anywhere whitespace-normal'
          >
            {label && <span className='text-muted-foreground'>{label} · </span>}
            {formatProfitPercent(sample.margin, locale)}
            {reason && sample.reason !== props.row.reason && (
              <span className='sr-only'>{reason}</span>
            )}
          </Badge>
        )
      })}
      {props.row.reason && (
        <p className='text-muted-foreground min-w-0 basis-full wrap-anywhere'>
          {formatMonitorMessage(props.row.reason, t)}
        </p>
      )}
    </div>
  )
}
