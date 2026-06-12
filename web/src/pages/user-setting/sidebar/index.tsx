import { IconFontFill } from '@/components/icon-font';
import { RAGFlowAvatar } from '@/components/ragflow-avatar';
import ThemeSwitch from '@/components/theme-switch';
import { Button } from '@/components/ui/button';
import { Domain } from '@/constants/common';
import { useLogout } from '@/hooks/use-login-request';
import {
  useFetchSystemVersion,
  useFetchUserInfo,
} from '@/hooks/use-user-setting-request';
import { cn } from '@/lib/utils';
import { Routes } from '@/routes';
import { TFunction } from 'i18next';
import {
  LucideBox,
  LucideLogOut,
  LucideServer,
  LucideUnplug,
  LucideUser,
  LucideUsers,
} from 'lucide-react';
import { useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { useHandleMenuClick } from './hooks';

const menuItems = (t: TFunction) => [
  {
    icon: <LucideServer className="size-[1em]" />,
    label: t('setting.dataSources'),
    key: Routes.DataSource,
  },
  {
    icon: <LucideBox className="size-[1em]" />,
    label: t('setting.model'),
    key: Routes.Model,
    'data-testid': 'settings-nav-model-providers',
  },
  {
    icon: <IconFontFill name="mcp" className="size-[1em]" />,
    label: 'MCP',
    key: Routes.Mcp,
  },
  {
    icon: <LucideUsers className="size-[1em]" />,
    label: t('setting.team'),
    key: Routes.Team,
  },
  {
    icon: <LucideUser className="size-[1em]" />,
    label: t('setting.profile'),
    key: Routes.Profile,
  },
  {
    icon: <LucideUnplug className="size-[1em]" />,
    label: t('setting.api'),
    key: Routes.Api,
  },
];

export function SideBar() {
  const { data: userInfo } = useFetchUserInfo();
  const { handleMenuClick, active: activeItemKey } = useHandleMenuClick();
  const { version, fetchSystemVersion } = useFetchSystemVersion();
  const { t } = useTranslation();
  useEffect(() => {
    if (location.host !== Domain) {
      fetchSystemVersion();
    }
  }, [fetchSystemVersion]);
  const { logout } = useLogout();

  return (
    <aside className="shrink-0 w-16 md:w-[303px] bg-bg-base flex flex-col overflow-hidden">
      <header>
        <h1 className="px-2 md:px-6 flex gap-2.5 items-center justify-center md:justify-start font-normal">
          <RAGFlowAvatar
            avatar={userInfo?.avatar}
            name={userInfo?.nickname}
            isPerson
          />

          <p className="hidden md:block text-sm text-text-primary truncate">
            {userInfo?.email}
          </p>
        </h1>
      </header>

      <nav className="flex-1 overflow-auto mt-4 py-1">
        <ul className="px-2 md:px-6 flex flex-col gap-2 md:gap-5 items-center md:items-stretch">
          {menuItems(t).map((item) => {
            const { key, icon, label, ...rest } = item;

            return (
              <li key={key} className="w-full md:w-auto">
                <Button
                  {...rest}
                  block
                  variant="ghost"
                  aria-label={label}
                  className={cn(
                    'relative h-10 text-base max-md:size-10 max-md:p-0 max-md:justify-center justify-start gap-2.5 px-2 md:px-3',
                    activeItemKey === key && 'bg-bg-card text-text-primary',
                  )}
                  onClick={handleMenuClick(key)}
                >
                  <span className="flex items-center gap-2.5 max-md:gap-0">
                    {icon}
                    <span className="hidden md:inline">{label}</span>
                  </span>
                </Button>
              </li>
            );
          })}
        </ul>
      </nav>

      <footer className="p-2 md:p-6 mt-auto">
        <div className="hidden md:flex items-center gap-2 mb-6 justify-between">
          <span className="text-xs text-accent-primary">{version}</span>

          <ThemeSwitch />
        </div>

        <Button
          block
          size="lg"
          variant="transparent"
          aria-label={t('setting.logout')}
          className="max-md:size-10 max-md:p-0 max-md:mx-auto max-md:justify-center"
          onClick={() => logout()}
        >
          <LucideLogOut className="size-[1em] md:hidden" />
          <span className="hidden md:inline">{t('setting.logout')}</span>
        </Button>
      </footer>
    </aside>
  );
}
