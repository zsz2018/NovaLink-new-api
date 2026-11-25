import type { Control, FieldValues, Path } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import { PasswordInput } from '@/components/password-input'
import { Combobox } from '@/components/ui/combobox'
import {
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'

export interface MonitorOption {
  value: string
  label: string
}

export function MonitorSelect(props: {
  value: string
  onChange: (value: string) => void
  options: MonitorOption[]
  label: string
  disabled?: boolean
  id?: string
  'aria-describedby'?: string
  'aria-invalid'?: boolean
}) {
  const { t } = useTranslation()
  return (
    <Select
      items={props.options}
      value={
        props.options.find((option) => option.value === props.value) ?? null
      }
      onValueChange={(option) => props.onChange(option?.value ?? '')}
      itemToStringLabel={(option) => option.label}
      itemToStringValue={(option) => JSON.stringify(option.value)}
      isItemEqualToValue={(option, selected) => option.value === selected.value}
      disabled={props.disabled}
    >
      <SelectTrigger
        id={props.id}
        aria-label={props.label}
        aria-describedby={props['aria-describedby']}
        aria-invalid={props['aria-invalid']}
        className='w-full'
      >
        <SelectValue placeholder={t('Select an option')} />
      </SelectTrigger>
      <SelectContent>
        {props.options.map((option) => (
          <SelectItem key={option.value} value={option}>
            {option.label}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  )
}

export function MonitorField<T extends FieldValues>(props: {
  control: Control<T>
  name: Path<T>
  label: string
  description?: string
  kind?: 'text' | 'number' | 'password' | 'textarea' | 'switch'
  options?: MonitorOption[]
  searchable?: boolean
  onValueChange?: (value: string) => void
  min?: number
  max?: number
  step?: string
  disabled?: boolean
}) {
  return (
    <FormField
      control={props.control}
      name={props.name}
      render={({ field }) => {
        let input
        if (props.options) {
          const value = String(field.value ?? '')
          const selectOption = (next: string) => {
            if (next === value) return
            field.onChange(
              typeof field.value === 'number' ? Number(next) : next
            )
            props.onValueChange?.(next)
          }
          input = props.searchable ? (
            <Combobox
              options={props.options}
              value={value}
              onValueChange={(next) => {
                if (next !== null) selectOption(next)
              }}
              onBlur={field.onBlur}
              ref={field.ref}
              aria-label={props.label}
              disabled={props.disabled}
            />
          ) : (
            <MonitorSelect
              value={value}
              onChange={selectOption}
              options={props.options}
              label={props.label}
              disabled={props.disabled}
            />
          )
        } else if (props.kind === 'switch') {
          input = (
            <Switch
              checked={Boolean(field.value)}
              onCheckedChange={field.onChange}
              disabled={props.disabled}
              aria-label={props.label}
            />
          )
        } else if (props.kind === 'textarea') {
          input = (
            <Textarea
              {...field}
              value={String(field.value ?? '')}
              disabled={props.disabled}
            />
          )
        } else if (props.kind === 'password') {
          input = (
            <PasswordInput
              {...field}
              value={String(field.value ?? '')}
              autoComplete='new-password'
              disabled={props.disabled}
            />
          )
        } else {
          input = (
            <Input
              {...field}
              type={props.kind === 'number' ? 'number' : 'text'}
              value={field.value ?? ''}
              min={props.min}
              max={props.max}
              step={props.step ?? (props.kind === 'number' ? 'any' : undefined)}
              onChange={(event) => {
                let value: string | number = event.target.value
                if (props.kind === 'number' && value !== '') {
                  value = Number(value)
                }
                field.onChange(value)
              }}
              disabled={props.disabled}
            />
          )
        }
        return (
          <FormItem className='content-start'>
            <FormLabel>{props.label}</FormLabel>
            <FormControl>{input}</FormControl>
            {props.description && (
              <FormDescription>{props.description}</FormDescription>
            )}
            <FormMessage />
          </FormItem>
        )
      }}
    />
  )
}
