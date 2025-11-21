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
      className={cn(
        'group/theme-switch relative rounded-full outline-none self-center focus-visible:ring-1 focus-visible:ring-accent-primary',
        className,
      )}
      {...props}
      checked={isDark}
      onCheckedChange={(value) =>
        setTheme(value ? ThemeEnum.Dark : ThemeEnum.Light)
      }
    >
      <div className="self-center p-3 py-2 rounded-full bg-bg-card transition-[background-color]">
        <div className="h-full flex items-center justify-between gap-4 relative z-[1] text-text-disabled transition-[text-color]">
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
          'absolute top-0 left-0 w-[calc(50%+.25rem)] p-0.5 h-full rounded-full overflow-hidden',
          'transition-all ease-out',
          'group-hover/theme-switch:w-[calc(50%+.66rem)] group-focus-visible/theme-switch:w-[calc(50%+.66rem)]',
          {
            'left-[calc(50%-.25rem)] group-hover/theme-switch:left-[calc(50%-.66rem)] group-focus-visible/theme-switch:left-[calc(50%-.66rem)]':
              isDark,
          },
        )}
      >
        <div className="size-full rounded-full bg-bg-base shadow-md transition-colors ease-out" />
      </Thumb>
    </Root>
  );
});

export default ThemeSwitch;
