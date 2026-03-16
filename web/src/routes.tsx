import { lazy, memo, Suspense } from 'react';
import { createBrowserRouter, Navigate, type RouteObject } from 'react-router';
import FallbackComponent from './components/fallback-component';
import { IS_ENTERPRISE } from './pages/admin/utils';

export enum Routes {
  Root = '/',
  Login = '/login-next',
  Logout = '/logout',
  Home = '/home',
  Datasets = '/datasets',
  DatasetBase = '/dataset',
  Dataset = `${Routes.DatasetBase}${Routes.DatasetBase}`,
  Agent = '/agent',
  AgentTemplates = '/agent-templates',
  Agents = '/agents',
  Explore = '/explore',
  AgentExplore = `${Routes.Agent}/:id/explore`,
  Memories = '/memories',
  Memory = '/memory',
  MemoryMessage = '/memory-message',
  MemorySetting = '/memory-setting',
  AgentList = '/agent-list',
  Searches = '/next-searches',
  Search = '/next-search',
  SearchShare = '/next-search/share',
  Chats = '/next-chats',
  Chat = '/next-chat',
  Files = '/files',
  ProfileSetting = '/profile-setting',
  Profile = '/profile',
  Api = '/api',
  Mcp = '/mcp',
  Team = '/team',
  Plan = '/plan',
  Model = '/model',
  Prompt = '/prompt',
  DataSource = '/data-source',
  DataSourceDetailPage = '/data-source-detail-page',
  ProfileMcp = `${ProfileSetting}${Mcp}`,
  ProfileTeam = `${ProfileSetting}${Team}`,
  ProfilePlan = `${ProfileSetting}${Plan}`,
  ProfileModel = `${ProfileSetting}${Model}`,
  ProfilePrompt = `${ProfileSetting}${Prompt}`,
  ProfileProfile = `${ProfileSetting}${Profile}`,
  DatasetTesting = '/testing',
  Chunk = '/chunk',
  ChunkResult = `${Chunk}${Chunk}`,
  Parsed = '/parsed',
  ParsedResult = `${Chunk}${Parsed}`,
  Result = '/result',
  ResultView = `${Chunk}${Result}`,
  KnowledgeGraph = '/knowledge-graph',
  AgentLogPage = '/agent-log-page',
  AgentShare = '/agent/share',
  ChatShare = `${Chats}/share`,
  ChatWidget = `${Chats}/widget`,
  UserSetting = '/user-setting',
  DataSetOverview = '/dataset-overview',
  DataSetSetting = '/dataset-setting',
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
    Component: () => import('@/layouts/next'),
    wrappers: ['@/wrappers/auth'],
    children: [
      {
        path: Routes.Root,
        Component: () => import('@/pages/home'),
      },
    ],
  },
  {
    path: Routes.Datasets,
    layout: false,
    Component: () => import('@/layouts/next'),
    children: [
      {
        path: Routes.Datasets,
        Component: () => import('@/pages/datasets'),
      },
    ],
  },
  {
    path: Routes.Chats,
    layout: false,
    Component: () => import('@/layouts/next'),
    children: [
      {
        path: Routes.Chats,
        Component: () => import('@/pages/next-chats'),
      },
    ],
  },
  {
    path: Routes.Chat + '/:id',
    layout: false,
    Component: () => import('@/pages/next-chats/chat'),
  },
  {
    path: Routes.Searches,
    layout: false,
    Component: () => import('@/layouts/next'),
    children: [
      {
        path: Routes.Searches,
        Component: () => import('@/pages/next-searches'),
      },
    ],
  },
  {
    path: Routes.Memories,
    layout: false,
    Component: () => import('@/layouts/next'),
    children: [
      {
        path: Routes.Memories,
        Component: () => import('@/pages/memories'),
      },
    ],
  },
  {
    path: `${Routes.Memory}`,
    layout: false,
    Component: () => import('@/layouts/next'),
    children: [
      {
        path: `${Routes.Memory}`,
        layout: false,
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
    ],
  },
  {
    path: `${Routes.Search}/:id`,
    layout: false,
    Component: () => import('@/pages/next-search'),
  },
  {
    path: `${Routes.SearchShare}`,
    layout: false,
    Component: () => import('@/pages/next-search/share'),
  },
  {
    path: Routes.Agents,
    layout: false,
    Component: () => import('@/layouts/next'),
    children: [
      {
        path: Routes.Agents,
        Component: () => import('@/pages/agents'),
      },
    ],
  },
  {
    path: `${Routes.AgentLogPage}/:id`,
    layout: false,
    Component: () => import('@/pages/agents/agent-log-page'),
  },
  {
    path: `${Routes.Agent}/:id`,
    layout: false,
    Component: () => import('@/pages/agent'),
  },
  {
    path: Routes.AgentExplore,
    layout: false,
    Component: () => import('@/pages/agent/explore'),
    errorElement: <FallbackComponent />,
  },
  {
    path: Routes.AgentTemplates,
    layout: false,
    Component: () => import('@/pages/agents/agent-templates'),
  },

  {
    path: Routes.Files,
    layout: false,
    Component: () => import('@/layouts/next'),
    children: [
      {
        path: Routes.Files,
        Component: () => import('@/pages/files'),
      },
    ],
  },
  {
    path: Routes.DatasetBase,
    layout: false,
    Component: () => import('@/layouts/next'),
    children: [
      {
        path: Routes.DatasetBase,
        element: <Navigate to={Routes.Dataset} replace />,
      },
    ],
  },
  {
    path: Routes.DatasetBase,
    layout: false,
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
        Component: () => import('@/pages/dataset/dataset-setting'),
      },
    ],
  },
  {
    path: `${Routes.DataflowResult}`,
    layout: false,
    Component: () => import('@/pages/dataflow-result'),
  },
  {
    path: `${Routes.ParsedResult}/chunks`,
    layout: false,
    Component: () =>
      import('@/pages/chunk/parsed-result/add-knowledge/components/knowledge-chunk'),
  },
  {
    path: Routes.Chunk,
    layout: false,
    children: [
      {
        path: Routes.Chunk,
        Component: () => import('@/pages/chunk'),
        children: [
          {
            path: `${Routes.ChunkResult}/:id`,
            Component: () => import('@/pages/chunk/chunk-result'),
          },
          {
            path: `${Routes.ResultView}/:id`,
            Component: () => import('@/pages/chunk/result-view'),
          },
        ],
      },
    ],
  },
  {
    path: Routes.Chunk,
    layout: false,
    Component: () => import('@/pages/chunk'),
  },
  {
    path: '/user-setting',
    Component: () => import('@/pages/user-setting'),
    layout: false,
    children: [
      {
        path: '/user-setting',
        element: <Navigate to={`/user-setting${Routes.DataSource}`} replace />,
      },
      {
        path: '/user-setting/profile',
        Component: () => import('@/pages/user-setting/profile'),
      },
      {
        path: '/user-setting/locale',
        Component: () => import('@/pages/user-setting/setting-locale'),
      },
      {
        path: '/user-setting/model',
        Component: () => import('@/pages/user-setting/setting-model'),
      },
      {
        path: '/user-setting/team',
        Component: () => import('@/pages/user-setting/setting-team'),
      },
      {
        path: `/user-setting${Routes.Api}`,
        Component: () => import('@/pages/user-setting/setting-api'),
      },
      {
        path: `/user-setting${Routes.Mcp}`,
        Component: () => import('@/pages/user-setting/mcp'),
      },
      {
        path: `/user-setting${Routes.DataSource}`,
        Component: () => import('@/pages/user-setting/data-source'),
      },
    ],
  },
  {
    path: `/user-setting${Routes.DataSource}${Routes.DataSourceDetailPage}`,
    Component: () =>
      import('@/pages/user-setting/data-source/data-source-detail-page'),

    layout: false,
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
