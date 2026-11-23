import { SecondarySidebarHeader } from '@/components/sidebar-header';
import { Button } from '@/components/ui/button';
import { useSearchSidebarCollapse } from '@/hooks/use-search-sidebar-collapse';
import { useSidebarCollapse } from '@/layouts/components/app-sidebar/use-sidebar-collapse';
import { cn } from '@/lib/utils';
import { LucideSearch, LucideSettings } from 'lucide-react';
import { useCallback, useMemo } from 'react';
import { useTranslation } from 'react-i18next';

enum SearchMenuKey {
  Search = 'search',
  Settings = 'settings',
}

type SearchSidebarProps = {
  title: string;
  settingsActive: boolean;
  onShowSearch: () => void;
  onShowSettings: () => void;
};

export function SearchSidebar({
  title,
  settingsActive,
  onShowSearch,
  onShowSettings,
}: SearchSidebarProps) {
  const { t } = useTranslation();
  const { setCollapsed } = useSearchSidebarCollapse();
  const { setCollapsed: setPrimarySidebarCollapsed } = useSidebarCollapse();

  const collapseSidebars = useCallback(() => {
    setCollapsed(true);
    setPrimarySidebarCollapsed(true);
  }, [setCollapsed, setPrimarySidebarCollapsed]);

  const collapsePrimarySidebar = useCallback(() => {
    setPrimarySidebarCollapsed(true);
  }, [setPrimarySidebarCollapsed]);

  const handleShowSearch = useCallback(() => {
    onShowSearch();
    collapsePrimarySidebar();
  }, [collapsePrimarySidebar, onShowSearch]);

  const handleShowSettings = useCallback(() => {
    onShowSettings();
    collapsePrimarySidebar();
  }, [collapsePrimarySidebar, onShowSettings]);

  const items = useMemo(
    () => [
      {
        key: SearchMenuKey.Search,
        icon: <LucideSearch className="size-4 shrink-0" />,
        label: t('common.search'),
        active: !settingsActive,
        onClick: handleShowSearch,
      },
      {
        key: SearchMenuKey.Settings,
        icon: <LucideSettings className="size-4 shrink-0" />,
        label: t('search.settings'),
        active: settingsActive,
        onClick: handleShowSettings,
      },
    ],
    [handleShowSearch, handleShowSettings, settingsActive, t],
  );

  return (
    <aside
      className="flex w-64 shrink-0 flex-col"
      data-testid="search-detail-sidebar"
    >
      <SecondarySidebarHeader title={title} onCollapse={collapseSidebars} />

      <nav className="overflow-y-auto px-5 pb-5 pt-1">
        <ul className="space-y-2">
          {items.map((item) => (
            <li key={item.key}>
              <Button
                block
                variant="ghost"
                aria-pressed={item.active}
                className={cn(
                  'relative h-10 justify-start gap-2.5 px-3 text-base',
                  item.active && 'bg-bg-card text-text-primary',
                )}
                onClick={item.onClick}
                data-testid={`search-menu-${item.key}`}
              >
                {item.icon}
                <span>{item.label}</span>
              </Button>
            </li>
          ))}
        </ul>
      </nav>
    </aside>
  );
}
