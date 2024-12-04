export enum Routes {
  Login = '/login',
  Home = '/home',
  Datasets = '/datasets',
  DatasetBase = '/dataset',
  Dataset = `${Routes.DatasetBase}${Routes.DatasetBase}`,
  Agent = '/agent',
  Search = '/next-search',
  Chat = '/next-chat',
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
    path: 'force',
    component: '@/pages/force-graph',
    layout: false,
  },
  {
    path: '/*',
    component: '@/pages/404',
    layout: false,
  },
  {
    path: '/demo',
    component: '@/pages/demo',
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
    path: Routes.Chat,
    layout: false,
    component: '@/layouts/next',
    routes: [
      {
        path: Routes.Chat,
        component: `@/pages${Routes.Chat}`,
      },
    ],
  },
  {
    path: Routes.Search,
    layout: false,
    component: '@/layouts/next',
    routes: [
      {
        path: Routes.Search,
        component: `@/pages${Routes.Search}`,
      },
    ],
  },
  {
    path: Routes.Agent,
    layout: false,
    component: '@/layouts/next',
    routes: [
      {
        path: Routes.Agent,
        component: `@/pages${Routes.Agent}`,
      },
    ],
  },
  {
    path: Routes.DatasetBase,
    layout: false,
    component: '@/layouts/next',
    routes: [
      { path: Routes.DatasetBase, redirect: Routes.Dataset },
      {
        path: Routes.DatasetBase,
        component: `@/pages${Routes.DatasetBase}`,
        routes: [
          {
            path: Routes.Dataset,
            component: `@/pages${Routes.Dataset}`,
          },
          {
            path: `${Routes.DatasetBase}/configuration`,
            component: `@/pages${Routes.DatasetBase}/settings`,
          },
          {
            path: `${Routes.DatasetBase}/testing`,
            component: `@/pages${Routes.DatasetBase}/testing`,
          },
        ],
      },
    ],
  },
  {
    path: '/profile-setting',
    layout: false,
    component: '@/pages/profile-setting',
    routes: [
      { path: '/profile-setting', redirect: '/profile-setting/profile' },
      {
        path: '/profile-setting/profile',
        component: '@/pages/profile-setting/profile',
      },
      {
        path: '/profile-setting/team',
        component: '@/pages/profile-setting/team',
      },
      {
        path: '/profile-setting/plan',
        component: '@/pages/profile-setting/plan',
      },
      {
        path: '/profile-setting/model',
        component: '@/pages/profile-setting/model',
      },
      {
        path: '/profile-setting/prompt',
        component: '@/pages/profile-setting/prompt',
      },
    ],
  },
];

export default routes;
