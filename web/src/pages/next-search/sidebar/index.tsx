import {
  SecondarySidebar,
  SecondarySidebarMenu,
} from '@/components/secondary-sidebar';
import { useSearchSidebarCollapse } from '@/hooks/use-search-sidebar-collapse';
import { useSidebarCollapse } from '@/layouts/components/app-sidebar/use-sidebar-collapse';
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
        onSelect: handleShowSearch,
        testId: `search-menu-${SearchMenuKey.Search}`,
      },
      {
        key: SearchMenuKey.Settings,
        icon: <LucideSettings className="size-4 shrink-0" />,
        label: t('search.settings'),
        active: settingsActive,
        onSelect: handleShowSettings,
        testId: `search-menu-${SearchMenuKey.Settings}`,
      },
    ],
    [handleShowSearch, handleShowSettings, settingsActive, t],
  );

  return (
    <SecondarySidebar
      title={title}
      onCollapse={collapseSidebars}
      data-testid="search-detail-sidebar"
    >
      <SecondarySidebarMenu items={items} navigationLabel={title} />
    </SecondarySidebar>
  );
}
