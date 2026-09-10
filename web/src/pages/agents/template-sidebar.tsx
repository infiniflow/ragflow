import { Button } from '@/components/ui/button';
import { cn } from '@/lib/utils';
import { lowerFirst } from 'lodash';
import {
  Box,
  Cable,
  ChartPie,
  Component,
  MessageCircleCode,
  PencilRuler,
  Route,
  Sparkle,
} from 'lucide-react';
import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
export enum MenuItemKey {
  CableIndustry = 'Cable Industry',
  Recommended = 'Recommended',
  Agent = 'Agent',
  CustomerSupport = 'Customer Support',
  Marketing = 'Marketing',
  ConsumerApp = 'Consumer App',
  Pipeline = 'Ingestion Pipeline',
  Other = 'Other',
}

export function SideBar({
  change,
  selected = MenuItemKey.Recommended,
  categories,
}: {
  change: (keyword: string) => void;
  selected?: string;
  categories: string[];
}) {
  const { t } = useTranslation();

  const handleMenuClick = (key: string) => {
    change(key);
  };

  const menuItems = useMemo(() => {
    const items = [
      {
        icon: Cable,
        label: t(
          'flow.' + lowerFirst(MenuItemKey.CableIndustry.replace(' ', '')),
        ),
        key: MenuItemKey.CableIndustry,
      },
      {
        icon: Sparkle,
        label: t('flow.' + lowerFirst(MenuItemKey.Recommended)),
        key: MenuItemKey.Recommended,
      },
      {
        icon: Box,
        label: t('flow.' + lowerFirst(MenuItemKey.Agent)),
        key: MenuItemKey.Agent,
      },
      {
        icon: MessageCircleCode,
        label: t(
          'flow.' +
            lowerFirst(MenuItemKey.CustomerSupport).replace(' ', ''),
        ),
        key: MenuItemKey.CustomerSupport,
      },
      {
        icon: ChartPie,
        label: t('flow.' + lowerFirst(MenuItemKey.Marketing)),
        key: MenuItemKey.Marketing,
      },
      {
        icon: Component,
        label: t(
          'flow.' + lowerFirst(MenuItemKey.ConsumerApp.replace(' ', '')),
        ),
        key: MenuItemKey.ConsumerApp,
      },
      {
        icon: Route,
        label: t(
          'flow.' + lowerFirst(MenuItemKey.Pipeline.replace(' ', '')),
        ),
        key: MenuItemKey.Pipeline,
      },
      {
        icon: PencilRuler,
        label: t('flow.' + lowerFirst(MenuItemKey.Other)),
        key: MenuItemKey.Other,
      },
    ];

    // Categories without a template would open an empty page: a cable-only
    // deployment only advertises its own domain. An unloaded list keeps every
    // entry so the sidebar does not flash empty while the request is in flight.
    const visibleItems =
      categories.length === 0
        ? items
        : items.filter((item) =>
            categories.some(
              (category) =>
                category.toLowerCase() === item.key.toLowerCase(),
            ),
          );

    return [
      {
        // section: 'All Templates',
        section: '',
        items: visibleItems,
      },
    ];
  }, [categories, t]);

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
