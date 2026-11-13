import { SidebarHeader } from '@/components/sidebar-header';
import { Button } from '@/components/ui/button';
import { Separator } from '@/components/ui/separator';
import { EnterpriseHeaderUpgradeBadge } from '@/ee/layouts/components/header-enterprise';
import { useAgentSidebarCollapse } from '@/hooks/use-agent-sidebar-collapse';
import { useChatSidebarCollapse } from '@/hooks/use-chat-sidebar-collapse';
import { useCompilationTemplateSidebarCollapse } from '@/hooks/use-compilation-template-sidebar-collapse';
import { useDatasetSidebarCollapse } from '@/hooks/use-dataset-sidebar-collapse';
import { cn } from '@/lib/utils';
import { Routes } from '@/routes';
import { LucideChevronRight, LucidePanelLeftOpen } from 'lucide-react';
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
  const {
    collapsed: agentSidebarCollapsed,
    toggleCollapsed: toggleAgentSidebar,
  } = useAgentSidebarCollapse();
  const {
    collapsed: chatSidebarCollapsed,
    toggleCollapsed: toggleChatSidebar,
  } = useChatSidebarCollapse();
  const {
    collapsed: compilationTemplateSidebarCollapsed,
    toggleCollapsed: toggleCompilationTemplateSidebar,
  } = useCompilationTemplateSidebarCollapse();
  const { pathname } = useLocation();

  const collapsedSecondarySidebar = [
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
      route: Routes.CompilationTemplatesEditNext,
      collapsed: compilationTemplateSidebarCollapsed,
      onExpand: toggleCompilationTemplateSidebar,
    },
  ].find(
    (sidebar) => sidebar.collapsed && matchesPath(pathname, sidebar.route),
  );

  return (
    <aside
      className={cn(
        'flex h-full w-16 shrink-0 flex-col overflow-hidden bg-bg-base transition-[width]',
        !collapsed && 'md:w-60',
        collapsed && 'border-r border-border-button',
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
              className={cn('size-8', { 'size-5': collapsed })}
            />
          </Link>
        )}
        {!collapsed && <EnterpriseHeaderUpgradeBadge />}
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

      <nav className="min-h-0 flex-1 overflow-y-auto px-2 py-3 md:px-3">
        <ul className="flex flex-col gap-1">
          {NavItems.map((item) => (
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
