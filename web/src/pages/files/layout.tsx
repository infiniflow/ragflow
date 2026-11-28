import { PageFrame } from '@/components/page-frame';
import { useFilesSidebarCollapse } from '@/hooks/use-files-sidebar-collapse';
import { useSidebarCollapse } from '@/layouts/components/app-sidebar/use-sidebar-collapse';
import { cn } from '@/lib/utils';
import { useCallback, useEffect } from 'react';
import { Outlet } from 'react-router';
import { FilesSidebar } from './sidebar';

export default function FilesLayout() {
  const { collapsed, setCollapsed } = useFilesSidebarCollapse();
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
    <article
      className={cn(
        'grid size-full grid-rows-1',
        collapsed
          ? 'grid-cols-[minmax(0,1fr)]'
          : 'grid-cols-[auto_minmax(0,1fr)]',
      )}
    >
      {!collapsed && (
        <FilesSidebar
          onCollapse={collapseSidebars}
          onMenuSelect={collapsePrimarySidebar}
        />
      )}
      <div className="min-h-0 min-w-0 pt-2.5">
        <PageFrame>
          <Outlet />
        </PageFrame>
      </div>
    </article>
  );
}
