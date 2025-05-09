import { RAGFlowAvatar } from '@/components/ragflow-avatar';
import { Button } from '@/components/ui/button';
import { useSecondPathName } from '@/hooks/route-hook';
import { useFetchKnowledgeBaseConfiguration } from '@/hooks/use-knowledge-request';
import { cn, formatBytes } from '@/lib/utils';
import { Routes } from '@/routes';
import { formatPureDate } from '@/utils/date';
import { Banknote, Database } from 'lucide-react';
import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { useHandleMenuClick } from './hooks';

export function SideBar() {
  const pathName = useSecondPathName();
  const { handleMenuClick } = useHandleMenuClick();
  const { data } = useFetchKnowledgeBaseConfiguration();
  const { t } = useTranslation();

  /** left-nav items */
  const items = useMemo(
    () => [
      {
        icon: Database,
        label: t('knowledgeDetails.dataset'),
        key: Routes.DatasetBase,
        disabled: false,
      },
      {
        icon: Banknote,
        label: t('knowledgeDetails.configuration'),
        key: Routes.DatasetSetting,
        disabled: true, // greyed out
      },
    ],
    [t],
  );

  return (
    <aside className="relative p-5 space-y-8">
      {/* KB card */}
      <div className="flex gap-2.5 max-w-[200px] items-center">
        <RAGFlowAvatar
          avatar={data.avatar}
          name={data.name}
          className="size-16"
        />
        <div className="text-text-sub-title text-xs space-y-1">
          <h3 className="text-lg font-semibold line-clamp-1 text-text-title">
            {data.name}
          </h3>
          <div className="flex justify-between">
            <span>{data.doc_num} files</span>
            <span>{formatBytes(data.size)}</span>
          </div>
          <div>Created {formatPureDate(data.create_time)}</div>
        </div>
      </div>

      {/* menu buttons */}
      <div className="w-[200px] flex flex-col gap-5">
        {items.map((item, idx) => {
          const active = '/' + pathName === item.key;
          return (
            <Button
              key={idx}
              variant={active ? 'secondary' : 'ghost'}
              className={cn(
                'w-full justify-start gap-2.5 px-3 relative h-10 text-text-sub-title-invert',
                {
                  'bg-background-card text-text-title': active,
                  'opacity-40 cursor-not-allowed': item.disabled && !active,
                },
              )}
              onClick={item.disabled ? undefined : handleMenuClick(item.key)}
              disabled={item.disabled}
            >
              <item.icon className="size-4" />
              <span>{item.label}</span>
            </Button>
          );
        })}
      </div>
    </aside>
  );
}
