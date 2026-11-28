import { Button } from '@/components/ui/button';
import { cn } from '@/lib/utils';
import { useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import { TemplateMenuItemKey, TemplateMenuItems } from './constant';

export function SideBar({
  change,
  selected = TemplateMenuItemKey.Recommended,
}: {
  change: (keyword: TemplateMenuItemKey) => void;
  selected?: TemplateMenuItemKey;
}) {
  const { t } = useTranslation();

  const handleMenuClick = useCallback(
    (key: TemplateMenuItemKey) => () => {
      change(key);
    },
    [change],
  );

  return (
    <aside className="flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden bg-bg-base">
      <h2 className="hidden px-2 text-base font-medium text-text-primary md:block md:px-3">
        {t('flow.createFromTemplate')}
      </h2>
      <nav className="mt-4 flex-1 overflow-auto py-1">
        <ul className="flex flex-col items-center gap-2 md:items-stretch md:gap-5">
          {TemplateMenuItems.map((item) => {
            const active = selected === item.key;
            const label = t(`flow.${item.labelKey}`);

            return (
              <li key={item.key} className="w-full md:w-auto">
                <Button
                  block
                  variant="ghost"
                  aria-label={label}
                  className={cn(
                    'h-10 text-base max-md:size-10 max-md:p-0 max-md:justify-center justify-start gap-2.5 px-2 md:px-3',
                    active && 'bg-bg-card text-text-primary',
                  )}
                  onClick={handleMenuClick(item.key)}
                >
                  <span className="flex min-w-0 items-center gap-2.5 max-md:gap-0">
                    <item.icon className="size-4 shrink-0" />
                    <span className="hidden min-w-0 truncate md:inline">
                      {label}
                    </span>
                  </span>
                </Button>
              </li>
            );
          })}
        </ul>
      </nav>
    </aside>
  );
}
