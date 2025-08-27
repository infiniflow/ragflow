export enum Routes {
  Root = '/',
  Login = '/login',
  Logout = '/logout',
  Home = '/home',
  Datasets = '/datasets',
  DatasetBase = '/dataset',
  Dataset = `${Routes.DatasetBase}${Routes.DatasetBase}`,
  Agent = '/agent',
  AgentTemplates = '/agent-templates',
  Agents = '/agents',
  AgentList = '/agent-list',
  Searches = '/next-searches',
  Search = '/next-search',
  SearchShare = '/next-search/share',
  Chats = '/next-chats',
  Chat = '/next-chat',
  Files = '/files',
  ProfileSetting = '/profile-setting',
  Profile = '/profile',
  Mcp = '/mcp',
  Team = '/team',
  Plan = '/plan',
  Model = '/model',
  Prompt = '/prompt',
  ProfileMcp = `${ProfileSetting}${Mcp}`,
  ProfileTeam = `${ProfileSetting}${Team}`,
  ProfilePlan = `${ProfileSetting}${Plan}`,
  ProfileModel = `${ProfileSetting}${Model}`,
  ProfilePrompt = `${ProfileSetting}${Prompt}`,
  ProfileProfile = `${ProfileSetting}${Profile}`,
  DatasetTesting = '/testing',
  DatasetSetting = '/setting',
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
  UserSetting = '/user-setting',
}

const routes = [
  {
    path: '/login',
    component: '@/pages/login',
    layout: false,
  },
  {
    path: '/login-next',
    component: '@/pages/login-next',
    layout: false,
  },
  {
    path: '/chat/share',
    component: '@/pages/chat/share',
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
    path: Routes.Home,
    component: '@/layouts',
    layout: false,
    redirect: '/knowledge',
  },
  {
    path: '/knowledge',
    component: '@/pages/knowledge',
  },
  {
    path: '/knowledge',
    component: '@/pages/add-knowledge',
    routes: [
      {
        path: 'dataset',
        component: '@/pages/add-knowledge/components/knowledge-dataset',
        routes: [
          {
            path: '',
            component: '@/pages/add-knowledge/components/knowledge-file',
          },
          {
            path: 'chunk',
            component: '@/pages/add-knowledge/components/knowledge-chunk',
          },
        ],
      },
      {
        path: 'configuration',
        component: '@/pages/add-knowledge/components/knowledge-setting',
      },
      {
        path: 'testing',
        component: '@/pages/add-knowledge/components/knowledge-testing',
      },
      {
        path: 'knowledgeGraph',
        component: '@/pages/add-knowledge/components/knowledge-graph',
      },
    ],
  },

  {
    path: '/chat',
    component: '@/pages/chat',
  },
  {
    path: '/file',
    component: '@/pages/file-manager',
  },
  {
    path: '/flow',
    component: '@/pages/flow/list',
  },
  {
    path: Routes.AgentList,
    component: `@/pages/${Routes.Agents}`,
  },
  {
    path: '/flow/:id',
    component: '@/pages/flow',
  },
  {
    path: '/search',
    component: '@/pages/search',
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
        path: `${Routes.DatasetBase}${Routes.DatasetSetting}/:id`,
        component: `@/pages${Routes.DatasetBase}${Routes.DatasetSetting}`,
      },
      {
        path: `${Routes.DatasetBase}${Routes.DatasetTesting}/:id`,
        component: `@/pages${Routes.DatasetBase}${Routes.DatasetTesting}`,
      },
      {
        path: `${Routes.DatasetBase}${Routes.KnowledgeGraph}/:id`,
        component: `@/pages${Routes.DatasetBase}${Routes.KnowledgeGraph}`,
      },
    ],
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
  {
    path: Routes.ProfileSetting,
    layout: false,
    component: `@/pages${Routes.ProfileSetting}`,
    routes: [
      {
        path: Routes.ProfileSetting,
        redirect: `${Routes.ProfileProfile}`,
      },
      {
        path: `${Routes.ProfileProfile}`,
        component: `@/pages${Routes.ProfileProfile}`,
      },
      {
        path: `${Routes.ProfileTeam}`,
        component: `@/pages${Routes.ProfileTeam}`,
      },
      {
        path: `${Routes.ProfilePlan}`,
        component: `@/pages${Routes.ProfilePlan}`,
      },
      {
        path: `${Routes.ProfileModel}`,
        component: `@/pages${Routes.ProfileModel}`,
      },
      {
        path: `${Routes.ProfilePrompt}`,
        component: `@/pages${Routes.ProfilePrompt}`,
      },
      {
        path: Routes.ProfileMcp,
        component: `@/pages${Routes.ProfileMcp}`,
      },
    ],
  },
  {
    path: '/user-setting',
    component: '@/pages/user-setting',
    layout: false,
    routes: [
      { path: '/user-setting', redirect: '/user-setting/profile' },
      {
        path: '/user-setting/profile',
        // component: '@/pages/user-setting/setting-profile',
        component: '@/pages/user-setting/setting-profile',
      },
      {
        path: '/user-setting/locale',
        component: '@/pages/user-setting/setting-locale',
      },
      {
        path: '/user-setting/password',
        component: '@/pages/user-setting/setting-password',
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
        path: '/user-setting/system',
        component: '@/pages/user-setting/setting-system',
      },
      {
        path: '/user-setting/api',
        component: '@/pages/user-setting/setting-api',
      },
      {
        path: `/user-setting${Routes.Mcp}`,
        component: `@/pages${Routes.ProfileMcp}`,
      },
    ],
  },
];

export default routes;
