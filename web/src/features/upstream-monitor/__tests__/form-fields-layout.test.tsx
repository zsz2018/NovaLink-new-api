import { render, screen } from '@testing-library/react'
import { useForm } from 'react-hook-form'
import { expect, test } from 'vitest'

import { Form } from '@/components/ui/form'

import { MonitorField } from '../components/form-fields'

function FieldsWithMixedDescriptions() {
  const form = useForm({
    defaultValues: { currency: 'CNY', exchange: 1, fee_bearer: 'self' },
  })
  return (
    <Form {...form}>
      <div className='grid gap-4 sm:grid-cols-2 lg:grid-cols-3'>
        <MonitorField
          control={form.control}
          name='currency'
          label='Reporting currency'
        />
        <MonitorField
          control={form.control}
          name='exchange'
          kind='number'
          label='USD exchange rate'
          description='Reporting currency for one USD of local model pricing.'
        />
        <MonitorField
          control={form.control}
          name='fee_bearer'
          label='Collection fee bearer'
          options={[{ value: 'self', label: 'Our site' }]}
        />
      </div>
    </Form>
  )
}

test('fields with and without descriptions keep their labels and controls at the top of a shared grid row', () => {
  render(<FieldsWithMixedDescriptions />)
  const currency = screen.getByRole('textbox', { name: 'Reporting currency' })
  const exchange = screen.getByRole('spinbutton', { name: 'USD exchange rate' })
  const bearer = screen.getByRole('combobox', { name: 'Collection fee bearer' })

  for (const control of [currency, exchange, bearer]) {
    expect(control.closest('[data-slot="form-item"]')).toHaveClass(
      'content-start'
    )
  }
  expect(exchange).toHaveAccessibleDescription(
    'Reporting currency for one USD of local model pricing.'
  )
})
