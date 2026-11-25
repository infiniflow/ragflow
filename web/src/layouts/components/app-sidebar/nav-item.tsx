import { Button } from '@/components/ui/button';
import { RAGFlowTooltip } from '@/components/ui/tooltip';
import { useVisibleIntegrationChildren } from '@/ee/hooks/use-visible-integration-children';
import { cn } from '@/lib/utils';
import { LucideChevronDown } from 'lucide-react';
import {
  useCallback,
  useEffect,
  useState,
  type PropsWithChildren,
} from 'react';
import { useTranslation } from 'react-i18next';
import { useLocation } from 'react-router';
import {
  IntegrationChildren,
  IntegrationsNavItem,
  matchesPath,
  NavItem,
  useActiveNavigation,
} from './navigation';
import { useSidebarCollapse } from './use-sidebar-collapse';

function NavItemTooltip({
  children,
  collapsed,
  label,
}: PropsWithChildren<{ collapsed: boolean; label: string }>) {
  if (!collapsed) {
    return children;
  }

  return (
    <RAGFlowTooltip asChild tooltip={label}>
      {children}
    </RAGFlowTooltip>
  );
}

export function NavItemLink({ item }: { item: NavItem }) {
  const { t } = useTranslation();
  const { activeNavKey } = useActiveNavigation();
  const { collapsed } = useSidebarCollapse();
  const isActive = activeNavKey === item.key;
  const Icon = item.icon;

  return (
    <NavItemTooltip collapsed={collapsed} label={t(item.labelKey)}>
      <Button
        asLink
        to={item.path}
        block={!collapsed}
        variant="ghost"
        aria-label={t(item.labelKey)}
        aria-current={isActive ? 'page' : undefined}
        className={cn(
          'h-10 justify-start gap-1 p-0 text-base',
          collapsed
            ? 'size-10 justify-center p-0'
            : 'max-md:size-10 max-md:justify-center max-md:p-0',
          isActive && 'bg-bg-card text-text-primary',
        )}
      >
        <span className="flex size-10 shrink-0 items-center justify-center">
          <Icon className="size-5 shrink-0 stroke-[1.5]" />
        </span>
        <span className={cn('hidden truncate', !collapsed && 'md:inline')}>
          {t(item.labelKey)}
        </span>
      </Button>
    </NavItemTooltip>
  );
}

export function IntegrationsNavGroup() {
  const { t } = useTranslation();
  const { pathname } = useLocation();
  const { collapsed } = useSidebarCollapse();
  const integrationChildren =
    useVisibleIntegrationChildren(IntegrationChildren);
  const activeIntegrationKey = integrationChildren.find((child) =>
    matchesPath(pathname, child.path),
  )?.key;
  const [expanded, setExpanded] = useState(Boolean(activeIntegrationKey));

  // Follow navigation into an integrations page even when the user
  // collapsed the group before.
  useEffect(() => {
    if (activeIntegrationKey) {
      setExpanded(true);
    }
  }, [activeIntegrationKey]);

  const toggleExpanded = useCallback(() => {
    setExpanded((prev) => !prev);
  }, []);

  const Icon = IntegrationsNavItem.icon;
  const isActive = Boolean(activeIntegrationKey);

  if (integrationChildren.length === 0) {
    return null;
  }

  return (
    <div>
      <NavItemTooltip
        collapsed={collapsed}
        label={t(IntegrationsNavItem.labelKey)}
      >
        <Button
          block={!collapsed}
          variant="ghost"
          aria-label={t(IntegrationsNavItem.labelKey)}
          aria-expanded={expanded}
          aria-controls="integrations-nav-children"
          onClick={toggleExpanded}
          className={cn(
            'h-10 justify-start gap-1 p-0 text-base',
            collapsed
              ? 'size-10 justify-center p-0'
              : 'max-md:size-10 max-md:justify-center max-md:p-0',
            isActive && 'text-text-primary',
          )}
        >
          <span className="flex size-10 shrink-0 items-center justify-center">
            <Icon className="size-5 shrink-0 stroke-[1.5]" />
          </span>
          <span
            className={cn(
              'hidden flex-1 truncate text-start',
              !collapsed && 'md:inline',
            )}
          >
            {t(IntegrationsNavItem.labelKey)}
          </span>
          <LucideChevronDown
            className={cn(
              'hidden size-4 shrink-0 transition-transform md:me-3',
              !collapsed && 'md:block',
              expanded && 'rotate-180',
            )}
          />
        </Button>
      </NavItemTooltip>

      {expanded && (
        <ul
          id="integrations-nav-children"
          className={cn(
            'mt-1 flex flex-col gap-1',
            collapsed ? 'items-center' : 'max-md:items-center',
          )}
        >
          {integrationChildren.map((child) => {
            const isChildActive = activeIntegrationKey === child.key;
            return (
              <li key={child.key}>
                <NavItemTooltip collapsed={collapsed} label={t(child.labelKey)}>
                  <Button
                    asLink
                    to={child.path}
                    block={!collapsed}
                    variant="ghost"
                    aria-label={t(child.labelKey)}
                    aria-current={isChildActive ? 'page' : undefined}
                    className={cn(
                      'h-9 justify-start px-3 text-sm',
                      collapsed
                        ? 'size-9 justify-center p-0'
                        : 'max-md:size-9 max-md:justify-center max-md:p-0 md:ps-11',
                      isChildActive && 'bg-bg-card text-text-primary',
                    )}
                  >
                    <span
                      className={cn(
                        'hidden truncate',
                        !collapsed && 'md:inline',
                      )}
                    >
                      {t(child.labelKey)}
                    </span>
                    <span className={cn(!collapsed && 'md:hidden')} aria-hidden>
                      {t(child.labelKey).charAt(0)}
                    </span>
                  </Button>
                </NavItemTooltip>
              </li>
            );
          })}
        </ul>
      )}
    </div>
  );
}

// Enterprise: integration children use an EE/OSS hook for role-based visibility.
