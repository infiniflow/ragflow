import { useId, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Link, useLocation } from 'react-router';

import {
  LucideBrain,
  LucideCpu,
  LucideDatabase,
  LucideFolderOpen,
  LucideHouse,
  LucideMenu,
  LucideMessageSquareText,
  LucideSearch,
} from 'lucide-react';

import { Button } from '@/components/ui/button';
import { Sheet, SheetContent, SheetTrigger } from '@/components/ui/sheet';
import { cn } from '@/lib/utils';
import { Routes } from '@/routes';
import { supportsCssAnchor } from '@/utils/css-support';

const PathMap = {
  [Routes.Datasets]: [Routes.Datasets, Routes.DatasetBase],
  [Routes.Chats]: [Routes.Chats, Routes.Chat],
  [Routes.Searches]: [Routes.Searches, Routes.Search],
  [Routes.Agents]: [Routes.Agents, Routes.AgentTemplates],
  [Routes.Memories]: [Routes.Memories, Routes.Memory, Routes.MemoryMessage],
  [Routes.Files]: [Routes.Files],
} as const;

// Match on path-segment boundaries, not a loose substring, so e.g.
// "/user-setting/chat-channel" does not match the "/chat" tab.
const matchesPath = (pathname: string, candidate: string) =>
  pathname === candidate || pathname.startsWith(`${candidate}/`);

const menuItems = [
  { path: Routes.Root, name: 'header.home', icon: LucideHouse },
  { path: Routes.Datasets, name: 'header.dataset', icon: LucideDatabase },
  {
    path: Routes.Chats,
    name: 'header.chat',
    icon: LucideMessageSquareText,
    'data-testid': 'nav-chat',
  },
  {
    path: Routes.Searches,
    name: 'header.search',
    icon: LucideSearch,
    'data-testid': 'nav-search',
  },
  {
    path: Routes.Agents,
    name: 'header.flow',
    icon: LucideCpu,
    'data-testid': 'nav-agent',
  },
  { path: Routes.Memories, name: 'header.memories', icon: LucideBrain },
  { path: Routes.Files, name: 'header.fileManager', icon: LucideFolderOpen },
];

function useActivePath() {
  const { pathname } = useLocation();

  return useMemo(() => {
    return (
      Object.keys(PathMap).find((x: string) =>
        PathMap[x as keyof typeof PathMap].some((y: string) =>
          matchesPath(pathname, y),
        ),
      ) || pathname
    );
  }, [pathname]);
}

const DesktopNavbarWithAnchor = () => {
  const { t } = useTranslation();
  const activePath = useActivePath();
  const navbarAnchorNamePrefix = useId().replace(/:/g, '');

  const activePathAnchorName = `--${navbarAnchorNamePrefix}${activePath === Routes.Root ? '-root' : activePath.replace('/', '-')}`;

  const hasAnyActive = useMemo(
    () => menuItems.some(({ path }) => path === activePath),
    [activePath],
  );

  return (
    <nav>
      <ul className="relative flex items-center p-1 bg-bg-card rounded-full border border-border-button">
        {menuItems.map(({ path, name, icon: Icon, ...props }) => {
          const isActive = path === activePath;
          const anchorName = `--${navbarAnchorNamePrefix}${path === Routes.Root ? '-root' : path.replace('/', '-')}`;

          return (
            <li key={path} className="relative" style={{ anchorName }}>
              <Link
                {...props}
                to={path}
                className={cn(
                  'h-10 px-4 xl:px-6 text-sm xl:text-base inline-flex items-center justify-center whitespace-nowrap',
                  'hover:text-current focus-visible:text-current rounded-full transition-all',
                  isActive && '!text-bg-base',
                )}
                aria-current={isActive ? 'page' : undefined}
              >
                {path === Routes.Root ? (
                  <>
                    <Icon className="size-6 stroke-[1.5]" />
                    <span className="sr-only">{t(name)}</span>
                  </>
                ) : (
                  <span>{t(name)}</span>
                )}
              </Link>
            </li>
          );
        })}

        <li
          className={cn(
            'absolute -z-[1] bg-text-primary border-b-2 border-b-accent-primary rounded-full opacity-0',
            'transition-all',
            hasAnyActive && 'opacity-100',
          )}
          role="presentation"
          style={{
            top: 'anchor(top)',
            left: 'anchor(left)',
            width: 'anchor-size(width)',
            height: 'anchor-size(height)',
            positionAnchor: activePathAnchorName,
          }}
        />
      </ul>
    </nav>
  );
};

const DesktopNavbarFallback = () => {
  const { t } = useTranslation();
  const activePath = useActivePath();

  return (
    <nav>
      <ul className="flex items-center p-1 bg-bg-card rounded-full border border-border-button">
        {menuItems.map(({ path, name, icon: Icon, ...props }) => {
          const isActive = path === activePath;

          return (
            <li key={path}>
              <Link
                {...props}
                to={path}
                className={cn(
                  'h-10 px-4 xl:px-6 text-sm xl:text-base inline-flex items-center justify-center whitespace-nowrap',
                  'hover:text-current focus-visible:text-current rounded-full transition-all',
                  isActive &&
                    '!text-bg-base bg-text-primary border-b-2 border-b-accent-primary',
                )}
                aria-label={t(name)}
                aria-current={isActive ? 'page' : undefined}
              >
                {path === Routes.Root ? (
                  <Icon className="size-6 stroke-[1.5]" />
                ) : (
                  <span>{t(name)}</span>
                )}
              </Link>
            </li>
          );
        })}
      </ul>
    </nav>
  );
};

export function DesktopNavbar() {
  return supportsCssAnchor ? (
    <DesktopNavbarWithAnchor />
  ) : (
    <DesktopNavbarFallback />
  );
}

function MobileNavItem({
  label,
  icon: Icon,
  isActive,
  onClick,
  ...linkProps
}: {
  label: string;
  icon: React.ComponentType<{ className?: string }>;
  isActive?: boolean;
  onClick?: () => void;
  to: string;
  'data-testid'?: string;
}) {
  return (
    <Link
      {...linkProps}
      onClick={onClick}
      className={cn(
        'flex w-full items-center gap-3.5 px-4 py-3.5 text-base',
        'text-text-secondary transition-colors hover:bg-bg-card hover:text-text-primary',
        'focus-visible:bg-bg-card focus-visible:text-text-primary',
        isActive &&
          'border-l-2 border-text-primary bg-bg-card font-medium text-text-primary',
      )}
      aria-current={isActive ? 'page' : undefined}
    >
      <Icon className="size-5 shrink-0 stroke-[1.5]" />
      <span className="truncate">{label}</span>
    </Link>
  );
}

type MobileNavbarProps = {
  renderFooter?: (close: () => void) => React.ReactNode;
};

export function MobileNavbar({ renderFooter }: MobileNavbarProps) {
  const { t } = useTranslation();
  const activePath = useActivePath();
  const [open, setOpen] = useState(false);

  const close = () => setOpen(false);

  return (
    <Sheet open={open} onOpenChange={setOpen}>
      <SheetTrigger asChild>
        <Button
          variant="ghost"
          size="icon"
          className="size-10 shrink-0"
          aria-label="Menu"
        >
          <LucideMenu className="size-6 stroke-[1.75]" />
        </Button>
      </SheetTrigger>

      <SheetContent
        side="left"
        closeIcon={false}
        className="flex w-[min(85vw,18rem)] flex-col gap-0 p-0 sm:w-72"
      >
        <div className="flex shrink-0 justify-center py-5">
          <img src="/logo.svg" alt="RAGFlow logo" className="size-9" />
        </div>

        <nav className="min-h-0 flex-1 overflow-y-auto py-3">
          <ul className="space-y-1">
            {menuItems.map(({ path, name, icon, ...props }) => (
              <li key={path}>
                <MobileNavItem
                  {...props}
                  to={path}
                  label={t(name)}
                  icon={icon}
                  isActive={path === activePath}
                  onClick={close}
                />
              </li>
            ))}
          </ul>
        </nav>

        {renderFooter?.(close)}
      </SheetContent>
    </Sheet>
  );
}

const GlobalNavbar = DesktopNavbar;

export default GlobalNavbar;
