import {
  SecondarySidebar,
  SecondarySidebarMenu,
} from '@/components/secondary-sidebar';
import { Routes } from '@/routes';
import { LucideFolderOpen } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { useMatch } from 'react-router';

type FilesSidebarProps = {
  onCollapse: () => void;
  onMenuSelect: () => void;
};

export function FilesSidebar({ onCollapse, onMenuSelect }: FilesSidebarProps) {
  const { t } = useTranslation();
  const title = t('sidebar.library');
  const isAllFiles = Boolean(useMatch({ path: Routes.Files, end: true }));
  const items = [
    {
      key: Routes.Files,
      icon: <LucideFolderOpen className="size-4 shrink-0" />,
      label: t('fileManager.allFiles'),
      active: isAllFiles,
      to: Routes.Files,
      onSelect: onMenuSelect,
    },
  ];

  return (
    <SecondarySidebar title={title} onCollapse={onCollapse}>
      <SecondarySidebarMenu items={items} navigationLabel={title} />
    </SecondarySidebar>
  );
}
