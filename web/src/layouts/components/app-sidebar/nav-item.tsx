import { Button } from '@/components/ui/button';
import { cn } from '@/lib/utils';
import { LucideChevronDown } from 'lucide-react';
import { useCallback, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  IntegrationChildren,
  IntegrationsNavItem,
  NavItem,
  useActiveNavigation,
} from './navigation';
import { useSidebarCollapse } from './use-sidebar-collapse';

export function NavItemLink({ item }: { item: NavItem }) {
  const { t } = useTranslation();
  const { activeNavKey } = useActiveNavigation();
  const { collapsed } = useSidebarCollapse();
  const isActive = activeNavKey === item.key;
  const Icon = item.icon;

  return (
    <Button
      asLink
      to={item.path}
      block
      variant="ghost"
      aria-label={t(item.labelKey)}
      aria-current={isActive ? 'page' : undefined}
      title={t(item.labelKey)}
      className={cn(
        'h-10 justify-start gap-2.5 px-3 text-base',
        collapsed
          ? 'size-10 justify-center p-0'
          : 'max-md:size-10 max-md:justify-center max-md:p-0',
        isActive && 'bg-bg-card text-text-primary',
      )}
    >
      <Icon className="size-5 shrink-0 stroke-[1.5]" />
      <span className={cn('hidden truncate', !collapsed && 'md:inline')}>
        {t(item.labelKey)}
      </span>
    </Button>
  );
}

export function IntegrationsNavGroup() {
  const { t } = useTranslation();
  const { activeIntegrationKey } = useActiveNavigation();
  const { collapsed } = useSidebarCollapse();
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

  return (
    <div>
      <Button
        block
        variant="ghost"
        aria-label={t(IntegrationsNavItem.labelKey)}
        title={t(IntegrationsNavItem.labelKey)}
        aria-expanded={expanded}
        aria-controls="integrations-nav-children"
        onClick={toggleExpanded}
        className={cn(
          'h-10 justify-start gap-2.5 px-3 text-base',
          collapsed
            ? 'size-10 justify-center p-0'
            : 'max-md:size-10 max-md:justify-center max-md:p-0',
          isActive && 'text-text-primary',
        )}
      >
        <Icon className="size-5 shrink-0 stroke-[1.5]" />
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
            'hidden size-4 shrink-0 transition-transform',
            !collapsed && 'md:block',
            expanded && 'rotate-180',
          )}
        />
      </Button>

      {expanded && (
        <ul
          id="integrations-nav-children"
          className={cn(
            'mt-1 flex flex-col gap-1',
            collapsed ? 'items-center' : 'max-md:items-center',
          )}
        >
          {IntegrationChildren.map((child) => {
            const isChildActive = activeIntegrationKey === child.key;
            return (
              <li key={child.key}>
                <Button
                  asLink
                  to={child.path}
                  block
                  variant="ghost"
                  aria-label={t(child.labelKey)}
                  aria-current={isChildActive ? 'page' : undefined}
                  title={t(child.labelKey)}
                  className={cn(
                    'h-9 justify-start px-3 text-sm',
                    collapsed
                      ? 'size-9 justify-center p-0'
                      : 'max-md:size-9 max-md:justify-center max-md:p-0 md:ps-11',
                    isChildActive && 'bg-bg-card text-text-primary',
                  )}
                >
                  <span
                    className={cn('hidden truncate', !collapsed && 'md:inline')}
                  >
                    {t(child.labelKey)}
                  </span>
                  <span className={cn(!collapsed && 'md:hidden')} aria-hidden>
                    {t(child.labelKey).charAt(0)}
                  </span>
                </Button>
              </li>
            );
          })}
        </ul>
      )}
    </div>
  );
}
