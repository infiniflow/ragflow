import { Routes } from '@/routes';
import {
  LucideBoxes,
  LucideHome,
  LucideLibrary,
  LucideMessagesSquare,
  LucidePlug,
  LucideSparkles,
  LucideUsers,
  type LucideIcon,
} from 'lucide-react';
import { useMemo } from 'react';
import { useLocation } from 'react-router';

// Match on path-segment boundaries, not a loose substring, so e.g.
// "/user-setting/chat-channel" does not match the "/chat" prefix.
export function matchesPath(pathname: string, candidate: string) {
  return pathname === candidate || pathname.startsWith(`${candidate}/`);
}

export enum NavItemKey {
  Home = 'home',
  Context = 'context',
  Application = 'application',
  Studio = 'studio',
  Library = 'library',
  Integrations = 'integrations',
  Team = 'team',
}

export interface NavItem {
  key: NavItemKey;
  path: string;
  labelKey: string;
  icon: LucideIcon;
  // Path prefixes (besides the destination itself) that keep this item
  // highlighted, e.g. detail pages living outside the list route.
  activeAliases: string[];
}

export const NavItems: NavItem[] = [
  {
    key: NavItemKey.Home,
    path: Routes.Root,
    labelKey: 'header.home',
    icon: LucideHome,
    activeAliases: [],
  },
  {
    key: NavItemKey.Context,
    path: Routes.Datasets,
    labelKey: 'sidebar.context',
    icon: LucideBoxes,
    activeAliases: [Routes.DatasetBase, Routes.Memories, Routes.Memory],
  },
  {
    key: NavItemKey.Application,
    path: Routes.Chats,
    labelKey: 'sidebar.application',
    icon: LucideMessagesSquare,
    activeAliases: [Routes.Chat, Routes.Searches, Routes.Search],
  },
  {
    key: NavItemKey.Studio,
    path: Routes.Agents,
    labelKey: 'sidebar.studio',
    icon: LucideSparkles,
    activeAliases: [
      Routes.AgentTemplates,
      Routes.Agent,
      Routes.Pipeline,
      Routes.AgentLogPage,
      Routes.CompilationTemplatesEditNext,
    ],
  },
  {
    key: NavItemKey.Library,
    path: Routes.Files,
    labelKey: 'sidebar.library',
    icon: LucideLibrary,
    activeAliases: [],
  },
  {
    key: NavItemKey.Team,
    path: Routes.Team,
    labelKey: 'sidebar.team',
    icon: LucideUsers,
    activeAliases: [],
  },
];

export interface IntegrationChild {
  key: string;
  path: string;
  labelKey: string;
}

export const IntegrationChildren: IntegrationChild[] = [
  {
    key: 'model',
    path: `${Routes.Integrations}${Routes.Model}`,
    labelKey: 'sidebar.models',
  },
  {
    key: 'data-source',
    path: `${Routes.Integrations}${Routes.DataSource}`,
    labelKey: 'setting.dataSources',
  },
  {
    key: 'chat-channel',
    path: `${Routes.Integrations}${Routes.ChatChannel}`,
    labelKey: 'setting.chatChannels',
  },
  {
    key: 'mcp',
    path: `${Routes.Integrations}${Routes.Mcp}`,
    labelKey: 'sidebar.mcp',
  },
];

export const IntegrationsNavItem = {
  key: NavItemKey.Integrations,
  labelKey: 'sidebar.integrations',
  icon: LucidePlug,
};

// Page-level pill tabs rendered above the list pages of a sidebar group.
export interface PageTabGroup {
  key: NavItemKey.Context | NavItemKey.Application;
  tabs: { path: string; labelKey: string }[];
}

export const PageTabGroups: PageTabGroup[] = [
  {
    key: NavItemKey.Context,
    tabs: [
      { path: Routes.Datasets, labelKey: 'header.dataset' },
      { path: Routes.Memories, labelKey: 'header.memories' },
    ],
  },
  {
    key: NavItemKey.Application,
    tabs: [
      { path: Routes.Chats, labelKey: 'header.chat' },
      { path: Routes.Searches, labelKey: 'header.search' },
    ],
  },
];

export function getActiveNavItemKey(pathname: string): NavItemKey | undefined {
  const item = NavItems.find(
    (x) =>
      (x.path === Routes.Root
        ? pathname === Routes.Root
        : matchesPath(pathname, x.path)) ||
      x.activeAliases.some((alias) => matchesPath(pathname, alias)),
  );
  return item?.key;
}

export function getActiveIntegrationKey(pathname: string): string | undefined {
  return IntegrationChildren.find((x) => matchesPath(pathname, x.path))?.key;
}

export function getActivePageTab(pathname: string) {
  const group = PageTabGroups.find((g) =>
    g.tabs.some((tab) => matchesPath(pathname, tab.path)),
  );
  if (!group) {
    return undefined;
  }
  const activeTab = group.tabs.find((tab) => matchesPath(pathname, tab.path));
  return { group, activePath: activeTab?.path ?? group.tabs[0].path };
}

export function useActiveNavigation() {
  const { pathname } = useLocation();

  return useMemo(
    () => ({
      activeNavKey: getActiveNavItemKey(pathname),
      activeIntegrationKey: getActiveIntegrationKey(pathname),
      pageTab: getActivePageTab(pathname),
    }),
    [pathname],
  );
}
