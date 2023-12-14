import BlankLayout from '@/layouts/blank-layout.vue';

const admins = [
  {
    path: '/welcome',
    name: 'welcome',
    meta: { title: '我的文件', icon: 'video-camera-out-lined' },
    component: () => import('@/views/admins/result.vue'),
    children: [
      {
        path: '/result?type=1',
        name: 'result',
        meta: { title: '视频', icon: 'icon-icon-test' },
        component: () => import('@/views/admins/result.vue'),
      },
      {
        path: '/result?type=2',
        name: 'result',
        meta: { title: '图片', icon: 'icon-icon-test' },
        component: () => import('@/views/admins/result.vue'),
      },
      {
        path: '/result?type=3',
        name: 'result',
        meta: { title: '音乐', icon: 'icon-icon-test' },
        component: () => import('@/views/admins/result.vue'),
      },
      {
        path: '/result?type=4',
        name: 'result',
        meta: { title: '文档', icon: 'icon-icon-test' },
        component: () => import('@/views/admins/result.vue'),
      }
    ]
  },
  // {
  //   path: '/admins',
  //   name: 'admins',
  //   meta: { title: '管理页', icon: 'icon-tuijian', flat: true },
  //   component: BlankLayout,
  //   redirect: () => ({ name: 'page1' }),
  //   children: [
  //     {
  //       path: 'page-search',
  //       name: 'page-search',
  //       meta: { title: '查询表格' },
  //       component: () => import('@/views/admins/page-search.vue'),
  //     },
  //   ],
  // },
  {
    path: '/version',
    name: 'version',
    meta: { title: 'Version', icon: 'icon-antdesign' },
    component: () => import('@/views/Detail.vue'),
  },
];

export default admins;
