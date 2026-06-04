import { useIsMobile } from '@/components/hooks/use-mobile';
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
  const isMobile = useIsMobile();
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
    <aside
      className={cn(
        'shrink-0 bg-bg-base flex flex-col overflow-hidden',
        isMobile ? 'w-16' : 'w-[303px]',
      )}
    >
      <header>
        <h1
          className={cn(
            'flex gap-2.5 items-center font-normal',
            isMobile ? 'px-2 justify-center' : 'px-6 justify-start',
          )}
        >
          <RAGFlowAvatar
            avatar={userInfo?.avatar}
            name={userInfo?.nickname}
            isPerson
          />

          {!isMobile && (
            <p className="text-sm text-text-primary truncate">
              {userInfo?.email}
            </p>
          )}
        </h1>
      </header>

      <nav className="flex-1 overflow-auto mt-4 py-1">
        <ul
          className={cn(
            'flex flex-col',
            isMobile ? 'px-2 gap-2 items-center' : 'px-6 gap-5',
          )}
        >
          {menuItems(t).map((item) => {
            const { key, icon, label, ...rest } = item;

            return (
              <li key={key} className={isMobile ? 'w-full' : undefined}>
                <Button
                  {...rest}
                  block={!isMobile}
                  variant="ghost"
                  aria-label={label}
                  className={cn(
                    'relative h-10 text-base',
                    isMobile
                      ? 'size-10 p-0 justify-center'
                      : 'justify-start gap-2.5 px-3 w-full',
                    activeItemKey === key && 'bg-bg-card text-text-primary',
                  )}
                  onClick={handleMenuClick(key)}
                >
                  <section
                    className={cn('flex items-center', !isMobile && 'gap-2.5')}
                  >
                    {icon}
                    {!isMobile && <span>{label}</span>}
                  </section>
                </Button>
              </li>
            );
          })}
        </ul>
      </nav>

      <footer className={cn('mt-auto', isMobile ? 'p-2' : 'p-6')}>
        {!isMobile && (
          <div className="flex items-center gap-2 mb-6 justify-between">
            <span className="text-xs text-accent-primary">{version}</span>

            <ThemeSwitch />
          </div>
        )}

        <Button
          block={!isMobile}
          size={isMobile ? 'icon-lg' : 'lg'}
          variant="transparent"
          aria-label={t('setting.logout')}
          className={cn(isMobile ? 'mx-auto' : 'w-full')}
          onClick={() => logout()}
        >
          {isMobile ? (
            <LucideLogOut className="size-[1em]" />
          ) : (
            t('setting.logout')
          )}
        </Button>
      </footer>
    </aside>
  );
}
