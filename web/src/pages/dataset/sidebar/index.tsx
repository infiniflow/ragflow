import { Avatar, AvatarFallback, AvatarImage } from '@/components/ui/avatar';
import { Button } from '@/components/ui/button';
import { useSecondPathName } from '@/hooks/route-hook';
import { useFetchKnowledgeBaseConfiguration } from '@/hooks/use-knowledge-request';
import { cn } from '@/lib/utils';
import { Routes } from '@/routes';
import { formatDate } from '@/utils/date';
import { Banknote, LayoutGrid, User } from 'lucide-react';
import { useHandleMenuClick } from './hooks';

const items = [
  { icon: User, label: 'Dataset', key: Routes.DatasetBase },
  {
    icon: LayoutGrid,
    label: 'Retrieval testing',
    key: Routes.DatasetTesting,
  },
  { icon: Banknote, label: 'Settings', key: Routes.DatasetSetting },
];

export function SideBar() {
  const pathName = useSecondPathName();
  const { handleMenuClick } = useHandleMenuClick();
  const { data } = useFetchKnowledgeBaseConfiguration();

  return (
    <aside className="w-60 relative border-r ">
      <div className="p-6 space-y-2 border-b">
        <Avatar className="size-20 rounded-lg">
          <AvatarImage src={data.avatar} />
          <AvatarFallback className="rounded-lg">CN</AvatarFallback>
        </Avatar>

        <h3 className="text-lg font-semibold mb-2">{data.name}</h3>
        <div className="text-sm opacity-80">
          {data.doc_num} files | {data.chunk_num} chunks
        </div>
        <div className="text-sm opacity-80">
          Created {formatDate(data.create_time)}
        </div>
      </div>
      <div className="mt-4">
        {items.map((item, itemIdx) => {
          const active = '/' + pathName === item.key;
          return (
            <Button
              key={itemIdx}
              variant={active ? 'secondary' : 'ghost'}
              className={cn('w-full justify-start gap-2.5 p-6 relative')}
              onClick={handleMenuClick(item.key)}
            >
              <item.icon className="w-6 h-6" />
              <span>{item.label}</span>
            </Button>
          );
        })}
      </div>
    </aside>
  );
}
