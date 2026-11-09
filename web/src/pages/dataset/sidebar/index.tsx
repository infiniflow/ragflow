import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';

import {
  LucideBookText,
  LucideFolderOpen,
  LucideLogs,
  LucidePanelLeftClose,
  LucideSettings,
  LucideTextSearch,
} from 'lucide-react';

import { TruncatedText } from '@/components/truncated-text';
import { Button } from '@/components/ui/button';
import { useSecondPathName } from '@/hooks/route-hook';
import { cn } from '@/lib/utils';
import { Routes } from '@/routes';

import { IDataset } from '@/interfaces/database/dataset';
import { useParams } from 'react-router';

type PropType = {
  dataset: IDataset;
  onCollapse: () => void;
};

export function SideBar({ dataset: data, onCollapse }: PropType) {
  const pathName = useSecondPathName();
  const { id } = useParams();
  const { t } = useTranslation();

  const items = useMemo(() => {
    const list = [
      {
        icon: <LucideFolderOpen className="size-[1em]" />,
        label: t(`knowledgeDetails.subbarFiles`),
        key: Routes.Files,
      },
      {
        icon: <LucideTextSearch className="size-[1em]" />,
        label: t(`knowledgeDetails.testing`),
        key: Routes.DatasetTesting,
      },
      {
        icon: <LucideLogs className="size-[1em]" />,
        label: t(`knowledgeDetails.overview`),
        key: Routes.DataSetOverview,
      },
      {
        icon: <LucideSettings className="size-[1em]" />,
        label: t(`knowledgeDetails.configuration`),
        key: Routes.DataSetSetting,
      },
      {
        icon: <LucideBookText className="size-[1em]" />,
        label: 'Artifacts',
        key: Routes.Compilation,
      },
    ];

    return list;
  }, [t]);

  return (
    <aside className="flex flex-col w-64 relative">
      <header className="flex items-center gap-2 px-5 pb-4">
        <TruncatedText
          as="h3"
          className="min-w-0 flex-1 truncate text-lg font-semibold text-text-primary"
          tooltip={data.name}
        >
          {data.name}
        </TruncatedText>

        <Button
          variant="ghost"
          size="icon-sm"
          aria-label={t('sidebar.collapseSidebar')}
          title={t('sidebar.collapseSidebar')}
          onClick={onCollapse}
        >
          <LucidePanelLeftClose className="size-4" />
        </Button>
      </header>

      <nav className="px-5 pt-1 pb-5 overflow-y-auto">
        <ul className="space-y-5">
          {items.map((item) => {
            const active = '/' + pathName === item.key;

            return (
              <li key={item.key}>
                <Button
                  asLink
                  block
                  variant="ghost"
                  className={cn(
                    'justify-start gap-2.5 px-3 relative h-10 text-base',
                    active && 'bg-bg-card text-text-primary',
                  )}
                  to={`${Routes.DatasetBase}${item.key}/${id}`}
                >
                  {item.icon}
                  <span>{item.label}</span>
                </Button>
              </li>
            );
          })}
        </ul>
      </nav>
    </aside>
  );
}
