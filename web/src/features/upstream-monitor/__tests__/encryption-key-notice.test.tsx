import { act, render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createInstance } from 'i18next'
import { I18nextProvider } from 'react-i18next'
import { describe, expect, test, vi } from 'vitest'

import en from '@/i18n/locales/en.json'
import fr from '@/i18n/locales/fr.json'
import ja from '@/i18n/locales/ja.json'
import ru from '@/i18n/locales/ru.json'
import viLocale from '@/i18n/locales/vi.json'
import zhTW from '@/i18n/locales/zh-TW.json'
import zh from '@/i18n/locales/zh.json'

import { EncryptionKeyNotice } from '../components/encryption-key-notice'

const resources = { en, zhCN: zh, zhTW, fr, ru, ja, vi: viLocale }

describe('upstream credential key setup', () => {
  test('a missing or invalid key shows expandable configuration steps with copyable commands and no preset secret', async () => {
    const user = userEvent.setup()
    const clipboard = vi.spyOn(navigator.clipboard, 'writeText')
    render(<EncryptionKeyNotice ready={false} />)
    const alert = screen.getByRole('alert')
    expect(alert).toHaveTextContent('Credential encryption is not configured')
    const summary = within(alert).getByText(
      'How to configure the encryption key'
    )
    const details = summary.closest('details')
    expect(details).not.toHaveAttribute('open')
    await user.click(summary)
    expect(details).toHaveAttribute('open')
    const steps = within(alert).getByRole('list')
    expect(within(steps).getAllByRole('listitem')).toHaveLength(3)
    expect(steps).toHaveTextContent('docker-compose.yml')
    expect(steps).toHaveTextContent('docker-compose.dev.yml')
    expect(steps).toHaveTextContent('- UPSTREAM_MONITOR_KEY=<BASE64_KEY>')
    expect(alert).toHaveTextContent('restore the original key')
    expect(alert).toHaveTextContent('working directory')
    const commands = [
      'openssl rand -base64 32',
      'docker compose up -d --no-deps --force-recreate new-api',
      'docker compose -f docker-compose.dev.yml up -d --no-deps --force-recreate new-api',
    ]
    const copy = within(alert).getAllByRole('button', {
      name: 'Copy to clipboard',
    })
    expect(copy).toHaveLength(commands.length)
    for (const [index, command] of commands.entries()) {
      expect(alert).toHaveTextContent(command)
      await user.click(copy[index])
      expect(clipboard).toHaveBeenLastCalledWith(command)
    }
    expect(alert.textContent).not.toMatch(/[A-Za-z0-9+/]{43}=/)
  })

  test('a valid key removes the warning and setup steps after state refresh', () => {
    const view = render(<EncryptionKeyNotice ready={false} />)
    expect(screen.getByRole('alert')).toBeInTheDocument()
    view.rerender(<EncryptionKeyNotice ready />)
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
    expect(
      screen.queryByText('How to configure the encryption key')
    ).not.toBeInTheDocument()
  })

  test('setup instructions follow all seven interface languages while commands stay exact', async () => {
    const translations = createInstance()
    await translations.init({
      lng: 'en',
      fallbackLng: 'en',
      keySeparator: false,
      nsSeparator: false,
      resources,
    })
    render(
      <I18nextProvider i18n={translations}>
        <EncryptionKeyNotice ready={false} />
      </I18nextProvider>
    )
    for (const [language, resource] of Object.entries(resources)) {
      await act(() => translations.changeLanguage(language))
      const alert = screen.getByRole('alert')
      const key = 'How to configure the encryption key'
      expect(within(alert).getByText(resource.translation[key])).toBeVisible()
      expect(alert).toHaveTextContent('openssl rand -base64 32')
      expect(alert).toHaveTextContent('- UPSTREAM_MONITOR_KEY=<BASE64_KEY>')
      expect(alert).toHaveTextContent(
        'docker compose up -d --no-deps --force-recreate new-api'
      )
    }
  })
})
