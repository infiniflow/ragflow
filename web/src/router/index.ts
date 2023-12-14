import { createRouter, createWebHistory } from 'vue-router';
import { NOT_FOUND_ROUTE, REDIRECT_MAIN } from './routes/base';
import createRouteGuard from './guard';
import { appRoutes } from './routes';
import { DEFAULT_ROUTE_NAME } from './constants';
import BasicLayout from '@/layouts/basic-layout.vue';
import 'nprogress/nprogress.css';

NProgress.configure({ showSpinner: false }); // NProgress Configuration

const router = createRouter({
  history: createWebHistory(import.meta.env.BASE_URL),
  scrollBehavior() {
    return { top: 0 };
  },
  routes: [
    {
      path: '/',
      name: 'index',
      meta: { title: 'Home' },
      component: BasicLayout,
      redirect: {
        name: DEFAULT_ROUTE_NAME,
      },
      children: [
        ...appRoutes,
      ],
    },
    {
      path: '/login',
      name: 'login',
      component: () => import('@/views/login/index.vue'),
    },
    NOT_FOUND_ROUTE,
    REDIRECT_MAIN,
  ],
});

createRouteGuard(router);

export default router;
