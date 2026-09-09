import { LucideMoon, LucideSun } from 'lucide-react';

import { useTheme } from '@/components/theme-provider';
import { Button } from '@/components/ui/button';
import { ThemeEnum } from '@/constants/common';
import { cn } from '@/lib/utils';

export default function ThemeButton({ className }: { className?: string }) {
  const { setTheme, theme } = useTheme();

  return (
    <Button
      variant="ghost"
      size="icon"
      className={cn('relative size-10 shrink-0 lg:size-8', className)}
      onClick={() =>
        setTheme(theme === ThemeEnum.Dark ? ThemeEnum.Light : ThemeEnum.Dark)
      }
    >
      {theme === ThemeEnum.Light ? (
        <LucideSun className="size-5 lg:size-4" />
      ) : (
        <LucideMoon className="size-5 lg:size-4" />
      )}
    </Button>
  );
}
