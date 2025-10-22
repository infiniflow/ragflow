import { ThemeEnum } from '@/constants/common';
import { Moon, Sun } from 'lucide-react';
import { FC, useCallback } from 'react';
import { useIsDarkTheme, useTheme } from './theme-provider';
import { Button } from './ui/button';

const ThemeToggle: FC = () => {
  const { setTheme } = useTheme();
  const isDarkTheme = useIsDarkTheme();
  const handleThemeChange = useCallback(
    (checked: boolean) => {
      setTheme(checked ? ThemeEnum.Dark : ThemeEnum.Light);
    },
    [setTheme],
  );
  return (
    <Button
      type="button"
      onClick={() => handleThemeChange(!isDarkTheme)}
      className="relative inline-flex h-6 w-14 items-center rounded-full  transition-colors p-0.5 border-none focus:border-none bg-bg-card hover:bg-bg-card"
      //   aria-label={isDarkTheme ? 'Switch to light mode' : 'Switch to dark mode'}
    >
      <div className="inline-flex h-full w-full items-center">
        <div
          className={`inline-flex transform items-center justify-center rounded-full  transition-transform ${
            isDarkTheme
              ? '  text-text-disabled h-4 w-5'
              : '  text-text-primary bg-bg-base h-full w-8 flex-1'
          }`}
        >
          <Sun />
        </div>

        <div
          className={`inline-flex  transform items-center justify-center rounded-full  transition-transform ${
            isDarkTheme
              ? ' text-text-primary bg-bg-base h-full w-8 flex-1'
              : 'text-text-disabled h-4 w-5'
          }`}
        >
          <Moon />
        </div>
      </div>
    </Button>
  );
};

export default ThemeToggle;
