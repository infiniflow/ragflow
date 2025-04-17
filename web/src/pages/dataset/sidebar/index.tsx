import { Button } from '@/components/ui/button';
import { useSecondPathName } from '@/hooks/route-hook';
import { cn } from '@/lib/utils';
import { Routes } from '@/routes';
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

const dataset = {
  id: 1,
  title: 'Legal knowledge base',
  files: '1,242 files',
  size: '152 MB',
  created: '12.02.2024',
  image: 'https://github.com/shadcn.png',
};

export function SideBar() {
  const pathName = useSecondPathName();
  const { handleMenuClick } = useHandleMenuClick();

  return (
    <aside className="w-[303px] relative border-r ">
      <div className="p-6 space-y-2 border-b">
        <div
          className="w-[70px] h-[70px] rounded-xl bg-cover"
          style={{ backgroundImage: `url(${dataset.image})` }}
        />

        <h3 className="text-lg font-semibold mb-2">{dataset.title}</h3>
        <div className="text-sm opacity-80">
          {dataset.files} | {dataset.size}
        </div>
        <div className="text-sm opacity-80">Created {dataset.created}</div>
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
              {active && (
                <div className="absolute right-0 w-[5px] h-[66px] bg-primary rounded-l-xl shadow-[0_0_5.94px_#7561ff,0_0_11.88px_#7561ff,0_0_41.58px_#7561ff,0_0_83.16px_#7561ff,0_0_142.56px_#7561ff,0_0_249.48px_#7561ff]" />
              )}
            </Button>
          );
        })}
      </div>
    </aside>
  );
}
