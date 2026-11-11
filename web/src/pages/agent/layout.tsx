import { useAgentSidebarCollapse } from '@/hooks/use-agent-sidebar-collapse';
import { useSidebarCollapse } from '@/layouts/components/app-sidebar/use-sidebar-collapse';
import { cn } from '@/lib/utils';
import { useCallback } from 'react';
import { Outlet } from 'react-router';
import { useFetchDataOnMount } from './hooks/use-fetch-data';
import { AgentSidebar } from './sidebar';

export default function AgentLayout() {
  const { flowDetail } = useFetchDataOnMount();
  const { collapsed, setCollapsed } = useAgentSidebarCollapse();
  const { setCollapsed: setPrimarySidebarCollapsed } = useSidebarCollapse();

  const collapsePrimarySidebar = useCallback(() => {
    setPrimarySidebarCollapsed(true);
  }, [setPrimarySidebarCollapsed]);

  const collapseSidebars = useCallback(() => {
    setCollapsed(true);
    collapsePrimarySidebar();
  }, [collapsePrimarySidebar, setCollapsed]);

  return (
    <article
      className={cn(
        'grid size-full grid-rows-1 pt-3',
        collapsed
          ? 'grid-cols-[minmax(0,1fr)]'
          : 'grid-cols-[auto_minmax(0,1fr)]',
      )}
    >
      {!collapsed && (
        <AgentSidebar
          title={flowDetail.title ?? ''}
          onCollapse={collapseSidebars}
          onMenuSelect={collapsePrimarySidebar}
        />
      )}
      <Outlet />
    </article>
  );
}
