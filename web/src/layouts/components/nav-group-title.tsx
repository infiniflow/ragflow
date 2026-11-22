import { useTranslation } from 'react-i18next';
import { NavItems, useActiveNavigation } from './app-sidebar/navigation';

export function NavGroupTitle() {
  const { t } = useTranslation();
  const { activeNavKey } = useActiveNavigation();
  const activeNavItem = NavItems.find((item) => item.key === activeNavKey);

  if (!activeNavItem) {
    return null;
  }

  return (
    <h1 className="min-w-0 overflow-hidden text-2xl font-semibold">
      {t(activeNavItem.labelKey)}
    </h1>
  );
}
