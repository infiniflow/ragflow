import { useIsDarkTheme, useTheme } from '@/components/theme-provider';
import { Button } from '@/components/ui/button';
import { Label } from '@/components/ui/label';
import { Switch } from '@/components/ui/switch';
import { ThemeEnum } from '@/constants/common';
import { useLogout } from '@/hooks/login-hooks';
import { useSecondPathName } from '@/hooks/route-hook';
import { cn } from '@/lib/utils';
import { Routes } from '@/routes';
import {
  AlignEndVertical,
  Banknote,
  Box,
  FileCog,
  LayoutGrid,
  LogOut,
  User,
} from 'lucide-react';
import { useCallback } from 'react';
import { useHandleMenuClick } from './hooks';

const menuItems = [
  {
    section: 'Account & collaboration',
    items: [
      { icon: User, label: 'Profile', key: Routes.Profile },
      { icon: LayoutGrid, label: 'Team', key: Routes.Team },
      { icon: Banknote, label: 'Plan', key: Routes.Plan },
      { icon: Banknote, label: 'MCP', key: Routes.Mcp },
    ],
  },
  {
    section: 'System configurations',
    items: [
      {
        icon: Box,
        label: 'Model management',
        key: Routes.Model,
      },
      {
        icon: FileCog,
        label: 'Prompt management',
        key: Routes.Prompt,
      },
      {
        icon: AlignEndVertical,
        label: 'Chunking method',
        key: Routes.Chunk,
      },
    ],
  },
];

export function SideBar() {
  const pathName = useSecondPathName();
  const { handleMenuClick } = useHandleMenuClick();
  const { setTheme } = useTheme();
  const isDarkTheme = useIsDarkTheme();

  const { logout } = useLogout();

  const handleThemeChange = useCallback(
    (checked: boolean) => {
      setTheme(checked ? ThemeEnum.Dark : ThemeEnum.Light);
    },
    [setTheme],
  );

  return (
    <aside className="w-[303px] bg-background border-r flex flex-col">
      <div className="flex-1 overflow-auto">
        {menuItems.map((section, idx) => (
          <div key={idx}>
            <h2 className="p-6 text-sm font-semibold">{section.section}</h2>
            {section.items.map((item, itemIdx) => {
              const active = pathName === item.key;
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
        ))}
      </div>

      <div className="p-6 mt-auto border-t">
        <div className="flex items-center gap-2 mb-6">
          <Switch
            id="dark-mode"
            onCheckedChange={handleThemeChange}
            checked={isDarkTheme}
          />
          <Label htmlFor="dark-mode" className="text-sm">
            Dark
          </Label>
        </div>
        <Button
          variant="outline"
          className="w-full gap-3"
          onClick={() => {
            logout();
          }}
        >
          <LogOut className="w-6 h-6" />
          Logout
        </Button>
      </div>
    </aside>
  );
}
