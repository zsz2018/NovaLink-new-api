import type { TFunction } from 'i18next'

// Exact executor/server messages only. Stored messages are translated when read;
// unknown upstream errors and operator text must never become i18next input.
const fixedMonitorMessages = new Set([
  'Monitor snapshots must contain captured prices only',
  'Upstream group cost comparison is unsupported',
  'Map account identity, balances and groups before collecting',
  'Invalid monitor official estimate policy',
  'Invalid monitor official estimate price',
  'Monitor official catalog is unavailable',
  'Standard global API text-token estimate. Cache writes: 30m; expression also retains 12h. Tools, media, batch and priority prices excluded.',
  'Standard global first-party text-token estimate. Cache writes: 5m; expression also retains 1h. Tools, fast, batch, regional and media prices excluded.',
  'The picker page is not authorized',
  'The page moved outside the task origins',
  'Iframe interaction requires an explicit supported frame',
  'Saved credentials require an explicit login page binding',
  'The login page binding requires HTTPS or loopback and no query or fragment',
  'The page moved outside the configured login page',
  'Choose an explicit target element',
  'Login actions require an explicit selector',
  'Invalid XPath element locator',
  'XPath must select an HTML element',
  'Invalid CSS element locator',
  'Invalid semantic element locator',
  'Invalid semantic scope CSS locator',
  'The semantic scope is not unique',
  'The selected control is not unique',
  'Iframe controls are not supported by this flow',
  'The login form submits outside its configured origin',
  'The login link leaves the approved origins',
  'The input target is not a text field',
  'The input field is not editable',
  'The saved password requires a password field',
  'The saved username requires an account field',
  'The condition URL is outside the task origins',
  'Saved credentials must be supplied through the bound credential variable',
  'Unsupported element state',
  'The target element changed before the action',
  'The target element could not receive focus',
  'The target element changed while receiving focus',
  'An element state requires an explicit selector',
  'The page element did not match the asserted state and expected text',
  'Timed out waiting for the page element and expected text',
  'Invalid scroll amount',
  'A login preparation click requires a non-submit button',
  'The selected control is not a checkbox',
  'The selected checkbox did not become checked',
  'The input field cannot be updated',
  'The input field did not retain its value after rendering',
  'The target is not a native select',
  'The requested option does not exist or is disabled',
  'The select cannot be updated',
  'The select did not retain its value after rendering',
  'Unsupported key',
  'There is no next focusable element',
  'The form submit control is unavailable',
  'Unsupported page action',
  'A condition requires a URL or explicit selector',
  'Element conditions require an explicit selector',
  'NewAPI TOKENS display normalized to USD using captured quota_per_unit; model pricing is denominated in USD.',
  'NewAPI plugin usage or provider variants cannot be represented by one ordinary token/request quote; excluded from procurement.',
  'Captured ordinary_pricing=true: ordinary pricing only; plugin provider variants are excluded.',
  'NewAPI endpoint pricing is outside the supported ordinary token/request schema; excluded from procurement.',
  'NewAPI image/audio pricing dimensions cannot be represented losslessly by this ordinary quote; excluded from procurement.',
  'Conflicting captured NewAPI model quotes; pricing remains pending.',
  'Captured NewAPI response exceeds the supported body limit',
  'NewAPI response must explicitly report success: true',
  'Captured NewAPI response exceeds the supported nesting limit',
  'Captured NewAPI response has duplicate JSON object keys',
  'Selected NewAPI responses contain conflicting values',
  'NewAPI CUSTOM currency requires an exact recognized ISO currency code; a symbol is ambiguous',
  'Captured NewAPI quota_display_type is missing or unsupported',
  'Captured NewAPI display currency conversion is missing or invalid',
  'NewAPI monetary conversion exceeds the supported numeric bounds',
  'NewAPI model row has an unsupported pricing contract version',
  'NewAPI row contains a price shape outside the verified native response contract',
  'NewAPI plugin billing metadata is malformed',
  'NewAPI supported endpoint metadata is malformed',
  'Captured NewAPI tiered_expr requires a valid bounded v1 billing_expr',
  'NewAPI billing mode or expression does not match the supported ordinary schema',
  'Captured NewAPI modality ratios are malformed or outside supported numeric bounds',
  'NewAPI quota_type must explicitly be 0 (token) or 1 (request)',
  'Captured NewAPI price or ratio is missing, invalid, or out of bounds',
  'NewAPI requires a complete successful GET capture from the expected endpoint',
  'NewAPI capture timestamp must be a positive safe integer',
  'Captured NewAPI response is not valid JSON',
  'Expected selected NewAPI response was not captured',
  'NewAPI data must be an object',
  'Captured NewAPI self.id must be a positive safe integer',
  'Captured NewAPI account identity does not match the configured account',
  'Captured NewAPI self.group must be a bounded canonical group key',
  'Captured NewAPI display currency does not match the site currency',
  'Captured NewAPI self.quota must be a safe integer',
  'Captured NewAPI quota_per_unit must be finite and positive',
  'Captured NewAPI balance is overdrawn; the result validator does not support negative balances',
  'Unsupported NewAPI pricing_version; only the verified current response contract is supported',
  'NewAPI usable_group and group_ratio must be bounded objects',
  'Captured NewAPI pricing usable_group does not include the authenticated self.group',
  'NewAPI usable_group requires bounded group names and string descriptions',
  'A usable NewAPI group has no captured group_ratio',
  'Captured NewAPI group_ratio must be finite and non-negative',
  'Selected NewAPI responses contain conflicting canonical group rows',
  'Captured NewAPI group rows exceed the result limit',
  'NewAPI pricing data must be a bounded array',
  'Captured NewAPI pricing contains a malformed model row or invalid model name',
  'One NewAPI pricing response contains duplicate model names',
  'NewAPI model enable_groups must be a bounded string array',
  'NewAPI enable_groups contains duplicate canonical keys',
  'A captured NewAPI model has no enabled group with a usable captured multiplier',
  'Selected NewAPI responses contain conflicting or duplicate model quotes',
  'Expanded NewAPI model rows exceed the result limit',
  'Network request did not finish',
  'Response body is unavailable',
  'Response exceeds the per-resource limit',
  'The response is not valid JSON',
  'Sanitized response exceeds the per-resource limit',
  'Task response budget exceeded',
  'Response body collection failed',
  'No confirmed complete upstream snapshot',
  'Upstream model price is missing',
  'Missing upstream binding',
  'Confirm which party bears the recharge fees',
  'Upstream pricing rules need confirmation',
  'Confirm upstream price currency and whether it includes the group ratio',
  'Upstream group ratio is unknown',
  'Scenario amount is outside the supported range',
  'This model already uses an expression.',
  'Gemini and OpenAI audio prices differ for this model. Use separate billing model names to convert them.',
  'Task pricing must be converted manually using the task usage schema.',
  'Video pricing must be converted manually.',
  'This OpenRouter Claude price derives cache-write usage from upstream cost and must be converted manually.',
  'The model routing configuration could not be verified.',
  'Realtime pricing must be converted manually.',
  'Configure an input price before converting this model.',
  'Local model pricing is not configured',
  'Local model expression is missing',
  'Invalid upstream price tiers',
  'Invalid upstream billing expression',
  'Invalid scenario request count',
  'Invalid scenario token count',
  'Local billing needs unsupported scenario input',
  'Local billing expression could not be evaluated',
  'Upstream billing needs unsupported scenario input',
  'Upstream billing expression could not be evaluated',
  'Upstream request price is unknown',
  'Upstream billing rule or token unit is unsupported',
  'A required upstream unit price is unknown',
  'Invalid monitor request',
  'Procurement route is no longer available',
  'Browser lease expired',
  'Unable to load execution credentials',
  'Task exceeded its 30-minute execution limit',
  'Browser executor restarted; interrupted task was stopped',
  'Browser worker restarted; interrupted task was stopped to prevent duplicate login',
  'Task was cancelled',
  'Task is no longer active',
  'Device is disconnected',
  'Task data could not be uploaded; retry from the admin site after checking the connection',
  'Open the extension and grant the task website permissions, then resume from the admin site',
  'Website permissions granted',
  'Complete the website verification and resume from the admin site',
  'Continuing after manual verification',
  'Task tab is unavailable',
  'Task page origin is not authorized',
  'The requested account credential is not configured',
  'The page action did not complete',
  'Open the extension sidebar to configure, record, test and save the flow',
  'Flow was configured and saved from the browser sidebar',
  'Browser flow saved',
  'Task cancelled by the admin site',
  'Browser task failed',
  'Debugger connection was detached; close DevTools and retry from the admin site',
  'The task tab was closed',
  'Could not manage an additional task browser tab',
  'The task tab was moved out of its NovaLink group',
  'The task tab navigated outside its authorized origins',
  'Recording exceeds the step limit',
  'Could not attach the recorder to the task page',
  'Recording paused; edit or test the flow in the sidebar',
  'Flow draft updated in the browser sidebar',
  'Recording page actions; pause and save from the browser sidebar',
  'A recorded step could not be saved safely; inspect the flow from the admin site',
  'Navigation URL is not authorized for this task',
  'Navigation failed to load a website document',
  'Page redirected outside the configured site origins',
  'Timed out waiting for navigation',
  'Temporary response preview is unavailable; visit the page again',
  'Captured identity is unavailable; identify the requests again',
  'Source facts are unavailable; collect the data again',
  'Open a regular browser window before starting a task',
  'Could not create a task tab',
  'Could not close all task-owned browser tabs',
  'Task opened too many browser tabs',
  'URL must not contain credentials',
  'Only HTTP(S) sites are supported',
  'Invalid origin',
  'Invalid task identity',
  'Task site mismatch',
  'Invalid step',
  'XPath element locator is empty',
  'Unsupported action',
  'Unsupported step field',
  'Invalid step field',
  'Step URL is outside the task origins',
  'Navigation requires a URL',
  'Waiting for text requires an explicit selector',
  'Invalid step state',
  'Unsupported credential variable',
  'Credentials can only be used by input steps',
  'A credential step cannot contain a literal value',
  'Invalid step timeout',
  'Choose a target element',
  'Invalid child steps',
  'Step exceeds its size limit',
  'Secret and one-time verification inputs must be handled manually',
  'Flow fields must not contain account passwords; use the password variable',
  'Login page origin is not authorized',
])

// Source-authored Chinese notes are aliases only; never rewrite the snapshot.
const fixedMonitorMessageAliases: Record<string, string> = {
  '所列为模型基础报价；搜索、抓取等额外工具费用未包含在此报价中。':
    'The quote is the base model price; additional search, fetch and other tool charges are excluded.',
  '原生计价表达式需要币种换算，当前不能无损转换；请核对上游计价口径。':
    'The native billing expression requires currency conversion that cannot currently be performed losslessly; verify the upstream pricing basis.',
  '价格响应未能关联到当前账户的登录授权。请在任务页登录并重新访问模型广场和账户页，再识别采集来源。':
    'The pricing response could not be linked to the current account login. Sign in on the task page and revisit the model plaza and account page before identifying sources again.',
  '尚未取得当前账户的可用分组与倍率。':
    'Available groups and multipliers for the current account have not been captured.',
  '尚未取得当前账户可用分组的模型报价。':
    "Model quotes for the current account's available groups have not been captured.",
  'Sub2API: 按请求计费的上下文或规格档位尚不能表示为单次固定价格':
    'Sub2API: Per-request context or specification tiers cannot be represented as one fixed price per call',
  'Sub2API: 缓存时长或图片 Token 使用不同单价，不能合并为单一文本及缓存价格':
    'Sub2API: Different cache durations or image tokens have different prices and cannot be combined into one text and cache price',
  'Sub2API: 上下文档位包含不同缓存时长的价格，不能合并为单一缓存价格':
    'Sub2API: Context tiers have different cache-duration prices and cannot be combined into one cache price',
  'Sub2API 采购报价待补全或确认':
    'Sub2API procurement quote is incomplete or requires confirmation',
}

const sub2APIWarningClauses: Record<string, string> = {
  'Sub2API simple 模式不提供此钱包扣款成本':
    'Sub2API simple mode does not provide wallet deduction cost',
  '订阅 USD 额度属于周期使用权益，不是钱包现金采购成本':
    'Subscription USD quota is a periodic usage entitlement, not a cash wallet purchase cost',
  按图片张数计费: 'Image-count billing',
  视频计费维度: 'Video billing dimension',
  当前适配未支持此上游计费单位:
    'This adapter does not support the upstream billing unit',
  订阅高峰时段倍率: 'Subscription peak-hour multiplier',
  'DeepSeek 内置峰谷规则的价格来源及字段覆盖状态未在响应中提供':
    'DeepSeek built-in peak/off-peak price source and field override state are absent from the response',
  按时区及时间段变化的模型倍率:
    'Model multiplier varies by time zone and time period',
  按推理强度变化的模型倍率: 'Model multiplier varies by reasoning effort',
}

const actionKeys: Record<string, string> = {
  navigate: 'Navigate',
  click: 'Click',
  check: 'Check agreement',
  input: 'Input',
  select: 'Select',
  key: 'Press key',
  wait: 'Wait',
  scroll: 'Scroll',
  checkpoint: 'Manual checkpoint',
  focus: 'Focus element',
  assert: 'Assert condition',
  wait_response: 'Wait for selected response',
  condition: 'Conditional branch',
  repeat: 'Repeat while condition matches',
}

// Replace the translated template once. i18next can otherwise expand a later
// variable inside a previously inserted operator name, even with skipOnVariables.
// Callers render this string as React text, which supplies HTML escaping.
function interpolateMonitorMessage(
  key: string,
  t: TFunction,
  values: Record<string, string>
): string {
  return t(key, {
    keySeparator: false,
    nsSeparator: false,
    skipInterpolation: true,
  }).replaceAll(/\{\{(\w+)\}\}/g, (placeholder, variable: string) =>
    Object.hasOwn(values, variable) ? values[variable] : placeholder
  )
}

function formatMonitorStepPath(path: string, t: TFunction): string | undefined {
  if (
    !/^(?:[1-9]\d{0,2}(?:\[(?:[1-9]\d?|100)\]|\.(?:满足|否则))\.){0,3}[1-9]\d{0,2}$/.test(
      path
    ) ||
    [...path.matchAll(/\d+/g)].some(([number]) => Number(number) > 500)
  ) {
    return undefined
  }
  return path
    .replaceAll('满足', t('When condition matches'))
    .replaceAll('否则', t('Otherwise'))
}

function formatMonitorStepFailure(
  message: string,
  t: TFunction
): string | undefined {
  const steps: { path: string; name: string; action: string }[] = []
  let remaining = message
  for (let depth = 0; depth < 4; depth++) {
    const step =
      /^步骤 ([^\p{Cc} ·]{1,100})( · [^\p{Cc}]{1,128}?)?（([a-z_]{1,13})）：([^\p{Cc}]{1,4096})$/u.exec(
        remaining
      )
    if (!step || step[0] !== remaining) break
    const path = formatMonitorStepPath(step[1], t)
    if (!path || !Object.hasOwn(actionKeys, step[3])) return undefined
    steps.push({ path, name: step[2] ?? '', action: t(actionKeys[step[3]]) })
    remaining = step[4]
  }
  if (!steps.length) return undefined
  let key: string | undefined
  if (fixedMonitorMessages.has(remaining)) {
    key = remaining
  } else if (Object.hasOwn(fixedMonitorMessageAliases, remaining)) {
    key = fixedMonitorMessageAliases[remaining]
  }
  if (!key) return undefined
  let result = t(key, { keySeparator: false, nsSeparator: false })
  for (const step of steps.reverse()) {
    result = interpolateMonitorMessage(
      'Step {{path}}{{name}} ({{action}}): {{message}}',
      t,
      { ...step, message: result }
    )
  }
  return result
}

export function formatMonitorMessage(message: string, t: TFunction): string {
  if (fixedMonitorMessages.has(message)) {
    return t(message, { keySeparator: false, nsSeparator: false })
  }

  const alias = Object.hasOwn(fixedMonitorMessageAliases, message)
    ? fixedMonitorMessageAliases[message]
    : undefined
  if (alias) return t(alias, { keySeparator: false, nsSeparator: false })

  const failure = formatMonitorStepFailure(message, t)
  if (failure) return failure

  if (message.startsWith('Sub2API: ')) {
    const clauses = message.slice('Sub2API: '.length).split('；')
    if (
      clauses.length <= 7 &&
      clauses.every((clause) => Object.hasOwn(sub2APIWarningClauses, clause))
    ) {
      return `Sub2API: ${clauses.map((clause) => t(sub2APIWarningClauses[clause], { keySeparator: false, nsSeparator: false })).join('; ')}`
    }
  }

  const currencyNote =
    /^上游显示币种 ([A-Z]{3}) 的换算倍率为 1，原始计价表达式保持不变。$/.exec(
      message
    )
  if (currencyNote && currencyNote[0] === message) {
    return t(
      'Upstream display currency {{currency}} has a conversion rate of 1; the native billing expression is preserved.',
      {
        currency: currencyNote[1],
        keySeparator: false,
        nsSeparator: false,
      }
    )
  }

  const unsupported = /^Unsupported billing unit: ([^\p{Cc}]{1,255})$/u.exec(
    message
  )
  if (
    unsupported &&
    unsupported[0] === message &&
    new TextEncoder().encode(unsupported[1]).byteLength <= 255
  ) {
    return interpolateMonitorMessage('Unsupported billing unit: {{model}}', t, {
      model: unsupported[1],
    })
  }

  const pending =
    /^Conditional pricing pending: ([1-9]\d{0,4}) model quotes$/.exec(message)
  if (pending && pending[0] === message && Number(pending[1]) <= 20_000) {
    return t('Conditional pricing pending: {{number}} model quotes', {
      number: pending[1],
      keySeparator: false,
      nsSeparator: false,
    })
  }

  // The executor emits at most four levels: a numbered step, then a bounded
  // repeat index or one of the two fixed branch names before each child step.
  const progress =
    /^Executing (login )?step ([^\p{Cc}]{1,100}): ([a-z_]{1,13})$/u.exec(
      message
    )
  const path = progress ? formatMonitorStepPath(progress[2], t) : undefined
  if (
    progress &&
    progress[0] === message &&
    path &&
    Object.hasOwn(actionKeys, progress[3])
  ) {
    return t(
      progress[1]
        ? 'Executing login step {{path}}: {{action}}'
        : 'Executing step {{path}}: {{action}}',
      { path, action: t(actionKeys[progress[3]]), nsSeparator: false }
    )
  }

  const recording = /^Recorded ([1-9]\d{0,2}) page steps$/.exec(message)
  if (recording && recording[0] === message && Number(recording[1]) <= 500) {
    return t('Recorded page steps: {{number}}', { number: recording[1] })
  }
  return message
}
