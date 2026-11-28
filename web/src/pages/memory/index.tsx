import { PageFrame } from '@/components/page-frame';
import Spotlight from '@/components/spotlight';
import { useMemorySidebarCollapse } from '@/hooks/use-memory-sidebar-collapse';
import { useSidebarCollapse } from '@/layouts/components/app-sidebar/use-sidebar-collapse';
import { useCallback, useEffect } from 'react';
import { Outlet } from 'react-router';
import { MemorySidebar } from './sidebar';

export default function MemoryLayout() {
  const { collapsed, setCollapsed } = useMemorySidebarCollapse();
  const { setCollapsed: setPrimarySidebarCollapsed } = useSidebarCollapse();

  useEffect(() => {
    setCollapsed(false);
  }, [setCollapsed]);

  const collapsePrimarySidebar = useCallback(() => {
    setPrimarySidebarCollapsed(true);
  }, [setPrimarySidebarCollapsed]);

  const collapseSidebars = useCallback(() => {
    setCollapsed(true);
    collapsePrimarySidebar();
  }, [collapsePrimarySidebar, setCollapsed]);

  return (
    <section className="flex h-full w-full flex-col">
      <div className="flex min-h-0 flex-1">
        {!collapsed && (
          <MemorySidebar
            onCollapse={collapseSidebars}
            onMenuSelect={collapsePrimarySidebar}
          />
        )}
        <div className="relative min-h-0 min-w-0 flex-1 pt-2.5">
          <PageFrame className="overflow-hidden">
            <Spotlight />
            <Outlet />
          </PageFrame>
        </div>
      </div>
    </section>
  );
}
