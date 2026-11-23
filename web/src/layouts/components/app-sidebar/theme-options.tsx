import { useTheme } from '@/components/theme-provider';
import {
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
} from '@/components/ui/dropdown-menu';
import { ThemeEnum } from '@/constants/common';
import {
  LucideMonitor,
  LucideMoon,
  LucidePalette,
  LucideSun,
} from 'lucide-react';
import { useCallback } from 'react';
import { useTranslation } from 'react-i18next';

const ThemeOptionClassName =
  'size-6 justify-center p-0 text-text-secondary data-[state=checked]:bg-bg-card data-[state=checked]:text-text-primary [&>span]:hidden';

export function ThemeOptions() {
  const { t } = useTranslation();
  const { theme, setTheme } = useTheme();

  const selectTheme = useCallback(
    (value: string) => {
      setTheme(value as ThemeEnum);
    },
    [setTheme],
  );

  return (
    <div className="flex items-center gap-2 rounded-sm px-3 py-1.5 text-sm text-text-secondary">
      <LucidePalette className="size-3.5 shrink-0" />
      <span className="me-auto">{t('sidebar.theme')}</span>
      <DropdownMenuRadioGroup
        className="flex items-center gap-1"
        aria-label={t('sidebar.theme')}
        value={theme}
        onValueChange={selectTheme}
      >
        <DropdownMenuRadioItem
          value={ThemeEnum.Dark}
          aria-label={t('sidebar.darkTheme')}
          className={ThemeOptionClassName}
        >
          <LucideMoon className="size-3" />
        </DropdownMenuRadioItem>
        <DropdownMenuRadioItem
          value={ThemeEnum.Light}
          aria-label={t('sidebar.lightTheme')}
          className={ThemeOptionClassName}
        >
          <LucideSun className="size-3" />
        </DropdownMenuRadioItem>
        <DropdownMenuRadioItem
          value={ThemeEnum.System}
          aria-label={t('sidebar.systemTheme')}
          className={ThemeOptionClassName}
        >
          <LucideMonitor className="size-3" />
        </DropdownMenuRadioItem>
      </DropdownMenuRadioGroup>
    </div>
  );
}
