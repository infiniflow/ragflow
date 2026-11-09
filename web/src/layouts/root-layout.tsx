import {
  RootLayoutBanner,
  RootLayoutContent,
  useRootLayoutChrome,
} from '@/ee/layouts/root-layout-shell';
import { DatasetSidebarCollapseProvider } from '@/hooks/use-dataset-sidebar-collapse';
import { Outlet } from 'react-router';
import { AppSidebar } from './components/app-sidebar';
import { SidebarCollapseProvider } from './components/app-sidebar/use-sidebar-collapse';

export function RootLayoutContainer({ children }: React.PropsWithChildren) {
  const { containerRef } = useRootLayoutChrome();

  return (
    <SidebarCollapseProvider>
      <DatasetSidebarCollapseProvider>
        <div
          className="size-full min-w-0 grid grid-cols-[auto_minmax(0,1fr)] grid-rows-[auto_minmax(0,1fr)]"
          ref={containerRef}
        >
          <div className="col-span-full">
            <RootLayoutBanner />
          </div>

          <AppSidebar />

          <main className="size-full min-w-0 min-h-0 overflow-hidden">
            {children}
          </main>
        </div>
      </DatasetSidebarCollapseProvider>
    </SidebarCollapseProvider>
  );
}

export default function RootLayout() {
  return (
    <RootLayoutContainer>
      <RootLayoutContent>
        <Outlet />
      </RootLayoutContent>
    </RootLayoutContainer>
  );
}
