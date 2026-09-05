import { useId, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Link, useLocation } from 'react-router';

import {
  FilePenLine,
  LucideBrain,
  LucideCpu,
  LucideDatabase,
  LucideFolderOpen,
  LucideHouse,
  LucideMenu,
  LucideMessageSquareText,
  LucideNetwork,
  LucideSearch,
  type LucideIcon,
} from 'lucide-react';

import { Button } from '@/components/ui/button';
import { Sheet, SheetContent, SheetTrigger } from '@/components/ui/sheet';
import type { NavigationSection } from '@/constants/navigation';
import { useSystemConfig } from '@/hooks/use-system-request';
import { cn } from '@/lib/utils';
import { Routes } from '@/routes';
import { supportsCssAnchor } from '@/utils/css-support';

const PathMap = {
  [Routes.Datasets]: [Routes.Datasets, Routes.DatasetBase],
  [Routes.Chats]: [Routes.Chats, Routes.Chat],
  [Routes.Searches]: [Routes.Searches, Routes.Search],
  [Routes.Agents]: [Routes.Agents, Routes.AgentTemplates],
  [Routes.Memories]: [Routes.Memories, Routes.Memory, Routes.MemoryMessage],
  [Routes.OpenMetadata]: [Routes.OpenMetadata],
  [Routes.BusinessDocuments]: [Routes.BusinessDocuments],
  [Routes.Files]: [Routes.Files],
} as const;

// Match on path-segment boundaries, not a loose substring, so e.g.
// "/user-setting/chat-channel" does not match the "/chat" tab.
const matchesPath = (pathname: string, candidate: string) =>
  pathname === candidate || pathname.startsWith(`${candidate}/`);

const menuItems: Array<{
  path: Routes;
  name: string;
  icon: LucideIcon;
  fallbackName?: string;
  section?: NavigationSection;
  'data-testid'?: string;
}> = [
  {
    path: Routes.Root,
    name: 'header.home',
    icon: LucideHouse,
    section: 'home',
    'data-testid': 'nav-home',
  },
  {
    path: Routes.Datasets,
    name: 'header.dataset',
    icon: LucideDatabase,
    section: 'dataset',
  },
  {
    path: Routes.Chats,
    name: 'header.chat',
    icon: LucideMessageSquareText,
    section: 'chat',
    'data-testid': 'nav-chat',
  },
  {
    path: Routes.Searches,
    name: 'header.search',
    icon: LucideSearch,
    section: 'search',
    'data-testid': 'nav-search',
  },
  {
    path: Routes.Agents,
    name: 'header.flow',
    icon: LucideCpu,
    section: 'agent',
    'data-testid': 'nav-agent',
  },
  {
    path: Routes.Memories,
    name: 'header.memories',
    icon: LucideBrain,
    section: 'memory',
  },
  {
    path: Routes.OpenMetadata,
    name: 'header.openMetadata',
    icon: LucideNetwork,
    section: 'catalog',
    'data-testid': 'nav-openmetadata',
  },
  {
    path: Routes.BusinessDocuments,
    name: 'header.businessDocuments',
    fallbackName: 'Business docs',
    icon: FilePenLine,
    section: 'business_documents',
    'data-testid': 'nav-business-documents',
  },
  {
    path: Routes.Files,
    name: 'header.fileManager',
    icon: LucideFolderOpen,
    section: 'file_manager',
  },
];

function useVisibleMenuItems() {
  const { config } = useSystemConfig();

  return useMemo(() => {
    const visibleSections = new Set(config?.visibleSections);
    return menuItems.filter(
      ({ section }) => !section || !config || visibleSections.has(section),
    );
  }, [config]);
}

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
  const visibleMenuItems = useVisibleMenuItems();
  const navbarAnchorNamePrefix = useId().replace(/:/g, '');

  const activePathAnchorName = `--${navbarAnchorNamePrefix}${activePath === Routes.Root ? '-root' : activePath.replace('/', '-')}`;

  const hasAnyActive = useMemo(
    () => visibleMenuItems.some(({ path }) => path === activePath),
    [activePath, visibleMenuItems],
  );

  return (
    <nav>
      <ul className="relative flex items-center p-1 bg-bg-card rounded-full border border-border-button">
        {visibleMenuItems.map((item) => {
          const { path, name, fallbackName, icon: Icon } = item;
          const isActive = path === activePath;
          const anchorName = `--${navbarAnchorNamePrefix}${path === Routes.Root ? '-root' : path.replace('/', '-')}`;

          return (
            <li key={path} className="relative" style={{ anchorName }}>
              <Link
                data-testid={item['data-testid']}
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
                    <span className="sr-only">
                      {t(name, { defaultValue: fallbackName ?? name })}
                    </span>
                  </>
                ) : (
                  <span>{t(name, { defaultValue: fallbackName ?? name })}</span>
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
  const visibleMenuItems = useVisibleMenuItems();

  return (
    <nav>
      <ul className="flex items-center p-1 bg-bg-card rounded-full border border-border-button">
        {visibleMenuItems.map((item) => {
          const { path, name, fallbackName, icon: Icon } = item;
          const isActive = path === activePath;

          return (
            <li key={path}>
              <Link
                data-testid={item['data-testid']}
                to={path}
                className={cn(
                  'h-10 px-4 xl:px-6 text-sm xl:text-base inline-flex items-center justify-center whitespace-nowrap',
                  'hover:text-current focus-visible:text-current rounded-full transition-all',
                  isActive &&
                    '!text-bg-base bg-text-primary border-b-2 border-b-accent-primary',
                )}
                aria-label={t(name, { defaultValue: fallbackName ?? name })}
                aria-current={isActive ? 'page' : undefined}
              >
                {path === Routes.Root ? (
                  <Icon className="size-6 stroke-[1.5]" />
                ) : (
                  <span>{t(name, { defaultValue: fallbackName ?? name })}</span>
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
  const visibleMenuItems = useVisibleMenuItems();
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
          <img src="/logo.svg" alt="Логотип Агент Раггер" className="size-9" />
        </div>

        <nav className="min-h-0 flex-1 overflow-y-auto py-3">
          <ul className="space-y-1">
            {visibleMenuItems.map((item) => (
              <li key={item.path}>
                <MobileNavItem
                  data-testid={item['data-testid']}
                  to={item.path}
                  label={t(item.name, {
                    defaultValue: item.fallbackName ?? item.name,
                  })}
                  icon={item.icon}
                  isActive={item.path === activePath}
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
