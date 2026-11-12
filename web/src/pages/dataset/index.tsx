import { useDatasetSidebarCollapse } from '@/hooks/use-dataset-sidebar-collapse';
import { useFetchKnowledgeBaseConfiguration } from '@/hooks/use-knowledge-request';
import { useSidebarCollapse } from '@/layouts/components/app-sidebar/use-sidebar-collapse';
import { cn } from '@/lib/utils';
import { OwnerTenantIdContext } from '@/pages/agent/context';
import { KnowledgeBaseProvider } from '@/pages/dataset/contexts/knowledge-base-context';
import { Routes } from '@/routes';
import { useCallback, useEffect } from 'react';

import { Outlet, useMatch } from 'react-router';
import { SideBar } from './sidebar';

export default function DatasetWrapper() {
  const { data, loading } = useFetchKnowledgeBaseConfiguration();
  const { collapsed, setCollapsed } = useDatasetSidebarCollapse();
  const { setCollapsed: setPrimarySidebarCollapsed } = useSidebarCollapse();
  const isCompilation = Boolean(
    useMatch(`${Routes.DatasetBase}${Routes.Compilation}/:id`),
  );

  useEffect(() => {
    setCollapsed(false);
  }, [setCollapsed]);

  useEffect(() => {
    if (isCompilation) {
      setPrimarySidebarCollapsed(true);
    }
  }, [isCompilation, setPrimarySidebarCollapsed]);

  const collapsePrimarySidebar = useCallback(() => {
    setPrimarySidebarCollapsed(true);
  }, [setPrimarySidebarCollapsed]);

  // Hiding the dataset sidebar also collapses the primary sidebar. Expanding
  // it back (from the primary sidebar header) leaves the primary as-is.
  const collapseSidebars = useCallback(() => {
    setCollapsed(true);
    collapsePrimarySidebar();
  }, [setCollapsed, collapsePrimarySidebar]);

  return (
    <OwnerTenantIdContext.Provider value={data?.tenant_id ?? ''}>
      <KnowledgeBaseProvider knowledgeBase={data} loading={loading}>
        <article
          className={cn(
            'grid size-full grid-rows-1',
            collapsed
              ? 'grid-cols-[minmax(0,1fr)]'
              : 'grid-cols-[auto_minmax(0,1fr)]',
          )}
        >
          {!collapsed && (
            <SideBar
              dataset={data}
              onCollapse={collapseSidebars}
              onMenuSelect={collapsePrimarySidebar}
            />
          )}

          <div className="min-h-0 min-w-0 pt-3">
            <Outlet />
          </div>
        </article>
      </KnowledgeBaseProvider>
    </OwnerTenantIdContext.Provider>
  );
}
