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
  AdminWhitelist = `${Admin}/whitelist`,
  AdminRoles = `${Admin}/roles`,
  AdminMonitoring = `${Admin}/monitoring`,
}

const routes = [
  {
    path: '/login',
    component: '@/pages/login-next',
    layout: false,
  },
  {
    path: '/login-next',
    component: '@/pages/login-next',
    layout: false,
  },
  {
    path: Routes.ChatShare,
    component: `@/pages${Routes.ChatShare}`,
    layout: false,
  },
  {
    path: Routes.AgentShare,
    component: `@/pages${Routes.AgentShare}`,
    layout: false,
  },
  {
    path: Routes.ChatWidget,
    component: `@/pages${Routes.ChatWidget}`,
    layout: false,
  },

  {
    path: Routes.AgentList,
    component: `@/pages/${Routes.Agents}`,
  },

  {
    path: '/document/:id',
    component: '@/pages/document-viewer',
    layout: false,
  },
  {
    path: '/*',
    component: '@/pages/404',
    layout: false,
  },
  {
    path: Routes.Root,
    layout: false,
    component: '@/layouts/next',
    wrappers: ['@/wrappers/auth'],
    routes: [
      {
        path: Routes.Root,
        component: `@/pages${Routes.Home}`,
      },
    ],
  },
  {
    path: Routes.Datasets,
    layout: false,
    component: '@/layouts/next',
    routes: [
      {
        path: Routes.Datasets,
        component: `@/pages${Routes.Datasets}`,
      },
    ],
  },
  {
    path: Routes.Chats,
    layout: false,
    component: '@/layouts/next',
    routes: [
      {
        path: Routes.Chats,
        component: `@/pages${Routes.Chats}`,
      },
    ],
  },
  {
    path: Routes.Chat + '/:id',
    layout: false,
    component: `@/pages${Routes.Chats}/chat`,
  },
  {
    path: Routes.Searches,
    layout: false,
    component: '@/layouts/next',
    routes: [
      {
        path: Routes.Searches,
        component: `@/pages${Routes.Searches}`,
      },
    ],
  },
  {
    path: Routes.Memories,
    layout: false,
    component: '@/layouts/next',
    routes: [
      {
        path: Routes.Memories,
        component: `@/pages${Routes.Memories}`,
      },
    ],
  },
  {
    path: `${Routes.Memory}`,
    layout: false,
    component: '@/layouts/next',
    routes: [
      {
        path: `${Routes.Memory}`,
        layout: false,
        component: `@/pages${Routes.Memory}`,
        routes: [
          {
            path: `${Routes.Memory}/${Routes.MemoryMessage}/:id`,
            component: `@/pages${Routes.Memory}${Routes.MemoryMessage}`,
          },
          {
            path: `${Routes.Memory}/${Routes.MemorySetting}/:id`,
            component: `@/pages${Routes.Memory}${Routes.MemorySetting}`,
          },
        ],
      },
    ],
    // component: `@/pages${Routes.DatasetBase}`,
    // component: `@/pages${Routes.Memory}`,
  },
  {
    path: `${Routes.Search}/:id`,
    layout: false,
    component: `@/pages${Routes.Search}`,
  },
  {
    path: `${Routes.SearchShare}`,
    layout: false,
    component: `@/pages${Routes.SearchShare}`,
  },
  {
    path: Routes.Agents,
    layout: false,
    component: '@/layouts/next',
    routes: [
      {
        path: Routes.Agents,
        component: `@/pages${Routes.Agents}`,
      },
    ],
  },
  {
    path: `${Routes.AgentLogPage}/:id`,
    layout: false,
    component: `@/pages${Routes.Agents}${Routes.AgentLogPage}`,
  },
  {
    path: `${Routes.Agent}/:id`,
    layout: false,
    component: `@/pages${Routes.Agent}`,
  },
  {
    path: Routes.AgentTemplates,
    layout: false,
    component: `@/pages${Routes.Agents}${Routes.AgentTemplates}`,
  },
  {
    path: Routes.Files,
    layout: false,
    component: '@/layouts/next',
    routes: [
      {
        path: Routes.Files,
        component: `@/pages${Routes.Files}`,
      },
    ],
  },
  {
    path: Routes.DatasetBase,
    layout: false,
    component: '@/layouts/next',
    routes: [{ path: Routes.DatasetBase, redirect: Routes.Dataset }],
  },
  {
    path: Routes.DatasetBase,
    layout: false,
    component: `@/pages${Routes.DatasetBase}`,
    routes: [
      {
        path: `${Routes.Dataset}/:id`,
        component: `@/pages${Routes.Dataset}`,
      },
      {
        path: `${Routes.DatasetBase}${Routes.DatasetTesting}/:id`,
        component: `@/pages${Routes.DatasetBase}${Routes.DatasetTesting}`,
      },
      {
        path: `${Routes.DatasetBase}${Routes.KnowledgeGraph}/:id`,
        component: `@/pages${Routes.DatasetBase}${Routes.KnowledgeGraph}`,
      },
      {
        path: `${Routes.DatasetBase}${Routes.DataSetOverview}/:id`,
        component: `@/pages${Routes.DatasetBase}${Routes.DataSetOverview}`,
      },
      {
        path: `${Routes.DatasetBase}${Routes.DataSetSetting}/:id`,
        component: `@/pages${Routes.DatasetBase}${Routes.DataSetSetting}`,
      },
    ],
  },
  {
    path: `${Routes.DataflowResult}`,
    layout: false,
    component: `@/pages${Routes.DataflowResult}`,
  },
  {
    path: `${Routes.ParsedResult}/chunks`,
    layout: false,
    component: `@/pages${Routes.Chunk}/parsed-result/add-knowledge/components/knowledge-chunk`,
  },
  {
    path: Routes.Chunk,
    layout: false,
    routes: [
      {
        path: Routes.Chunk,
        component: `@/pages${Routes.Chunk}`,
        routes: [
          // {
          //   path: `${Routes.ParsedResult}/:id`,
          //   component: `@/pages${Routes.Chunk}/parsed-result`,
          // },
          {
            path: `${Routes.ChunkResult}/:id`,
            component: `@/pages${Routes.Chunk}/chunk-result`,
          },
          {
            path: `${Routes.ResultView}/:id`,
            component: `@/pages${Routes.Chunk}/result-view`,
          },
        ],
      },
    ],
  },
  {
    path: Routes.Chunk,
    layout: false,
    component: `@/pages${Routes.Chunk}`,
  },
  // {
  //   path: Routes.ProfileSetting,
  //   layout: false,
  //   component: `@/pages${Routes.ProfileSetting}`,
  //   routes: [
  //     {
  //       path: Routes.ProfileSetting,
  //       redirect: `${Routes.ProfileProfile}`,
  //     },
  //     {
  //       path: `${Routes.ProfileProfile}`,
  //       component: `@/pages${Routes.ProfileProfile}`,
  //     },
  //     {
  //       path: `${Routes.ProfileTeam}`,
  //       component: `@/pages${Routes.ProfileTeam}`,
  //     },
  //     {
  //       path: `${Routes.ProfilePlan}`,
  //       component: `@/pages${Routes.ProfilePlan}`,
  //     },
  //     {
  //       path: `${Routes.ProfileModel}`,
  //       component: `@/pages${Routes.ProfileModel}`,
  //     },
  //     {
  //       path: `${Routes.ProfilePrompt}`,
  //       component: `@/pages${Routes.ProfilePrompt}`,
  //     },
  //     {
  //       path: Routes.ProfileMcp,
  //       component: `@/pages${Routes.ProfileMcp}`,
  //     },
  //   ],
  // },
  {
    path: '/user-setting',
    component: '@/pages/user-setting',
    layout: false,
    routes: [
      { path: '/user-setting', redirect: `/user-setting${Routes.DataSource}` },
      {
        path: '/user-setting/profile',
        // component: '@/pages/user-setting/setting-profile',
        component: '@/pages/user-setting/profile',
      },
      {
        path: '/user-setting/locale',
        component: '@/pages/user-setting/setting-locale',
      },
      {
        path: '/user-setting/model',
        component: '@/pages/user-setting/setting-model',
      },
      {
        path: '/user-setting/team',
        component: '@/pages/user-setting/setting-team',
      },
      {
        path: `/user-setting${Routes.Api}`,
        component: '@/pages/user-setting/setting-api',
      },
      {
        path: `/user-setting${Routes.Mcp}`,
        component: `@/pages/user-setting/${Routes.Mcp}`,
      },
      {
        path: `/user-setting${Routes.DataSource}`,
        component: `@/pages/user-setting${Routes.DataSource}`,
      },
    ],
  },

  {
    path: `/user-setting${Routes.DataSource}${Routes.DataSourceDetailPage}`,
    component: `@/pages/user-setting${Routes.DataSource}${Routes.DataSourceDetailPage}`,
    layout: false,
  },

  // Admin routes
  {
    path: Routes.Admin,
    layout: false,
    component: `@/pages/admin/layouts/root-layout`,
    routes: [
      {
        path: '',
        component: `@/pages/admin/login`,
      },
      {
        path: `${Routes.AdminUserManagement}/:id`,
        wrappers: ['@/pages/admin/wrappers/authorized'],
        component: `@/pages/admin/user-detail`,
      },
      {
        path: Routes.Admin,
        component: `@/pages/admin/layouts/navigation-layout`,
        wrappers: ['@/pages/admin/wrappers/authorized'],
        routes: [
          {
            path: Routes.AdminServices,
            component: `@/pages/admin/service-status`,
          },
          {
            path: Routes.AdminUserManagement,
            component: `@/pages/admin/users`,
          },

          ...(IS_ENTERPRISE
            ? [
                {
                  path: Routes.AdminWhitelist,
                  component: `@/pages/admin/whitelist`,
                },
                {
                  path: Routes.AdminRoles,
                  component: `@/pages/admin/roles`,
                },
                {
                  path: Routes.AdminMonitoring,
                  component: `@/pages/admin/monitoring`,
                },
              ]
            : []),
        ],
      },
    ],
  },
];

export default routes;
