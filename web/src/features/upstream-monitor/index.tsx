import { useQuery, useQueryClient } from '@tanstack/react-query'
import { RefreshCw } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import {
  sideDrawerContentClassName,
  sideDrawerFormClassName,
  sideDrawerHeaderClassName,
} from '@/components/drawer-layout'
import { ErrorState } from '@/components/error-state'
import { SectionPageLayout } from '@/components/layout'
import { LoadingState } from '@/components/loading-state'
import { Button } from '@/components/ui/button'
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
  SheetTrigger,
} from '@/components/ui/sheet'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'

import { getMonitorState, monitorQueryKey, saveMonitorConfig } from './api'
import { BindingsPanel } from './components/bindings-panel'
import { DevicesPanel } from './components/devices-panel'
import { EncryptionKeyNotice } from './components/encryption-key-notice'
import { FlowsPanel } from './components/flows-panel'
import { JobDetail } from './components/job-detail'
import { JobsPanel } from './components/jobs-panel'
import { ProfitPanel } from './components/profit-panel'
import { RunDialog } from './components/run-dialog'
import { SettingsPanel } from './components/settings-panel'
import { SitesPanel } from './components/sites-panel'
import { SnapshotReview } from './components/snapshot-review'
import { monitorJobActive } from './lib/jobs'
import type { MonitorAccount, MonitorConfig, MonitorFlow } from './types'

export function UpstreamMonitor() {
  const { t } = useTranslation()
  const client = useQueryClient()
  const [tab, setTab] = useState('profit')
  const [run, setRun] = useState<MonitorAccount | null>(null)
  const [configure, setConfigure] = useState<{ flow?: MonitorFlow } | null>(
    null
  )
  const [jobID, setJobID] = useState('')
  const [snapshotID, setSnapshotID] = useState('')
  const query = useQuery({
    queryKey: monitorQueryKey,
    queryFn: getMonitorState,
    refetchInterval: (current) =>
      current.state.data?.jobs.some(monitorJobActive) ? 4000 : 15000,
  })
  const state = query.data
  const save = async (config: MonitorConfig) => {
    await saveMonitorConfig(config)
    await client.invalidateQueries({ queryKey: monitorQueryKey })
    toast.success(t('Upstream monitor configuration saved'))
  }
  const snapshot = state?.snapshots.find((item) => item.id === snapshotID)
  return (
    <>
      <SectionPageLayout fixedContent>
        <SectionPageLayout.Title>
          {t('Upstream monitoring')}
        </SectionPageLayout.Title>
        <SectionPageLayout.Actions>
          <Button
            variant='outline'
            disabled={query.isFetching}
            onClick={() => void query.refetch()}
          >
            <RefreshCw className={query.isFetching ? 'animate-spin' : ''} />
            {t('Refresh')}
          </Button>
          {state && tab === 'profit' && (
            <Sheet>
              <SheetTrigger render={<Button variant='outline' />}>
                {t('Channel cost bindings')}
              </SheetTrigger>
              <SheetContent
                className={sideDrawerContentClassName('sm:w-4/5 sm:max-w-5xl')}
              >
                <SheetHeader
                  className={sideDrawerHeaderClassName('pr-12 sm:pr-12')}
                >
                  <SheetTitle>{t('Channel cost bindings')}</SheetTitle>
                </SheetHeader>
                <div className={sideDrawerFormClassName()}>
                  <BindingsPanel state={state} onSave={save} />
                </div>
              </SheetContent>
            </Sheet>
          )}
        </SectionPageLayout.Actions>
        <SectionPageLayout.Content>
          {query.isLoading && <LoadingState />}
          {query.isError && (
            <ErrorState
              title={t('Could not load upstream monitoring')}
              onRetry={() => void query.refetch()}
            />
          )}
          {state && (
            <div className='flex h-full min-h-0 flex-col gap-3'>
              <EncryptionKeyNotice ready={state.key_ready} />
              <Tabs
                value={tab}
                onValueChange={setTab}
                className='flex min-h-0 flex-1 flex-col'
              >
                <div className='shrink-0 overflow-x-auto border-b'>
                  <TabsList
                    variant='line'
                    className='w-max justify-start gap-4'
                  >
                    <TabsTrigger value='profit'>
                      {t('Profit comparison')}
                    </TabsTrigger>
                    <TabsTrigger value='sites'>
                      {t('Sites and accounts')}
                    </TabsTrigger>
                    <TabsTrigger value='flows'>
                      {t('Collection flows')}
                    </TabsTrigger>
                    <TabsTrigger value='devices'>{t('Browsers')}</TabsTrigger>
                    <TabsTrigger value='jobs'>
                      {t('Tasks and records')}
                    </TabsTrigger>
                    <TabsTrigger value='settings'>
                      {t('Cost settings')}
                    </TabsTrigger>
                  </TabsList>
                </div>
                <TabsContent
                  value='profit'
                  className='min-h-0 flex-1 overflow-y-auto pt-2 lg:overflow-hidden'
                >
                  <ProfitPanel state={state} onSnapshot={setSnapshotID} />
                </TabsContent>
                <TabsContent
                  value='sites'
                  className='min-h-0 flex-1 overflow-y-auto pt-3'
                >
                  <SitesPanel state={state} onSave={save} onRun={setRun} />
                </TabsContent>
                <TabsContent
                  value='flows'
                  className='min-h-0 flex-1 overflow-y-auto pt-3'
                >
                  <FlowsPanel
                    state={state}
                    onSave={save}
                    onConfigure={(flow) => setConfigure({ flow })}
                  />
                </TabsContent>
                <TabsContent
                  value='devices'
                  className='min-h-0 flex-1 overflow-y-auto pt-3'
                >
                  <DevicesPanel devices={state.devices} />
                </TabsContent>
                <TabsContent
                  value='jobs'
                  className='min-h-0 flex-1 overflow-y-auto pt-3'
                >
                  <JobsPanel
                    state={state}
                    onJob={setJobID}
                    onSnapshot={setSnapshotID}
                  />
                </TabsContent>
                <TabsContent
                  value='settings'
                  className='min-h-0 flex-1 overflow-y-auto pt-3'
                >
                  <SettingsPanel
                    key={state.config.version}
                    config={state.config}
                    groups={state.groups}
                    onSave={save}
                  />
                </TabsContent>
              </Tabs>
            </div>
          )}
        </SectionPageLayout.Content>
      </SectionPageLayout>
      {run && state && (
        <RunDialog
          account={run}
          state={state}
          onClose={() => setRun(null)}
          onCreated={(id) => {
            setJobID(id)
            setTab('jobs')
          }}
        />
      )}
      {configure && state && (
        <RunDialog
          flow={configure.flow}
          mode='record'
          state={state}
          onClose={() => setConfigure(null)}
          onCreated={(id) => {
            setJobID(id)
            setTab('jobs')
          }}
        />
      )}
      {jobID && state && (
        <JobDetail
          key={jobID}
          id={jobID}
          config={state.config}
          onClose={() => setJobID('')}
          onJobCreated={setJobID}
        />
      )}
      {snapshot && (
        <SnapshotReview
          key={snapshot.id}
          snapshot={snapshot}
          onClose={() => setSnapshotID('')}
        />
      )}
    </>
  )
}
