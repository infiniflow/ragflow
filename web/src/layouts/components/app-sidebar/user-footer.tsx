import { IconFontFill } from '@/components/icon-font';
import { RAGFlowAvatar } from '@/components/ragflow-avatar';
import { useIsDarkTheme, useTheme } from '@/components/theme-provider';
import { Button } from '@/components/ui/button';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuSub,
  DropdownMenuSubContent,
  DropdownMenuSubTrigger,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { Domain, ThemeEnum } from '@/constants/common';
import {
  EnterpriseHeaderPlanBadge,
  EnterpriseHeaderTicketsLink,
} from '@/ee/layouts/components/header-enterprise';
import { useChangeLanguage } from '@/hooks/logic-hooks';
import { useNavigatePage } from '@/hooks/logic-hooks/navigate-hooks';
import { useLogout } from '@/hooks/use-login-request';
import {
  useFetchSystemVersion,
  useFetchUserInfo,
  useListTenant,
} from '@/hooks/use-user-setting-request';
import { cn } from '@/lib/utils';
import { supportedLanguages } from '@/locales/config';
import { TenantMembershipStatus } from '@/pages/user-setting/constants';
import {
  LucideChevronLeft,
  LucideCircleHelp,
  LucideLanguages,
  LucideLogOut,
  LucideMoon,
  LucideSun,
  LucideUnplug,
  LucideUser,
} from 'lucide-react';
import { useCallback, useEffect, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { BellButton } from '../bell-button';
import { useSidebarCollapse } from './use-sidebar-collapse';

const HelpUrl = 'https://ragflow.io/docs/dev/category/user-guides';
const GitHubUrl = 'https://github.com/infiniflow/ragflow';
const DiscordUrl = 'https://discord.com/invite/NjYzJD3GM3';

export function UserFooter() {
  const { t } = useTranslation();
  const { navigateToUserSettingProfile, navigateToUserSettingApi } =
    useNavigatePage();
  const changeLanguage = useChangeLanguage();
  const { logout } = useLogout();
  const { theme, setTheme } = useTheme();
  const isDark = useIsDarkTheme();
  const { collapsed, toggleCollapsed } = useSidebarCollapse();

  const {
    data: { language = 'en', avatar, nickname, email },
  } = useFetchUserInfo();

  const { data: tenantData } = useListTenant();
  const hasNotification = useMemo(
    () => tenantData?.some((x) => x.status === TenantMembershipStatus.Invited),
    [tenantData],
  );

  const { version, fetchSystemVersion } = useFetchSystemVersion();
  useEffect(() => {
    if (location.host !== Domain) {
      fetchSystemVersion();
    }
  }, [fetchSystemVersion]);

  const toggleTheme = useCallback(() => {
    setTheme(theme === ThemeEnum.Dark ? ThemeEnum.Light : ThemeEnum.Dark);
  }, [setTheme, theme]);

  const handleLanguageChange = useCallback(
    (code: string) => () => {
      changeLanguage(code);
    },
    [changeLanguage],
  );

  const openHelp = useCallback(() => {
    window.open(HelpUrl, '_blank', 'noreferrer,noopener');
  }, []);

  const openGitHub = useCallback(() => {
    window.open(GitHubUrl, '_blank', 'noreferrer,noopener');
  }, []);

  const openDiscord = useCallback(() => {
    window.open(DiscordUrl, '_blank', 'noreferrer,noopener');
  }, []);

  const handleLogout = useCallback(() => {
    logout();
  }, [logout]);

  const ThemeIcon = isDark ? LucideMoon : LucideSun;

  return (
    <footer
      className={cn(
        'mt-auto flex shrink-0 flex-col gap-2 p-2',
        !collapsed && 'md:p-3',
      )}
    >
      <div
        className={cn(
          'flex items-center justify-center gap-1',
          !collapsed && 'md:justify-end',
        )}
      >
        {hasNotification && <BellButton className="!size-8" />}
        <EnterpriseHeaderTicketsLink />
      </div>

      <div className="flex items-center gap-1">
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button
              variant="ghost"
              aria-label={nickname || email}
              data-testid="settings-entrypoint"
              className={cn(
                'h-auto justify-start gap-2.5 px-2 py-2',
                !collapsed && 'md:min-w-0 md:flex-1',
                collapsed
                  ? 'size-10 justify-center p-0'
                  : 'max-md:size-10 max-md:justify-center max-md:p-0',
              )}
            >
              <span className="relative shrink-0">
                <RAGFlowAvatar
                  name={nickname}
                  avatar={avatar}
                  isPerson
                  className="size-8"
                />
                <EnterpriseHeaderPlanBadge />
              </span>
              <span
                className={cn(
                  'hidden min-w-0 flex-1 truncate text-start text-sm',
                  !collapsed && 'md:inline',
                )}
              >
                {nickname || email}
              </span>
            </Button>
          </DropdownMenuTrigger>

          <DropdownMenuContent side="top" align="start" className="w-56">
            <DropdownMenuLabel className="truncate font-normal text-text-secondary">
              {email}
            </DropdownMenuLabel>
            <DropdownMenuSeparator />

            <DropdownMenuItem onClick={navigateToUserSettingProfile}>
              <LucideUser />
              {t('setting.profile')}
            </DropdownMenuItem>
            <DropdownMenuItem onClick={navigateToUserSettingApi}>
              <LucideUnplug />
              {t('setting.api')}
            </DropdownMenuItem>

            <DropdownMenuSeparator />

            <DropdownMenuItem onClick={toggleTheme}>
              <ThemeIcon />
              {t('sidebar.theme')}
            </DropdownMenuItem>

            <DropdownMenuSub>
              <DropdownMenuSubTrigger>
                <LucideLanguages />
                {t('sidebar.language')}
              </DropdownMenuSubTrigger>
              <DropdownMenuSubContent>
                {supportedLanguages.map((x) => (
                  <DropdownMenuItem
                    key={x.code}
                    onClick={handleLanguageChange(x.code)}
                  >
                    <span
                      className={cn(x.code === language && 'font-semibold')}
                    >
                      {x.displayName}
                    </span>
                  </DropdownMenuItem>
                ))}
              </DropdownMenuSubContent>
            </DropdownMenuSub>

            <DropdownMenuSeparator />

            <DropdownMenuItem onClick={openHelp}>
              <LucideCircleHelp />
              {t('header.help')}
            </DropdownMenuItem>
            <DropdownMenuItem onClick={openGitHub}>
              <IconFontFill name="GitHub" />
              {t('header.github')}
            </DropdownMenuItem>
            <DropdownMenuItem onClick={openDiscord}>
              <IconFontFill name="a-DiscordIconSVGVectorIcon" />
              {t('header.discord')}
            </DropdownMenuItem>

            <DropdownMenuSeparator />

            {version && (
              <DropdownMenuLabel className="text-xs font-normal text-accent-primary">
                {version}
              </DropdownMenuLabel>
            )}
            <DropdownMenuItem onClick={handleLogout}>
              <LucideLogOut />
              {t('setting.logout')}
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
        {!collapsed && (
          <Button
            variant="ghost"
            size="icon-sm"
            aria-label={t('sidebar.collapseSidebar')}
            title={t('sidebar.collapseSidebar')}
            onClick={toggleCollapsed}
            className="shrink-0 max-md:hidden"
          >
            <LucideChevronLeft className="size-4" />
          </Button>
        )}
      </div>
    </footer>
  );
}
