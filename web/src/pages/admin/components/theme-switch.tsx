import { useIsDarkTheme, useTheme } from '@/components/theme-provider';
import { ThemeEnum } from '@/constants/common';
import { cn } from '@/lib/utils';
import { Root, Thumb } from '@radix-ui/react-switch';
import { LucideMoon, LucideSun } from 'lucide-react';
import { forwardRef } from 'react';

const ThemeSwitch = forwardRef<
  React.ElementRef<typeof Root>,
  React.ComponentPropsWithoutRef<typeof Root>
>(({ className, ...props }, ref) => {
  const { setTheme } = useTheme();
  const isDark = useIsDarkTheme();

  return (
    <Root
      ref={ref}
      className={cn('relative rounded-full', className)}
      {...props}
      checked={isDark}
      onCheckedChange={(value) =>
        setTheme(value ? ThemeEnum.Dark : ThemeEnum.Light)
      }
    >
      <div className="px-3 py-2 rounded-full border border-border-button bg-bg-card transition-[background-color] duration-200">
        <div className="flex items-center justify-between gap-4 relative z-[1] text-text-disabled transition-[text-color] duration-200">
          <LucideSun
            className={cn('size-[1em]', !isDark && 'text-text-primary')}
          />
          <LucideMoon
            className={cn('size-[1em]', isDark && 'text-text-primary')}
          />
        </div>
      </div>

      <Thumb
        className={cn(
          'absolute top-0 left-0 w-[calc(50%+.25rem)] h-full rounded-full bg-bg-base border border-border-button',
          'transition-all duration-200',
          { 'left-[calc(50%-.25rem)]': isDark },
        )}
      />
    </Root>
  );
});

export default ThemeSwitch;
