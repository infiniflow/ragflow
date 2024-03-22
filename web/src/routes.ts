const routes = [
  {
    path: '/login',
    component: '@/pages/login',
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
                path: '/knowledge/dataset/upload',
                component:
                  '@/pages/add-knowledge/components/knowledge-dataset/knowledge-upload-file',
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
        ],
      },
      {
        path: '/file',
        component: '@/pages/file',
      },
    ],
  },
  {
    path: '/*',
    component: '@/pages/404',
    layout: false,
  },
];

export default routes;
