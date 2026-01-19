import { RAGFlowAvatar } from '@/components/ragflow-avatar';
import { Button } from '@/components/ui/button';
import { useSecondPathName } from '@/hooks/route-hook';
import { cn } from '@/lib/utils';
import { Routes } from '@/routes';
import { formatPureDate } from '@/utils/date';
import { MemoryStick, Settings } from 'lucide-react';
import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { useFetchMemoryBaseConfiguration } from '../hooks/use-memory-setting';
import { useHandleMenuClick } from './hooks';

export function SideBar() {
  const pathName = useSecondPathName();
  const { handleMenuClick } = useHandleMenuClick();
  // refreshCount: be for avatar img sync update on top left
  const { data } = useFetchMemoryBaseConfiguration();
  const { t } = useTranslation();

  const items = useMemo(() => {
    const list = [
      {
        icon: <MemoryStick className="size-4" />,
        label: t(`memory.sideBar.messages`),
        key: Routes.MemoryMessage,
      },
      {
        icon: <Settings className="size-4" />,
        label: t(`memory.sideBar.configuration`),
        key: Routes.MemorySetting,
      },
    ];
    return list;
  }, [t]);

  return (
    <aside className="relative p-5 space-y-8">
      <div className="flex gap-2.5 max-w-[200px] items-center">
        <RAGFlowAvatar
          avatar={data.avatar}
          name={data.name}
          className="size-16"
        ></RAGFlowAvatar>
        <div className=" text-text-secondary text-xs space-y-1 overflow-hidden">
          <h3 className="text-lg font-semibold line-clamp-1 text-text-primary text-ellipsis overflow-hidden">
            {data.name}
          </h3>
          <div className="flex justify-between">
            <span className="truncate ">{data.description}</span>
            {/* <span>{formatBytes(data.size)}</span> */}
          </div>
          <div>
            {t('knowledgeDetails.created')} {formatPureDate(data.create_time)}
          </div>
        </div>
      </div>

      <div className="w-[200px] flex flex-col gap-5">
        {items.map((item, itemIdx) => {
          const active = '/' + pathName === item.key;
          return (
            <Button
              key={itemIdx}
              variant={active ? 'secondary' : 'ghost'}
              className={cn(
                'w-full justify-start gap-2.5 px-3 relative h-10 text-text-secondary',
                {
                  'bg-bg-card': active,
                  'text-text-primary': active,
                },
              )}
              onClick={handleMenuClick(item.key)}
            >
              {item.icon}
              <span>{item.label}</span>
            </Button>
          );
        })}
      </div>
    </aside>
  );
}
