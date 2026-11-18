/*
 *  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 */

import { lazy, memo, Suspense } from 'react';
import { createBrowserRouter, Navigate, type RouteObject } from 'react-router';
import FallbackComponent from './components/fallback-component';
import { QueryPreservingNavigate } from './components/query-preserving-navigate';
import { IS_ENTERPRISE } from './pages/admin/utils';

export enum Routes {
  Root = '/',
  Login = '/login-next',
  Logout = '/logout',
  Home = '/home',
  Datasets = '/datasets',
  DatasetBase = '/dataset',
  Files = '/files',
  Dataset = `${Routes.DatasetBase}/${Routes.Files}`,
  Agent = '/agent',
  AgentTemplates = '/agent-templates',
  Agents = '/agents',
  Pipeline = '/pipeline',
  Explore = '/explore',
  AgentExplore = `${Routes.Agent}/:id/explore`,
  AgentSetting = `${Routes.Agent}/:id/setting`,
  Memories = '/memories',
  Memory = '/memory',
  MemoryMessage = '/memory-message',
  MemorySetting = '/memory-setting',
  AgentList = '/agent-list',
  Searches = '/searches',
  Search = '/search',
  SearchShare = '/search/share',
  Chats = '/chats',
  Chat = '/chat',

  Skills = '/files/skills',
  ProfileSetting = '/profile-setting',
  Profile = '/profile',
  Api = '/api',
  Mcp = '/mcp',
  Team = '/team',
  Integrations = '/integrations',
  Plan = '/plan',
  Model = '/model',
  Prompt = '/prompt',
  CompilationTemplatesEditNext = '/compilation-templates/edit-next',
  DataSource = '/data-source',
  DataSourceDetailPage = '/data-source-detail-page',
  ChatChannel = '/chat-channel',
  ProfileMcp = `${ProfileSetting}${Mcp}`,
  ProfileTeam = `${ProfileSetting}${Team}`,
  ProfilePlan = `${ProfileSetting}${Plan}`,
  ProfileModel = `${ProfileSetting}${Model}`,
  ProfilePrompt = `${ProfileSetting}${Prompt}`,
  ProfileProfile = `${ProfileSetting}${Profile}`,
  DatasetTesting = '/retrieval',
  Chunk = '/chunk',
  Parsed = '/parsed',
  ParsedResult = `${Chunk}${Parsed}`,
  KnowledgeGraph = '/knowledge-graph',
  Compilation = '/compilation',
  AgentLogPage = '/agent-log-page',
  AgentShare = '/agent/share',
  ChatShare = `${Chats}/share`,
  ChatWidget = `${Chats}/widget`,
  UserSetting = '/user-setting',
  DataSetOverview = '/logs',
  DataSetSetting = '/configuration',
  DataflowResult = '/dataflow-result',
  Admin = '/admin',
  AdminServices = `${Admin}/services`,
  AdminUserManagement = `${Admin}/users`,
  AdminSandboxSettings = `${Admin}/sandbox-settings`,
  AdminWhitelist = `${Admin}/whitelist`,
  AdminRoles = `${Admin}/roles`,
  AdminMonitoring = `${Admin}/monitoring`,
}

const defaultRouteFallback = (
  <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/30 backdrop-blur-[1px]">
    <div className="h-8 w-8 animate-spin rounded-full border-2 border-white/70 border-t-transparent" />
  </div>
);

type LazyRouteConfig = Omit<RouteObject, 'Component' | 'children'> & {
  Component?: () => Promise<{ default: React.ComponentType<any> }>;
  children?: LazyRouteConfig[];
};

const withLazyRoute = (
  importer: () => Promise<{ default: React.ComponentType<any> }>,
  fallback: React.ReactNode = defaultRouteFallback,
) => {
  const LazyComponent = lazy(importer);
  const Wrapped: React.FC<any> = (props) => (
    <Suspense fallback={fallback}>
      <LazyComponent {...props} />
    </Suspense>
  );
  Wrapped.displayName = `LazyRoute(${
    (LazyComponent as unknown as React.ComponentType<any>).displayName ||
    LazyComponent.name ||
    'Component'
  })`;
  return process.env.NODE_ENV === 'development' ? LazyComponent : memo(Wrapped);
};

const routeConfigOptions = [
  {
    path: '/login',
    Component: () => import('@/pages/login-next'),
    layout: false,
  },
  {
    path: '/login-next',
    Component: () => import('@/pages/login-next'),
    layout: false,
  },
  {
    path: Routes.ChatShare,
    Component: () => import('@/pages/next-chats/share'),
    layout: false,
  },
  {
    path: Routes.AgentShare,
    Component: () => import('@/pages/agent/share'),
    layout: false,
  },
  {
    path: Routes.ChatWidget,
    Component: () => import('@/pages/next-chats/widget'),
    layout: false,
  },
  {
    path: Routes.AgentList,
    Component: () => import('@/pages/agents'),
  },
  {
    path: '/document/:id',
    Component: () => import('@/pages/document-viewer'),
    layout: false,
  },
  {
    path: '/*',
    Component: () => import('@/pages/404'),
    layout: false,
  },
  {
    path: Routes.Root,
    layout: false,
    Component: () => import('@/layouts/root-layout'),
    children: [
      {
        path: Routes.Root,
        Component: () => import('@/pages/home'),
      },
    ],
  },
  {
    path: Routes.Chat + '/:id',
    Component: () => import('@/pages/next-chats/chat'),
  },
  {
    path: Routes.UserSetting,
    Component: () => import('@/pages/user-setting'),
    layout: false,
    children: [
      {
        path: Routes.UserSetting,
        element: <Navigate to={`${Routes.UserSetting}/profile`} replace />,
      },
      {
        path: `${Routes.UserSetting}/profile`,
        Component: () => import('@/pages/user-setting/profile'),
      },
      {
        path: `${Routes.UserSetting}${Routes.Api}`,
        Component: () => import('@/pages/user-setting/setting-api'),
      },
      {
        path: `${Routes.UserSetting}${Routes.Model}`,
        element: (
          <QueryPreservingNavigate
            to={`${Routes.Integrations}${Routes.Model}`}
          />
        ),
      },
      {
        path: `${Routes.UserSetting}${Routes.Team}`,
        element: <QueryPreservingNavigate to={Routes.Team} />,
      },
      {
        path: `${Routes.UserSetting}${Routes.Mcp}`,
        element: (
          <QueryPreservingNavigate to={`${Routes.Integrations}${Routes.Mcp}`} />
        ),
      },
      {
        path: `${Routes.UserSetting}${Routes.DataSource}`,
        element: (
          <QueryPreservingNavigate
            to={`${Routes.Integrations}${Routes.DataSource}`}
          />
        ),
      },
      {
        path: `${Routes.UserSetting}${Routes.ChatChannel}`,
        element: (
          <QueryPreservingNavigate
            to={`${Routes.Integrations}${Routes.ChatChannel}`}
          />
        ),
      },
    ],
  },
  {
    path: Routes.Root,
    Component: () => import('@/layouts/root-layout'),
    children: [
      {
        Component: () => import('@/layouts/nav-group-layout'),
        children: [
          {
            path: Routes.Datasets,
            Component: () => import('@/pages/datasets'),
          },
          {
            path: Routes.Memories,
            Component: () => import('@/pages/memories'),
          },
        ],
      },
      {
        path: Routes.DatasetBase,
        Component: () => import('@/pages/dataset'),
        children: [
          {
            path: `${Routes.Dataset}/:id`,
            Component: () => import('@/pages/dataset/dataset'),
          },
          {
            path: `${Routes.DatasetBase}${Routes.DatasetTesting}/:id`,
            Component: () => import('@/pages/dataset/testing'),
          },
          {
            path: `${Routes.DatasetBase}${Routes.KnowledgeGraph}/:id`,
            Component: () => import('@/pages/dataset/knowledge-graph'),
          },
          {
            path: `${Routes.DatasetBase}${Routes.DataSetOverview}/:id`,
            Component: () => import('@/pages/dataset/dataset-overview'),
          },
          {
            path: `${Routes.DatasetBase}${Routes.DataSetSetting}/:id`,
            Component: () => import('@/pages/dataset/setting'),
          },
          {
            path: `${Routes.DatasetBase}${Routes.Compilation}/:id`,
            Component: () => import('@/pages/dataset/compilation'),
          },
        ],
      },
      {
        // The chunk page shows the (collapsed) primary sidebar like the
        // dataset pages it is entered from, so it lives under the root layout.
        path: `${Routes.ParsedResult}/chunks`,
        Component: () => import('@/pages/chunk'),
      },
      {
        Component: () => import('@/layouts/nav-group-layout'),
        children: [
          {
            path: Routes.Chats,
            Component: () => import('@/pages/next-chats'),
          },
          {
            path: Routes.Searches,
            Component: () => import('@/pages/next-searches'),
          },
        ],
      },
      {
        path: `${Routes.Search}/:id`,
        layout: false,
        Component: () => import('@/pages/next-search'),
      },
      {
        path: Routes.Agents,
        Component: () => import('@/pages/agents'),
      },
      {
        path: Routes.CompilationTemplatesEditNext,
        Component: () =>
          import('@/pages/user-setting/compilation-templates/edit-next'),
      },
      {
        path: `${Routes.CompilationTemplatesEditNext}/:id`,
        Component: () =>
          import('@/pages/user-setting/compilation-templates/edit-next'),
      },
      {
        path: Routes.AgentTemplates,
        layout: false,
        Component: () => import('@/pages/agents/agent-templates'),
      },
      {
        path: `${Routes.Memory}`,
        Component: () => import('@/pages/memory'),
        children: [
          {
            path: `${Routes.Memory}/${Routes.MemoryMessage}/:id`,
            Component: () => import('@/pages/memory/memory-message'),
          },
          {
            path: `${Routes.Memory}/${Routes.MemorySetting}/:id`,
            Component: () => import('@/pages/memory/memory-setting'),
          },
        ],
      },
      {
        path: Routes.Files,
        Component: () => import('@/pages/files'),
      },
      {
        path: Routes.Skills,
        Component: () => import('@/pages/skills'),
      },
      {
        path: Routes.Integrations,
        element: (
          <Navigate to={`${Routes.Integrations}${Routes.Model}`} replace />
        ),
      },
      {
        Component: () => import('@/layouts/settings-page-layout'),
        children: [
          {
            path: `${Routes.Integrations}${Routes.Model}`,
            Component: () => import('@/pages/user-setting/setting-model'),
          },
          {
            path: `${Routes.Integrations}${Routes.DataSource}`,
            Component: () => import('@/pages/user-setting/data-source'),
          },
          {
            path: `${Routes.Integrations}${Routes.ChatChannel}`,
            Component: () => import('@/pages/user-setting/chat-channel'),
          },
          {
            path: `${Routes.Integrations}${Routes.Mcp}`,
            Component: () => import('@/pages/user-setting/mcp'),
          },
          {
            path: Routes.Team,
            Component: () => import('@/pages/user-setting/setting-team'),
          },
        ],
      },
      {
        path: `${Routes.Integrations}${Routes.DataSource}${Routes.DataSourceDetailPage}`,
        Component: () =>
          import('@/pages/user-setting/data-source/data-source-detail-page'),
      },
      {
        path: `${Routes.UserSetting}${Routes.DataSource}${Routes.DataSourceDetailPage}`,
        element: (
          <QueryPreservingNavigate
            to={`${Routes.Integrations}${Routes.DataSource}${Routes.DataSourceDetailPage}`}
          />
        ),
      },
    ],
  },
  {
    path: `${Routes.SearchShare}`,
    Component: () => import('@/pages/next-search/share'),
  },
  {
    path: Routes.Root,
    Component: () => import('@/layouts/root-layout'),
    children: [
      {
        path: Routes.Agent,
        children: [
          {
            path: `${Routes.Agent}/:id`,
            Component: () => import('@/pages/agent/layout'),
            children: [
              {
                index: true,
                Component: () => import('@/pages/agent'),
              },
              {
                path: Routes.AgentExplore,
                Component: () => import('@/pages/agent/explore'),
                errorElement: <FallbackComponent />,
              },
              {
                path: Routes.AgentSetting,
                Component: () => import('@/pages/agent/setting'),
              },
            ],
          },
        ],
      },
      {
        path: `${Routes.Pipeline}/:id`,
        Component: () => import('@/pages/builtin-pipeline'),
      },
    ],
  },
  {
    path: `${Routes.AgentLogPage}/:id`,
    Component: () => import('@/pages/agents/agent-log-page'),
  },
  {
    path: `${Routes.DataflowResult}`,
    Component: () => import('@/pages/dataflow-result'),
  },
  {
    path: Routes.Admin,
    Component: () => import('@/pages/admin/layouts/root-layout'),
    children: [
      {
        path: Routes.Admin,
        Component: () => import('@/pages/admin/login'),
      },
      {
        path: Routes.Admin,
        Component: () => import('@/pages/admin/layouts/authorized-layout'),
        children: [
          {
            path: `${Routes.AdminUserManagement}/:id`,
            Component: () => import('@/pages/admin/user-detail'),
          },
          {
            Component: () => import('@/pages/admin/layouts/navigation-layout'),
            children: [
              {
                path: Routes.AdminServices,
                Component: () => import('@/pages/admin/service-status'),
              },
              {
                path: Routes.AdminUserManagement,
                Component: () => import('@/pages/admin/users'),
              },
              {
                path: Routes.AdminSandboxSettings,
                Component: () => import('@/pages/admin/sandbox-settings'),
              },
              ...(IS_ENTERPRISE
                ? [
                    {
                      path: Routes.AdminWhitelist,
                      Component: () => import('@/pages/admin/whitelist'),
                    },
                    {
                      path: Routes.AdminRoles,
                      Component: () => import('@/pages/admin/roles'),
                    },
                    {
                      path: Routes.AdminMonitoring,
                      Component: () => import('@/pages/admin/monitoring'),
                    },
                  ]
                : []),
            ],
          },
        ],
      },
    ],
  } satisfies LazyRouteConfig,
];

const wrapRoutes = (routes: LazyRouteConfig[]): RouteObject[] =>
  routes.map((item) => {
    const { Component, children, ...rest } = item;
    const next: RouteObject = { ...rest, errorElement: <FallbackComponent /> };
    if (Component) {
      next.Component = withLazyRoute(Component);
    }
    if (children) {
      next.children = wrapRoutes(children);
    }
    return next;
  });

const routeConfig = wrapRoutes(routeConfigOptions);

const routers = createBrowserRouter(routeConfig, {
  basename: import.meta.env.VITE_BASE_URL || '/',
});

export { routers };
