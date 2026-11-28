import {
  RootLayoutBanner,
  RootLayoutContent,
  useRootLayoutChrome,
} from '@/ee/layouts/root-layout-shell';
import { AgentSidebarCollapseProvider } from '@/hooks/use-agent-sidebar-collapse';
import { ChatSidebarCollapseProvider } from '@/hooks/use-chat-sidebar-collapse';
import { CompilationTemplateSidebarCollapseProvider } from '@/hooks/use-compilation-template-sidebar-collapse';
import { DatasetSidebarCollapseProvider } from '@/hooks/use-dataset-sidebar-collapse';
import { FilesSidebarCollapseProvider } from '@/hooks/use-files-sidebar-collapse';
import { MemorySidebarCollapseProvider } from '@/hooks/use-memory-sidebar-collapse';
import { SearchSidebarCollapseProvider } from '@/hooks/use-search-sidebar-collapse';
import { Outlet } from 'react-router';
import { AppSidebar } from './components/app-sidebar';
import { SidebarCollapseProvider } from './components/app-sidebar/use-sidebar-collapse';

export function RootLayoutContainer({ children }: React.PropsWithChildren) {
  const { containerRef } = useRootLayoutChrome();

  return (
    <SidebarCollapseProvider>
      <CompilationTemplateSidebarCollapseProvider>
        <DatasetSidebarCollapseProvider>
          <FilesSidebarCollapseProvider>
            <AgentSidebarCollapseProvider>
              <ChatSidebarCollapseProvider>
                <SearchSidebarCollapseProvider>
                  <MemorySidebarCollapseProvider>
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
                  </MemorySidebarCollapseProvider>
                </SearchSidebarCollapseProvider>
              </ChatSidebarCollapseProvider>
            </AgentSidebarCollapseProvider>
          </FilesSidebarCollapseProvider>
        </DatasetSidebarCollapseProvider>
      </CompilationTemplateSidebarCollapseProvider>
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
