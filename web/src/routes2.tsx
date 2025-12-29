import { lazy } from 'react';
import { createBrowserRouter } from 'react-router';

const routeConfig = [
  {
    path: '/',
    Component: lazy(() => import('@/pages/login-next')),
    layout: false,
  },
  {
    path: '/login',
    Component: lazy(() => import('@/pages/login-next')),
    layout: false,
  },
];

const routers = createBrowserRouter(routeConfig, {
  basename: '/v3',
});
// const list = [
//   {
//     path: '/',
//     name: 'Home',
//     Component: Login,
//   },
//   {
//     path: '/login',
//     name: 'Login',
//     Component: Login,
//   },
// ];
// const routers = createBrowserRouter(list, {
//   basename: '/v3',
// });
// export { routers };
export { routers };
