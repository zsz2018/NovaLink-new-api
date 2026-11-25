import type {
  MonitorFieldMapping,
  MonitorFlow,
  MonitorJob,
  MonitorSnapshot,
  MonitorStep,
} from '../types'

export function countMonitorSteps(steps: MonitorStep[]): number {
  return steps.reduce(
    (total, step) =>
      total +
      1 +
      countMonitorSteps(step.steps ?? []) +
      countMonitorSteps(step.otherwise ?? []),
    0
  )
}

export function latestMonitorSnapshots(snapshots: MonitorSnapshot[]) {
  const latest = new Map<string, MonitorSnapshot>()
  for (const snapshot of snapshots) {
    if (!snapshot.confirmed || !snapshot.complete) continue
    const previous = latest.get(snapshot.account_id)
    if (
      !previous ||
      snapshot.created_at > previous.created_at ||
      (snapshot.created_at === previous.created_at && snapshot.id > previous.id)
    ) {
      latest.set(snapshot.account_id, snapshot)
    }
  }
  return latest
}

export function monitorCollectionMode(
  flow: MonitorFlow
): 'Ordinary' | 'NewAPI' | 'Sub2API' {
  if (flow.extraction?.adapter === 'newapi') return 'NewAPI'
  if (flow.extraction?.adapter === 'sub2api') return 'Sub2API'
  return 'Ordinary'
}

function monitorFieldObsolete(field: MonitorFieldMapping): boolean {
  return (
    !['path', 'parent', 'constant', 'key', 'group'].includes(field.kind) ||
    'values' in field ||
    Boolean(field.multiply && monitorFieldObsolete(field.multiply)) ||
    Boolean(field.divide && monitorFieldObsolete(field.divide))
  )
}

export function monitorFlowObsolete(flow: MonitorFlow): boolean {
  const extraction = flow.extraction
  if (!extraction) return false
  return (
    'model_coverage' in extraction ||
    extraction.mappings.some(
      (mapping) =>
        ['key_path', 'expand', 'where', 'filters', 'billing_overrides'].some(
          (key) => key in mapping
        ) ||
        Object.values(mapping.fields).some(monitorFieldObsolete) ||
        Object.values(mapping.tiers?.fields ?? {}).some(monitorFieldObsolete)
    )
  )
}

export function monitorFlowReady(flow: MonitorFlow): boolean {
  const extraction = flow.extraction
  if (
    extraction?.version !== 1 ||
    monitorFlowObsolete(flow) ||
    extraction.requests.length === 0 ||
    extraction.requests.some((request) => !request.page_urls?.length)
  ) {
    return false
  }

  if (extraction.adapter !== undefined) {
    return (
      (extraction.adapter === 'newapi' || extraction.adapter === 'sub2api') &&
      extraction.mappings.length === 0
    )
  }
  return ['account_id', 'balances', 'groups'].every((target) =>
    extraction.mappings.some((mapping) => mapping.target === target)
  )
}

export function monitorJobActive(job: MonitorJob): boolean {
  return ['queued', 'running', 'awaiting_user'].includes(job.status)
}
