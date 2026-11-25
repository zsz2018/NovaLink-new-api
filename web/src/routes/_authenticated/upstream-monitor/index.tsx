import { createFileRoute, redirect } from '@tanstack/react-router'

import { UpstreamMonitor } from '@/features/upstream-monitor'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

export const Route = createFileRoute('/_authenticated/upstream-monitor/')({
  beforeLoad: () => {
    if (useAuthStore.getState().auth.user?.role !== ROLE.SUPER_ADMIN) {
      throw redirect({ to: '/403' })
    }
  },
  component: UpstreamMonitor,
})
