export type FeeBearer = 'self' | 'other' | 'unknown'
export interface MonitorSite {
  id: string
  name: string
  origin: string
  allowed_origins: string[]
  currency: string
  fx_rate: number
}
export interface MonitorOfficialEstimatePolicy {
  catalog_version: string
  models: { name: string; group: string; entry_id: string }[]
}
export interface MonitorAccount {
  official_estimate?: MonitorOfficialEstimatePolicy
  id: string
  site_id: string
  name: string
  username: string
  external_id: string
  password?: string
  has_password: boolean
  clear_password?: boolean
  device_id: string
  flow_id: string
  interval_minutes: number
  enabled: boolean
  purchase_factor: number
  fee_rate: number
  fee_bearer: FeeBearer
}
export type MonitorAction =
  | 'navigate'
  | 'click'
  | 'check'
  | 'input'
  | 'select'
  | 'key'
  | 'wait'
  | 'scroll'
  | 'checkpoint'
  | 'focus'
  | 'assert'
  | 'wait_response'
  | 'condition'
  | 'repeat'
export type MonitorStateCondition =
  | 'visible'
  | 'hidden'
  | 'enabled'
  | 'disabled'
export interface MonitorCondition {
  selector?: string
  url?: string
  state?: MonitorStateCondition
  text?: string
}
export interface MonitorStep {
  id?: string
  name?: string
  action: MonitorAction
  url?: string
  selector?: string
  text?: string
  value?: string
  variable?: string
  timeout_ms?: number
  group?: string
  request_id?: string
  state?: MonitorStateCondition
  condition?: MonitorCondition
  steps?: MonitorStep[]
  otherwise?: MonitorStep[]
  max_iterations?: number
}
export interface MonitorFieldMapping {
  kind: 'path' | 'parent' | 'constant' | 'key' | 'group'
  translate?: Record<string, string>
  path?: string
  fallback_path?: string
  source?: string
  value?: string | number | boolean | null
  multiply?: MonitorFieldMapping
  divide?: MonitorFieldMapping
  scale?: number
}
type MonitorTierMode = 'whole_request' | 'graduated'
type MonitorTierBasis = 'context' | 'input'
type MonitorTierField =
  | 'input_threshold'
  | 'input'
  | 'output'
  | 'cache_read'
  | 'cache_write'

export interface MonitorBillingMapping {
  rules: {
    match: 'all' | 'any'
    conditions: {
      path: string
      op: 'gt' | 'gte' | 'eq' | 'lt' | 'lte' | 'exists' | 'missing'
      value?: number
    }[]
    unit: 'token' | 'request' | 'unsupported'
    request_price?: { kind: 'path'; path: string; scale?: number }
    notes?: string
  }[]
}

export interface MonitorExtraction {
  version: 1
  adapter?: 'newapi' | 'sub2api'
  requests: {
    id: string
    url: string
    method: string
    query?: Record<string, string>
    page_urls?: string[]
  }[]
  mappings: {
    id: string
    request_id: string
    target: 'account_id' | 'balances' | 'groups' | 'models' | 'topups'
    root: string
    iterate: 'single' | 'array' | 'entries' | 'nested_array'
    child_root?: string
    billing?: MonitorBillingMapping
    tiers?: {
      path: string
      mode: MonitorTierMode
      basis: MonitorTierBasis
      fields: Partial<Record<MonitorTierField, MonitorFieldMapping>>
    }
    fields: Record<string, MonitorFieldMapping>
  }[]
}
export interface MonitorFlow {
  id: string
  site_id: string
  name: string
  revision: number
  steps: MonitorStep[]
  login?: {
    url: string
    steps: MonitorStep[]
    timeout_ms?: number
    success?: MonitorCondition
  }
  capture_seconds: number
  extraction?: MonitorExtraction
}
export interface MonitorBinding {
  id: string
  channel_id: number
  account_id: string
  upstream_group: string
  local_model: string
  upstream_model: string
}
export interface MonitorScenario {
  currency: string
  usd_exchange: number
  input_tokens: number
  output_tokens: number
  cache_read: number
  cache_write: number
  requests: number
  revenue_factor: number
  fee_rate: number
  fee_bearer: FeeBearer
  user_group: string
}
export interface MonitorConfig {
  version: number
  sites: MonitorSite[]
  accounts: MonitorAccount[]
  flows: MonitorFlow[]
  bindings: MonitorBinding[]
  scenario: MonitorScenario
}
export interface MonitorBalance {
  amount: number | null
  currency: string
  kind: string
}
export interface MonitorGroup {
  cost_unsupported?: string
  name: string
  display_name?: string
  ratio: number | null
}
export interface MonitorPriceTier {
  input_threshold: number | null
  input: number | null
  output: number | null
  cache_read: number | null
  cache_write: number | null
}
export interface MonitorPriceSource {
  kind: 'official_estimate'
  catalog_version: string
  entry_id: string
  provider: string
  model: string
  source_url: string
  verified_at: string
}
export interface MonitorModelPrice {
  name: string
  group: string
  currency: string
  billing_unit: string
  token_unit: number | null
  input: number | null
  output: number | null
  cache_read: number | null
  cache_write: number | null
  request_price: number | null
  includes_group: boolean | null
  notes: string
  tiers?: MonitorPriceTier[]
  tier_mode?: MonitorTierMode
  tier_basis?: MonitorTierBasis
  billing_expression?: string
  pricing_pending?: boolean
}
export interface MonitorTopup {
  reference: string
  paid: number | null
  credited: number | null
  currency: string
  fee: number | null
  fee_bearer: string
}
export interface MonitorResultSource {
  request_id: string
  url: string
  method: string
  page_url: string
  group: string
  captured_at: number
}
export interface MonitorOfficialCatalogEntry {
  id: string
  provider: string
  model: string
  source_url: string
  verified_at: string
  quote: MonitorModelPrice
}
export interface MonitorOfficialCatalog {
  version: string
  verified_at: string
  entries: MonitorOfficialCatalogEntry[]
}
export interface MonitorData {
  pricing_status?: 'available' | 'partial' | 'unavailable'
  schema_version?: 1
  sources?: MonitorResultSource[]
  missing?: string[]
  account_id: string
  balances: MonitorBalance[]
  groups: MonitorGroup[]
  models: MonitorModelPrice[]
  topups: MonitorTopup[]
  warnings: string[]
}
export interface MonitorBrowserSetup {
  sidebar_open: boolean
  granted_origins: string[]
}

export interface MonitorDevice {
  id: string
  name: string
  last_seen_at: number
  created_at: number
  revoked_at: number
  expires_at: number
}
export type MonitorJobStatus =
  | 'queued'
  | 'running'
  | 'awaiting_user'
  | 'completed'
  | 'failed'
  | 'cancelled'
export interface MonitorJob {
  id: string
  account_id: string
  device_id: string
  flow_id: string
  mode: 'record' | 'collect'
  status: MonitorJobStatus
  step: number
  message: string
  error?: string
  created_at: number
  updated_at: number
  complete: boolean
  control?: string
  config_version?: number
  flow_revision?: number
  lease_until?: number
}
export interface MonitorSnapshot {
  id: string
  job_id: string
  account_id: string
  data: MonitorData
  confirmed: boolean
  created_at: number
  confirmed_at?: number
  complete: boolean
}
export interface MonitorProfitRow {
  estimated: boolean
  price_source?: MonitorPriceSource
  binding_id?: string
  local_group: string
  local_ratio: number
  channel_id: number
  channel_name: string
  account_id: string
  site_name: string
  upstream_group: string
  upstream_ratio: number | null
  local_model: string
  upstream_model: string
  local_tier?: string
  upstream_tier?: string
  revenue: number | null
  cost: number | null
  profit: number | null
  margin: number | null
  status: string
  reason: string
  snapshot_id: string
  captured_at: number
  tier_samples?: MonitorProfitTierSample[]
}
export interface MonitorProfitSide {
  currency: string
  unit?: 'token' | 'request' | 'expression'
  token_unit?: number
  tier?: string
  expression?: string
  tier_mode?: MonitorTierMode
  tier_basis?: MonitorTierBasis
  tiers?: MonitorPriceTier[]
  base_prices?: MonitorPriceTier
  includes_group?: boolean
  base_amount: number | null
  group_ratio: number | null
  fx_rate: number
  factor: number
  fee_rate: number
  fee_bearer: FeeBearer
  fee_multiplier: number | null
  amount: number | null
}

export interface MonitorProfitComponent {
  kind: 'input' | 'output' | 'cache_read' | 'cache_write' | 'request'
  quantity: number
  local_unit_price: number | null
  upstream_unit_price: number | null
  unit_profit: number | null
  unit_margin: number | null
  revenue: number | null
  cost: number | null
}

export interface MonitorProfitDetailRequest {
  binding_id: string
  local_group: string
  local_model: string
  input_tokens?: number
}

export type MonitorProfitTierSample = Pick<
  MonitorProfitRow,
  | 'local_tier'
  | 'upstream_tier'
  | 'revenue'
  | 'cost'
  | 'profit'
  | 'margin'
  | 'status'
  | 'reason'
> & {
  input_tokens: number
  context_tokens: number
  components?: MonitorProfitComponent[]
}

export interface MonitorProfitDetail {
  row: MonitorProfitRow
  scenario: MonitorScenario
  context_tokens: number
  local?: MonitorProfitSide
  upstream?: MonitorProfitSide & {
    estimated?: boolean
    price_source?: MonitorPriceSource
  }
  components: MonitorProfitComponent[]
  tier_samples?: MonitorProfitTierSample[]
  per_million_context?: {
    revenue: number
    cost: number
    profit: number
    margin: number | null
  }
}

export interface MonitorState {
  official_catalogs?: MonitorOfficialCatalog[]
  official_catalog_latest_version?: string
  config: MonitorConfig
  devices: MonitorDevice[]
  jobs: MonitorJob[]
  snapshots: MonitorSnapshot[]
  channels: {
    id: number
    name: string
    group: string
    models: string
    model_mapping?: string
  }[]
  groups: Record<string, number>
  profit_rows: MonitorProfitRow[]
  key_ready: boolean
}
export interface MonitorJobDetail {
  job: MonitorJob
  recorded_steps: MonitorStep[]
  snapshot?: MonitorSnapshot
}
export type SaveMonitorConfig = (config: MonitorConfig) => Promise<void>
