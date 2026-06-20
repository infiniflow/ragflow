import { RAGFlowAvatar } from '@/components/ragflow-avatar';
import ThemeLogo from '@/components/theme-logo';
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
import { LucideChevronDown, LucideCircleHelp, Menu, X } from 'lucide-react';
import React, { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Link, useLocation, useNavigate } from 'react-router';
import { BellButton } from './bell-button';
import GlobalNavbar, { menuItems, PathMap } from './global-navbar';
import ThemeButton from './theme-button';

import { supportedLanguages } from '@/locales/config';

const headerChats = [
  {
    id: 'chat-1',
    title: 'Chat Tesla',
    subject: 'Tesla PDF',
    prompt: 'Compare competitor pricing and summarize market gaps.',
  },
  {
    id: 'chat-2',
    title: 'Chat Company Economy',
    subject: 'Studying',
    prompt: 'Draft helpful answers from ticket history and policy docs.',
  },
  {
    id: 'chat-3',
    title: 'Chat Studying',
    subject: 'Company',
    prompt: 'Turn account notes into discovery questions and next steps.',
  },
];

const headerHistory = [
  {
    question: 'How much money does this company get?',
    answer: 'It generated about $24.3M in the latest dummy report.',
    chatId: 'chat-3',
  },
  {
    question: 'Summarize the Tesla PDF in five bullets',
    answer:
      'Focused on revenue, delivery growth, margin pressure, and outlook.',
    chatId: 'chat-1',
  },
  {
    question: 'Make me a study plan for chapter 4',
    answer: 'Split the chapter into three review blocks and one quiz block.',
    chatId: 'chat-2',
  },
];

type ChatHistorySidebarProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
};

export function ChatHistorySidebar({
  open,
  onOpenChange,
}: ChatHistorySidebarProps) {
  const navigate = useNavigate();

  const openHomeChat = (chatId: string) => {
    navigate(`${Routes.Root}?chat=${chatId}`);
    onOpenChange(false);
  };

  return (
    <aside
      aria-hidden={!open}
      className={cn(
        'fixed inset-0 z-50 h-svh overflow-hidden border-r border-[var(--border)] bg-[var(--bg-card)] text-[var(--text)] transition-transform duration-300 xl:relative xl:inset-auto xl:z-auto xl:col-start-1 xl:row-span-2 xl:row-start-1 xl:h-full xl:min-h-0 xl:translate-x-0 xl:transition-[width,border-color]',
        open
          ? 'w-full translate-x-0 border-[var(--border)] xl:w-[min(360px,100vw)]'
          : 'w-full -translate-x-full border-transparent xl:w-0',
      )}
    >
      <div
        className={cn(
          'flex h-full w-full flex-col p-4 transition-opacity duration-200 xl:w-[min(360px,100vw)]',
          open ? 'opacity-100' : 'pointer-events-none opacity-0',
        )}
      >
        <div className="mb-5 flex items-center justify-between gap-3">
          <div className="flex items-center gap-3">
            <span className="inline-flex size-10 items-center justify-center rounded-2xl bg-[var(--bg-input)]">
              <ThemeLogo className="h-7 w-7" />
            </span>
            <div>
              <h2 className="text-lg font-semibold">MetaGross</h2>
              <p className="text-sm text-[var(--text-muted)]">
                Chats and history
              </p>
            </div>
          </div>

          <Button
            aria-label="Close chat sidebar"
            className="size-9 rounded-full p-0"
            onClick={() => onOpenChange(false)}
            size="auto"
            type="button"
            variant="ghost"
          >
            <X className="size-5" />
          </Button>
        </div>

        <div className="min-h-0 flex-1 overflow-y-auto pr-1">
          <div className="mb-3 flex items-center justify-between">
            <h3 className="text-sm font-semibold uppercase tracking-normal text-[var(--text-muted)]">
              Chats
            </h3>
            <Button
              className="h-8 rounded-full border-[var(--border-button)] bg-[var(--bg-input)] px-3 text-xs text-[var(--text)] hover:bg-[var(--bg-component)]"
              onClick={() => {
                navigate('/chats');
                onOpenChange(false);
              }}
              size="auto"
              type="button"
              variant="outline"
            >
              Start chat
            </Button>
          </div>

          <div className="flex flex-col gap-2">
            {headerChats.map((chat) => (
              <button
                className="rounded-2xl border border-[var(--border-button)] bg-[var(--bg-input)] p-3 text-left transition hover:border-[var(--accent-primary)] hover:bg-[var(--bg-component)]"
                key={chat.id}
                onClick={() => openHomeChat(chat.id)}
                type="button"
              >
                <span className="block font-medium text-[var(--text)]">
                  {chat.title}
                </span>
                <span className="mt-1 block text-xs text-[var(--text-muted)]">
                  For {chat.subject}
                </span>
                <span className="mt-2 line-clamp-2 block text-sm leading-6 text-[var(--text-muted)]">
                  {chat.prompt}
                </span>
              </button>
            ))}
          </div>

          <div className="my-5 border-t border-[var(--border)]" />

          <h3 className="mb-3 text-sm font-semibold uppercase tracking-normal text-[var(--text-muted)]">
            History
          </h3>

          <div className="flex flex-col gap-2">
            {headerHistory.map((item) => (
              <button
                className="rounded-2xl border border-[var(--border-button)] bg-[var(--bg-input)] p-3 text-left transition hover:border-[var(--accent-primary)] hover:bg-[var(--bg-component)]"
                key={item.question}
                onClick={() => openHomeChat(item.chatId)}
                type="button"
              >
                <span className="line-clamp-2 block font-medium text-[var(--text)]">
                  {item.question}
                </span>
                <span className="mt-2 line-clamp-2 block text-sm leading-6 text-[var(--text-muted)]">
                  {item.answer}
                </span>
              </button>
            ))}
          </div>
        </div>
      </div>
    </aside>
  );
}

type HeaderProps = React.HTMLAttributes<HTMLElement> & ChatHistorySidebarProps;

type MobileNavbarMenuProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  activePath: string;
  changeLanguage: (language: string) => void;
  currentLanguageName?: string;
  avatar?: string;
  nickname?: string;
  hasNotification?: boolean;
};

function MobileNavbarMenu({
  open,
  onOpenChange,
  activePath,
  changeLanguage,
  currentLanguageName,
  avatar,
  nickname,
  hasNotification,
}: MobileNavbarMenuProps) {
  const { t } = useTranslation();

  return (
    <div
      aria-hidden={!open}
      className={cn(
        'fixed inset-0 z-40 bg-[var(--bg-card)] text-[var(--text)] transition-transform duration-300 xl:hidden',
        open ? 'translate-y-0' : '-translate-y-full',
      )}
    >
      <div
        className={cn(
          'flex h-full flex-col p-4 transition-opacity duration-200',
          open ? 'opacity-100' : 'pointer-events-none opacity-0',
        )}
      >
        <div className="mb-6 flex items-center justify-between">
          <div className="flex items-center gap-3">
            <ThemeLogo className="size-10" />
            <span className="text-lg font-semibold">MetaGross-AI</span>
          </div>

          <Button
            aria-label="Close menu"
            className="size-10 rounded-full p-0"
            onClick={() => onOpenChange(false)}
            size="auto"
            type="button"
            variant="ghost"
          >
            <X className="size-5" />
          </Button>
        </div>

        <nav className="min-h-0 flex-1 overflow-y-auto">
          <div className="flex flex-col gap-2">
            {menuItems.map(({ path, name, icon: Icon, ...props }) => {
              const isActive = path === activePath;

              return (
                <Link
                  {...props}
                  aria-current={isActive ? 'page' : undefined}
                  className={cn(
                    'flex min-h-12 items-center gap-3 rounded-lg px-3 text-base font-medium transition hover:bg-[var(--bg-input)]',
                    isActive &&
                      'bg-[var(--text-primary)] text-[var(--bg-base)] hover:bg-[var(--text-primary)]',
                  )}
                  key={path}
                  onClick={() => onOpenChange(false)}
                  to={path}
                >
                  {Icon && <Icon className="size-5 stroke-[1.7]" />}
                  <span>{t(name)}</span>
                </Link>
              );
            })}
          </div>

          <div className="my-6 border-t border-[var(--border)]" />

          <section className="space-y-3">
            <h2 className="px-1 text-xs font-semibold uppercase text-[var(--text-muted)]">
              Account
            </h2>

            <Link
              className="flex min-h-12 items-center gap-3 rounded-lg px-3 transition hover:bg-[var(--bg-input)]"
              data-testid="settings-entrypoint-mobile"
              onClick={() => onOpenChange(false)}
              to={Routes.UserSetting}
            >
              <RAGFlowAvatar
                name={nickname}
                avatar={avatar}
                isPerson
                className="size-8"
              />
              <span className="min-w-0 truncate font-medium">
                {nickname || 'Account'}
              </span>
            </Link>

            <div className="rounded-lg border border-[var(--border-button)] p-3">
              <div className="mb-2 text-sm text-[var(--text-muted)]">
                Language
              </div>
              <div className="grid grid-cols-2 gap-2 sm:grid-cols-3">
                {supportedLanguages.map((x) => (
                  <Button
                    className={cn(
                      'justify-start',
                      currentLanguageName === x.displayName &&
                        'border-[var(--accent-primary)]',
                    )}
                    key={x.code}
                    onClick={() => changeLanguage(x.code)}
                    type="button"
                    variant="outline"
                  >
                    {x.displayName}
                  </Button>
                ))}
              </div>
            </div>

            <div className="grid grid-cols-2 gap-2">
              <Button
                asLink
                className="justify-start"
                rel="noreferrer noopener"
                target="_blank"
                to="https://metagross.ai/docs/dev/category/user-guides"
                variant="outline"
              >
                <LucideCircleHelp className="size-[1em]" />
                Docs
              </Button>

              <div className="flex min-h-10 items-center justify-center rounded-md border border-[var(--border-button)]">
                <ThemeButton />
              </div>
            </div>

            {hasNotification && (
              <div className="flex min-h-12 items-center justify-between rounded-lg border border-[var(--border-button)] px-3">
                <span className="font-medium">Notifications</span>
                <BellButton />
              </div>
            )}
          </section>
        </nav>
      </div>
    </div>
  );
}

export function Header({
  className,
  open: isChatSidebarOpen,
  onOpenChange,
  ...props
}: HeaderProps) {
  const { pathname } = useLocation();
  const [isMobileMenuOpen, setIsMobileMenuOpen] = useState(false);

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
  const activePath = useMemo(() => {
    return (
      Object.keys(PathMap).find((x: string) =>
        PathMap[x as keyof typeof PathMap].some((y: string) =>
          pathname.includes(y),
        ),
      ) || pathname
    );
  }, [pathname]);

  // const langItems = LanguageList.map((x) => ({
  //   key: x,
  //   label: <span>{LanguageMap[x as keyof typeof LanguageMap]}</span>,
  // }));

  return (
    <header
      key="app-navbar"
      className={cn(
        'w-full grid grid-cols-[auto_minmax(0,1fr)_auto] grid-rows-1 items-center gap-3 xl:grid-cols-[1fr_auto_1fr] xl:gap-8',
        className,
      )}
      {...props}
    >
      <div className="inline-flex items-center">
        <button
          aria-current={pathname === Routes.Root ? 'page' : undefined}
          aria-label={
            isChatSidebarOpen ? 'Close chat sidebar' : 'Open chat sidebar'
          }
          className="rounded-2xl p-1 transition hover:bg-[var(--bg-input)]"
          onClick={() => onOpenChange(!isChatSidebarOpen)}
          type="button"
        >
          <ThemeLogo className="size-10" />
        </button>
      </div>

      <div className="hidden xl:block">
        <GlobalNavbar />
      </div>

      <div
        className="hidden items-center justify-end gap-4 text-text-badge xl:flex"
        data-testid="auth-status"
      >
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button className="flex items-center gap-1" variant="ghost">
              {currentLanguage?.displayName}
              <LucideChevronDown className="size-[1em]" />
            </Button>
          </DropdownMenuTrigger>

          <DropdownMenuContent>
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

        <Button
          asLink
          variant="ghost"
          size="icon"
          to="https://metagross.ai/docs/dev/category/user-guides"
          target="_blank"
          rel="noreferrer noopener"
        >
          <LucideCircleHelp className="size-[1em]" />
        </Button>

        <ThemeButton />

        {hasNotification && <BellButton />}

        <Link
          to={Routes.UserSetting}
          className="relative ms-3"
          data-testid="settings-entrypoint"
        >
          <RAGFlowAvatar
            name={nickname}
            avatar={avatar}
            isPerson
            className="size-8"
          />
          {/* Temporarily hidden */}
          {/* <Badge className="h-5 w-8 absolute font-normal p-0 justify-center -right-8 -top-2 text-bg-base bg-gradient-to-l from-[#42D7E7] to-[#478AF5]">
            Pro
          </Badge> */}
        </Link>
      </div>

      <Button
        aria-label="Open menu"
        className="size-10 justify-self-end rounded-full p-0 xl:hidden"
        onClick={() => setIsMobileMenuOpen(true)}
        size="auto"
        type="button"
        variant="ghost"
      >
        <Menu className="size-5" />
      </Button>

      <MobileNavbarMenu
        activePath={activePath}
        avatar={avatar}
        changeLanguage={changeLanguage}
        currentLanguageName={currentLanguage?.displayName}
        hasNotification={hasNotification}
        nickname={nickname}
        onOpenChange={setIsMobileMenuOpen}
        open={isMobileMenuOpen}
      />
    </header>
  );
}
