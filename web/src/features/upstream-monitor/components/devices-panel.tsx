import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { CopyButton } from '@/components/copy-button'
import { StaticDataTable } from '@/components/data-table'
import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  SecureVerificationDialog,
  useSecureVerification,
} from '@/features/auth/secure-verification'
import { formatTimestamp } from '@/lib/format'

import {
  deleteMonitorDeviceRecord,
  monitorQueryKey,
  pairMonitorDevice,
  revokeMonitorDevice,
} from '../api'
import type { MonitorDevice } from '../types'

export function DevicesPanel(props: { devices: MonitorDevice[] }) {
  const { t } = useTranslation()
  const client = useQueryClient()
  const verification = useSecureVerification()
  const [name, setName] = useState('')
  const [pair, setPair] = useState<{ code: string; expires_at: number } | null>(
    null
  )
  const [remove, setRemove] = useState<MonitorDevice | null>(null)
  const pairMutation = useMutation({
    mutationFn: async () => {
      const proof = await verification.requestVerification({
        scope: 'upstream_monitor.device.pair',
        context: {},
      })
      if (!proof) return null
      return pairMonitorDevice(name.trim(), proof.proof_token)
    },
    onSuccess: (data) => {
      if (data) {
        setPair(data)
        setName('')
        void client.invalidateQueries({ queryKey: monitorQueryKey })
      }
    },
  })
  const removeMutation = useMutation({
    mutationFn: (device: MonitorDevice) =>
      device.revoked_at
        ? deleteMonitorDeviceRecord(device.id)
        : revokeMonitorDevice(device.id),
    onSuccess: () => {
      setRemove(null)
      void client.invalidateQueries({ queryKey: monitorQueryKey })
    },
  })
  return (
    <div className='space-y-5'>
      <div className='space-y-3 rounded-xl border p-4'>
        <h3 className='font-medium'>{t('Connect a browser')}</h3>
        <p className='text-muted-foreground text-sm'>
          {t(
            'Install the independent browser extension, create a pairing code here, then enter this site origin and the code in the extension popup.'
          )}
        </p>
        <div className='flex flex-wrap items-end gap-3'>
          <div className='min-w-48 flex-1 space-y-2'>
            <Label htmlFor='monitor-device-name'>{t('Browser name')}</Label>
            <Input
              id='monitor-device-name'
              value={name}
              onChange={(event) => setName(event.target.value)}
            />
          </div>
          <Button
            disabled={!name.trim() || pairMutation.isPending}
            onClick={() => pairMutation.mutate()}
          >
            {t('Create pairing code')}
          </Button>
        </div>
      </div>
      <StaticDataTable
        data={props.devices}
        getRowKey={(device) => device.id}
        emptyContent={t('No browsers paired.')}
        columns={[
          { id: 'name', header: t('Browser'), cell: (device) => device.name },
          {
            id: 'status',
            header: t('Status'),
            cell: (device) => {
              if (device.revoked_at) return t('Revoked')
              if (device.expires_at && device.expires_at * 1000 < Date.now()) {
                return t('Expired')
              }
              return device.last_seen_at &&
                Date.now() - device.last_seen_at * 1000 < 90000
                ? t('Online')
                : t('Offline')
            },
          },
          {
            id: 'seen',
            header: t('Last seen'),
            cell: (device) =>
              device.last_seen_at
                ? formatTimestamp(device.last_seen_at)
                : t('Never connected'),
          },
          {
            id: 'actions',
            header: t('Actions'),
            cell: (device) => (
              <Button
                size='sm'
                variant='outline'
                onClick={() => setRemove(device)}
              >
                {device.revoked_at
                  ? t('Delete browser record')
                  : t('Revoke browser')}
              </Button>
            ),
          },
        ]}
      />
      <Dialog
        open={Boolean(pair)}
        onOpenChange={(open) => {
          if (!open) setPair(null)
        }}
        title={t('Pair browser extension')}
        description={t(
          'This code is temporary and can be used once. Pair only a browser you control.'
        )}
      >
        {pair && (
          <div className='space-y-4'>
            <div>
              <Label>{t('Site origin')}</Label>
              <div className='flex items-center gap-2'>
                <code className='break-all'>{window.location.origin}</code>
                <CopyButton value={window.location.origin} />
              </div>
            </div>
            <div>
              <Label>{t('Pairing code')}</Label>
              <div className='flex items-center gap-2'>
                <code className='text-lg break-all'>{pair.code}</code>
                <CopyButton value={pair.code} />
              </div>
            </div>
            <p className='text-muted-foreground text-sm'>
              {t('Expires at {{time}}', {
                time: formatTimestamp(pair.expires_at),
              })}
            </p>
          </div>
        )}
      </Dialog>
      <ConfirmDialog
        open={Boolean(remove)}
        onOpenChange={(open) => {
          if (!open && !removeMutation.isPending) setRemove(null)
        }}
        title={
          remove?.revoked_at
            ? t('Delete this browser record?')
            : t('Revoke this browser?')
        }
        desc={
          remove?.revoked_at
            ? t(
                'The browser record will be removed and linked account schedules disabled. Collected data and task history will be kept.'
              )
            : t(
                'The browser will no longer be able to receive tasks or account credentials.'
              )
        }
        destructive
        isLoading={removeMutation.isPending}
        handleConfirm={() => {
          if (remove) removeMutation.mutate(remove)
        }}
      />
      <SecureVerificationDialog {...verification.dialogProps} />
    </div>
  )
}
