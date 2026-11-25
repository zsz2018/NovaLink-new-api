import { expect, test } from 'vitest'

import { toIntlLocale } from '@/i18n/languages'

import { formatLocalCurrencyAmount } from '../currency'
import { formatNumber } from '../format'

const precision = { fullPrecision: true }

test.each([
  ['en', '0.085', '0.000000000000085123456789', '8.5%'],
  ['zhCN', '0.085', '0.000000000000085123456789', '8.5%'],
  ['zhTW', '0.085', '0.000000000000085123456789', '8.5%'],
  ['fr', '0,085', '0,000000000000085123456789', '8,5 %'],
  ['ru', '0,085', '0,000000000000085123456789', '8,5 %'],
  ['ja', '0.085', '0.000000000000085123456789', '8.5%'],
  ['vi', '0,085', '0,000000000000085123456789', '8,5%'],
  ['not a language', '0.085', '0.000000000000085123456789', '8.5%'],
])(
  'full precision in %s preserves ratios, tiny amounts and percentages',
  (language, ratio, amount, percent) => {
    const locale = toIntlLocale(language)
    expect(formatNumber(0.085, locale, precision)).toBe(ratio)
    expect(
      formatLocalCurrencyAmount(8.5123456789e-14, {
        locale,
        showSymbol: false,
        abbreviate: false,
        ...precision,
      })
    ).toBe(amount)
    expect(
      formatNumber(0.085, locale, { ...precision, style: 'percent' })
    ).toBe(percent)
    expect(
      formatNumber(0.085, locale, {
        ...precision,
        style: 'percent',
        showSymbol: false,
      })
    ).toBe(percent.replace('%', '').trim())
  }
)

test('full precision retains nonzero digits, omits trailing zeros and keeps unknown values unavailable', () => {
  for (const value of [
    0.085, 0.1234567890123456, 1.0000000000000002, 1.23, 1, 0, -0.085,
  ]) {
    expect(formatNumber(value, 'en', precision)).toBe(String(value))
    expect(
      formatLocalCurrencyAmount(value, {
        locale: 'en',
        showSymbol: false,
        abbreviate: false,
        ...precision,
      })
    ).toBe(String(value))
  }
  expect(formatNumber(null, 'en', precision)).toBe('-')
  expect(formatNumber(undefined, 'en', precision)).toBe('-')
  expect(formatNumber(Number.NaN, 'en', precision)).toBe('-')
  expect(formatLocalCurrencyAmount(null, precision)).toBe('-')
  expect(formatLocalCurrencyAmount(undefined, precision)).toBe('-')
})

test('full precision is opt-in and leaves existing number and currency rounding unchanged', () => {
  expect(formatNumber(0.085, 'en')).toBe('0.09')
  expect(
    formatLocalCurrencyAmount(0.085, {
      locale: 'en',
      showSymbol: false,
      digitsSmall: 2,
    })
  ).toBe('0.09')
})
