import { lazy } from 'react';
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

const routeConfig = [
  {
    path: '/login',
    Component: lazy(() => import('@/pages/login-next')),
    layout: false,
    errorElement: <FallbackComponent />,
  },
  {
    path: '/login-next',
    Component: lazy(() => import('@/pages/login-next')),
    layout: false,
    errorElement: <FallbackComponent />,
  },
  {
    path: Routes.ChatShare,
    Component: lazy(() => import('@/pages/next-chats/share')),
    layout: false,
    errorElement: <FallbackComponent />,
  },
  {
    path: Routes.AgentShare,
    Component: lazy(() => import('@/pages/agent/share')),
    layout: false,
    errorElement: <FallbackComponent />,
  },
  {
    path: Routes.ChatWidget,
    Component: lazy(() => import('@/pages/next-chats/widget')),
    layout: false,
    errorElement: <FallbackComponent />,
  },
  {
    path: Routes.AgentList,
    Component: lazy(() => import('@/pages/agents')),
    errorElement: <FallbackComponent />,
  },
  {
    path: '/document/:id',
    Component: lazy(() => import('@/pages/document-viewer')),
    layout: false,
    errorElement: <FallbackComponent />,
  },
  {
    path: '/*',
    Component: lazy(() => import('@/pages/404')),
    layout: false,
    errorElement: <FallbackComponent />,
  },
  {
    path: Routes.Root,
    layout: false,
    Component: lazy(() => import('@/layouts/next')),
    wrappers: ['@/wrappers/auth'],
    children: [
      {
        path: Routes.Root,
        Component: lazy(() => import('@/pages/home')),
      },
    ],
    errorElement: <FallbackComponent />,
  },
  {
    path: Routes.Datasets,
    layout: false,
    Component: lazy(() => import('@/layouts/next')),
    children: [
      {
        path: Routes.Datasets,
        Component: lazy(() => import('@/pages/datasets')),
      },
    ],
    errorElement: <FallbackComponent />,
  },
  {
    path: Routes.Chats,
    layout: false,
    Component: lazy(() => import('@/layouts/next')),
    children: [
      {
        path: Routes.Chats,
        Component: lazy(() => import('@/pages/next-chats')),
      },
    ],
    errorElement: <FallbackComponent />,
  },
  {
    path: Routes.Chat + '/:id',
    layout: false,
    Component: lazy(() => import('@/pages/next-chats/chat')),
    errorElement: <FallbackComponent />,
  },
  {
    path: Routes.Searches,
    layout: false,
    Component: lazy(() => import('@/layouts/next')),
    children: [
      {
        path: Routes.Searches,
        Component: lazy(() => import('@/pages/next-searches')),
      },
    ],
    errorElement: <FallbackComponent />,
  },
  {
    path: Routes.Memories,
    layout: false,
    Component: lazy(() => import('@/layouts/next')),
    children: [
      {
        path: Routes.Memories,
        Component: lazy(() => import('@/pages/memories')),
      },
    ],
    errorElement: <FallbackComponent />,
  },
  {
    path: `${Routes.Memory}`,
    layout: false,
    Component: lazy(() => import('@/layouts/next')),
    children: [
      {
        path: `${Routes.Memory}`,
        layout: false,
        Component: lazy(() => import('@/pages/memory')),
        children: [
          {
            path: `${Routes.Memory}/${Routes.MemoryMessage}/:id`,
            Component: lazy(() => import('@/pages/memory/memory-message')),
          },
          {
            path: `${Routes.Memory}/${Routes.MemorySetting}/:id`,
            Component: lazy(() => import('@/pages/memory/memory-setting')),
          },
        ],
      },
    ],
    errorElement: <FallbackComponent />,
  },
  {
    path: `${Routes.Search}/:id`,
    layout: false,
    Component: lazy(() => import('@/pages/next-search')),
    errorElement: <FallbackComponent />,
  },
  {
    path: `${Routes.SearchShare}`,
    layout: false,
    Component: lazy(() => import('@/pages/next-search/share')),
    errorElement: <FallbackComponent />,
  },
  {
    path: Routes.Agents,
    layout: false,
    Component: lazy(() => import('@/layouts/next')),
    children: [
      {
        path: Routes.Agents,
        Component: lazy(() => import('@/pages/agents')),
      },
    ],
    errorElement: <FallbackComponent />,
  },
  {
    path: `${Routes.AgentLogPage}/:id`,
    layout: false,
    Component: lazy(() => import('@/pages/agents/agent-log-page')),
    errorElement: <FallbackComponent />,
  },
  {
    path: `${Routes.Agent}/:id`,
    layout: false,
    Component: lazy(() => import('@/pages/agent')),
    errorElement: <FallbackComponent />,
  },
  {
    path: Routes.AgentTemplates,
    layout: false,
    Component: lazy(() => import('@/pages/agents/agent-templates')),
    errorElement: <FallbackComponent />,
  },
  {
    path: Routes.Files,
    layout: false,
    Component: lazy(() => import('@/layouts/next')),
    children: [
      {
        path: Routes.Files,
        Component: lazy(() => import('@/pages/files')),
      },
    ],
    errorElement: <FallbackComponent />,
  },
  {
    path: Routes.DatasetBase,
    layout: false,
    Component: lazy(() => import('@/layouts/next')),
    children: [
      {
        path: Routes.DatasetBase,
        element: <Navigate to={Routes.Dataset} replace />,
      },
    ],
    errorElement: <FallbackComponent />,
  },
  {
    path: Routes.DatasetBase,
    layout: false,
    Component: lazy(() => import('@/pages/dataset')),
    children: [
      {
        path: `${Routes.Dataset}/:id`,
        Component: lazy(() => import('@/pages/dataset/dataset')),
      },
      {
        path: `${Routes.DatasetBase}${Routes.DatasetTesting}/:id`,
        Component: lazy(() => import('@/pages/dataset/testing')),
      },
      {
        path: `${Routes.DatasetBase}${Routes.KnowledgeGraph}/:id`,
        Component: lazy(() => import('@/pages/dataset/knowledge-graph')),
      },
      {
        path: `${Routes.DatasetBase}${Routes.DataSetOverview}/:id`,
        Component: lazy(() => import('@/pages/dataset/dataset-overview')),
      },
      {
        path: `${Routes.DatasetBase}${Routes.DataSetSetting}/:id`,
        Component: lazy(() => import('@/pages/dataset/dataset-setting')),
      },
    ],
    errorElement: <FallbackComponent />,
  },
  {
    path: `${Routes.DataflowResult}`,
    layout: false,
    Component: lazy(() => import('@/pages/dataflow-result')),
    errorElement: <FallbackComponent />,
  },
  {
    path: `${Routes.ParsedResult}/chunks`,
    layout: false,
    Component: lazy(
      () =>
        import('@/pages/chunk/parsed-result/add-knowledge/components/knowledge-chunk'),
    ),
    errorElement: <FallbackComponent />,
  },
  {
    path: Routes.Chunk,
    layout: false,
    children: [
      {
        path: Routes.Chunk,
        Component: lazy(() => import('@/pages/chunk')),
        children: [
          {
            path: `${Routes.ChunkResult}/:id`,
            Component: lazy(() => import('@/pages/chunk/chunk-result')),
          },
          {
            path: `${Routes.ResultView}/:id`,
            Component: lazy(() => import('@/pages/chunk/result-view')),
          },
        ],
      },
    ],
    errorElement: <FallbackComponent />,
  },
  {
    path: Routes.Chunk,
    layout: false,
    Component: lazy(() => import('@/pages/chunk')),
    errorElement: <FallbackComponent />,
  },
  {
    path: '/user-setting',
    Component: lazy(() => import('@/pages/user-setting')),
    layout: false,
    children: [
      {
        path: '/user-setting',
        element: <Navigate to={`/user-setting${Routes.DataSource}`} replace />,
      },
      {
        path: '/user-setting/profile',
        Component: lazy(() => import('@/pages/user-setting/profile')),
      },
      {
        path: '/user-setting/locale',
        Component: lazy(() => import('@/pages/user-setting/setting-locale')),
      },
      {
        path: '/user-setting/model',
        Component: lazy(() => import('@/pages/user-setting/setting-model')),
      },
      {
        path: '/user-setting/team',
        Component: lazy(() => import('@/pages/user-setting/setting-team')),
      },
      {
        path: `/user-setting${Routes.Api}`,
        Component: lazy(() => import('@/pages/user-setting/setting-api')),
      },
      {
        path: `/user-setting${Routes.Mcp}`,
        Component: lazy(() => import('@/pages/user-setting/mcp')),
      },
      {
        path: `/user-setting${Routes.DataSource}`,
        Component: lazy(() => import('@/pages/user-setting/data-source')),
      },
    ],
    errorElement: <FallbackComponent />,
  },
  {
    path: `/user-setting${Routes.DataSource}${Routes.DataSourceDetailPage}`,
    Component: lazy(
      () => import('@/pages/user-setting/data-source/data-source-detail-page'),
    ),
    layout: false,
    errorElement: <FallbackComponent />,
  },
  {
    path: Routes.Admin,
    Component: lazy(() => import('@/pages/admin/layouts/root-layout')),
    errorElement: <FallbackComponent />,
    children: [
      {
        path: Routes.Admin,
        Component: lazy(() => import('@/pages/admin/login')),
      },
      {
        path: Routes.Admin,
        Component: lazy(
          () => import('@/pages/admin/layouts/authorized-layout'),
        ),
        children: [
          {
            path: `${Routes.AdminUserManagement}/:id`,
            Component: lazy(() => import('@/pages/admin/user-detail')),
          },
          {
            Component: lazy(
              () => import('@/pages/admin/layouts/navigation-layout'),
            ),
            children: [
              {
                path: Routes.AdminServices,
                Component: lazy(() => import('@/pages/admin/service-status')),
              },
              {
                path: Routes.AdminUserManagement,
                Component: lazy(() => import('@/pages/admin/users')),
              },
              {
                path: Routes.AdminSandboxSettings,
                Component: lazy(() => import('@/pages/admin/sandbox-settings')),
              },
              ...(IS_ENTERPRISE
                ? [
                    {
                      path: Routes.AdminWhitelist,
                      Component: lazy(() => import('@/pages/admin/whitelist')),
                    },
                    {
                      path: Routes.AdminRoles,
                      Component: lazy(() => import('@/pages/admin/roles')),
                    },
                    {
                      path: Routes.AdminMonitoring,
                      Component: lazy(() => import('@/pages/admin/monitoring')),
                    },
                  ]
                : []),
            ],
          },
        ],
      },
    ],
  } satisfies RouteObject,
];

const routers = createBrowserRouter(routeConfig, {
  basename: import.meta.env.VITE_BASE_URL || '/',
});

export { routers };
