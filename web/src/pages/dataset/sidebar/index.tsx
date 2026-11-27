import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';

import {
  LucideBookText,
  LucideFolderOpen,
  LucideLogs,
  LucideSettings,
  LucideTextSearch,
} from 'lucide-react';

import {
  SecondarySidebar,
  SecondarySidebarMenu,
} from '@/components/secondary-sidebar';
import { useDatasetSidebarExtraItems } from '@/ee/hooks/use-dataset-sidebar-extra-items';
import { useSecondPathName } from '@/hooks/route-hook';
import { Routes } from '@/routes';

import { IDataset } from '@/interfaces/database/dataset';
import { useParams } from 'react-router';

type DatasetSidebarProps = {
  dataset: IDataset;
  onCollapse: () => void;
  onMenuSelect: () => void;
};

export function DatasetSidebar({
  dataset: data,
  onCollapse,
  onMenuSelect,
}: DatasetSidebarProps) {
  const pathName = useSecondPathName();
  const { id } = useParams();
  const { t } = useTranslation();
  const extraItems = useDatasetSidebarExtraItems();

  const items = useMemo(() => {
    const list = [
      {
        icon: <LucideFolderOpen className="size-4 shrink-0" />,
        label: t(`knowledgeDetails.subbarFiles`),
        key: Routes.Files,
      },
      {
        icon: <LucideTextSearch className="size-4 shrink-0" />,
        label: t(`knowledgeDetails.testing`),
        key: Routes.DatasetTesting,
      },
      {
        icon: <LucideLogs className="size-4 shrink-0" />,
        label: t(`knowledgeDetails.overview`),
        key: Routes.DataSetOverview,
      },
      {
        icon: <LucideSettings className="size-4 shrink-0" />,
        label: t(`knowledgeDetails.configuration`),
        key: Routes.DataSetSetting,
      },
      {
        icon: <LucideBookText className="size-4 shrink-0" />,
        label: 'Artifacts',
        key: Routes.Compilation,
      },
    ];

    return [...list, ...extraItems].map((item) => ({
      ...item,
      active: `/${pathName}` === item.key,
      to: `${Routes.DatasetBase}${item.key}/${id}`,
      onSelect: onMenuSelect,
    }));
  }, [extraItems, id, onMenuSelect, pathName, t]);

  return (
    <SecondarySidebar title={data.name} onCollapse={onCollapse}>
      <SecondarySidebarMenu items={items} navigationLabel={data.name} />
    </SecondarySidebar>
  );
}
