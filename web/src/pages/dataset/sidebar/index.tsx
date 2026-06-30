import { isEmpty } from 'lodash';

import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';

import {
  LucideFolderOpen,
  LucideLogs,
  LucideSettings,
  LucideTextSearch,
} from 'lucide-react';

import { IconFontFill } from '@/components/icon-font';
import { RAGFlowAvatar } from '@/components/ragflow-avatar';
import { Button } from '@/components/ui/button';
import { useSecondPathName } from '@/hooks/route-hook';
import { useFetchKnowledgeGraph } from '@/hooks/use-knowledge-request';
import { cn, formatBytes } from '@/lib/utils';
import { Routes } from '@/routes';
import { formatPureDate } from '@/utils/date';

import { IDataset } from '@/interfaces/database/dataset';
import { useParams } from 'react-router';

type PropType = {
  refreshCount?: number;
  dataset: IDataset;
};

export function SideBar({ dataset: data }: PropType) {
  const pathName = useSecondPathName();
  const { id } = useParams();
  const { data: routerData } = useFetchKnowledgeGraph();
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
    ];

    if (!isEmpty(routerData?.graph)) {
      list.push({
        icon: <IconFontFill name="knowledgegraph" className="size-[1em]" />,
        label: t(`knowledgeDetails.knowledgeGraph`),
        key: Routes.KnowledgeGraph,
      });
    }

    return list;
  }, [t, routerData]);

  return (
    <aside className="flex flex-col w-64 relative">
      <header
        className="px-5 pb-4 grid grid-cols-[auto_1fr] grid-rows-[auto_auto] gap-x-3"
        style={{
          gridTemplateAreas: '"avatar title" "avatar stats"',
        }}
      >
        <RAGFlowAvatar
          avatar={data.avatar}
          name={data.name}
          className="size-16"
          style={{ gridArea: 'avatar' }}
        />

        <h3
          className="text-lg font-semibold line-clamp-1 text-text-primary text-ellipsis overflow-hidden"
          style={{ gridArea: 'title' }}
        >
          {data.name}
        </h3>

        <div
          className="self-end text-text-secondary text-xs overflow-hidden"
          style={{ gridArea: 'stats' }}
        >
          <div className="flex justify-between">
            <span>
              {data.document_count} {t('knowledgeDetails.files')}
            </span>
            <span>{data.size ? formatBytes(data.size) : ''}</span>
          </div>

          <div className="mt-0.5">
            {t('knowledgeDetails.created')} {formatPureDate(data.create_time)}
          </div>
        </div>
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
