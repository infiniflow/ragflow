import { Button } from '@/components/ui/button';
import { cn } from '@/lib/utils';
import {
  Box,
  ChartPie,
  Component,
  MessageCircleCode,
  PencilRuler,
  Sparkle,
} from 'lucide-react';
export enum MenuItemKey {
  Recommended = 'Recommended',
  Agent = 'Agent',
  CustomerSupport = 'Customer Support',
  Marketing = 'Marketing',
  ConsumerApp = 'Consumer App',
  Other = 'Other',
}
const menuItems = [
  {
    // section: 'All Templates',
    section: '',
    items: [
      {
        icon: Sparkle,
        label: MenuItemKey.Recommended,
        key: MenuItemKey.Recommended,
      },
      { icon: Box, label: MenuItemKey.Agent, key: MenuItemKey.Agent },
      {
        icon: MessageCircleCode,
        label: MenuItemKey.CustomerSupport,
        key: MenuItemKey.CustomerSupport,
      },
      {
        icon: ChartPie,
        label: MenuItemKey.Marketing,
        key: MenuItemKey.Marketing,
      },
      {
        icon: Component,
        label: MenuItemKey.ConsumerApp,
        key: MenuItemKey.ConsumerApp,
      },
      { icon: PencilRuler, label: MenuItemKey.Other, key: MenuItemKey.Other },
    ],
  },
];

export function SideBar({
  change,
  selected = MenuItemKey.Recommended,
}: {
  change: (keyword: string) => void;
  selected?: string;
}) {
  const handleMenuClick = (key: string) => {
    change(key);
  };

  return (
    <aside className="w-[303px] bg-text-title-invert border-r flex flex-col">
      <div className="flex-1 overflow-auto">
        {menuItems.map((section, idx) => (
          <div key={idx}>
            {section.section && (
              <h2
                className="p-6 text-sm font-semibold hover:bg-muted/50 cursor-pointer"
                onClick={() => handleMenuClick('')}
              >
                {section.section}
              </h2>
            )}
            {section.items.map((item, itemIdx) => {
              const active = selected === item.key;
              return (
                <Button
                  key={itemIdx}
                  variant={active ? 'secondary' : 'ghost'}
                  className={cn(
                    'w-full justify-start gap-4 px-6 py-8 relative rounded-none',
                  )}
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
