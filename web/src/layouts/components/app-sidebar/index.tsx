import { Button } from '@/components/ui/button';
import { EnterpriseHeaderUpgradeBadge } from '@/ee/layouts/components/header-enterprise';
import { useDatasetSidebarCollapse } from '@/hooks/use-dataset-sidebar-collapse';
import { cn } from '@/lib/utils';
import { Routes } from '@/routes';
import {
  LucideChevronRight,
  LucidePanelLeftClose,
  LucidePanelLeftOpen,
} from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { Link, useLocation } from 'react-router';
import { IntegrationsNavGroup, NavItemLink } from './nav-item';
import { matchesPath, NavItems } from './navigation';
import { useSidebarCollapse } from './use-sidebar-collapse';
import { UserFooter } from './user-footer';

function AppSidebarContent() {
  const { t } = useTranslation();
  const { collapsed, toggleCollapsed } = useSidebarCollapse();
  const {
    collapsed: datasetSidebarCollapsed,
    toggleCollapsed: toggleDatasetSidebar,
  } = useDatasetSidebarCollapse();
  const { pathname } = useLocation();

  // While the dataset secondary sidebar is hidden, the logo slot becomes the
  // expand entry for it (dataset routes only).
  const showDatasetSidebarExpand =
    datasetSidebarCollapsed && matchesPath(pathname, Routes.DatasetBase);

  return (
    <aside
      className={cn(
        'flex h-full w-16 shrink-0 flex-col overflow-hidden bg-bg-base transition-[width]',
        !collapsed && 'md:w-60',
      )}
    >
      <div
        className={cn(
          'flex shrink-0 items-center justify-center gap-2 px-3 py-4',
          !collapsed && 'md:justify-start',
        )}
      >
        {showDatasetSidebarExpand ? (
          <div className="flex size-10 shrink-0 items-center justify-center">
            <Button
              variant="ghost"
              size="icon-sm"
              aria-label={t('sidebar.expandSidebar')}
              title={t('sidebar.expandSidebar')}
              onClick={toggleDatasetSidebar}
            >
              <LucidePanelLeftOpen className="size-4" />
            </Button>
          </div>
        ) : (
          <Link
            to={Routes.Root}
            aria-label="RAGFlow"
            className="flex size-10 shrink-0 items-center justify-center"
          >
            <img src="/logo.svg" alt="RAGFlow logo" className="size-10" />
          </Link>
        )}
        {!collapsed && <EnterpriseHeaderUpgradeBadge />}
        {!collapsed && (
          <Button
            variant="ghost"
            size="icon-sm"
            aria-label={t('sidebar.collapseSidebar')}
            title={t('sidebar.collapseSidebar')}
            onClick={toggleCollapsed}
            className="ml-auto max-md:hidden"
          >
            <LucidePanelLeftClose className="size-4" />
          </Button>
        )}
      </div>

      <nav className="min-h-0 flex-1 overflow-y-auto px-2 py-3 md:px-3">
        <ul className="flex flex-col gap-1">
          {NavItems.map((item) => (
            <li key={item.key}>
              <NavItemLink item={item} />
            </li>
          ))}
          <li>
            <IntegrationsNavGroup />
          </li>
        </ul>
      </nav>

      <UserFooter />

      {collapsed && (
        <div className="flex shrink-0 justify-center pb-3 max-md:hidden">
          <Button
            variant="ghost"
            size="icon-sm"
            aria-label={t('sidebar.expandSidebar')}
            title={t('sidebar.expandSidebar')}
            onClick={toggleCollapsed}
          >
            <LucideChevronRight className="size-4" />
          </Button>
        </div>
      )}
    </aside>
  );
}

export function AppSidebar() {
  return <AppSidebarContent />;
}
