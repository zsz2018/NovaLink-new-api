// Keep the preset order in sync with the browser extension.
const MONITOR_CURRENCIES = [
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
]

export function monitorCurrencyOptions(
  locale: string | undefined,
  savedCurrency?: string
): { value: string; label: string }[] {
  const names = new Intl.DisplayNames(locale, { type: 'currency' })
  const options = MONITOR_CURRENCIES.map((currency) => ({
    value: currency,
    label: `${currency} · ${names.of(currency) ?? currency}`,
  }))
  if (savedCurrency && !MONITOR_CURRENCIES.includes(savedCurrency)) {
    options.push({ value: savedCurrency, label: savedCurrency })
  }
  return options
}
