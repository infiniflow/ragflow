import { SidebarHeader } from '@/components/sidebar-header';
import { Button } from '@/components/ui/button';
import { Separator } from '@/components/ui/separator';
import { EnterpriseSidebarTenantSwitcher } from '@/ee/layouts/components/header-enterprise';
import { OrganizationNavGroup } from '@/ee/layouts/components/app-sidebar/organization-nav-group';
import { useVisibleNavItems } from '@/ee/hooks/use-visible-nav-items';
import { useAgentSidebarCollapse } from '@/hooks/use-agent-sidebar-collapse';
import { useChatSidebarCollapse } from '@/hooks/use-chat-sidebar-collapse';
import { useCompilationTemplateSidebarCollapse } from '@/hooks/use-compilation-template-sidebar-collapse';
import { useDatasetSidebarCollapse } from '@/hooks/use-dataset-sidebar-collapse';
import { useFilesSidebarCollapse } from '@/hooks/use-files-sidebar-collapse';
import { useMemorySidebarCollapse } from '@/hooks/use-memory-sidebar-collapse';
import { useSearchSidebarCollapse } from '@/hooks/use-search-sidebar-collapse';
import { cn } from '@/lib/utils';
import { Routes } from '@/routes';
import { LucidePanelLeftOpen } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { Link, useLocation } from 'react-router';
import { IntegrationsNavGroup, NavItemLink } from './nav-item';
import { matchesPath, NavItemKey, NavItems } from './navigation';
import { useSidebarCollapse } from './use-sidebar-collapse';
import { UserFooter } from './user-footer';

function AppSidebarContent() {
  const { t } = useTranslation();
  const { collapsed } = useSidebarCollapse();
  const {
    collapsed: datasetSidebarCollapsed,
    toggleCollapsed: toggleDatasetSidebar,
  } = useDatasetSidebarCollapse();
  const {
    collapsed: filesSidebarCollapsed,
    toggleCollapsed: toggleFilesSidebar,
  } = useFilesSidebarCollapse();
  const {
    collapsed: agentSidebarCollapsed,
    toggleCollapsed: toggleAgentSidebar,
  } = useAgentSidebarCollapse();
  const {
    collapsed: chatSidebarCollapsed,
    toggleCollapsed: toggleChatSidebar,
  } = useChatSidebarCollapse();
  const {
    collapsed: searchSidebarCollapsed,
    toggleCollapsed: toggleSearchSidebar,
  } = useSearchSidebarCollapse();
  const {
    collapsed: compilationTemplateSidebarCollapsed,
    toggleCollapsed: toggleCompilationTemplateSidebar,
  } = useCompilationTemplateSidebarCollapse();
  const {
    collapsed: memorySidebarCollapsed,
    toggleCollapsed: toggleMemorySidebar,
  } = useMemorySidebarCollapse();
  const { pathname } = useLocation();
  const visibleNavItems = useVisibleNavItems(NavItems);
  const primaryNavItems = visibleNavItems.filter(
    (item) => item.key !== NavItemKey.Team,
  );
  const teamNavItems = visibleNavItems.filter(
    (item) => item.key === NavItemKey.Team,
  );

  const activeSecondarySidebar =
    pathname === Routes.Files
      ? {
          collapsed: filesSidebarCollapsed,
          onExpand: toggleFilesSidebar,
        }
      : [
          {
            route: Routes.DatasetBase,
            collapsed: datasetSidebarCollapsed,
            onExpand: toggleDatasetSidebar,
          },
          {
            route: Routes.Agent,
            collapsed: agentSidebarCollapsed,
            onExpand: toggleAgentSidebar,
          },
          {
            route: Routes.Chat,
            collapsed: chatSidebarCollapsed,
            onExpand: toggleChatSidebar,
          },
          {
            route: Routes.Search,
            collapsed: searchSidebarCollapsed,
            onExpand: toggleSearchSidebar,
          },
          {
            route: Routes.CompilationTemplatesEditNext,
            collapsed: compilationTemplateSidebarCollapsed,
            onExpand: toggleCompilationTemplateSidebar,
          },
          {
            route: Routes.Memory,
            collapsed: memorySidebarCollapsed,
            onExpand: toggleMemorySidebar,
          },
        ].find((sidebar) => matchesPath(pathname, sidebar.route));
  const collapsedSecondarySidebar = activeSecondarySidebar?.collapsed
    ? activeSecondarySidebar
    : undefined;

  return (
    <aside
      className={cn(
        'flex h-full w-16 shrink-0 flex-col overflow-hidden bg-bg-base transition-[width] duration-300 ease-in-out',
        !collapsed && 'md:w-60',
        collapsed && activeSecondarySidebar && 'border-r border-border-button',
      )}
    >
      <SidebarHeader
        className={cn(
          'justify-center px-4',
          !collapsed && 'md:justify-start pl-4',
        )}
      >
        {!(collapsed && collapsedSecondarySidebar) && (
          <Link
            to={Routes.Root}
            aria-label="RAGFlow"
            className={cn(
              'flex size-10 shrink-0 items-center justify-center',
              collapsedSecondarySidebar && 'max-md:hidden',
            )}
          >
            <img
              src="/logo.svg"
              alt="RAGFlow logo"
              className={cn(
                'size-8 transition-[width,height] duration-300 ease-in-out',
                { 'size-5': collapsed },
              )}
            />
          </Link>
        )}
        {!collapsed && (
          <div className="hidden w-36 shrink-0 md:block">
            <EnterpriseSidebarTenantSwitcher />
          </div>
        )}
        {collapsedSecondarySidebar && (
          <div
            className={cn(
              'flex size-10 shrink-0 items-center justify-center',
              !collapsed && 'md:ml-auto',
            )}
          >
            <Button
              variant="ghost"
              size="icon-sm"
              aria-label={t('sidebar.expandSidebar')}
              title={t('sidebar.expandSidebar')}
              onClick={collapsedSecondarySidebar.onExpand}
            >
              <LucidePanelLeftOpen className="size-4" />
            </Button>
          </div>
        )}
      </SidebarHeader>

      <nav className="min-h-0 min-w-0 flex-1 overflow-x-hidden overflow-y-auto px-2 py-3 md:px-3">
        <ul className="flex min-w-0 flex-col gap-1">
          {primaryNavItems.map((item) => (
            <li key={item.key}>
              <NavItemLink item={item} />
            </li>
          ))}
          <li aria-hidden>
            <Separator className="my-2" />
          </li>
          <li>
            <IntegrationsNavGroup />
          </li>
          <OrganizationNavGroup />
          {teamNavItems.map((item) => (
            <li key={item.key}>
              <NavItemLink item={item} />
            </li>
          ))}
        </ul>
      </nav>

      <UserFooter />
    </aside>
  );
}

export function AppSidebar() {
  return <AppSidebarContent />;
}
