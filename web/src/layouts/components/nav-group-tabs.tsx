import { PillToggleGroup } from '@/components/pill-toggle-group';
import { useCallback, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router';
import { useActiveNavigation } from './app-sidebar/navigation';

// Page-level pill tabs for sidebar groups with sibling list pages
// (Context: Dataset/Memory, Application: Chat/Search).
export function NavGroupTabs() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const { pageTab } = useActiveNavigation();

  const options = useMemo(
    () =>
      pageTab?.group.tabs.map((tab) => ({
        value: tab.path,
        label: t(tab.labelKey),
      })) ?? [],
    [pageTab, t],
  );

  const handleTabChange = useCallback(
    (value: string) => {
      if (value !== pageTab?.activePath) {
        navigate(value);
      }
    },
    [navigate, pageTab],
  );

  if (!pageTab) {
    return null;
  }

  return (
    <PillToggleGroup
      options={options}
      value={pageTab.activePath}
      onChange={handleTabChange}
    />
  );
}
