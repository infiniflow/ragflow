import { RAGFlowAvatar } from '@/components/ragflow-avatar';
import { Button } from '@/components/ui/button';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { useChangeLanguage } from '@/hooks/logic-hooks';
import {
  useFetchUserInfo,
  useListTenant,
} from '@/hooks/use-user-setting-request';
import { cn } from '@/lib/utils';
import { TenantRole } from '@/pages/user-setting/constants';
import { Routes } from '@/routes';
import {
  LucideCircleHelp,
  LucideLanguages,
} from 'lucide-react';
import React, { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { Link, useLocation } from 'react-router';
import { BellButton } from './bell-button';
import { BrandMark } from './brand-mark';
import { DesktopNavbar, MobileNavbar } from './global-navbar';
import { MobileMenuFooter } from './mobile-menu-footer';
import ThemeButton from './theme-button';
import { useHeaderNavLayout } from './use-header-nav-layout';

import { supportedLanguages } from '@/locales/config';

/**
 * One shared shape for every header control, so the right-hand cluster reads as
 * a single row of micro-components instead of a row of mixed buttons.
 */
const headerControlClass =
  'size-8 shrink-0 rounded-lg p-0 text-text-secondary transition-colors hover:bg-cable-brand-soft hover:text-cable-brand focus-visible:text-cable-brand';

export function Header({
  className,
  ...props
}: React.HTMLAttributes<HTMLElement>) {
  const { t } = useTranslation();
  const { pathname } = useLocation();
  const changeLanguage = useChangeLanguage();

  const {
    data: { language = 'en', avatar, nickname },
  } = useFetchUserInfo();

  const { data: tenantData } = useListTenant();
  const hasNotification = useMemo(
    () => tenantData?.some((x) => x.role === TenantRole.Invite),
    [tenantData],
  );

  const currentLanguage = supportedLanguages.find((x) => x.code === language);

  const {
    headerRef,
    logoRef,
    expandedRightMeasureRef,
    navMeasureRef,
    isCompact,
  } = useHeaderNavLayout(`${hasNotification}-${language}`);

  return (
    <>
      <header
        ref={headerRef}
        key="app-navbar"
        className={cn(
          'mx-auto flex w-full max-w-[1280px] min-w-0 items-center gap-2 px-6 py-4 sm:gap-4 md:px-12',
          className,
        )}
        {...props}
      >
        <div className="inline-flex shrink-0 items-center gap-2">
          {isCompact && (
            <MobileNavbar
              renderFooter={(close) => <MobileMenuFooter onClose={close} />}
            />
          )}
          <div ref={logoRef} className="inline-flex shrink-0 items-center">
            <Link
              to={Routes.Root}
              aria-current={pathname === Routes.Root ? 'page' : undefined}
              className="-m-1 flex shrink-0 items-center gap-3 rounded-xl p-1 transition-colors hover:bg-cable-brand-soft"
            >
              <BrandMark label={t('header.brandShort')} />
              <span className="hidden text-base font-semibold tracking-tight text-cable-brand md:inline">
                {t('header.brandShort')}
              </span>
            </Link>
          </div>
        </div>

        {!isCompact && (
          <div className="flex min-w-0 flex-1 justify-center overflow-hidden">
            <DesktopNavbar />
          </div>
        )}

        {isCompact && <div className="flex-1" aria-hidden />}

        <div
          className={cn(
            'flex shrink-0 items-center justify-end text-text-badge',
            isCompact ? 'gap-0.5' : 'gap-1',
          )}
          data-testid="auth-status"
        >
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button
                variant="ghost"
                className={headerControlClass}
                aria-label={currentLanguage?.displayName}
                title={currentLanguage?.displayName}
              >
                <LucideLanguages className="size-[1.05rem]" />
              </Button>
            </DropdownMenuTrigger>

            <DropdownMenuContent align="end">
              {supportedLanguages.map((x) => (
                <DropdownMenuItem
                  key={x.code}
                  onClick={() => changeLanguage(x.code)}
                >
                  {x.displayName}
                </DropdownMenuItem>
              ))}
            </DropdownMenuContent>
          </DropdownMenu>

          {!isCompact && (
            <>
              <Button
                asLink
                variant="ghost"
                className={headerControlClass}
                to="https://ragflow.io/docs/dev/category/user-guides"
                target="_blank"
                rel="noreferrer noopener"
                aria-label={t('header.help')}
                title={t('header.help')}
              >
                <LucideCircleHelp className="size-[1.05rem]" />
              </Button>

              {hasNotification && <BellButton className={headerControlClass} />}
            </>
          )}

          <ThemeButton className={headerControlClass} />

          <Link
            to={Routes.UserSetting}
            className={cn(
              'relative flex size-8 shrink-0 items-center justify-center rounded-full',
              'ring-1 ring-cable-border transition-[box-shadow] hover:ring-2 hover:ring-cable-accent',
              !isCompact && 'ms-2',
            )}
            data-testid="settings-entrypoint"
          >
            <RAGFlowAvatar
              name={nickname}
              avatar={avatar}
              isPerson
              className="size-8"
            />
          </Link>
        </div>
      </header>

      <div
        className="pointer-events-none invisible fixed -left-[9999px] top-0"
        aria-hidden
      >
        <div ref={navMeasureRef}>
          <DesktopNavbar />
        </div>
        {/* Mirrors the expanded right-hand cluster so the compact/nav-overflow
            measurement matches what actually renders. Keep the two in sync. */}
        <div
          ref={expandedRightMeasureRef}
          className="inline-flex shrink-0 items-center justify-end gap-1 text-text-badge"
        >
          <Button variant="ghost" className={headerControlClass}>
            <LucideLanguages className="size-[1.05rem]" />
          </Button>
          <Button variant="ghost" className={headerControlClass}>
            <LucideCircleHelp className="size-[1.05rem]" />
          </Button>
          <ThemeButton className={headerControlClass} />
          {hasNotification && <BellButton className={headerControlClass} />}
          <div className="relative ms-2 flex size-8 shrink-0 items-center justify-center rounded-full">
            <RAGFlowAvatar
              name={nickname}
              avatar={avatar}
              isPerson
              className="size-8"
            />
          </div>
        </div>
      </div>
    </>
  );
}
