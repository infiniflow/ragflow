import {
  SecondarySidebar,
  SecondarySidebarMenu,
} from '@/components/secondary-sidebar';
import { useSecondPathName } from '@/hooks/route-hook';
import { Routes } from '@/routes';
import { MemoryStick, Settings } from 'lucide-react';
import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { useParams } from 'react-router';
import { useFetchMemoryBaseConfiguration } from '../hooks/use-memory-setting';

type MemorySidebarProps = {
  onCollapse: () => void;
  onMenuSelect: () => void;
};

export function MemorySidebar({
  onCollapse,
  onMenuSelect,
}: MemorySidebarProps) {
  const pathName = useSecondPathName();
  const { id } = useParams();
  const { data } = useFetchMemoryBaseConfiguration();
  const { t } = useTranslation();

  const items = useMemo(
    () =>
      [
        {
          icon: <MemoryStick className="size-4 shrink-0" />,
          label: t('memory.sideBar.messages'),
          key: Routes.MemoryMessage,
        },
        {
          icon: <Settings className="size-4 shrink-0" />,
          label: t('memory.sideBar.configuration'),
          key: Routes.MemorySetting,
        },
      ].map((item) => ({
        ...item,
        active: `/${pathName}` === item.key,
        to: `${Routes.Memory}${item.key}/${id}`,
        onSelect: onMenuSelect,
      })),
    [id, onMenuSelect, pathName, t],
  );

  return (
    <SecondarySidebar title={data.name} onCollapse={onCollapse}>
      <SecondarySidebarMenu items={items} navigationLabel={data.name} />
    </SecondarySidebar>
  );
}
