import { useDatasetSidebarCollapse } from '@/hooks/use-dataset-sidebar-collapse';
import { useFetchKnowledgeBaseConfiguration } from '@/hooks/use-knowledge-request';
import { useSidebarCollapse } from '@/layouts/components/app-sidebar/use-sidebar-collapse';
import { cn } from '@/lib/utils';
import { OwnerTenantIdContext } from '@/pages/agent/context';
import { KnowledgeBaseProvider } from '@/pages/dataset/contexts/knowledge-base-context';
import { useCallback } from 'react';

import { Outlet } from 'react-router';
import { SideBar } from './sidebar';

export default function DatasetWrapper() {
  const { data, loading } = useFetchKnowledgeBaseConfiguration();
  const { collapsed, setCollapsed } = useDatasetSidebarCollapse();
  const { setCollapsed: setPrimarySidebarCollapsed } = useSidebarCollapse();

  // Hiding the dataset sidebar also collapses the primary sidebar. Expanding
  // it back (from the primary sidebar header) leaves the primary as-is.
  const collapseSidebars = useCallback(() => {
    setCollapsed(true);
    setPrimarySidebarCollapsed(true);
  }, [setCollapsed, setPrimarySidebarCollapsed]);

  return (
    <OwnerTenantIdContext.Provider value={data?.tenant_id ?? ''}>
      <KnowledgeBaseProvider knowledgeBase={data} loading={loading}>
        <article
          className={cn(
            'pt-3 size-full grid grid-rows-1',
            collapsed
              ? 'grid-cols-[minmax(0,1fr)]'
              : 'grid-cols-[auto_minmax(0,1fr)]',
          )}
        >
          {!collapsed && (
            <SideBar dataset={data} onCollapse={collapseSidebars} />
          )}

          <Outlet />
        </article>
      </KnowledgeBaseProvider>
    </OwnerTenantIdContext.Provider>
  );
}
