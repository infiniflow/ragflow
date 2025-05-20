export enum Routes {
  Login = '/login',
  Home = '/home',
  Datasets = '/datasets',
  DatasetBase = '/dataset',
  Dataset = `${Routes.DatasetBase}${Routes.DatasetBase}`,
  Agent = '/agent',
  AgentTemplates = '/agent-templates',
  Agents = '/agents',
  Searches = '/next-searches',
  Search = '/next-search',
  Chats = '/next-chats',
  Chat = '/next-chat',
  Files = '/files',
  ProfileSetting = '/profile-setting',
  DatasetTesting = '/testing',
  DatasetSetting = '/setting',
  Chunk = '/chunk',
  ChunkResult = `${Chunk}${Chunk}`,
  Parsed = '/parsed',
  ParsedResult = `${Chunk}${Parsed}`,
  Result = '/result',
  ResultView = `${Chunk}${Result}`,
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
    path: '/',
    component: '@/layouts',
    layout: false,
    wrappers: ['@/wrappers/auth'],
    routes: [
      { path: '/', redirect: '/knowledge' },
      {
        path: '/knowledge',
        component: '@/pages/knowledge',
      },
      {
        path: '/knowledge',
        component: '@/pages/add-knowledge',
        routes: [
          {
            path: '/knowledge/dataset',
            component: '@/pages/add-knowledge/components/knowledge-dataset',
            routes: [
              {
                path: '/knowledge/dataset',
                component: '@/pages/add-knowledge/components/knowledge-file',
              },
              {
                path: '/knowledge/dataset/chunk',
                component: '@/pages/add-knowledge/components/knowledge-chunk',
              },
            ],
          },
          {
            path: '/knowledge/configuration',
            component: '@/pages/add-knowledge/components/knowledge-setting',
          },
          {
            path: '/knowledge/testing',
            component: '@/pages/add-knowledge/components/knowledge-testing',
          },
          {
            path: '/knowledge/knowledgeGraph',
            component: '@/pages/add-knowledge/components/knowledge-graph',
          },
        ],
      },
      {
        path: '/chat',
        component: '@/pages/chat',
      },
      {
        path: '/user-setting',
        component: '@/pages/user-setting',
        routes: [
          { path: '/user-setting', redirect: '/user-setting/profile' },
          {
            path: '/user-setting/profile',
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
        ],
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
        path: '/flow/:id',
        component: '@/pages/flow',
      },
      {
        path: '/search',
        component: '@/pages/search',
      },
    ],
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
    path: Routes.Home,
    layout: false,
    component: '@/layouts/next',
    routes: [
      {
        path: Routes.Home,
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
    path: Routes.Chat,
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
    path: Routes.Search,
    layout: false,
    component: `@/pages${Routes.Search}`,
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
    ],
  },
  {
    path: Routes.Chunk,
    layout: false,
    routes: [
      {
        path: Routes.Chunk,
        component: `@/pages${Routes.Chunk}`,
        routes: [
          {
            path: `${Routes.ParsedResult}/:id`,
            component: `@/pages${Routes.Chunk}/parsed-result`,
          },
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
        redirect: `${Routes.ProfileSetting}/profile`,
      },
      {
        path: `${Routes.ProfileSetting}/profile`,
        component: `@/pages${Routes.ProfileSetting}/profile`,
      },
      {
        path: `${Routes.ProfileSetting}/team`,
        component: `@/pages${Routes.ProfileSetting}/team`,
      },
      {
        path: `${Routes.ProfileSetting}/plan`,
        component: `@/pages${Routes.ProfileSetting}/plan`,
      },
      {
        path: `${Routes.ProfileSetting}/model`,
        component: `@/pages${Routes.ProfileSetting}/model`,
      },
      {
        path: `${Routes.ProfileSetting}/prompt`,
        component: `@/pages${Routes.ProfileSetting}/prompt`,
      },
    ],
  },
];

export default routes;
