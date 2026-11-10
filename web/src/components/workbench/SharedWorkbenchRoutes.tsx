import { lazy, memo } from 'react'
import type { ComponentProps } from 'react'
import { WorkbenchRouteLayer } from './WorkbenchRouteHost'
import type { WorkbenchRouteId, WorkbenchRoutePresentation } from './WorkbenchRouteHost'

const HomeView = memo(lazy(() => import('@/components/Home/HomeView').then((module) => ({ default: module.HomeView }))))
const SettingsView = memo(lazy(() => import('@/features/settings/SettingsView').then((module) => ({ default: module.SettingsView }))))

interface SharedWorkbenchRoutesProps {
  route: WorkbenchRouteId
  isMounted: WorkbenchRoutePresentation['isMounted']
  loadingLabel: string
  home: ComponentProps<typeof HomeView>
}

/** Renders the full-workbench routes shared by Writing and Game modes. */
export function SharedWorkbenchRoutes({
  route,
  isMounted,
  loadingLabel,
  home,
}: SharedWorkbenchRoutesProps) {
  return (
    <>
      {isMounted('books') && (
        <WorkbenchRouteLayer visible={route === 'books'} loadingLabel={loadingLabel}>
          <HomeView {...home} />
        </WorkbenchRouteLayer>
      )}
      {isMounted('settings') && (
        <WorkbenchRouteLayer visible={route === 'settings'} loadingLabel={loadingLabel}>
          <SettingsView visible={route === 'settings'} />
        </WorkbenchRouteLayer>
      )}
    </>
  )
}
