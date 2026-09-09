import { IconFontFill } from '@/components/icon-font';
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
  LucideChevronDown,
  LucideCircleHelp,
  LucideLanguages,
} from 'lucide-react';
import React, { useMemo } from 'react';
import { Link, useLocation } from 'react-router';
import { BellButton } from './bell-button';
import { DesktopNavbar, MobileNavbar } from './global-navbar';
import { MobileMenuFooter } from './mobile-menu-footer';
import ThemeButton from './theme-button';
import { useHeaderNavLayout } from './use-header-nav-layout';

import { supportedLanguages } from '@/locales/config';

export function Header({
  className,
  ...props
}: React.HTMLAttributes<HTMLElement>) {
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
          'w-full min-w-0 flex items-center gap-2 sm:gap-4',
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
              className="flex size-10 shrink-0 items-center justify-center"
            >
              <img src={'/logo.svg'} alt="RAGFlow logo" className="size-10" />
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
            isCompact ? 'gap-0.5' : 'gap-4',
          )}
          data-testid="auth-status"
        >
          {!isCompact && (
            <>
              <a
                className="inline-flex p-2 text-text-secondary hover:text-text-primary focus-visible:text-text-primary"
                target="_blank"
                href="https://discord.com/invite/NjYzJD3GM3"
                rel="noreferrer noopener"
              >
                <IconFontFill name="a-DiscordIconSVGVectorIcon" />
              </a>

              <a
                className="inline-flex p-2 text-text-secondary hover:text-text-primary focus-visible:text-text-primary"
                target="_blank"
                href="https://github.com/infiniflow/ragflow"
                rel="noreferrer noopener"
              >
                <IconFontFill name="GitHub" />
              </a>
            </>
          )}

          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button
                variant="ghost"
                className={cn(
                  'size-10 shrink-0 px-0',
                  !isCompact && 'size-auto gap-1 px-4',
                )}
                aria-label={currentLanguage?.displayName}
              >
                {isCompact && <LucideLanguages className="size-5" />}
                {!isCompact && (
                  <>
                    {currentLanguage?.displayName}
                    <LucideChevronDown className="size-[1em]" />
                  </>
                )}
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
                size="icon"
                className="size-8"
                to="https://ragflow.io/docs/dev/category/user-guides"
                target="_blank"
                rel="noreferrer noopener"
              >
                <LucideCircleHelp className="size-[1em]" />
              </Button>

              {hasNotification && <BellButton className="!size-8" />}
            </>
          )}

          <ThemeButton className={cn(!isCompact && '!size-8')} />

          <Link
            to={Routes.UserSetting}
            className={cn(
              'relative flex size-10 shrink-0 items-center justify-center',
              !isCompact && 'ms-3',
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
        <div
          ref={expandedRightMeasureRef}
          className="inline-flex shrink-0 items-center justify-end gap-4 text-text-badge"
        >
          <a className="inline-flex p-2">
            <IconFontFill name="a-DiscordIconSVGVectorIcon" />
          </a>
          <a className="inline-flex p-2">
            <IconFontFill name="GitHub" />
          </a>
          <Button variant="ghost" className="size-auto gap-1 px-4">
            {currentLanguage?.displayName}
            <LucideChevronDown className="size-[1em]" />
          </Button>
          <Button variant="ghost" size="icon" className="size-8">
            <LucideCircleHelp className="size-[1em]" />
          </Button>
          <ThemeButton className="!size-8" />
          {hasNotification && <BellButton className="!size-8" />}
          <div className="relative ms-3 flex size-10 shrink-0 items-center justify-center">
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
