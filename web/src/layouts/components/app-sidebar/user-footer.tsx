import { RAGFlowAvatar } from '@/components/ragflow-avatar';
import { Button } from '@/components/ui/button';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import {
  EnterpriseHeaderPlanBadge,
  EnterpriseHeaderTicketsLink,
  EnterpriseHeaderUpgradeBadge,
} from '@/ee/layouts/components/header-enterprise';
import { useNavigatePage } from '@/hooks/logic-hooks/navigate-hooks';
import { useLogout } from '@/hooks/use-login-request';
import {
  useFetchUserInfo,
  useListTenant,
} from '@/hooks/use-user-setting-request';
import { cn } from '@/lib/utils';
import { TenantMembershipStatus } from '@/pages/user-setting/constants';
import {
  LucideChevronLeft,
  LucideChevronRight,
  LucideLogOut,
  LucideUnplug,
  LucideUser,
} from 'lucide-react';
import { useCallback, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { BellButton } from '../bell-button';
import { HelpMenu } from './help-menu';
import { ThemeOptions } from './theme-options';
import { useSidebarCollapse } from './use-sidebar-collapse';

export function UserFooter() {
  const { t } = useTranslation();
  const { navigateToUserSettingProfile, navigateToUserSettingApi } =
    useNavigatePage();
  const { logout } = useLogout();
  const { collapsed, toggleCollapsed } = useSidebarCollapse();

  const {
    data: { avatar, nickname, email },
  } = useFetchUserInfo();

  const { data: tenantData } = useListTenant();
  const hasNotification = useMemo(
    () => tenantData?.some((x) => x.status === TenantMembershipStatus.Invited),
    [tenantData],
  );

  const handleLogout = useCallback(() => {
    logout();
  }, [logout]);

  const collapseLabel = collapsed
    ? t('sidebar.expandSidebar')
    : t('sidebar.collapseSidebar');
  const CollapseIcon = collapsed ? LucideChevronRight : LucideChevronLeft;

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

      <div
        className={cn(
          'flex items-center gap-1 max-md:flex-col',
          collapsed && 'md:flex-col',
        )}
      >
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button
              variant="ghost"
              aria-label={nickname || email}
              data-testid="settings-entrypoint"
              className={cn(
                'h-auto justify-start gap-2.5 px-2 py-2 md:px-1',
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

          <DropdownMenuContent side="top" align="start" className="w-64 p-2">
            <DropdownMenuLabel className="flex items-center gap-3 px-2 py-3 font-normal">
              <RAGFlowAvatar
                name={nickname}
                avatar={avatar}
                isPerson
                className="size-9 shrink-0"
              />
              <span className="min-w-0 flex-1 truncate" title={email}>
                {email}
              </span>
            </DropdownMenuLabel>
            <DropdownMenuSeparator />

            <DropdownMenuItem
              justifyBetween={false}
              className="px-3 py-2.5 [&_svg]:size-3.5"
              onClick={navigateToUserSettingProfile}
            >
              <LucideUser />
              {t('setting.profile')}
            </DropdownMenuItem>
            <DropdownMenuItem
              justifyBetween={false}
              className="px-3 py-2.5 [&_svg]:size-3.5"
              onClick={navigateToUserSettingApi}
            >
              <LucideUnplug />
              {t('setting.api')}
            </DropdownMenuItem>

            <DropdownMenuSeparator />
            <ThemeOptions />
            <DropdownMenuSeparator />

            <DropdownMenuItem
              justifyBetween={false}
              className="px-3 py-2.5 [&_svg]:size-3.5"
              onClick={handleLogout}
            >
              <LucideLogOut />
              {t('setting.logout')}
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>

        <div className={cn('hidden shrink-0', !collapsed && 'md:block')}>
          <EnterpriseHeaderUpgradeBadge />
        </div>
        <HelpMenu />
      </div>

      <Button
        variant="ghost"
        size="auto"
        aria-label={collapseLabel}
        title={collapseLabel}
        onClick={toggleCollapsed}
        className={cn(
          'h-8 w-full shrink-0 bg-bg-card px-2 max-md:hidden',
          collapsed ? 'justify-center' : 'justify-end',
        )}
      >
        <CollapseIcon className="size-4" />
      </Button>
    </footer>
  );
}
