import { Button } from '@/components/ui/button';
import { useSecondPathName } from '@/hooks/route-hook';
import { cn } from '@/lib/utils';
import { Banknote, LayoutGrid, User } from 'lucide-react';

const menuItems = [
  {
    section: 'All Templates',
    items: [
      { icon: User, label: 'Assistant', key: 'Assistant' },
      { icon: LayoutGrid, label: 'chatbot', key: 'chatbot' },
      { icon: Banknote, label: 'generator', key: 'generator' },
      { icon: Banknote, label: 'Intel', key: 'Intel' },
    ],
  },
];

export function SideBar({ change }: { change: (keyword: string) => void }) {
  const pathName = useSecondPathName();
  const handleMenuClick = (key: string) => {
    change(key);
  };

  return (
    <aside className="w-[303px] bg-background border-r flex flex-col">
      <div className="flex-1 overflow-auto">
        {menuItems.map((section, idx) => (
          <div key={idx}>
            <h2
              className="p-6 text-sm font-semibold hover:bg-muted/50 cursor-pointer"
              onClick={() => handleMenuClick('')}
            >
              {section.section}
            </h2>
            {section.items.map((item, itemIdx) => {
              const active = pathName === item.key;
              return (
                <Button
                  key={itemIdx}
                  variant={active ? 'secondary' : 'ghost'}
                  className={cn('w-full justify-start gap-2.5 p-6 relative')}
                  onClick={() => handleMenuClick(item.key)}
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
        ))}
      </div>
    </aside>
  );
}
