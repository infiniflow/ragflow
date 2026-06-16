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
import { LucideChevronDown, LucideCircleHelp, X } from 'lucide-react';
import React, { useMemo, useState } from 'react';
import { Link, useLocation, useNavigate } from 'react-router';
import { BellButton } from './bell-button';
import GlobalNavbar from './global-navbar';
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
    answer: 'Focused on revenue, delivery growth, margin pressure, and outlook.',
    chatId: 'chat-1',
  },
  {
    question: 'Make me a study plan for chapter 4',
    answer: 'Split the chapter into three review blocks and one quiz block.',
    chatId: 'chat-2',
  },
];

export function Header({
  className,
  ...props
}: React.HTMLAttributes<HTMLElement>) {
  const { pathname } = useLocation();
  const navigate = useNavigate();
  const [isChatSidebarOpen, setIsChatSidebarOpen] = useState(false);

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

  const openHomeChat = (chatId: string) => {
    navigate(`${Routes.Root}?chat=${chatId}`);
    setIsChatSidebarOpen(false);
  };

  // const langItems = LanguageList.map((x) => ({
  //   key: x,
  //   label: <span>{LanguageMap[x as keyof typeof LanguageMap]}</span>,
  // }));

  return (
    <>
      {isChatSidebarOpen && (
        <button
          aria-label="Close chat sidebar"
          className="fixed inset-0 z-40 bg-black/20"
          onClick={() => setIsChatSidebarOpen(false)}
          type="button"
        />
      )}

      <aside
        className={cn(
          'fixed inset-y-0 left-0 z-50 flex w-[min(360px,calc(100%-2rem))] flex-col border-r border-[var(--border)] bg-[var(--bg-card)] p-4 text-[var(--text)] shadow-[24px_0_60px_rgba(13,27,62,0.16)] transition-transform duration-300 dark:shadow-[24px_0_60px_rgba(0,0,0,0.32)]',
          isChatSidebarOpen ? 'translate-x-0' : '-translate-x-full',
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
            onClick={() => setIsChatSidebarOpen(false)}
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
                setIsChatSidebarOpen(false);
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
      </aside>

      <header
        key="app-navbar"
        className={cn(
          'w-full grid grid-cols-[1fr_auto_1fr] grid-rows-1 items-center gap-8',
          className,
        )}
        {...props}
      >
        <div className="inline-flex items-center">
          <button
            aria-current={pathname === Routes.Root ? 'page' : undefined}
            aria-label="Open chat sidebar"
            className="rounded-2xl p-1 transition hover:bg-[var(--bg-input)]"
            onClick={() => setIsChatSidebarOpen(true)}
            type="button"
          >
            <ThemeLogo className="size-10" />
          </button>
        </div>

      <GlobalNavbar />

      <div
        className="flex items-center justify-end gap-4 text-text-badge"
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
      </header>
    </>
  );
}
