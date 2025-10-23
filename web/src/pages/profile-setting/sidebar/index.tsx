import { RAGFlowAvatar } from '@/components/ragflow-avatar';
import ThemeToggle from '@/components/theme-toggle';
import { Button } from '@/components/ui/button';
import { useLogout } from '@/hooks/login-hooks';
import { useSecondPathName } from '@/hooks/route-hook';
import { useFetchUserInfo } from '@/hooks/use-user-setting-request';
import { cn } from '@/lib/utils';
import { Routes } from '@/routes';
import {
  AlignEndVertical,
  Banknote,
  Box,
  FileCog,
  User,
  Users,
} from 'lucide-react';
import { useHandleMenuClick } from './hooks';

const menuItems = [
  {
    section: 'Account & collaboration',
    items: [
      { icon: User, label: 'Profile', key: Routes.Profile },
      { icon: Users, label: 'Team', key: Routes.Team },
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
  const { data: userInfo } = useFetchUserInfo();
  const { handleMenuClick, active } = useHandleMenuClick();

  const { logout } = useLogout();

  return (
    <aside className="w-[303px] bg-bg-base flex flex-col">
      <div className="px-6 flex gap-2 items-center">
        <RAGFlowAvatar
          avatar={userInfo?.avatar}
          name={userInfo?.nickname}
          isPerson
        />
        <p className="text-sm text-text-primary">{userInfo?.email}</p>
      </div>
      <div className="flex-1 overflow-auto">
        {menuItems.map((section, idx) => (
          <div key={idx}>
            {/* <h2 className="p-6 text-sm font-semibold">{section.section}</h2> */}
            {section.items.map((item, itemIdx) => {
              const hoverKey = pathName === item.key;
              return (
                <div key={itemIdx} className="mx-6 my-5 ">
                  <Button
                    variant={hoverKey ? 'secondary' : 'ghost'}
                    className={cn('w-full justify-start gap-2.5 p-3 relative', {
                      'bg-bg-card text-text-primary': active === item.key,
                      'bg-bg-base text-text-secondary': active !== item.key,
                    })}
                    onClick={handleMenuClick(item.key)}
                  >
                    <item.icon className="w-6 h-6" />
                    <span>{item.label}</span>
                    {/* {active && (
                    <div className="absolute right-0 w-[5px] h-[66px] bg-primary rounded-l-xl shadow-[0_0_5.94px_#7561ff,0_0_11.88px_#7561ff,0_0_41.58px_#7561ff,0_0_83.16px_#7561ff,0_0_142.56px_#7561ff,0_0_249.48px_#7561ff]" />
                  )} */}
                  </Button>
                </div>
              );
            })}
          </div>
        ))}
      </div>

      <div className="p-6 mt-auto ">
        <div className="flex items-center gap-2 mb-6 justify-end">
          <ThemeToggle />
        </div>
        <Button
          variant="outline"
          className="w-full gap-3 !bg-bg-base border !border-border-button !text-text-secondary"
          onClick={() => {
            logout();
          }}
        >
          Log Out
        </Button>
      </div>
    </aside>
  );
}
