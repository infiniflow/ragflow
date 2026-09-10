import { useId, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Link, useLocation } from 'react-router';

import { LucideHouse, LucideMenu } from 'lucide-react';

import { Button } from '@/components/ui/button';
import { Sheet, SheetContent, SheetTrigger } from '@/components/ui/sheet';
import { cn } from '@/lib/utils';
import { Routes } from '@/routes';
import { supportsCssAnchor } from '@/utils/css-support';
import { HomeIcon } from '@/components/svg-icon';
import { BrandMark } from './brand-mark';

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

// Wrapper so dataset icon shares the same ComponentType<{ className? }>
// shape as Lucide icons in menuItems (avoids Element-vs-component union).
const MenuItemsIcon = ({
  className,
  name,
}: {
  className?: string;
  name?: string;
}) => <HomeIcon imgClass={className} name={name || 'datasets'} width={20} />;

const menuItems = [
  { path: Routes.Root, name: 'header.home', icon: LucideHouse },
  {
    path: Routes.Datasets,
    name: 'header.dataset',
    icon: MenuItemsIcon,
    icon_name: 'datasets',
  },
  {
    path: Routes.Chats,
    name: 'header.chat',
    icon: MenuItemsIcon,
    icon_name: 'chats',
    'data-testid': 'nav-chat',
  },
  {
    path: Routes.Searches,
    name: 'header.search',
    icon: MenuItemsIcon,
    icon_name: 'searches',
    'data-testid': 'nav-search',
  },
  {
    path: Routes.Agents,
    name: 'header.flow',
    icon: MenuItemsIcon,
    icon_name: 'agents',
    'data-testid': 'nav-agent',
  },
  {
    path: Routes.Memories,
    name: 'header.memories',
    icon: MenuItemsIcon,
    icon_name: 'memory',
  },
  {
    path: Routes.Files,
    name: 'header.fileManager',
    icon: MenuItemsIcon,
    icon_name: 'file',
  },
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
      <ul className="relative flex items-center gap-1 rounded-xl border border-cable-border bg-cable-surface-muted p-1">
        {menuItems.map(({ path, name, icon: Icon, ...props }) => {
          const isActive = path === activePath;
          const anchorName = `--${navbarAnchorNamePrefix}${path === Routes.Root ? '-root' : path.replace('/', '-')}`;

          return (
            <li key={path} className="relative" style={{ anchorName }}>
              <Link
                {...props}
                to={path}
                className={cn(
                  'inline-flex items-center justify-center whitespace-nowrap rounded-lg px-3 py-1.5 text-sm xl:text-base',
                  'transition-colors',
                  isActive
                    ? 'font-semibold text-cable-nav-active-text'
                    : 'text-cable-nav hover:text-cable-nav-hover focus-visible:text-cable-nav-hover',
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

        {/* Sliding highlight: a soft brand-tinted capsule closing with a 2px
            indicator line, positioned over the active item by CSS anchor
            positioning. */}
        <li
          className={cn(
            'absolute -z-[1] rounded-lg border-b-2 border-b-cable-nav-indicator bg-cable-nav-active-bg opacity-0',
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
      <ul className="flex items-center gap-1 rounded-xl border border-cable-border bg-cable-surface-muted p-1">
        {menuItems.map(({ path, name, icon: Icon, ...props }) => {
          const isActive = path === activePath;

          return (
            <li key={path}>
              <Link
                {...props}
                to={path}
                className={cn(
                  'inline-flex items-center justify-center whitespace-nowrap rounded-lg px-3 py-1.5 text-sm xl:text-base',
                  'transition-colors',
                  isActive
                    ? 'border-b-2 border-b-cable-nav-indicator bg-cable-nav-active-bg font-semibold text-cable-nav-active-text'
                    : 'text-cable-nav hover:bg-cable-nav-active-bg hover:text-cable-nav-hover focus-visible:text-cable-nav-hover',
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
  icon_name,
  isActive,
  onClick,
  ...linkProps
}: {
  label: string;
  icon: React.ComponentType<{ className?: string; name?: string }>;
  icon_name?: string;
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
        'text-cable-nav transition-colors hover:bg-cable-nav-active-bg hover:text-cable-nav-hover',
        'focus-visible:bg-cable-nav-active-bg focus-visible:text-cable-nav-hover',
        isActive &&
          'border-l-2 border-cable-nav-indicator bg-cable-nav-active-bg font-semibold text-cable-nav-active-text',
      )}
      aria-current={isActive ? 'page' : undefined}
    >
      <Icon className="size-5 shrink-0 stroke-[1.5]" name={icon_name} />
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
        <div className="flex shrink-0 items-center justify-center gap-3 py-5">
          <BrandMark label={t('header.brandShort')} />
          <span className="text-base font-semibold tracking-tight text-cable-brand">
            {t('header.brandShort')}
          </span>
        </div>

        <nav className="min-h-0 flex-1 overflow-y-auto py-3">
          <ul className="space-y-1">
            {menuItems.map(({ path, name, icon_name, icon, ...props }) => (
              <li key={path}>
                <MobileNavItem
                  {...props}
                  to={path}
                  label={t(name)}
                  icon={icon}
                  icon_name={icon_name}
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
