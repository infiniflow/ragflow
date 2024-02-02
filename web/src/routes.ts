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
        path: '/knowledge/:module',
        component: '@/pages/add-knowledge',
      },
      {
        path: '/chat',
        component: '@/pages/chat',
      },
      {
        path: '/setting',
        component: '@/pages/setting',
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
